package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Bill terms (BUG-052/126) and the bill detail (menu 19/20)
// ---------------------------------------------------------------------------

// loadInstallments returns a bill's terms in due order (nil = paid at once).
func loadInstallments(ctx context.Context, q queryer, schoolID, billID string) ([]model.Installment, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT seq,due_at,amount_idr FROM bill_installments WHERE school_id=$1 AND bill_id::text=$2 ORDER BY seq`, schoolID, billID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Installment
	for rows.Next() {
		var t model.Installment
		if err := rows.Scan(&t.Seq, &t.DueAt, &t.AmountIDR); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// saveInstallments replaces a bill's terms (nil/empty = paid at once).
func saveInstallments(ctx context.Context, q queryer, schoolID, billID string, ins []model.Installment) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM bill_installments WHERE school_id=$1 AND bill_id::text=$2`, schoolID, billID); err != nil {
		return err
	}
	for _, t := range ins {
		if _, err := q.ExecContext(ctx,
			`INSERT INTO bill_installments(school_id,bill_id,seq,due_at,amount_idr) VALUES($1,$2::uuid,$3,$4,$5)`,
			schoolID, billID, t.Seq, t.DueAt, t.AmountIDR); err != nil {
			return err
		}
	}
	return nil
}

// syncInstallmentDue moves a split bill's due_at to the first term that is
// not fully paid (the last term once everything is paid), so aging,
// reminders and the overdue flag follow the terms.
func syncInstallmentDue(ctx context.Context, q queryer, schoolID, billID string, paid int64) error {
	ins, err := loadInstallments(ctx, q, schoolID, billID)
	if err != nil || len(ins) == 0 {
		return err
	}
	due := ins[len(ins)-1].DueAt
	if next := model.NextDue(ins, paid); next != nil {
		due = *next
	}
	_, err = q.ExecContext(ctx, `UPDATE student_bills SET due_at=$1 WHERE id::text=$2 AND school_id=$3 AND due_at IS DISTINCT FROM $1`,
		due, billID, schoolID)
	return err
}

