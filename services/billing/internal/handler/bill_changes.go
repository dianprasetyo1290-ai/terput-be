package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Bill cancellation / adjustment through the Approval Center (BUG-053)
//
// A finance staff member asks to cancel or re-price an issued bill; the
// request goes to core's Approval Center as request_type bill_adjustment and
// the bill stays active until an approver (never the maker) decides. On
// approval the change is applied in one transaction:
//   - an adjustment row (bill_adjustments, kind cancel/adjust) plus signed
//     adjustment lines per fee component, so the daily journal reverses or
//     adds receivable/revenue per component;
//   - paid money above the new net is reversed into the student deposit;
//   - a cancellation sets the bill to void and keeps base/discount/net as
//     they were (the original value stays on the bill);
//   - an adjustment updates net and rescales the bill's terms.
// ---------------------------------------------------------------------------

const reqTypeBillAdjustment = "bill_adjustment"

type billChangeRow struct {
	ID            string     `json:"id"`
	BillID        string     `json:"bill_id"`
	StudentUserID string     `json:"student_user_id"`
	StudentName   string     `json:"student_name"`
	PeriodYear    int        `json:"period_year"`
	PeriodMonth   int        `json:"period_month"`
	Kind          string     `json:"kind"`
	NetBeforeIDR  int64      `json:"net_before_idr"`
	NewNetIDR     *int64     `json:"new_net_idr"`
	Reason        string     `json:"reason"`
	Status        string     `json:"status"`
	ApprovalID    string     `json:"approval_id"`
	RequestedBy   string     `json:"requested_by"`
	DecidedBy     string     `json:"decided_by"`
	DecisionNote  string     `json:"decision_note"`
	FailureReason string     `json:"failure_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	DecidedAt     *time.Time `json:"decided_at"`
}

// billChangeRequests lists change requests (optionally for one bill / one
// status), newest first.
func (a *App) billChangeRequests(ctx context.Context, schoolID, billID, status string, limit, offset int) ([]billChangeRow, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT c.id::text,c.bill_id::text,b.student_user_id::text,b.period_year,b.period_month,c.kind,
		        COALESCE((c.before_data->>'net_idr')::bigint,0),c.new_net_idr,c.reason,c.status,COALESCE(c.approval_id,''),
		        c.requested_by::text,COALESCE(c.decided_by,''),COALESCE(c.decision_note,''),COALESCE(c.failure_reason,''),
		        c.created_at,c.decided_at
		   FROM bill_change_requests c JOIN student_bills b ON b.id=c.bill_id
		  WHERE c.school_id=$1 AND ($2='' OR c.bill_id::text=$2) AND ($3='' OR c.status=$3)
		  ORDER BY c.created_at DESC LIMIT $4 OFFSET $5`, schoolID, billID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []billChangeRow{}
	for rows.Next() {
		var x billChangeRow
		var newNet sql.NullInt64
		var decided sql.NullTime
		if err := rows.Scan(&x.ID, &x.BillID, &x.StudentUserID, &x.PeriodYear, &x.PeriodMonth, &x.Kind, &x.NetBeforeIDR, &newNet,
			&x.Reason, &x.Status, &x.ApprovalID, &x.RequestedBy, &x.DecidedBy, &x.DecisionNote, &x.FailureReason,
			&x.CreatedAt, &decided); err != nil {
			return nil, err
		}
		if newNet.Valid {
			v := newNet.Int64
			x.NewNetIDR = &v
		}
		if decided.Valid {
			t := decided.Time
			x.DecidedAt = &t
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out))
	for _, x := range out {
		ids = append(ids, x.StudentUserID)
	}
	names := a.userNames(ctx, ids)
	for i := range out {
		out[i].StudentName = names[out[i].StudentUserID]
	}
	return out, nil
}

