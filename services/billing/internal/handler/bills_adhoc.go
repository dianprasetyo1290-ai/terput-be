package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/adhoc"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// BILL-ADHOC (T12 §4.7): internal endpoints other modules (library,
// transport, event, extracurricular) use to raise, re-price, void and read
// ad-hoc bills. The bills live in student_bills (scheme_id NULL) so every
// existing parent/student/cashier/gateway flow shows and collects them.

// adhocErr maps a validation error of the pure package to 422.
func adhocErr(err error) error {
	var ve *adhoc.ValidationError
	if errors.As(err, &ve) {
		return &httpError{http.StatusUnprocessableEntity, ve.Msg}
	}
	return err
}

// adhocBill is the state of an ad-hoc bill.
type adhocBill struct {
	ID, StudentUserID, Status string
	NetIDR                    int64
	DueAt                     sql.NullTime
}

func lockAdhocBill(ctx context.Context, tx *sql.Tx, schoolID, module, ref string) (adhocBill, error) {
	var b adhocBill
	err := tx.QueryRowContext(ctx,
		`SELECT id::text,student_user_id::text,status,net_idr,due_at FROM student_bills
		 WHERE school_id=$1 AND source_module=$2 AND source_ref=$3 FOR UPDATE`, schoolID, module, ref).
		Scan(&b.ID, &b.StudentUserID, &b.Status, &b.NetIDR, &b.DueAt)
	if err == sql.ErrNoRows {
		return b, &httpError{http.StatusNotFound, "tagihan tidak ditemukan"}
	}
	return b, err
}

// originalItems are the items of an ad-hoc bill as first created.
func originalItems(ctx context.Context, q *sql.Tx, billID string) ([]adhoc.Item, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT name,amount_idr FROM bill_items WHERE bill_id=$1 AND adjustment_id IS NULL ORDER BY created_at,id`, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adhoc.Item
	for rows.Next() {
		var it adhoc.Item
		if err := rows.Scan(&it.Name, &it.AmountIDR); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// InternalCreateAdhocBill is POST /internal/bills/adhoc. Idempotent per
// (school_id, source_module, source_ref): the same body returns the same
// bill (200), a different body is 409 (use adjust).
func (a *App) InternalCreateAdhocBill(w http.ResponseWriter, r *http.Request) {
	var in adhoc.CreateRequest
	if !common.Decode(w, r, &in) {
		return
	}
	in.SchoolID, in.StudentUserID, in.SourceRef = strings.TrimSpace(in.SchoolID), strings.TrimSpace(in.StudentUserID), strings.TrimSpace(in.SourceRef)
	total, due, err := adhoc.ValidateCreate(in)
	if err != nil {
		writeErr(w, adhocErr(err))
		return
	}
	dueAt := model.DueDate(due.Year(), int(due.Month()), due.Day())
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := lockAdhocBill(r.Context(), tx, in.SchoolID, in.SourceModule, in.SourceRef)
	if err == nil {
		items, err := originalItems(r.Context(), tx, existing.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		same := existing.StudentUserID == in.StudentUserID && adhoc.SameItems(items, in.Items) &&
			existing.DueAt.Valid && existing.DueAt.Time.Equal(dueAt)
		if !same {
			common.WriteError(w, http.StatusConflict, "tagihan untuk source_ref ini sudah ada dengan isi berbeda, gunakan adjust")
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]any{"bill_id": existing.ID, "net_idr": existing.NetIDR, "status": existing.Status}, nil)
		return
	}
	var he *httpError
	if !errors.As(err, &he) || he.status != http.StatusNotFound {
		writeErr(w, err)
		return
	}
	var billID string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO student_bills(school_id,student_user_id,scheme_id,source_module,source_ref,period_year,period_month,base_idr,discount_idr,net_idr,due_at,status)
		 VALUES($1,$2,NULL,$3,$4,$5,$6,$7,0,$7,$8,'unpaid') RETURNING id::text`,
		in.SchoolID, in.StudentUserID, in.SourceModule, in.SourceRef, due.Year(), int(due.Month()), total, dueAt).Scan(&billID)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "tagihan sedang dibuat oleh permintaan lain, ulangi")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, it := range in.Items {
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO bill_items(school_id,bill_id,name,amount_idr,discount_idr,net_idr) VALUES($1,$2,$3,$4,0,$4)`,
			in.SchoolID, billID, strings.TrimSpace(it.Name), it.AmountIDR); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), in.SchoolID, "bill", billID, "adhoc_bill_created", "internal", "internal:"+in.SourceModule, "", "", nil,
		map[string]any{"source_module": in.SourceModule, "source_ref": in.SourceRef, "student_user_id": in.StudentUserID, "net_idr": total, "items": in.Items})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"bill_id": billID, "net_idr": total, "status": "unpaid"}, nil)
}

// hasPendingGateway reports a pending online payment on the bill.
func hasPendingGateway(ctx context.Context, tx *sql.Tx, billID string) (bool, error) {
	var pending bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM gateway_transaction_bills tb JOIN gateway_transactions t ON t.id=tb.transaction_id
		                WHERE tb.bill_id=$1 AND t.status='pending')`, billID).Scan(&pending)
	return pending, err
}