// SetBillInstallments is PUT /admin/bills/{id}/installments
// {installments:[{due_date:"YYYY-MM-DD", amount_idr}]}: splits a bill into
// terms with their own due dates (BUG-052). An empty list removes the
// split. The total never changes, so no approval is needed; it is audited.
func (a *App) SetBillInstallments(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in struct {
		Installments []struct {
			DueDate   string `json:"due_date"`
			AmountIDR int64  `json:"amount_idr"`
		} `json:"installments"`
	}
	if !common.Decode(w, r, &in) {
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
	var status string
	var dueAt sql.NullTime
	var dueDay int
	err = tx.QueryRowContext(ctx,
		`SELECT b.status,b.due_at,COALESCE(s.due_day,10) FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		  WHERE b.id::text=$1 AND b.school_id=$2 FOR UPDATE OF b`, id, schoolID).Scan(&status, &dueAt, &dueDay)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status == string(model.BillVoid) {
		common.WriteError(w, http.StatusConflict, "tagihan sudah dibatalkan")
		return
	}
	net, verified, reversed, _, err := billTotalsQ(ctx, tx, schoolID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	before, err := loadInstallments(ctx, tx, schoolID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var terms []model.Installment
	if len(in.Installments) > 0 {
		raw := make([]model.Installment, len(in.Installments))
		for i, t := range in.Installments {
			d, err := time.Parse("2006-01-02", strings.TrimSpace(t.DueDate))
			if err != nil {
				common.WriteError(w, http.StatusUnprocessableEntity, "due_date termin harus berformat YYYY-MM-DD")
				return
			}
			raw[i] = model.Installment{DueAt: d, AmountIDR: t.AmountIDR}
		}
		if terms, err = model.ValidateInstallments(net, raw); err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	if err = saveInstallments(ctx, tx, schoolID, id, terms); err != nil {
		writeErr(w, err)
		return
	}
	if len(terms) == 0 && len(before) > 0 {
		// Back to a single payment: due on the last former term.
		if _, err = tx.ExecContext(ctx, `UPDATE student_bills SET due_at=$1,updated_at=now() WHERE id::text=$2`,
			before[len(before)-1].DueAt, id); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = syncInstallmentDue(ctx, tx, schoolID, id, verified-reversed); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.auditBoth(ctx, schoolID, "billing", "bill", id, "bill_installments_set", c.ActiveRole, c.UserID, "",
		map[string]any{"installments": before}, map[string]any{"installments": terms})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "installments": model.InstallmentProgress(terms, verified-reversed)}, nil)
}

// GetBill is GET /admin/bills/{id}: the bill with its lines, discount steps,
// terms (with what is paid on each), adjustments and change requests.
func (a *App) GetBill(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	id := param(r, "id")
	ctx := r.Context()
	var (
		student, scheme, schemeName, status, voidReason string
		y, m                                            int
		base, discount, net                             int64
		dueAt                                           sql.NullTime
		createdAt                                       time.Time
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT b.student_user_id::text,COALESCE(b.scheme_id::text,''),COALESCE(s.name,b.source_module,''),b.status,COALESCE(b.void_reason,''),
		        b.period_year,b.period_month,b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at
		   FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		  WHERE b.id::text=$1 AND b.school_id=$2`, id, schoolID).
		Scan(&student, &scheme, &schemeName, &status, &voidReason, &y, &m, &base, &discount, &net, &dueAt, &createdAt)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	_, verified, reversed, outstanding, err := billTotalsQ(ctx, a.DB, schoolID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := a.billSummary(id, scheme, schemeName, status, y, m, base, discount, net, verified, reversed, outstanding, dueAt, createdAt)
	out["student_user_id"] = student
	out["student_name"] = a.userNames(ctx, []string{student})[student]
	out["void_reason"] = voidReason

	type line struct {
		Name          string `json:"name"`
		ComponentCode string `json:"component_code"`
		AmountIDR     int64  `json:"amount_idr"`
		Adjustment    bool   `json:"adjustment"`
	}
	items := []line{}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT name,COALESCE(component_code,''),amount_idr,adjustment_id IS NOT NULL FROM bill_items
		  WHERE bill_id::text=$1 AND school_id=$2 ORDER BY created_at,id`, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.Name, &l.ComponentCode, &l.AmountIDR, &l.Adjustment); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		items = append(items, l)
	}
	rows.Close()
	out["items"] = items

	type step struct {
		Label      string `json:"label"`
		Source     string `json:"source"`
		AppliedIDR int64  `json:"applied_idr"`
	}
	steps := []step{}
	rows, err = a.DB.QueryContext(ctx,
		`SELECT label,source,applied_idr FROM bill_discount_steps WHERE bill_id::text=$1 AND school_id=$2 ORDER BY sequence`, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var s step
		if err := rows.Scan(&s.Label, &s.Source, &s.AppliedIDR); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		steps = append(steps, s)
	}
	rows.Close()
	out["discount_steps"] = steps

	ins, err := loadInstallments(ctx, a.DB, schoolID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	out["installments"] = model.InstallmentProgress(ins, verified-reversed)

	type adjustment struct {
		Kind         string    `json:"kind"`
		DeltaIDR     int64     `json:"delta_idr"`
		NetBeforeIDR int64     `json:"net_before_idr"`
		NetAfterIDR  int64     `json:"net_after_idr"`
		DepositIDR   int64     `json:"deposit_idr"`
		Reason       string    `json:"reason"`
		CreatedAt    time.Time `json:"created_at"`
	}
	adjs := []adjustment{}
	rows, err = a.DB.QueryContext(ctx,
		`SELECT kind,delta_idr,net_before_idr,net_after_idr,deposit_idr,reason,created_at FROM bill_adjustments
		  WHERE bill_id::text=$1 AND school_id=$2 ORDER BY created_at`, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var ad adjustment
		if err := rows.Scan(&ad.Kind, &ad.DeltaIDR, &ad.NetBeforeIDR, &ad.NetAfterIDR, &ad.DepositIDR, &ad.Reason, &ad.CreatedAt); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		adjs = append(adjs, ad)
	}
	rows.Close()
	out["adjustments"] = adjs

	reqs, err := a.billChangeRequests(ctx, schoolID, id, "", 50, 0)
	if err != nil {
		writeErr(w, err)
		return
	}
	out["change_requests"] = reqs
	common.WriteJSON(w, http.StatusOK, out, nil)
}
