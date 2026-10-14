package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// Refund (PRD menu 25): requested -> approved|rejected -> processed.
// The approver must differ from the requester (segregation of duties);
// processing reverses verified payments of the bill (bill_payment_reversals
// tagged with refund_id), lowering the bill's paid balance.

// adminSchool is the shared finance-staff + tenant guard (Finance Admin and
// Finance Manager, PRD menus 18–25).
func adminSchool(w http.ResponseWriter, r *http.Request) (*common.Claims, string) {
	return common.RequireFinanceStaff(w, r)
}

// ListRefunds is GET /admin/refunds?status=&student_user_id=.
func (a *App) ListRefunds(w http.ResponseWriter, r *http.Request) {
	c, schoolID := adminSchool(w, r)
	if c == nil {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	studentID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))

	summary := map[string]any{}
	sumRows, err := a.DB.QueryContext(r.Context(),
		`SELECT status,count(*),COALESCE(sum(amount_idr),0) FROM bill_refunds
		 WHERE school_id=$1 AND ($2='' OR student_user_id::text=$2) GROUP BY status`, schoolID, studentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for sumRows.Next() {
		var st string
		var n int
		var amt int64
		if err := sumRows.Scan(&st, &n, &amt); err != nil {
			sumRows.Close()
			writeErr(w, err)
			return
		}
		summary[st] = map[string]any{"count": n, "amount_idr": amt}
	}
	sumRows.Close()

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT f.id,f.student_bill_id::text,f.student_user_id::text,f.amount_idr,f.reason,f.status,
		        f.requested_by::text,COALESCE(f.decided_by::text,''),f.decided_at,COALESCE(f.decision_note,''),
		        COALESCE(f.processed_by::text,''),f.processed_at,f.created_at,
		        b.period_year,b.period_month,b.net_idr,COALESCE(s.name,'')
		 FROM bill_refunds f
		 JOIN student_bills b ON b.id=f.student_bill_id
		 LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE f.school_id=$1 AND ($2='' OR f.status=$2) AND ($3='' OR f.student_user_id::text=$3)
		 ORDER BY f.created_at DESC LIMIT 500`, schoolID, status, studentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, billID, student, reason, st, reqBy, decBy, note, procBy, schemeName string
			amount, net                                                             int64
			decAt, procAt                                                           sql.NullTime
			createdAt                                                               time.Time
			year, month                                                             int
		)
		if err := rows.Scan(&id, &billID, &student, &amount, &reason, &st, &reqBy, &decBy, &decAt, &note,
			&procBy, &procAt, &createdAt, &year, &month, &net, &schemeName); err != nil {
			writeErr(w, err)
			return
		}
		m := map[string]any{
			"id": id, "student_bill_id": billID, "student_user_id": student, "amount_idr": amount,
			"reason": reason, "status": st, "requested_by": reqBy, "decided_by": decBy,
			"decision_note": note, "processed_by": procBy, "created_at": createdAt,
			"period_year": year, "period_month": month, "bill_net_idr": net, "scheme_name": schemeName,
		}
		if decAt.Valid {
			m["decided_at"] = decAt.Time
		}
		if procAt.Valid {
			m["processed_at"] = procAt.Time
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, map[string]any{"summary": summary, "viewer_id": c.UserID})
}

// refundableQ is the bill's paid balance minus refunds already requested or
// approved but not yet processed (excludeID skips the refund being processed).
func refundableQ(r *http.Request, q queryer, schoolID, billID, excludeID string) (int64, error) {
	_, verified, reversed, _, err := billTotalsQ(r.Context(), q, schoolID, billID)
	if err != nil {
		return 0, err
	}
	var pending int64
	if err := q.QueryRowContext(r.Context(),
		`SELECT COALESCE(sum(amount_idr),0) FROM bill_refunds
		 WHERE school_id=$1 AND student_bill_id::text=$2 AND status IN ('requested','approved') AND id::text<>$3`,
		schoolID, billID, excludeID).Scan(&pending); err != nil {
		return 0, err
	}
	return verified - reversed - pending, nil
}

// CreateRefund is POST /admin/refunds {student_bill_id, amount_idr, reason}.
func (a *App) CreateRefund(w http.ResponseWriter, r *http.Request) {
	c, schoolID := adminSchool(w, r)
	if c == nil {
		return
	}
	var in struct {
		StudentBillID string `json:"student_bill_id"`
		AmountIDR     int64  `json:"amount_idr"`
		Reason        string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.StudentBillID == "" || in.AmountIDR <= 0 || in.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "tagihan, nominal (> 0) dan alasan wajib diisi")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var studentID, billStatus string
	err = tx.QueryRowContext(r.Context(),
		`SELECT student_user_id::text,status FROM student_bills WHERE id::text=$1 AND school_id=$2 FOR UPDATE`,
		in.StudentBillID, schoolID).Scan(&studentID, &billStatus)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if billStatus == string(model.BillVoid) {
		common.WriteError(w, http.StatusUnprocessableEntity, "tagihan sudah dibatalkan")
		return
	}
	refundable, err := refundableQ(r, tx, schoolID, in.StudentBillID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	if in.AmountIDR > refundable {
		common.WriteError(w, http.StatusUnprocessableEntity, "nominal refund melebihi saldo terbayar yang dapat dikembalikan")
		return
	}
	var id string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO bill_refunds(school_id,student_bill_id,student_user_id,amount_idr,reason,requested_by)
		 VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		schoolID, in.StudentBillID, studentID, in.AmountIDR, in.Reason, c.UserID).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "refund", id, "refund_requested", "admin", c.UserID, "", in.Reason,
		nil, map[string]any{"bill_id": in.StudentBillID, "amount_idr": in.AmountIDR})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "requested"}, nil)
}

// DecideRefund is POST /admin/refunds/{id}/decision {action: approve|reject, note}.
func (a *App) DecideRefund(w http.ResponseWriter, r *http.Request) {
	c, schoolID := adminSchool(w, r)
	if c == nil {
		return
	}
	var in struct {
		Action string `json:"action"`
		Note   string `json:"note"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	target := map[string]string{"approve": "approved", "reject": "rejected"}[in.Action]
	if target == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "action harus approve atau reject")
		return
	}
	if target == "rejected" && in.Note == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan penolakan wajib diisi")
		return
	}
	id := param(r, "id")
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var status, requestedBy string
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,requested_by::text FROM bill_refunds WHERE id::text=$1 AND school_id=$2 FOR UPDATE`,
		id, schoolID).Scan(&status, &requestedBy)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "refund tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != "requested" {
		common.WriteError(w, http.StatusUnprocessableEntity, "refund sudah diputuskan")
		return
	}
	if requestedBy == c.UserID {
		common.WriteError(w, http.StatusForbidden, "penyetuju harus berbeda dari pengaju refund")
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE bill_refunds SET status=$1,decided_by=$2,decided_at=now(),decision_note=NULLIF($3,''),updated_at=now()
		 WHERE id::text=$4 AND school_id=$5`, target, c.UserID, in.Note, id, schoolID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "refund", id, "refund_"+target, "admin", c.UserID, "", in.Note,
		map[string]any{"status": status}, map[string]any{"status": target})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": target}, nil)
}

