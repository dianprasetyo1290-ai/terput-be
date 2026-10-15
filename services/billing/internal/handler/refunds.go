package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

const reqTypeRefund = "refund"

// Refund (PRD menu 25): requested -> approved|rejected -> processed.
// The decision is made in core's Approval Center (maker != checker, stages
// by amount) and arrives through RefundCallback; processing reverses verified payments of the bill (bill_payment_reversals
// tagged with refund_id), lowering the bill's paid balance.

// adminSchool is the shared finance-staff + tenant guard (Finance Admin and
// Finance Manager, PRD menus 18–25).
func adminSchool(w http.ResponseWriter, r *http.Request) (*common.Claims, string) {
	return common.RequireFinanceStaff(w, r)
}

// ListRefunds is GET /admin/refunds?status=&student_user_id=&page=&per_page=.
// meta carries the pagination plus the per-status summary of the filter.
func (a *App) ListRefunds(w http.ResponseWriter, r *http.Request) {
	c, schoolID := adminSchool(w, r)
	if c == nil {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	studentID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	page, perPage := common.Page(r)

	summary := map[string]any{}
	sumRows, err := a.DB.QueryContext(r.Context(),
		`SELECT status,count(*),COALESCE(sum(amount_idr),0) FROM bill_refunds
		 WHERE school_id=$1 AND ($2='' OR student_user_id::text=$2) GROUP BY status`, schoolID, studentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	total := 0
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
		if status == "" || status == st {
			total += n
		}
	}
	sumRows.Close()

	// Student-level refunds (no bill, e.g. withdrawal credit) are listed too.
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT f.id,COALESCE(f.student_bill_id::text,''),f.student_user_id::text,f.amount_idr,f.reason,f.status,
		        f.requested_by::text,COALESCE(f.decided_by::text,''),f.decided_at,COALESCE(f.decision_note,''),
		        COALESCE(f.processed_by::text,''),f.processed_at,f.created_at,
		        COALESCE(b.period_year,0),COALESCE(b.period_month,0),COALESCE(b.net_idr,0),COALESCE(s.name,''),
		        COALESCE(f.approval_id,''),COALESCE(f.transfer_ref,''),COALESCE(to_char(f.transfer_date,'YYYY-MM-DD'),''),
		        COALESCE(f.process_note,''),f.deposit_idr,COALESCE(f.wallet_id::text,''),COALESCE(wl.kind,'')
		 FROM bill_refunds f
		 LEFT JOIN wallets wl ON wl.id=f.wallet_id
		 LEFT JOIN student_bills b ON b.id=f.student_bill_id
		 LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE f.school_id=$1 AND ($2='' OR f.status=$2) AND ($3='' OR f.student_user_id::text=$3)
		 ORDER BY f.created_at DESC, f.id LIMIT $4 OFFSET $5`, schoolID, status, studentID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, billID, student, reason, st, reqBy, decBy, note, procBy, schemeName string
			approvalID, transferRef, transferDate, processNote, walletID, walletKind string
			amount, net, depositIDR                                                 int64
			decAt, procAt                                                           sql.NullTime
			createdAt                                                               time.Time
			year, month                                                             int
		)
		if err := rows.Scan(&id, &billID, &student, &amount, &reason, &st, &reqBy, &decBy, &decAt, &note,
			&procBy, &procAt, &createdAt, &year, &month, &net, &schemeName,
			&approvalID, &transferRef, &transferDate, &processNote, &depositIDR, &walletID, &walletKind); err != nil {
			writeErr(w, err)
			return
		}
		m := map[string]any{
			"id": id, "student_bill_id": billID, "student_user_id": student, "amount_idr": amount,
			"reason": reason, "status": st, "requested_by": reqBy, "decided_by": decBy,
			"decision_note": note, "processed_by": procBy, "created_at": createdAt,
			"period_year": year, "period_month": month, "bill_net_idr": net, "scheme_name": schemeName,
			"approval_id": approvalID, "via_approval_center": approvalID != "",
			"transfer_ref": transferRef, "transfer_date": transferDate, "process_note": processNote,
			"deposit_idr": depositIDR, "wallet_id": walletID, "wallet_kind": walletKind,
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
	common.WriteJSON(w, http.StatusOK, items, map[string]any{
		"page": page, "per_page": perPage, "total": total, "summary": summary, "viewer_id": c.UserID,
	})
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
		`SELECT `+pendingSQL("$3")+` FROM student_bills b WHERE b.school_id=$1 AND b.id::text=$2`,
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
	var year, month int
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO bill_refunds(school_id,student_bill_id,student_user_id,amount_idr,reason,requested_by)
		 VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
		schoolID, in.StudentBillID, studentID, in.AmountIDR, in.Reason, c.UserID).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.QueryRowContext(r.Context(), `SELECT period_year,period_month FROM student_bills WHERE id::text=$1`,
		in.StudentBillID).Scan(&year, &month); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	// The decision belongs to core's Approval Center (request type 'refund',
	// stages by amount). If core does not take the request, nothing remains.
	amount := in.AmountIDR
	resp, err := a.submitApproval(r.Context(), client.ApprovalRequest{
		SchoolID: schoolID, RequestType: reqTypeRefund, SourceID: id,
		Title: fmt.Sprintf("Refund tagihan %02d/%d", month, year),
		Summary: map[string]any{
			"student_user_id": studentID, "student_bill_id": in.StudentBillID,
			"period_year": year, "period_month": month, "amount_idr": in.AmountIDR,
			"refundable_idr": refundable, "reason": in.Reason,
		},
		AmountIDR: &amount, MakerUserID: c.UserID,
		CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/refunds/" + id + "/callback",
		CorrelationID: id, IdempotencyKey: "refund:" + id,
	})
	if err != nil {
		_, _ = a.DB.ExecContext(r.Context(), `DELETE FROM bill_refunds WHERE id::text=$1 AND status='requested'`, id)
		writeErr(w, err)
		return
	}
	if _, err = a.DB.ExecContext(r.Context(), `UPDATE bill_refunds SET approval_id=$1,updated_at=now() WHERE id::text=$2`, resp.ID, id); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(r.Context(), schoolID, "billing", "refund", id, "refund_requested", c.ActiveRole, c.UserID, in.Reason,
		nil, map[string]any{"bill_id": in.StudentBillID, "amount_idr": in.AmountIDR, "approval_id": resp.ID})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "requested", "approval_id": resp.ID}, nil)
}

// DecideRefund is POST /admin/refunds/{id}/decision {action: approve|reject, note}.
// It only decides refunds created before the Approval Center integration
// (no approval_id); every newer refund is decided in core.
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

	var status, requestedBy, approvalID string
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,requested_by::text,COALESCE(approval_id,'') FROM bill_refunds WHERE id::text=$1 AND school_id=$2 FOR UPDATE`,
		id, schoolID).Scan(&status, &requestedBy, &approvalID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "refund tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if approvalID != "" {
		common.WriteError(w, http.StatusConflict, "refund ini diputuskan melalui Approval Center, bukan dari menu Refund")
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
	if err = syncWalletClosureQ(r.Context(), tx, schoolID, id, target); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(r.Context(), schoolID, "billing", "refund", id, "refund_"+target, c.ActiveRole, c.UserID, in.Note,
		map[string]any{"status": status}, map[string]any{"status": target})
	a.notifyRefund(r.Context(), schoolID, id, target, in.Note)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": target}, nil)
}

// ProcessRefund is POST /admin/refunds/{id}/process {transfer_ref,
// transfer_date?, note?}. Only an approved refund is processed, against the
// bank transfer that paid it out. Bill portions reverse the bill's verified
// payments newest first; a deposit portion reduces student_deposits with a
// deposit movement and leaves bill payments untouched.
func (a *App) ProcessRefund(w http.ResponseWriter, r *http.Request) {
	c, schoolID := adminSchool(w, r)
	if c == nil {
		return
	}
	var in struct {
		TransferRef  string `json:"transfer_ref"`
		TransferDate string `json:"transfer_date"`
		Note         string `json:"note"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	proof, perr := model.ParseTransferProof(in.TransferRef, in.TransferDate, a.now(), a.location())
	if perr != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, perr.Error())
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	id := param(r, "id")
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var status, studentID, reason string
	var billID sql.NullString
	var amount, depositIDR int64
	var walletID sql.NullString
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,student_bill_id::text,student_user_id::text,amount_idr,deposit_idr,reason,wallet_id::text FROM bill_refunds
		 WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, id, schoolID).
		Scan(&status, &billID, &studentID, &amount, &depositIDR, &reason, &walletID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "refund tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != model.RefundApproved {
		common.WriteError(w, http.StatusUnprocessableEntity, "hanya refund yang sudah disetujui yang dapat diproses")
		return
	}
	// A wallet refund (FR-125-005) empties the wallet instead of touching
	// bills or the deposit.
	var walletRefund walletRow
	var walletBefore int64
	if walletID.Valid {
		if walletRefund, walletBefore, err = payRefundFromWallet(r.Context(), tx, schoolID, id, walletID.String, c.UserID, amount); err != nil {
			writeErr(w, err)
			return
		}
	}
	// The bill portions reverse payments of their bills; the deposit
	// portion only reduces the student's deposit balance.
	shares, err := refundBillShares(r.Context(), tx, schoolID, id, billID, amount-depositIDR)
	if err != nil {
		writeErr(w, err)
		return
	}
	sortShares(shares)
	for _, sh := range shares {
		if err = reverseBillShare(r.Context(), tx, schoolID, id, reason, c.UserID, sh); err != nil {
			if errors.Is(err, errRefundSourceShort) {
				common.WriteError(w, http.StatusUnprocessableEntity, "saldo terbayar tagihan tidak lagi mencukupi nominal refund")
				return
			}
			writeErr(w, err)
			return
		}
	}
	var depositBefore, depositAfter int64
	if depositIDR > 0 {
		depositBefore, depositAfter, err = payRefundFromDeposit(r.Context(), tx, schoolID, studentID, id, reason, depositIDR)
		if errors.Is(err, errRefundSourceShort) {
			common.WriteError(w, http.StatusUnprocessableEntity, "saldo deposit siswa tidak lagi mencukupi porsi deposit refund")
			return
		}
		if err != nil {
			writeErr(w, err)
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE bill_refunds SET status='processed',processed_by=$1,processed_at=now(),updated_at=now(),
		        transfer_ref=$4,transfer_date=$5,process_note=NULLIF($6,'')
		 WHERE id::text=$2 AND school_id=$3`, c.UserID, id, schoolID, proof.Ref, proof.Date, in.Note); err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "nomor referensi transfer sudah dipakai refund lain")
			return
		}
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	billParts := make([]map[string]any, 0, len(shares))
	for _, sh := range shares {
		billParts = append(billParts, map[string]any{"student_bill_id": sh.BillID, "amount_idr": sh.AmountIDR})
	}
	a.auditBoth(r.Context(), schoolID, "billing", "refund", id, "refund_processed", c.ActiveRole, c.UserID, reason,
		map[string]any{"status": status}, map[string]any{"status": "processed", "bill_id": billID.String, "amount_idr": amount,
			"deposit_idr": depositIDR, "bill_parts": billParts,
			"transfer_ref": proof.Ref, "transfer_date": proof.Date.Format("2006-01-02")})
	if walletID.Valid {
		a.auditBoth(r.Context(), schoolID, "wallet", "wallet", walletRefund.ID, "wallet_refunded", c.ActiveRole, c.UserID, reason,
			map[string]any{"balance_idr": walletBefore, "status": "closing"},
			map[string]any{"balance_idr": walletRefund.BalanceIDR, "status": "closed", "refund_id": id, "amount_idr": amount, "kind": walletRefund.Kind})
	}
	if depositIDR > 0 {
		a.auditBoth(r.Context(), schoolID, "billing", "student_deposit", studentID, "deposit_refunded", c.ActiveRole, c.UserID, reason,
			map[string]any{"balance_idr": depositBefore}, map[string]any{"balance_idr": depositAfter, "refund_id": id, "amount_idr": depositIDR})
	}
	a.notifyRefund(r.Context(), schoolID, id, model.RefundProcessed, "")
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "processed", "transfer_ref": proof.Ref}, nil)
}