// InternalAdjustAdhocBill is POST /internal/bills/adhoc/adjust (proration or
// adjustment). Old items stay; one marked adjustment line carries the delta.
// Money paid above the new net is reversed off the bill into the student's
// deposit (BR-FIN-04).
func (a *App) InternalAdjustAdhocBill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SchoolID         string       `json:"school_id"`
		SourceModule     string       `json:"source_module"`
		SourceRef        string       `json:"source_ref"`
		Kind             string       `json:"kind"`
		Items            []adhoc.Item `json:"items"`
		NewNetIDR        *int64       `json:"new_net_idr"`
		Reason           string       `json:"reason"`
		SourceRefVersion string       `json:"source_ref_version"`
		IdempotencyKey   string       `json:"idempotency_key"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.SchoolID, in.SourceRef, in.Reason, in.IdempotencyKey = strings.TrimSpace(in.SchoolID), strings.TrimSpace(in.SourceRef), strings.TrimSpace(in.Reason), strings.TrimSpace(in.IdempotencyKey)
	if in.SchoolID == "" || !adhoc.SourceModules[in.SourceModule] || in.SourceRef == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id, source_module dan source_ref wajib valid")
		return
	}
	if in.Kind != "prorate" && in.Kind != "adjust" {
		common.WriteError(w, http.StatusUnprocessableEntity, "kind harus prorate atau adjust")
		return
	}
	if in.Reason == "" || in.IdempotencyKey == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "reason dan idempotency_key wajib diisi")
		return
	}
	var newNet int64
	switch {
	case in.NewNetIDR != nil && len(in.Items) == 0:
		newNet = *in.NewNetIDR
	case in.NewNetIDR == nil && len(in.Items) > 0:
		total, err := adhoc.Total(in.Items)
		if err != nil {
			writeErr(w, adhocErr(err))
			return
		}
		newNet = total
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "isi salah satu: items atau new_net_idr")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	bill, err := lockAdhocBill(r.Context(), tx, in.SchoolID, in.SourceModule, in.SourceRef)
	if err != nil {
		writeErr(w, err)
		return
	}
	var prevDeposit int64
	switch err := tx.QueryRowContext(r.Context(),
		`SELECT deposit_idr FROM bill_adjustments WHERE school_id=$1 AND idempotency_key=$2`, in.SchoolID, in.IdempotencyKey).Scan(&prevDeposit); err {
	case nil:
		common.WriteJSON(w, http.StatusOK, map[string]any{"bill_id": bill.ID, "net_idr": bill.NetIDR, "status": bill.Status, "deposit_idr": prevDeposit}, nil)
		return
	case sql.ErrNoRows:
	default:
		writeErr(w, err)
		return
	}
	if pending, err := hasPendingGateway(r.Context(), tx, bill.ID); err != nil {
		writeErr(w, err)
		return
	} else if pending {
		common.WriteError(w, http.StatusConflict, "tagihan sedang dibayar online, coba lagi setelah transaksi selesai")
		return
	}
	_, verified, reversed, _, err := billTotalsQ(r.Context(), tx, in.SchoolID, bill.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	adj, err := adhoc.Adjust(adhoc.Bill{NetIDR: bill.NetIDR, Status: bill.Status}, verified-reversed, newNet)
	if err != nil {
		code := http.StatusUnprocessableEntity
		if errors.Is(err, adhoc.ErrVoid) {
			code = http.StatusConflict
		}
		common.WriteError(w, code, err.Error())
		return
	}
	var adjID string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO bill_adjustments(school_id,bill_id,kind,delta_idr,net_before_idr,net_after_idr,deposit_idr,reason,source_ref_version,idempotency_key)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10) RETURNING id::text`,
		in.SchoolID, bill.ID, in.Kind, adj.DeltaIDR, bill.NetIDR, adj.NetIDR, adj.DepositIDR, in.Reason, in.SourceRefVersion, in.IdempotencyKey).Scan(&adjID); err != nil {
		writeErr(w, err)
		return
	}
	label := map[string]string{"prorate": "Prorata", "adjust": "Penyesuaian"}[in.Kind] + ": " + in.Reason
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO bill_items(school_id,bill_id,name,amount_idr,discount_idr,net_idr,adjustment_id) VALUES($1,$2,$3,$4,0,$4,$5)`,
		in.SchoolID, bill.ID, label, adj.DeltaIDR, adjID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE student_bills SET base_idr=$1,discount_idr=0,net_idr=$1,status=$2,updated_at=now() WHERE id=$3`, adj.NetIDR, adj.Status, bill.ID); err != nil {
		writeErr(w, err)
		return
	}
	if adj.DepositIDR > 0 {
		if err = a.moveOverpaymentToDeposit(r.Context(), tx, in.SchoolID, bill, adj.DepositIDR, adjID, in.Reason); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), in.SchoolID, "bill", bill.ID, "adhoc_bill_adjusted", "internal", "internal:"+in.SourceModule, "", in.Reason,
		map[string]any{"net_idr": bill.NetIDR, "status": bill.Status}, map[string]any{"net_idr": adj.NetIDR, "status": adj.Status, "deposit_idr": adj.DepositIDR})
	common.WriteJSON(w, http.StatusOK, map[string]any{"bill_id": bill.ID, "net_idr": adj.NetIDR, "status": adj.Status, "deposit_idr": adj.DepositIDR}, nil)
}

