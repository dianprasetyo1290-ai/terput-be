package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// Refunds for dropped applicants (BUG-040). When the drop job releases a
// seat, whatever the guardian already paid for that stage becomes a refund
// record sized by the period policy (refund_policy_bp minus
// refund_admin_fee_idr). The committee then records the payout (transfer or
// cash) or waives it with a reason; both are audited and the guardian is
// notified. applications.refund_due_idr mirrors the open amount.

// recordRefundDue stores the refund owed for a dropped stage (idempotent per
// application+stage) and returns the refundable amount.
func recordRefundDue(ctx context.Context, tx *sql.Tx, schoolID, applicationID, stage string, paid int64) (int64, error) {
	if paid <= 0 {
		return 0, nil
	}
	var (
		policyBP int
		adminFee int64
	)
	if err := tx.QueryRowContext(ctx,
		`SELECT p.refund_policy_bp,p.refund_admin_fee_idr FROM applications a JOIN admission_periods p ON p.id=a.period_id
		 WHERE a.id=$1 AND a.school_id=$2`, applicationID, schoolID).Scan(&policyBP, &adminFee); err != nil {
		return 0, err
	}
	refundable := model.RefundAmount(paid, policyBP, adminFee)
	status := "due"
	if refundable == 0 {
		status = "waived"
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO admission_refunds(school_id,application_id,stage,paid_idr,refundable_idr,policy_bp,admin_fee_idr,status,note)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,CASE WHEN $8='waived' THEN 'Tidak ada dana yang dapat dikembalikan sesuai kebijakan' END)
		 ON CONFLICT (application_id,stage) DO NOTHING`,
		schoolID, applicationID, stage, paid, refundable, policyBP, adminFee, status)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 || refundable == 0 {
		return refundable, nil
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE applications SET refund_due_idr=COALESCE(refund_due_idr,0)+$2 WHERE id=$1`, applicationID, refundable)
	return refundable, err
}

