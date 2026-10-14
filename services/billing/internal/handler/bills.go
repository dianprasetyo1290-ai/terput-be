package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Shared bill services (used by the admin, internal and parent handlers)
// ---------------------------------------------------------------------------

// generateRequest is the shared payload for both
// POST /admin/bills/generate and POST /internal/bills/generate.
type generateRequest struct {
	SchemeID       string   `json:"scheme_id"`
	PeriodYear     int      `json:"period_year"`
	PeriodMonth    int      `json:"period_month"`
	StudentUserIDs []string `json:"student_user_ids"`
	// StudentUserID is an optional single-student convenience for the
	// scheduler-driven internal endpoint.
	StudentUserID string `json:"student_user_id"`
	// Actor is who asked for the run (audit); set by the handler, never
	// read from the body.
	ActorType string `json:"-"`
	ActorID   string `json:"-"`
}

// generateResult reports what a generation run did. Running the same period
// twice yields generated=0 the second time.
type generateResult struct {
	Generated int                       `json:"generated"`
	Skipped   int                       `json:"skipped"`
	BillIDs   []string                  `json:"bill_ids"`
	Rejected  []model.RejectedRecipient `json:"rejected"`
	// DepositAppliedIDR is the student deposit used to settle the new bills.
	DepositAppliedIDR int64 `json:"deposit_applied_idr"`
}

// queryer is satisfied by both *sql.DB and *sql.Tx.
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// billTotalsQ returns the snapshot net, verified allocation total, reversal
// total and outstanding balance for one bill, computed from the ledger.
func billTotalsQ(ctx context.Context, q queryer, schoolID, billID string) (net, verified, reversed, outstanding int64, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT net_idr FROM student_bills WHERE id=$1 AND school_id=$2`, billID, schoolID).Scan(&net)
	if err == sql.ErrNoRows {
		return 0, 0, 0, 0, &httpError{http.StatusNotFound, "bill not found"}
	}
	if err != nil {
		return
	}
	if err = q.QueryRowContext(ctx,
		`SELECT COALESCE(sum(a.amount_idr),0)
		 FROM bill_payment_allocations a JOIN bill_payments p ON p.id=a.payment_id
		 WHERE p.student_bill_id=$1 AND p.school_id=$2 AND p.status='verified'`,
		billID, schoolID).Scan(&verified); err != nil {
		return
	}
	if err = q.QueryRowContext(ctx,
		`SELECT COALESCE(sum(r.amount_idr),0)
		 FROM bill_payment_reversals r JOIN bill_payments p ON p.id=r.payment_id
		 WHERE p.student_bill_id=$1 AND p.school_id=$2`,
		billID, schoolID).Scan(&reversed); err != nil {
		return
	}
	outstanding, err = model.Outstanding(net, []int64{verified}, []int64{reversed})
	return
}

// recomputeBillStatusQ re-derives a bill's status from its ledger and writes
// it back. Void bills are never touched.
func recomputeBillStatusQ(ctx context.Context, q queryer, schoolID, billID string) error {
	net, verified, reversed, _, err := billTotalsQ(ctx, q, schoolID, billID)
	if err != nil {
		return err
	}
	status := model.DeriveBillStatus(net, verified-reversed)
	_, err = q.ExecContext(ctx,
		`UPDATE student_bills SET status=$1,updated_at=now() WHERE id=$2 AND school_id=$3 AND status<>'void'`,
		status, billID, schoolID)
	return err
}

// discountRule is a stored bill_discount plus its scope metadata.
type discountRule struct {
	candidate     model.DiscountCandidate
	scope         string
	schemeID      string
	gradeLevel    string
	studentUserID string
}

// rulesForStudent filters school-wide, scheme, grade and student discounts
// down to the ones that apply to a single student's bill.
func rulesForStudent(rules []discountRule, schemeID, gradeLevel, studentUserID string) []model.DiscountCandidate {
	out := make([]model.DiscountCandidate, 0, len(rules))
	for _, r := range rules {
		switch r.scope {
		case "school":
		case "scheme":
			if r.schemeID != schemeID {
				continue
			}
		case "grade":
			if r.gradeLevel == "" || r.gradeLevel != gradeLevel {
				continue
			}
		case "student":
			if r.studentUserID != studentUserID {
				continue
			}
		default:
			continue
		}
		out = append(out, r.candidate)
	}
	return out
}

// loadDiscountRules loads every active discount for the school. The list is
// small (per school) so it is filtered in Go per student.
func (a *App) loadDiscountRules(ctx context.Context, schoolID string) ([]discountRule, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,name,kind,value,COALESCE(cap_idr,0),priority,scope,
		        COALESCE(scheme_id::text,''),COALESCE(grade_level,''),COALESCE(student_user_id::text,''),created_at
		 FROM bill_discounts WHERE school_id=$1 AND active
		 ORDER BY priority,created_at,id`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []discountRule
	for rows.Next() {
		var (
			r          discountRule
			name, kind string
			value, cap int64
			priority   int
		)
		if err := rows.Scan(&r.candidate.ID, &name, &kind, &value, &cap, &priority, &r.scope,
			&r.schemeID, &r.gradeLevel, &r.studentUserID, &r.candidate.CreatedAt); err != nil {
			return nil, err
		}
		r.candidate.Source = "bill_discount"
		r.candidate.Label = name
		r.candidate.Kind = model.DiscountKind(kind)
		r.candidate.Value = value
		r.candidate.Cap = cap
		r.candidate.Priority = priority
		out = append(out, r)
	}
	return out, rows.Err()
}