// moveOverpaymentToDeposit reverses amount of the bill's verified payments
// (newest first) and credits the student's deposit by the same amount.
func (a *App) moveOverpaymentToDeposit(ctx context.Context, tx *sql.Tx, schoolID string, bill adhocBill, amount int64, adjID, reason string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT p.id::text,p.amount_idr-COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv WHERE rv.payment_id=p.id),0)
		 FROM bill_payments p WHERE p.student_bill_id=$1 AND p.school_id=$2 AND p.status='verified' ORDER BY p.verified_at DESC,p.id`, bill.ID, schoolID)
	if err != nil {
		return err
	}
	var sources []model.RefundSource
	for rows.Next() {
		var s model.RefundSource
		if err := rows.Scan(&s.PaymentID, &s.Remaining); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, s)
	}
	rows.Close()
	parts, err := model.SpreadRefund(amount, sources)
	if err != nil {
		return err
	}
	for _, p := range parts {
		// actor_id is NOT NULL uuid; an internal re-pricing has no user, so
		// the bill's student is recorded (same convention as InternalStudentRefund).
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bill_payment_reversals(school_id,payment_id,amount_idr,reason,actor_id,idempotency_key)
			 VALUES($1,$2,$3,$4,$5,$6)`,
			schoolID, p.PaymentID, p.AmountIDR, "Kelebihan bayar ke deposit: "+reason, bill.StudentUserID, "adhoc-adjust:"+adjID+":"+p.PaymentID); err != nil {
			return err
		}
	}
	var balance int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO student_deposits(school_id,student_user_id,balance_idr) VALUES($1,$2,$3)
		 ON CONFLICT (school_id,student_user_id) DO UPDATE SET balance_idr=student_deposits.balance_idr+EXCLUDED.balance_idr,updated_at=now()
		 RETURNING balance_idr`, schoolID, bill.StudentUserID, amount).Scan(&balance); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO deposit_movements(school_id,student_user_id,amount_idr,balance_after_idr,reason,ref) VALUES($1,$2,$3,$4,$5,$6)`,
		schoolID, bill.StudentUserID, amount, balance, "Kelebihan bayar tagihan ad-hoc", "bill_adjustment:"+adjID)
	return err
}