// ListBillChangeRequests is GET /admin/bill-change-requests?status=&page=&per_page=.
func (a *App) ListBillChangeRequests(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	page, perPage := common.Page(r)
	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM bill_change_requests WHERE school_id=$1 AND ($2='' OR status=$2)`, schoolID, status).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	items, err := a.billChangeRequests(r.Context(), schoolID, "", status, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, &common.Meta{Page: page, PerPage: perPage, Total: total})
}

// CreateBillChangeRequest is POST /admin/bills/{id}/change-requests
// {kind: cancel|adjust, new_net_idr?, reason}. 202: waiting for approval.
func (a *App) CreateBillChangeRequest(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in struct {
		Kind      string `json:"kind"`
		NewNetIDR *int64 `json:"new_net_idr"`
		Reason    string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Kind, in.Reason = strings.TrimSpace(in.Kind), strings.TrimSpace(in.Reason)
	billID := param(r, "id")
	ctx := r.Context()
	var (
		studentID, status string
		year, month       int
		base, disc, net   int64
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT student_user_id::text,status,period_year,period_month,base_idr,discount_idr,net_idr
		   FROM student_bills WHERE id::text=$1 AND school_id=$2`, billID, schoolID).
		Scan(&studentID, &status, &year, &month, &base, &disc, &net)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := model.ValidateBillChange(in.Kind, in.NewNetIDR, in.Reason, status, net); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	_, verified, reversed, outstanding, err := billTotalsQ(ctx, a.DB, schoolID, billID)
	if err != nil {
		writeErr(w, err)
		return
	}
	before := map[string]any{"status": status, "base_idr": base, "discount_idr": disc, "net_idr": net,
		"paid_idr": verified - reversed, "outstanding_idr": outstanding}
	var id string
	err = a.DB.QueryRowContext(ctx,
		`INSERT INTO bill_change_requests(school_id,bill_id,kind,new_net_idr,reason,before_data,requested_by,requested_role,idempotency_key)
		 VALUES($1,$2::uuid,$3,$4,$5,$6,$7::uuid,$8,$9) RETURNING id::text`,
		schoolID, billID, in.Kind, in.NewNetIDR, in.Reason, jsonRaw(before), c.UserID, c.ActiveRole, idempotencyKey(r)).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "tagihan ini masih memiliki pengajuan pembatalan/penyesuaian yang menunggu approval")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	title := fmt.Sprintf("Pembatalan tagihan %02d/%d", month, year)
	amount := net
	if in.Kind == model.BillChangeAdjust {
		title = fmt.Sprintf("Penyesuaian tagihan %02d/%d: %s → %s", month, year, formatRupiah(net), formatRupiah(*in.NewNetIDR))
		amount = *in.NewNetIDR - net
		if amount < 0 {
			amount = -amount
		}
	}
	resp, err := a.submitApproval(ctx, client.ApprovalRequest{
		SchoolID: schoolID, RequestType: reqTypeBillAdjustment, SourceID: id, Title: title,
		Summary: map[string]any{
			"bill_id": billID, "student_user_id": studentID, "period_year": year, "period_month": month,
			"kind": in.Kind, "net_idr": net, "new_net_idr": in.NewNetIDR, "paid_idr": verified - reversed, "reason": in.Reason,
		},
		AmountIDR: &amount, MakerUserID: c.UserID, SubjectUserID: studentID,
		CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/bills/change-requests/" + id + "/callback",
		CorrelationID: id, IdempotencyKey: "bill-change:" + id,
	})
	if err != nil {
		_, _ = a.DB.ExecContext(ctx, `DELETE FROM bill_change_requests WHERE id::text=$1 AND status='pending_approval'`, id)
		writeErr(w, err)
		return
	}
	if _, err = a.DB.ExecContext(ctx, `UPDATE bill_change_requests SET approval_id=$1 WHERE id::text=$2`, resp.ID, id); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "bill", billID, "bill_change_requested", c.ActiveRole, c.UserID, in.Reason,
		before, map[string]any{"kind": in.Kind, "new_net_idr": in.NewNetIDR, "request_id": id, "approval_id": resp.ID})
	common.WriteJSON(w, http.StatusAccepted, map[string]any{"id": id, "status": model.BillChangePending, "approval_id": resp.ID}, nil)
}