// ProcessRefund is POST /admin/refunds/{id}/process. Only an approved refund
// is processed; it reverses the bill's verified payments newest first.
func (a *App) ProcessRefund(w http.ResponseWriter, r *http.Request) {
	c, schoolID := adminSchool(w, r)
	if c == nil {
		return
	}
	id := param(r, "id")
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var status, billID, reason string
	var amount int64
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,student_bill_id::text,amount_idr,reason FROM bill_refunds
		 WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, id, schoolID).Scan(&status, &billID, &amount, &reason)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "refund tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != "approved" {
		common.WriteError(w, http.StatusUnprocessableEntity, "hanya refund yang sudah disetujui yang dapat diproses")
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`SELECT 1 FROM student_bills WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, billID, schoolID); err != nil {
		writeErr(w, err)
		return
	}

	rows, err := tx.QueryContext(r.Context(),
		`SELECT p.id::text,p.amount_idr-COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv WHERE rv.payment_id=p.id),0)
		 FROM bill_payments p WHERE p.student_bill_id::text=$1 AND p.school_id=$2 AND p.status='verified'
		 ORDER BY p.verified_at DESC,p.id`, billID, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var sources []model.RefundSource
	for rows.Next() {
		var s model.RefundSource
		if err := rows.Scan(&s.PaymentID, &s.Remaining); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		sources = append(sources, s)
	}
	rows.Close()
	parts, err := model.SpreadRefund(amount, sources)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "saldo terbayar tagihan tidak lagi mencukupi nominal refund")
		return
	}
	for _, p := range parts {
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO bill_payment_reversals(school_id,payment_id,amount_idr,reason,actor_id,idempotency_key,refund_id)
			 VALUES($1,$2,$3,$4,$5,$6,$7)`,
			schoolID, p.PaymentID, p.AmountIDR, "Refund: "+reason, c.UserID, "refund:"+id+":"+p.PaymentID, id); err != nil {
			writeErr(w, err)
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE bill_refunds SET status='processed',processed_by=$1,processed_at=now(),updated_at=now()
		 WHERE id::text=$2 AND school_id=$3`, c.UserID, id, schoolID); err != nil {
		writeErr(w, err)
		return
	}
	if err = recomputeBillStatusQ(r.Context(), tx, schoolID, billID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "refund", id, "refund_processed", "admin", c.UserID, "", reason,
		map[string]any{"status": status}, map[string]any{"status": "processed", "bill_id": billID, "amount_idr": amount})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "processed"}, nil)
}