// RefundCallback is POST /internal/refunds/{id}/callback (core Approval
// Center). Idempotent: a refund that is no longer pending is acknowledged.
func (a *App) RefundCallback(w http.ResponseWriter, r *http.Request) {
	var cb client.ApprovalCallback
	if !common.Decode(w, r, &cb) {
		return
	}
	id := param(r, "id")
	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var schoolID, status, requestedBy, approvalID string
	err = tx.QueryRowContext(ctx,
		`SELECT school_id::text,status,requested_by::text,COALESCE(approval_id,'') FROM bill_refunds WHERE id::text=$1 FOR UPDATE`, id).
		Scan(&schoolID, &status, &requestedBy, &approvalID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "refund tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if approvalID != "" && cb.ApprovalID != "" && cb.ApprovalID != approvalID {
		common.WriteError(w, http.StatusConflict, "approval_id tidak cocok dengan pengajuan refund")
		return
	}
	next, changed, derr := model.RefundDecision(status, cb.Decision, cb.DecidedBy, requestedBy)
	if derr != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, derr.Error())
		return
	}
	if !changed {
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": next}, nil)
		return
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE bill_refunds SET status=$1,decided_by=$2::uuid,decided_at=now(),decision_note=NULLIF($3,''),updated_at=now()
		 WHERE id::text=$4`, next, cb.DecidedBy, strings.TrimSpace(cb.Comment), id); err != nil {
		writeErr(w, err)
		return
	}
	if err = syncWalletClosureQ(ctx, tx, schoolID, id, next); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "refund", id, "refund_"+next, "approver", cb.DecidedBy, cb.Comment,
		map[string]any{"status": status}, map[string]any{"status": next, "approval_id": approvalID})
	a.notifyRefund(ctx, schoolID, id, next, cb.Comment)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": next}, nil)
}

// notifyRefund tells the requesting staff member and the student's
// guardians about a refund decision or payout. Delivery failures go to
// core's Integration Exception Queue; they never undo the refund step.
func (a *App) notifyRefund(ctx context.Context, schoolID, refundID, status, note string) {
	var studentID, requestedBy, transferRef string
	var amount int64
	if err := a.DB.QueryRowContext(ctx,
		`SELECT student_user_id::text,requested_by::text,amount_idr,COALESCE(transfer_ref,'') FROM bill_refunds WHERE id::text=$1`,
		refundID).Scan(&studentID, &requestedBy, &amount, &transferRef); err != nil {
		common.CaptureError(err)
		return
	}
	title, body := model.RefundNoticeText(status, formatRupiah(amount), note, transferRef)
	if title == "" {
		return
	}
	data := map[string]any{"refund_id": refundID, "status": status, "student_user_id": studentID}
	send := func(userID, audience string) {
		key := "refund:" + refundID + ":" + status + ":" + userID
		if a.Notification == nil || !a.Notification.Configured() {
			a.raiseNotificationFailure(ctx, schoolID, key, "notification service not configured", data)
			return
		}
		d := map[string]any{}
		for k, v := range data {
			d[k] = v
		}
		d["url"] = map[string]string{"staff": "/keuangan/refund", "guardian": "/orang-tua/tagihan"}[audience]
		if err := a.Notification.Notify(ctx, key, client.Notification{
			SchoolID: schoolID, UserID: userID, Type: "refund_" + status, Title: title, Body: body,
			Data: d, Channels: []string{"in_app", "email"},
		}); err != nil {
			a.raiseNotificationFailure(ctx, schoolID, key, err.Error(), data)
		}
	}
	send(requestedBy, "staff")
	if status == model.RefundApproved {
		return // guardians hear about the outcome: rejection or payout.
	}
	if a.IdentitySvc == nil || !a.IdentitySvc.Configured() {
		a.raiseNotificationFailure(ctx, schoolID, "refund:"+refundID+":"+status+":guardians", "identity service not configured", data)
		return
	}
	guardians, err := a.IdentitySvc.StudentGuardianIDs(ctx, schoolID, studentID)
	if err != nil {
		a.raiseNotificationFailure(ctx, schoolID, "refund:"+refundID+":"+status+":guardians", err.Error(), data)
		return
	}
	for _, g := range guardians {
		send(g, "guardian")
	}
}