// generateBills creates the bills a request asks for, idempotently. It is the
// single implementation behind both the admin and internal generate routes so
// the two can never drift.
func (a *App) generateBills(ctx context.Context, schoolID string, in generateRequest) (generateResult, error) {
	result := generateResult{BillIDs: []string{}, Rejected: []model.RejectedRecipient{}}

	students := normalizeStudentIDs(in.StudentUserIDs, in.StudentUserID)
	if strings.TrimSpace(in.SchemeID) == "" {
		return result, &httpError{http.StatusUnprocessableEntity, "scheme_id is required"}
	}
	if in.PeriodMonth < 1 || in.PeriodMonth > 12 {
		return result, &httpError{http.StatusUnprocessableEntity, "period_month must be between 1 and 12"}
	}
	if in.PeriodYear < 2000 || in.PeriodYear > 2100 {
		return result, &httpError{http.StatusUnprocessableEntity, "period_year is out of range"}
	}
	if len(students) == 0 {
		return result, &httpError{http.StatusUnprocessableEntity, "at least one student_user_id is required"}
	}

	// Resolve the scheme (school-scoped).
	var (
		gradeLevel     string
		dueDay         int
		schemeState    string
		effFrom, effTo sql.NullTime
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(grade_level,''),due_day,status,effective_from,effective_to
		 FROM billing_schemes WHERE id::text=$1 AND school_id=$2`, in.SchemeID, schoolID).
		Scan(&gradeLevel, &dueDay, &schemeState, &effFrom, &effTo)
	if err == sql.ErrNoRows {
		return result, &httpError{http.StatusNotFound, "scheme not found"}
	}
	if err != nil {
		return result, err
	}
	if schemeState == "archived" {
		return result, &httpError{http.StatusUnprocessableEntity, "scheme is archived"}
	}
	if schemeState == "draft" {
		return result, &httpError{http.StatusUnprocessableEntity, "skema masih draft; aktifkan skema sebelum menerbitkan tagihan"}
	}
	var from, to *time.Time
	if effFrom.Valid {
		from = &effFrom.Time
	}
	if effTo.Valid {
		to = &effTo.Time
	}
	if !model.PeriodInWindow(in.PeriodYear, in.PeriodMonth, from, to) {
		return result, &httpError{http.StatusUnprocessableEntity, "periode tagihan di luar masa berlaku tarif skema"}
	}

	// Line items from the scheme's components.
	itemRows, err := a.DB.QueryContext(ctx,
		`SELECT code,name,amount_idr FROM billing_scheme_components
		 WHERE scheme_id=$1 AND school_id=$2 ORDER BY sort_order,code`, in.SchemeID, schoolID)
	if err != nil {
		return result, err
	}
	var items []model.BillItem
	for itemRows.Next() {
		var code, name string
		var amount int64
		if err := itemRows.Scan(&code, &name, &amount); err != nil {
			itemRows.Close()
			return result, err
		}
		items = append(items, model.BillItem{Name: name, ComponentCode: code, AmountIDR: amount})
	}
	itemRows.Close()
	if err := itemRows.Err(); err != nil {
		return result, err
	}
	if len(items) == 0 {
		return result, &httpError{http.StatusUnprocessableEntity, "scheme has no bill components"}
	}
	base, err := model.TotalBase(items)
	if err != nil {
		return result, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	// BUG-050: a bill whose components cannot be journaled is never issued.
	journalSet, err := a.loadJournalSettings(ctx, schoolID)
	if err != nil {
		return result, err
	}
	compAccounts, err := a.loadComponentAccounts(ctx, schoolID)
	if err != nil {
		return result, err
	}
	if err := model.SchemeIssueMapping(journalSet, compAccounts, items); err != nil {
		return result, &httpError{http.StatusUnprocessableEntity,
			"mapping akun belum lengkap, tagihan tidak diterbitkan: " + err.Error() + " (atur di menu Mapping Akun Jurnal)"}
	}

	rules, err := a.loadDiscountRules(ctx, schoolID)
	if err != nil {
		return result, err
	}

	// Existing bills for this period+scheme make generation idempotent.
	existing, err := a.existingBillKeys(ctx, schoolID, in.SchemeID, in.PeriodYear, in.PeriodMonth, students)
	if err != nil {
		return result, err
	}
	// Exclude exited students (FR-78-005: hentikan penerbitan tagihan berikutnya)
	exitedStudents := map[string]bool{}
	if exitRows, err := a.DB.QueryContext(ctx, `
		SELECT student_user_id::text FROM student_billing_exits
		WHERE school_id=$1 AND effective_date <= make_date($2, $3, 1)
	`, schoolID, in.PeriodYear, in.PeriodMonth); err == nil {
		defer exitRows.Close()
		for exitRows.Next() {
			var sid string
			if err := exitRows.Scan(&sid); err == nil {
				exitedStudents[sid] = true
			}
		}
	}

	candidates := make([]model.BillCandidate, 0, len(students))
	for _, s := range students {
		if exitedStudents[s] {
			continue
		}
		candidates = append(candidates, model.BillCandidate{Key: model.BillKey{
			StudentUserID: s, Year: in.PeriodYear, Month: in.PeriodMonth, SchemeID: in.SchemeID,
		}})
	}
	plan, err := model.PlanGeneration(existing, candidates)
	if err != nil {
		return result, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	result.Skipped = len(students) - len(plan)

	// BUG-054: only active students in the scheme's grade receive a bill.
	planned := make([]string, 0, len(plan))
	for _, c := range plan {
		planned = append(planned, c.Key.StudentUserID)
	}
	enrollments, err := a.studentEnrollments(ctx, schoolID, planned)
	if err != nil {
		return result, err
	}
	eligible, rejected := model.ScreenRecipients(planned, enrollments, gradeLevel)
	result.Rejected = append(result.Rejected, rejected...)
	keep := map[string]bool{}
	for _, s := range eligible {
		keep[s] = true
	}
	screened := plan[:0]
	for _, c := range plan {
		if keep[c.Key.StudentUserID] {
			screened = append(screened, c)
		}
	}
	plan = screened

	dueAt := model.DueDate(in.PeriodYear, in.PeriodMonth, dueDay)

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	var newBills []newBillNotificationInfo

	for _, c := range plan {
		studentID := c.Key.StudentUserID
		studentGrade := enrollments[studentID].GradeLevel
		if studentGrade == "" {
			studentGrade = gradeLevel
		}
		net, steps, err := model.ApplyDiscounts(base, rulesForStudent(rules, in.SchemeID, studentGrade, studentID))
		if err != nil {
			return result, &httpError{http.StatusUnprocessableEntity, err.Error()}
		}

		var billID string
		err = tx.QueryRowContext(ctx,
			`INSERT INTO student_bills(school_id,student_user_id,scheme_id,period_year,period_month,base_idr,discount_idr,net_idr,due_at,discount_snapshot,status)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'unpaid')
			 ON CONFLICT(school_id,student_user_id,period_year,period_month,scheme_id) DO NOTHING
			 RETURNING id`,
			schoolID, studentID, in.SchemeID, in.PeriodYear, in.PeriodMonth, base, base-net, net, dueAt, jsonRaw(steps)).Scan(&billID)
		if err == sql.ErrNoRows {
			// Lost a race with a concurrent run: the bill already exists.
			result.Skipped++
			continue
		}
		if err != nil {
			return result, err
		}
		for _, it := range items {
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO bill_items(school_id,bill_id,name,component_code,amount_idr,discount_idr,net_idr)
				 VALUES($1,$2,$3,NULLIF($4,''),$5,0,$5)`,
				schoolID, billID, it.Name, it.ComponentCode, it.AmountIDR); err != nil {
				return result, err
			}
		}
		for i, step := range steps {
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO bill_discount_steps(school_id,bill_id,discount_id,sequence,source,label,priority,base_before_idr,requested_idr,applied_idr,base_after_idr)
				 VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11)`,
				schoolID, billID, step.ID, i+1, step.Source, step.Label, step.Priority,
				step.BaseBefore, step.Requested, step.Applied, step.BaseAfter); err != nil {
				return result, err
			}
		}
		// BUG-131: a deposit (overpayment) settles the new bill right away.
		applied, err := applyDepositToBill(ctx, tx, schoolID, studentID, billID, net, in.ActorID)
		if err != nil {
			return result, err
		}
		result.DepositAppliedIDR += applied
		result.Generated++
		result.BillIDs = append(result.BillIDs, billID)
		newBills = append(newBills, newBillNotificationInfo{
			BillID:      billID,
			StudentID:   studentID,
			NetIDR:      net,
			PeriodYear:  in.PeriodYear,
			PeriodMonth: in.PeriodMonth,
		})
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}

	actorType := in.ActorType
	if actorType == "" {
		actorType = "internal"
	}
	a.audit(ctx, schoolID, "bill", in.SchemeID, "bills_generated", actorType, in.ActorID, "",
		"", nil, map[string]any{
			"period_year": in.PeriodYear, "period_month": in.PeriodMonth,
			"generated": result.Generated, "skipped": result.Skipped, "rejected": result.Rejected,
			"deposit_applied_idr": result.DepositAppliedIDR,
		})

	if len(newBills) > 0 {
		a.notifyGuardiansForNewBills(schoolID, newBills)
	}

	return result, nil
}

// enrollmentChecker is the part of academic-service generation needs; the
// production roster (*client.Client) implements it.
type enrollmentChecker interface {
	StudentEnrollment(ctx context.Context, schoolID, studentUserID string) (bool, string, error)
}

// studentEnrollments asks academic-service about each recipient. Without a
// roster the run is refused: bills are never issued to unverified users.
func (a *App) studentEnrollments(ctx context.Context, schoolID string, students []string) (map[string]model.StudentEnrollment, error) {
	out := make(map[string]model.StudentEnrollment, len(students))
	if len(students) == 0 {
		return out, nil
	}
	ec, ok := a.Roster.(enrollmentChecker)
	if a.Roster == nil || !ok || !a.Roster.Configured() {
		return nil, &httpError{http.StatusServiceUnavailable, "layanan akademik belum dikonfigurasi; status siswa tidak dapat diverifikasi"}
	}
	for _, s := range students {
		enrolled, grade, err := ec.StudentEnrollment(ctx, schoolID, s)
		if err != nil {
			return nil, &httpError{http.StatusBadGateway, "gagal memeriksa status siswa di layanan akademik: " + err.Error()}
		}
		out[s] = model.StudentEnrollment{Enrolled: enrolled, GradeLevel: grade}
	}
	return out, nil
}

// applyDepositToBill settles a freshly issued bill from the student's deposit
// balance: a negative deposit movement plus a verified 'deposit' payment whose
// source_id is that movement, then the status is re-derived.
func applyDepositToBill(ctx context.Context, tx *sql.Tx, schoolID, studentID, billID string, netIDR int64, actorID string) (int64, error) {
	var balance int64
	err := tx.QueryRowContext(ctx,
		`SELECT balance_idr FROM student_deposits WHERE school_id = $1 AND student_user_id = $2 FOR UPDATE`,
		schoolID, studentID).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	use := model.DepositToApply(balance, netIDR)
	if use == 0 {
		return 0, nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE student_deposits SET balance_idr = balance_idr - $3, updated_at = now() WHERE school_id = $1 AND student_user_id = $2`,
		schoolID, studentID, use); err != nil {
		return 0, err
	}
	var movementID string
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO deposit_movements (school_id, student_user_id, amount_idr, balance_after_idr, reason, ref)
		 VALUES ($1, $2, $3, $4, 'Pemakaian deposit untuk tagihan baru', $5) RETURNING id::text`,
		schoolID, studentID, -use, balance-use, "bill:"+billID).Scan(&movementID); err != nil {
		return 0, err
	}
	uploader := actorID
	if uploader == "" {
		uploader = studentID
	}
	var paymentID string
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO bill_payments (school_id, student_bill_id, amount_idr, transfer_at, reference, status, source, source_id,
		                            idempotency_key, uploader_user_id, verified_at)
		 VALUES ($1, $2, $3, now(), 'DEPOSIT', 'verified', 'deposit', $4::uuid, $5, $6::uuid, now())
		 RETURNING id::text`,
		schoolID, billID, use, movementID, "deposit-apply:"+billID, uploader).Scan(&paymentID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO bill_payment_allocations (school_id, payment_id, amount_idr) VALUES ($1, $2, $3)`,
		schoolID, paymentID, use); err != nil {
		return 0, err
	}
	return use, recomputeBillStatusQ(ctx, tx, schoolID, billID)
}

type newBillNotificationInfo struct {
	BillID      string
	StudentID   string
	NetIDR      int64
	PeriodYear  int
	PeriodMonth int
}

func (a *App) notifyGuardiansForNewBills(schoolID string, bills []newBillNotificationInfo) {
	if a.NotifySync {
		a.sendBillNotifications(context.Background(), schoolID, bills)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		a.sendBillNotifications(ctx, schoolID, bills)
	}()
}

func (a *App) sendBillNotifications(ctx context.Context, schoolID string, bills []newBillNotificationInfo) {
	if a.Notification == nil || !a.Notification.Configured() {
		return
	}
	if a.IdentitySvc == nil || !a.IdentitySvc.Configured() {
		return
	}

	months := []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

	for _, b := range bills {
		guardians, err := a.IdentitySvc.StudentGuardianIDs(ctx, schoolID, b.StudentID)
		if err != nil {
			a.raiseNotificationFailure(ctx, schoolID, "bill-issued:"+b.BillID, "data wali tidak dapat dibaca: "+err.Error(),
				map[string]any{"type": "bill_issued", "bill_id": b.BillID, "student_user_id": b.StudentID})
			continue
		}

		monthName := ""
		if b.PeriodMonth >= 1 && b.PeriodMonth <= 12 {
			monthName = months[b.PeriodMonth]
		}
		periodStr := fmt.Sprintf("%s %d", monthName, b.PeriodYear)
		if monthName == "" {
			periodStr = fmt.Sprintf("%d/%d", b.PeriodMonth, b.PeriodYear)
		}
		body := fmt.Sprintf("Tagihan %s sebesar %s", periodStr, formatRupiah(b.NetIDR))

		for _, g := range guardians {
			idemKey := fmt.Sprintf("bill-issued:%s:%s", b.BillID, g)
			err := a.Notification.Notify(ctx, idemKey, client.Notification{
				SchoolID: schoolID,
				UserID:   g,
				Type:     "bill_issued",
				Title:    "Tagihan baru terbit",
				Body:     body,
				Data: map[string]any{
					"bill_id": b.BillID,
					"url":     fmt.Sprintf("/orang-tua/tagihan/%s", b.BillID),
				},
				Channels: []string{"in_app"},
			})
			if err != nil {
				a.raiseNotificationFailure(ctx, schoolID, idemKey, err.Error(),
					map[string]any{"type": "bill_issued", "bill_id": b.BillID, "student_user_id": b.StudentID, "guardian_user_id": g})
			}
		}
	}
}

func formatRupiah(amount int64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	s := strconv.FormatInt(amount, 10)
	n := len(s)
	if n <= 3 {
		return fmt.Sprintf("Rp %s%s", sign, s)
	}
	var sb strings.Builder
	sb.WriteString("Rp ")
	sb.WriteString(sign)
	rem := n % 3
	if rem > 0 {
		sb.WriteString(s[:rem])
		if rem < n {
			sb.WriteString(".")
		}
	}
	for i := rem; i < n; i += 3 {
		sb.WriteString(s[i : i+3])
		if i+3 < n {
			sb.WriteString(".")
		}
	}
	return sb.String()
}

func (a *App) existingBillKeys(ctx context.Context, schoolID, schemeID string, year, month int, students []string) ([]model.BillKey, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT student_user_id::text FROM student_bills
		 WHERE school_id=$1 AND scheme_id=$2 AND period_year=$3 AND period_month=$4
		   AND student_user_id::text = ANY($5::text[])`,
		schoolID, schemeID, year, month, students)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.BillKey
	for rows.Next() {
		var studentID string
		if err := rows.Scan(&studentID); err != nil {
			return nil, err
		}
		out = append(out, model.BillKey{StudentUserID: studentID, Year: year, Month: month, SchemeID: schemeID})
	}
	return out, rows.Err()
}

// normalizeStudentIDs merges the array and single-student forms, trims and
// de-duplicates them while preserving order.
func normalizeStudentIDs(ids []string, single string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids)+1)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, id := range ids {
		add(id)
	}
	add(single)
	return out
}