// ListRefunds is GET /admin/refunds?status=.
func (a *App) ListRefunds(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT f.id,f.application_id,a.registration_number,ap.full_name,f.stage,f.paid_idr,f.refundable_idr,f.policy_bp,
		        f.admin_fee_idr,f.status,COALESCE(f.method,''),COALESCE(f.bank_name,''),COALESCE(f.account_name,''),
		        COALESCE(f.account_number,''),COALESCE(f.reference,''),f.paid_at,COALESCE(f.note,''),
		        COALESCE(f.processed_by_name,''),f.processed_at,f.created_at
		 FROM admission_refunds f JOIN applications a ON a.id=f.application_id JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE f.school_id=$1 AND ($2='' OR f.status=$2) ORDER BY f.created_at DESC LIMIT 200`,
		common.EffectiveSchoolID(r), r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, appID, number, name, stage, status, method, bank, accName, accNo, ref, note, by string
			paid, refundable, fee                                                               int64
			bp                                                                                  int
			paidAt, processedAt                                                                 sql.NullTime
			createdAt                                                                           time.Time
		)
		if err := rows.Scan(&id, &appID, &number, &name, &stage, &paid, &refundable, &bp, &fee, &status, &method, &bank,
			&accName, &accNo, &ref, &paidAt, &note, &by, &processedAt, &createdAt); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{"id": id, "application_id": appID, "registration_number": number, "applicant_name": name,
			"stage": stage, "stage_label": stageLabel(stage), "paid_idr": paid, "refundable_idr": refundable,
			"policy_bp": bp, "admin_fee_idr": fee, "status": status, "method": method, "bank_name": bank,
			"account_name": accName, "account_number": model.MaskAccountNumber(accNo), "reference": ref, "note": note,
			"processed_by_name": by, "created_at": createdAt}
		timeIfSet(item, "paid_at", nullTimePtr(paidAt))
		timeIfSet(item, "processed_at", nullTimePtr(processedAt))
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type refundPayoutInput struct {
	Method        string `json:"method"`
	BankName      string `json:"bank_name"`
	AccountName   string `json:"account_name"`
	AccountNumber string `json:"account_number"`
	Reference     string `json:"reference"`
	PaidAt        string `json:"paid_at"`
	Note          string `json:"note"`
}

// ProcessRefund is POST /admin/refunds/{id}/process: records the payout.
func (a *App) ProcessRefund(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	var in refundPayoutInput
	if !common.Decode(w, r, &in) {
		return
	}
	in.BankName, in.AccountName = strings.TrimSpace(in.BankName), strings.TrimSpace(in.AccountName)
	in.AccountNumber, in.Reference = strings.TrimSpace(in.AccountNumber), strings.TrimSpace(in.Reference)
	switch in.Method {
	case "transfer":
		if in.BankName == "" || in.AccountName == "" || in.AccountNumber == "" || in.Reference == "" {
			common.WriteError(w, http.StatusUnprocessableEntity, "bank, nama dan nomor rekening, serta nomor referensi transfer wajib diisi")
			return
		}
	case "cash":
		if in.Reference == "" {
			common.WriteError(w, http.StatusUnprocessableEntity, "nomor kuitansi wajib diisi untuk pengembalian tunai")
			return
		}
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "metode pengembalian harus transfer atau tunai")
		return
	}
	paidAt := time.Now()
	if in.PaidAt != "" {
		t, err := parseTime(in.PaidAt)
		if err != nil || t.After(time.Now().Add(time.Minute)) {
			common.WriteError(w, http.StatusUnprocessableEntity, "tanggal pengembalian tidak valid")
			return
		}
		paidAt = t
	}
	a.closeRefund(w, r, "paid", in, paidAt)
}

// WaiveRefund is POST /admin/refunds/{id}/waive {reason}: closes a refund
// without payout (e.g. the guardian moved the funds to a sibling's bill).
func (a *App) WaiveRefund(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan wajib diisi")
		return
	}
	a.closeRefund(w, r, "waived", refundPayoutInput{Note: strings.TrimSpace(in.Reason)}, time.Time{})
}

func (a *App) closeRefund(w http.ResponseWriter, r *http.Request, status string, in refundPayoutInput, paidAt time.Time) {
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var (
		applicationID, current string
		refundable             int64
	)
	err = tx.QueryRowContext(ctx,
		`SELECT application_id,status,refundable_idr FROM admission_refunds WHERE id::text=$1 AND school_id=$2 FOR UPDATE`,
		param(r, "id"), schoolID).Scan(&applicationID, &current, &refundable)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "data pengembalian dana tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if current != "due" {
		common.WriteError(w, http.StatusConflict, "pengembalian dana ini sudah diproses")
		return
	}
	var paidAtArg any
	if status == "paid" {
		paidAtArg = paidAt
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE admission_refunds SET status=$3,method=NULLIF($4,''),bank_name=NULLIF($5,''),account_name=NULLIF($6,''),
		        account_number=NULLIF($7,''),reference=NULLIF($8,''),paid_at=$9,note=NULLIF($10,''),
		        processed_by=$11,processed_by_name=$12,processed_at=now()
		 WHERE id::text=$1 AND school_id=$2`,
		param(r, "id"), schoolID, status, in.Method, in.BankName, in.AccountName, in.AccountNumber, in.Reference,
		paidAtArg, in.Note, claims.UserID, claims.Name); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE applications SET refund_due_idr=GREATEST(refund_due_idr-$2,0) WHERE id=$1`, applicationID, refundable); err != nil {
		writeErr(w, err)
		return
	}
	after := map[string]any{"status": status, "amount_idr": refundable, "method": in.Method, "reference": in.Reference,
		"account_number": model.MaskAccountNumber(in.AccountNumber)}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data,reason)
		 VALUES($1,'application',$2,$3,'admin',$4,$5,$6,NULLIF($7,''))`,
		schoolID, applicationID, "refund_"+status, claims.UserID, claims.Name, jsonRaw(after), in.Note); err != nil {
		writeErr(w, err)
		return
	}
	title, body := "Dana dikembalikan", "Pengembalian dana sebesar "+formatIDR(refundable)+" telah diproses"
	if status == "paid" {
		if in.Method == "transfer" {
			body += " melalui transfer ke rekening " + in.BankName + " " + model.MaskAccountNumber(in.AccountNumber)
		} else {
			body += " secara tunai"
		}
		body += " pada " + formatDateWIB(paidAt) + "."
	} else {
		title, body = "Pengembalian dana ditutup", "Pengembalian dana pendaftaran ditutup oleh panitia: "+in.Note
	}
	if err = a.notify(ctx, tx, schoolID, applicationID, "refund_"+status, title, body, "refund:"+param(r, "id"), nil); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id"), "status": status}, nil)
}

// applicationRefunds lists the refunds of one application for the detail
// views (staff and guardian). Account numbers are masked.
func applicationRefunds(ctx context.Context, q queryer, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id,stage,paid_idr,refundable_idr,policy_bp,admin_fee_idr,status,COALESCE(method,''),COALESCE(bank_name,''),
		        COALESCE(account_number,''),COALESCE(reference,''),paid_at,created_at
		 FROM admission_refunds WHERE school_id=$1 AND application_id=$2 ORDER BY created_at`, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, stage, status, method, bank, accNo, ref string
			paid, refundable, fee                       int64
			bp                                          int
			paidAt                                      sql.NullTime
			createdAt                                   time.Time
		)
		if err := rows.Scan(&id, &stage, &paid, &refundable, &bp, &fee, &status, &method, &bank, &accNo, &ref, &paidAt, &createdAt); err != nil {
			return nil, err
		}
		item := map[string]any{"id": id, "stage": stage, "stage_label": stageLabel(stage), "paid_idr": paid,
			"refundable_idr": refundable, "policy_bp": bp, "admin_fee_idr": fee, "status": status, "method": method,
			"bank_name": bank, "account_number": model.MaskAccountNumber(accNo), "reference": ref, "created_at": createdAt}
		timeIfSet(item, "paid_at", nullTimePtr(paidAt))
		out = append(out, item)
	}
	return out, rows.Err()
}
