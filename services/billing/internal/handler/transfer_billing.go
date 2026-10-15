package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// GET /internal/students/{id}/financial-summary?school_id=
// ---------------------------------------------------------------------------

// InternalStudentFinancialSummary returns the student's complete financial summary:
// open bills, unmatured bills, deposits, and refundable amounts (FR-78-002, AC-78-01).
func (a *App) InternalStudentFinancialSummary(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	studentID := param(r, "id")
	if studentID == "" {
		studentID = param(r, "studentUserId")
	}
	if studentID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "student_user_id is required")
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT b.id, COALESCE(b.scheme_id::text,''), COALESCE(s.name,b.source_module,''), b.status, b.period_year, b.period_month,
		        b.base_idr, b.discount_idr, b.net_idr, b.due_at, b.created_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'), 0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id), 0),
		        `+pendingSQL("''")+`
		 FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE b.school_id=$1 AND b.student_user_id::text=$2
		 ORDER BY b.period_year, b.period_month, b.created_at`,
		schoolID, studentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	loc, _ := time.LoadLocation("Asia/Jakarta")
	if loc == nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	now := time.Now().In(loc)

	var totalOutstanding, openBills, unmaturedBills, totalRefundable int64
	bills := []map[string]any{}

	for rows.Next() {
		var (
			id, scheme, schemeName, status          string
			y, m                                    int
			base, discount, net, verified, reversed int64
			pendingRefund                           int64
			dueAt                                   sql.NullTime
			createdAt                               time.Time
		)
		if err := rows.Scan(&id, &scheme, &schemeName, &status, &y, &m,
			&base, &discount, &net, &dueAt, &createdAt, &verified, &reversed, &pendingRefund); err != nil {
			writeErr(w, err)
			return
		}

		outstanding, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}

		var dueTime *time.Time
		if dueAt.Valid {
			t := dueAt.Time.In(loc)
			dueTime = &t
		}

		isOpen, isUnmatured := model.CategorizeBillOutstanding(dueTime, now)
		if isOpen {
			openBills += outstanding
		} else if isUnmatured {
			unmaturedBills += outstanding
		}
		totalOutstanding += outstanding

		refundable := verified - reversed - pendingRefund
		if refundable > 0 {
			totalRefundable += refundable
		}

		bSummary := a.billSummary(id, scheme, schemeName, status, y, m, base, discount, net, verified, reversed, outstanding, dueAt, createdAt)
		bSummary["refundable_idr"] = refundable
		bills = append(bills, bSummary)
	}

	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}

	// Overpayment deposit (BR-FIN-04) of this unit. Per BR-STU-03 it stays
	// recorded at the origin unit on a unit transfer; Menu 78 shows it so
	// finance can approve a refund (FR-78-002/006, AC-78-03).
	var depositIDR int64
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT COALESCE(balance_idr,0) FROM student_deposits WHERE school_id=$1 AND student_user_id::text=$2`,
		schoolID, studentID).Scan(&depositIDR); err != nil && err != sql.ErrNoRows {
		writeErr(w, err)
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"student_user_id":       studentID,
		"school_id":             schoolID,
		"total_outstanding_idr": totalOutstanding,
		"open_bills_idr":        openBills,
		"unmatured_bills_idr":   unmaturedBills,
		"deposit_idr":           depositIDR,
		"refundable_idr":        totalRefundable,
		"bills":                 bills,
	}, nil)
}

// ---------------------------------------------------------------------------
// POST /internal/students/{id}/exit
// ---------------------------------------------------------------------------

type internalExitRequest struct {
	SchoolID      string `json:"school_id"`
	EffectiveDate string `json:"effective_date"`
	Policy        string `json:"policy"` // void | keep
}