// InternalVoidBill is POST /internal/bills/{id}/void {school_id, reason}:
// only ad-hoc bills without a settled payment; idempotent.
func (a *App) InternalVoidBill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SchoolID string `json:"school_id"`
		Reason   string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.SchoolID, in.Reason = strings.TrimSpace(in.SchoolID), strings.TrimSpace(in.Reason)
	if in.SchoolID == "" || in.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id dan reason wajib diisi")
		return
	}
	id := param(r, "id")
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	var module sql.NullString
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,source_module FROM student_bills WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, id, in.SchoolID).Scan(&status, &module)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if !module.Valid {
		common.WriteError(w, http.StatusUnprocessableEntity, "hanya tagihan ad-hoc yang dapat dibatalkan lewat endpoint ini")
		return
	}
	if status == string(model.BillVoid) {
		common.WriteJSON(w, http.StatusOK, map[string]any{"bill_id": id, "status": status}, nil)
		return
	}
	_, verified, reversed, _, err := billTotalsQ(r.Context(), tx, in.SchoolID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if verified-reversed > 0 {
		common.WriteError(w, http.StatusConflict, "tagihan sudah dibayar, gunakan adjust ke 0 agar kelebihan masuk deposit")
		return
	}
	if pending, err := hasPendingGateway(r.Context(), tx, id); err != nil {
		writeErr(w, err)
		return
	} else if pending {
		common.WriteError(w, http.StatusConflict, "tagihan sedang dibayar online")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE student_bills SET status='void',void_reason=$1,updated_at=now() WHERE id::text=$2`, in.Reason, id); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), in.SchoolID, "bill", id, "adhoc_bill_voided", "internal", "internal:"+module.String, "", in.Reason,
		map[string]any{"status": status}, map[string]any{"status": "void"})
	common.WriteJSON(w, http.StatusOK, map[string]any{"bill_id": id, "status": "void"}, nil)
}

// InternalBillBySource is GET /internal/bills/by-source?school_id=&source_module=&source_ref=.
func (a *App) InternalBillBySource(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	schoolID, module, ref := strings.TrimSpace(q.Get("school_id")), strings.TrimSpace(q.Get("source_module")), strings.TrimSpace(q.Get("source_ref"))
	if schoolID == "" || module == "" || ref == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id, source_module dan source_ref wajib diisi")
		return
	}
	var id, status string
	var net int64
	var due sql.NullTime
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id::text,status,net_idr,due_at FROM student_bills WHERE school_id=$1 AND source_module=$2 AND source_ref=$3`, schoolID, module, ref).
		Scan(&id, &status, &net, &due)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	_, verified, reversed, outstanding, err := billTotalsQ(r.Context(), a.DB, schoolID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if status == string(model.BillVoid) {
		outstanding = 0
	}
	var dueAt any
	if due.Valid {
		dueAt = due.Time.Format(time.RFC3339)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"bill_id": id, "net_idr": net, "paid_idr": verified - reversed,
		"outstanding_idr": outstanding, "status": status, "due_at": dueAt}, nil)
}