// BillChangeCallback is POST /internal/bills/change-requests/{id}/callback
// (Approval Center decision). Replays answer 200 with the stored status.
func (a *App) BillChangeCallback(w http.ResponseWriter, r *http.Request) {
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
	var (
		schoolID, billID, kind, reason, status, requestedBy, approvalID string
		newNet                                                          sql.NullInt64
	)
	err = tx.QueryRowContext(ctx,
		`SELECT school_id::text,bill_id::text,kind,new_net_idr,reason,status,requested_by::text,COALESCE(approval_id,'')
		   FROM bill_change_requests WHERE id::text=$1 FOR UPDATE`, id).
		Scan(&schoolID, &billID, &kind, &newNet, &reason, &status, &requestedBy, &approvalID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pengajuan perubahan tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if approvalID != "" && cb.ApprovalID != "" && cb.ApprovalID != approvalID {
		common.WriteError(w, http.StatusConflict, "approval_id tidak cocok dengan pengajuan")
		return
	}
	approved, changed, derr := model.ApprovalOutcome(status, model.BillChangePending, cb.Decision, cb.DecidedBy, requestedBy)
	if derr != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, derr.Error())
		return
	}
	if !changed {
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status}, nil)
		return
	}
	finish := func(next, failure, adjID string) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE bill_change_requests SET status=$1,decided_by=$2,decision_note=NULLIF($3,''),failure_reason=NULLIF($4,''),
			        adjustment_id=NULLIF($5,'')::uuid,decided_at=now() WHERE id::text=$6`,
			next, cb.DecidedBy, strings.TrimSpace(cb.Comment), failure, adjID, id)
		return err
	}
	respond := func(next, failure string, before, after any) {
		if err := tx.Commit(); err != nil {
			writeErr(w, err)
			return
		}
		a.auditBoth(ctx, schoolID, "billing", "bill", billID, "bill_change_"+next, "approver", cb.DecidedBy,
			strings.TrimSpace(cb.Comment+" "+failure), before, after)
		a.notifyBillChange(ctx, schoolID, id, requestedBy, kind, next, failure)
		body := map[string]any{"id": id, "status": next}
		if failure != "" {
			body["reason"] = failure
		}
		common.WriteJSON(w, http.StatusOK, body, nil)
	}
	if !approved {
		if err := finish(model.BillChangeRejected, "", ""); err != nil {
			writeErr(w, err)
			return
		}
		respond(model.BillChangeRejected, "", map[string]any{"request_id": id}, map[string]any{"status": model.BillChangeRejected})
		return
	}
	res, failure, err := a.applyBillChange(ctx, tx, schoolID, billID, id, kind, newNet.Int64, reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	if failure != "" {
		if err := finish(model.BillChangeFailed, failure, ""); err != nil {
			writeErr(w, err)
			return
		}
		respond(model.BillChangeFailed, failure, map[string]any{"request_id": id}, map[string]any{"status": model.BillChangeFailed})
		return
	}
	if err := finish(model.BillChangeApplied, "", res.adjustmentID); err != nil {
		writeErr(w, err)
		return
	}
	respond(model.BillChangeApplied, "", res.before, res.after)
}

type billChangeResult struct {
	adjustmentID  string
	before, after map[string]any
}

// applyBillChange applies an approved change inside tx. A business reason
// the change can no longer be applied (bill voided meanwhile, online payment
// in flight, ...) is returned as failure, not as an error.
func (a *App) applyBillChange(ctx context.Context, tx *sql.Tx, schoolID, billID, requestID, kind string, newNet int64, reason string) (billChangeResult, string, error) {
	var res billChangeResult
	var b adhocBill
	var base, disc int64
	err := tx.QueryRowContext(ctx,
		`SELECT id::text,student_user_id::text,status,net_idr,due_at,base_idr,discount_idr FROM student_bills
		  WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, billID, schoolID).
		Scan(&b.ID, &b.StudentUserID, &b.Status, &b.NetIDR, &b.DueAt, &base, &disc)
	if err != nil {
		return res, "", err
	}
	if b.Status == string(model.BillVoid) {
		return res, "tagihan sudah dibatalkan sebelum pengajuan disetujui", nil
	}
	if kind == model.BillChangeAdjust && newNet == b.NetIDR {
		return res, "nilai tagihan sudah sama dengan nilai yang diajukan", nil
	}
	if pending, err := hasPendingGateway(ctx, tx, b.ID); err != nil {
		return res, "", err
	} else if pending {
		return res, "tagihan sedang dibayar online saat disetujui; ajukan ulang setelah transaksi selesai", nil
	}
	_, verified, reversed, _, err := billTotalsQ(ctx, tx, schoolID, b.ID)
	if err != nil {
		return res, "", err
	}
	plan, err := model.PlanBillChange(kind, b.NetIDR, verified-reversed, newNet)
	if err != nil {
		return res, err.Error(), nil
	}
	res.before = map[string]any{"status": b.Status, "base_idr": base, "discount_idr": disc, "net_idr": b.NetIDR, "paid_idr": verified - reversed}
	label := map[string]string{model.BillChangeCancel: "Pembatalan", model.BillChangeAdjust: "Penyesuaian"}[kind] + ": " + reason
	if plan.DeltaIDR != 0 {
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO bill_adjustments(school_id,bill_id,kind,delta_idr,net_before_idr,net_after_idr,deposit_idr,reason,idempotency_key)
			 VALUES($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`,
			schoolID, b.ID, kind, plan.DeltaIDR, b.NetIDR, plan.NetAfterIDR, plan.DepositIDR, reason, "bill-change:"+requestID).
			Scan(&res.adjustmentID); err != nil {
			return res, "", err
		}
		if err := insertAdjustmentLines(ctx, tx, schoolID, b.ID, res.adjustmentID, label, plan.DeltaIDR); err != nil {
			return res, "", err
		}
	}
	if kind == model.BillChangeCancel {
		// The original base/discount/net stay on the bill.
		if _, err := tx.ExecContext(ctx, `UPDATE student_bills SET status='void',void_reason=$1,updated_at=now() WHERE id=$2::uuid`,
			reason, b.ID); err != nil {
			return res, "", err
		}
	} else {
		// base = discount + net: the adjustment line changes the gross,
		// the discounts already granted stay as they were.
		if _, err := tx.ExecContext(ctx, `UPDATE student_bills SET base_idr=discount_idr+$1,net_idr=$1,status=$2,updated_at=now() WHERE id=$3::uuid`,
			plan.NetAfterIDR, string(plan.Status), b.ID); err != nil {
			return res, "", err
		}
		ins, err := loadInstallments(ctx, tx, schoolID, b.ID)
		if err != nil {
			return res, "", err
		}
		if len(ins) > 0 {
			if err := saveInstallments(ctx, tx, schoolID, b.ID, model.RescaleInstallments(ins, plan.NetAfterIDR)); err != nil {
				return res, "", err
			}
		}
	}
	if plan.DepositIDR > 0 {
		if err := a.moveOverpaymentToDeposit(ctx, tx, schoolID, b, plan.DepositIDR, res.adjustmentID, reason); err != nil {
			return res, "", err
		}
	}
	if kind == model.BillChangeAdjust {
		if err := recomputeBillStatusQ(ctx, tx, schoolID, b.ID); err != nil {
			return res, "", err
		}
	}
	res.after = map[string]any{"status": string(plan.Status), "net_idr": plan.NetAfterIDR, "deposit_idr": plan.DepositIDR,
		"adjustment_id": res.adjustmentID, "request_id": requestID}
	if kind == model.BillChangeCancel {
		res.after["net_idr_original"] = b.NetIDR
	}
	return res, "", nil
}

// insertAdjustmentLines books delta as signed adjustment lines per fee
// component, in proportion to the bill's original component lines, so the
// daily journal moves receivable and revenue per component.
func insertAdjustmentLines(ctx context.Context, tx *sql.Tx, schoolID, billID, adjID, label string, delta int64) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT COALESCE(component_code,''),sum(amount_idr) FROM bill_items
		  WHERE bill_id=$1::uuid AND school_id=$2 AND adjustment_id IS NULL
		  GROUP BY 1 HAVING sum(amount_idr) > 0 ORDER BY 1`, billID, schoolID)
	if err != nil {
		return err
	}
	var codes []string
	var weights []int64
	for rows.Next() {
		var code string
		var amt int64
		if err := rows.Scan(&code, &amt); err != nil {
			rows.Close()
			return err
		}
		codes = append(codes, code)
		weights = append(weights, amt)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(codes) == 0 {
		codes, weights = []string{""}, []int64{1}
	}
	for i, part := range model.SplitSigned(delta, weights) {
		if part == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bill_items(school_id,bill_id,name,component_code,amount_idr,discount_idr,net_idr,adjustment_id)
			 VALUES($1,$2::uuid,$3,NULLIF($4,''),$5,0,$5,$6::uuid)`,
			schoolID, billID, label, codes[i], part, adjID); err != nil {
			return err
		}
	}
	return nil
}

// notifyBillChange tells the requester how their request ended. Delivery
// failures go to core's Integration Exception Queue.
func (a *App) notifyBillChange(ctx context.Context, schoolID, requestID, requestedBy, kind, status, failure string) {
	what := map[string]string{model.BillChangeCancel: "Pembatalan", model.BillChangeAdjust: "Penyesuaian"}[kind] + " tagihan"
	title := map[string]string{
		model.BillChangeApplied:  what + " disetujui",
		model.BillChangeRejected: what + " ditolak",
		model.BillChangeFailed:   what + " gagal diterapkan",
	}[status]
	body := title + "."
	if failure != "" {
		body = title + ": " + failure
	}
	key := "bill-change:" + requestID + ":" + status
	data := map[string]any{"request_id": requestID, "status": status, "url": "/keuangan/detail-siswa"}
	if a.Notification == nil || !a.Notification.Configured() {
		a.raiseNotificationFailure(ctx, schoolID, key, "notification service not configured", data)
		return
	}
	if err := a.Notification.Notify(ctx, key, client.Notification{
		SchoolID: schoolID, UserID: requestedBy, Type: "bill_change_" + status, Title: title, Body: body,
		Data: data, Channels: []string{"in_app"},
	}); err != nil {
		a.raiseNotificationFailure(ctx, schoolID, key, err.Error(), data)
	}
}