// InternalStudentExit records a student exit and applies the exit billing policy (FR-78-005).
func (a *App) InternalStudentExit(w http.ResponseWriter, r *http.Request) {
	studentID := param(r, "id")
	if studentID == "" {
		studentID = param(r, "studentUserId")
	}
	if studentID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "student_user_id is required")
		return
	}

	var in internalExitRequest
	if !common.Decode(w, r, &in) {
		return
	}
	schoolID := strings.TrimSpace(in.SchoolID)
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}

	effDate := strings.TrimSpace(in.EffectiveDate)
	if effDate == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "effective_date is required")
		return
	}

	policy := in.Policy
	if policy != "keep" {
		policy = "void"
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Record or update student exit
	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO student_billing_exits (school_id, student_user_id, effective_date, policy, created_at)
		VALUES ($1, $2, $3::date, $4, now())
		ON CONFLICT (school_id, student_user_id)
		DO UPDATE SET effective_date = EXCLUDED.effective_date, policy = EXCLUDED.policy
	`, schoolID, studentID, effDate, policy)
	if err != nil {
		writeErr(w, err)
		return
	}

	voidedCount := 0
	if policy == "void" {
		// Bills due after the effective date are voided
		rows, err := tx.QueryContext(r.Context(), `
			SELECT id, status, due_at FROM student_bills
			WHERE school_id=$1 AND student_user_id::text=$2
			  AND status IN ('unpaid', 'partial')
			  AND due_at::date > $3::date
			FOR UPDATE
		`, schoolID, studentID, effDate)
		if err != nil {
			writeErr(w, err)
			return
		}
		defer rows.Close()

		var voidIDs []string
		for rows.Next() {
			var bid, bst string
			var due sql.NullTime
			if err := rows.Scan(&bid, &bst, &due); err != nil {
				writeErr(w, err)
				return
			}
			voidIDs = append(voidIDs, bid)
		}
		rows.Close()

		for _, bid := range voidIDs {
			_, err = tx.ExecContext(r.Context(), `
				UPDATE student_bills
				SET status = 'void', void_reason = 'Student exited on ' || $1, updated_at = now()
				WHERE id = $2 AND school_id = $3
			`, effDate, bid, schoolID)
			if err != nil {
				writeErr(w, err)
				return
			}
			voidedCount++
		}
	}

	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	if voidedCount > 0 {
		a.audit(r.Context(), schoolID, "student_exit", studentID, "student_exited_bills_voided", "internal",
			"system", "", "Student exit policy applied", nil,
			map[string]any{"effective_date": effDate, "policy": policy, "voided_count": voidedCount})
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"student_user_id": studentID,
		"school_id":       schoolID,
		"effective_date":  effDate,
		"policy":          policy,
		"voided_count":    voidedCount,
	}, nil)
}

// ---------------------------------------------------------------------------
// POST /internal/students/{id}/refund
// ---------------------------------------------------------------------------

type internalRefundRequest struct {
	SchoolID       string `json:"school_id"`
	AmountIDR      int64  `json:"amount_idr"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
	RequestedBy    string `json:"requested_by"`
}

// InternalStudentRefund creates a refund request for a student with an idempotency key (FR-78-006, AC-78-03).
func (a *App) InternalStudentRefund(w http.ResponseWriter, r *http.Request) {
	studentID := param(r, "id")
	if studentID == "" {
		studentID = param(r, "studentUserId")
	}
	if studentID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "student_user_id is required")
		return
	}

	var in internalRefundRequest
	if !common.Decode(w, r, &in) {
		return
	}
	schoolID := strings.TrimSpace(in.SchoolID)
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	if in.AmountIDR <= 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "amount_idr must be > 0")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		in.Reason = "Pengembalian deposit / penarikan siswa"
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Idempotency check
	if in.IdempotencyKey != "" {
		var existingID, existingStatus string
		err = tx.QueryRowContext(r.Context(), `
			SELECT id::text, status FROM bill_refunds
			WHERE school_id=$1 AND idempotency_key=$2
		`, schoolID, in.IdempotencyKey).Scan(&existingID, &existingStatus)
		if err == nil {
			_ = tx.Rollback()
			common.WriteJSON(w, http.StatusOK, map[string]any{
				"id":     existingID,
				"status": existingStatus,
			}, nil)
			return
		}
		if err != sql.ErrNoRows {
			writeErr(w, err)
			return
		}
	}

	// Split the amount: the overpayment deposit first (paid out of
	// student_deposits, no bill touched), then the paid balance of bills,
	// newest first. A refund the two cannot cover is refused.
	depositAvail, refundables, err := studentRefundablesQ(r.Context(), tx, schoolID, studentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	depositIDR, shares, serr := model.SplitStudentRefund(in.AmountIDR, depositAvail, refundables)
	if serr != nil {
		common.WriteError(w, http.StatusUnprocessableEntity,
			"nominal refund melebihi deposit dan saldo terbayar tagihan siswa yang masih dapat dikembalikan")
		return
	}
	var newRefundID string

	var reqKey any = nil
	if in.IdempotencyKey != "" {
		reqKey = in.IdempotencyKey
	}

	// The maker is the staff member who filed the Menu 78 request (sent by
	// academic as requested_by). Older callers send none; the student id is
	// then recorded, as before, and core decides whether it accepts it.
	makerID := strings.TrimSpace(in.RequestedBy)
	if makerID == "" {
		makerID = studentID
	}

	err = tx.QueryRowContext(r.Context(), `
		INSERT INTO bill_refunds (
			school_id, student_bill_id, student_user_id, amount_idr, deposit_idr, reason, status, requested_by, idempotency_key
		) VALUES (
			$1, NULL, $2, $3, $4, $5, 'requested', $6, $7
		) RETURNING id
	`, schoolID, studentID, in.AmountIDR, depositIDR, in.Reason, makerID, reqKey).Scan(&newRefundID)
	if err != nil {
		writeErr(w, err)
		return
	}
	billShares := make([]map[string]any, 0, len(shares))
	for _, sh := range shares {
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO bill_refund_parts(school_id,refund_id,student_bill_id,amount_idr) VALUES($1,$2,$3,$4)`,
			schoolID, newRefundID, sh.BillID, sh.AmountIDR); err != nil {
			writeErr(w, err)
			return
		}
		billShares = append(billShares, map[string]any{"student_bill_id": sh.BillID, "amount_idr": sh.AmountIDR})
	}

	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	// Same rule as /admin/refunds: the decision belongs to core's Approval
	// Center (request type 'refund'). If core does not take the request the
	// row is removed so a retry with the same idempotency key starts clean.
	amount := in.AmountIDR
	resp, err := a.submitApproval(r.Context(), client.ApprovalRequest{
		SchoolID: schoolID, RequestType: reqTypeRefund, SourceID: newRefundID,
		Title: "Refund penarikan/mutasi siswa",
		Summary: map[string]any{
			"student_user_id": studentID, "amount_idr": in.AmountIDR, "reason": in.Reason,
			"deposit_idr": depositIDR, "bill_parts": billShares,
			"origin": "academic_status_request", "idempotency_key": in.IdempotencyKey,
		},
		AmountIDR: &amount, MakerUserID: makerID,
		CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/refunds/" + newRefundID + "/callback",
		CorrelationID: newRefundID, IdempotencyKey: "refund:" + newRefundID,
	})
	if err != nil {
		_, _ = a.DB.ExecContext(r.Context(), `DELETE FROM bill_refunds WHERE id::text=$1 AND status='requested'`, newRefundID)
		a.reportRefundApprovalFailure(r.Context(), schoolID, studentID, newRefundID, in.IdempotencyKey, in.Reason, in.AmountIDR, err)
		writeErr(w, err)
		return
	}
	if _, err = a.DB.ExecContext(r.Context(),
		`UPDATE bill_refunds SET approval_id=$1,updated_at=now() WHERE id::text=$2`, resp.ID, newRefundID); err != nil {
		writeErr(w, err)
		return
	}

	a.audit(r.Context(), schoolID, "refund", newRefundID, "refund_requested_internal", "internal",
		makerID, "", in.Reason, nil, map[string]any{"amount_idr": in.AmountIDR, "idempotency_key": in.IdempotencyKey,
			"approval_id": resp.ID, "student_user_id": studentID, "deposit_idr": depositIDR, "bill_parts": billShares})

	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":          newRefundID,
		"status":      "requested",
		"amount_idr":  in.AmountIDR,
		"deposit_idr": depositIDR,
		"bill_parts":  billShares,
		"approval_id": resp.ID,
	}, nil)
}

// ---------------------------------------------------------------------------
// Student Bill Visibility (Menu 82)
// ---------------------------------------------------------------------------

type VisibilityItem struct {
	GradeLevel string `json:"grade_level"`
	Enabled    bool   `json:"enabled"`
}

// GetStudentBillVisibility is GET /admin/student-bill-visibility (admin/superadmin).
func (a *App) GetStudentBillVisibility(w http.ResponseWriter, r *http.Request) {
	_, schoolID := adminSchool(w, r)
	if schoolID == "" {
		return
	}

	var grades []string
	if a.Roster != nil && a.Roster.Configured() {
		gList, err := a.Roster.SchoolGradeLevels(r.Context(), schoolID)
		if err == nil {
			grades = gList
		}
	}

	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT grade_level, enabled FROM student_bill_visibility WHERE school_id=$1
	`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	configMap := map[string]bool{}
	for rows.Next() {
		var gl string
		var en bool
		if err := rows.Scan(&gl, &en); err != nil {
			writeErr(w, err)
			return
		}
		configMap[gl] = en
	}

	// Combine known grade levels from academic + configured ones
	seen := map[string]bool{}
	var items []VisibilityItem
	for _, g := range grades {
		seen[g] = true
		items = append(items, VisibilityItem{GradeLevel: g, Enabled: configMap[g]})
	}
	for g, en := range configMap {
		if !seen[g] {
			items = append(items, VisibilityItem{GradeLevel: g, Enabled: en})
		}
	}

	common.WriteJSON(w, http.StatusOK, items, nil)
}

// UpdateStudentBillVisibility is PUT /admin/student-bill-visibility (admin/superadmin).
func (a *App) UpdateStudentBillVisibility(w http.ResponseWriter, r *http.Request) {
	_, schoolID := adminSchool(w, r)
	if schoolID == "" {
		return
	}

	var in struct {
		Items []VisibilityItem `json:"items"`
	}
	if !common.Decode(w, r, &in) {
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	for _, item := range in.Items {
		gl := strings.TrimSpace(item.GradeLevel)
		if gl == "" {
			continue
		}
		_, err := tx.ExecContext(r.Context(), `
			INSERT INTO student_bill_visibility (school_id, grade_level, enabled)
			VALUES ($1, $2, $3)
			ON CONFLICT (school_id, grade_level)
			DO UPDATE SET enabled = EXCLUDED.enabled
		`, schoolID, gl, item.Enabled)
		if err != nil {
			writeErr(w, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	common.WriteJSON(w, http.StatusOK, in.Items, nil)
}

// GetMyBillVisibility is GET /me/bills/visibility (siswa).
func (a *App) GetMyBillVisibility(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil {
		return
	}

	enabled, gradeLevel := a.resolveStudentBillVisibility(r.Context(), c.SchoolID, c.UserID)
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":     enabled,
		"grade_level": gradeLevel,
	}, nil)
}

func (a *App) resolveStudentBillVisibility(ctx context.Context, schoolID, studentUserID string) (bool, string) {
	var gradeLevel string
	if a.Roster != nil && a.Roster.Configured() {
		gl, err := a.Roster.StudentGradeLevel(ctx, schoolID, studentUserID)
		if err == nil {
			gradeLevel = gl
		}
	}
	if gradeLevel == "" {
		return false, ""
	}

	var enabled bool
	err := a.DB.QueryRowContext(ctx, `
		SELECT enabled FROM student_bill_visibility WHERE school_id=$1 AND grade_level=$2
	`, schoolID, gradeLevel).Scan(&enabled)
	if err != nil {
		// Missing row -> disabled by default (FR-82-001)
		return false, gradeLevel
	}
	return enabled, gradeLevel
}

// checkStudentBillVisibility guards student bill endpoints (AC-82-01).
func (a *App) checkStudentBillVisibility(w http.ResponseWriter, r *http.Request, schoolID, studentUserID string) bool {
	enabled, _ := a.resolveStudentBillVisibility(r.Context(), schoolID, studentUserID)
	if !enabled {
		common.WriteError(w, http.StatusForbidden, "Tagihan tidak ditampilkan untuk jenjang ini")
		return false
	}
	return true
}
