package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Guardian (wali) ownership
// ---------------------------------------------------------------------------

// guardianCanAccess asks identity whether the JWT's user is a verified
// guardian of studentUserID at their school. It fails closed: any error —
// including identity being unreachable or not configured — denies access.
func (a *App) guardianCanAccess(ctx context.Context, c *common.Claims, studentUserID string) bool {
	if a.Identity == nil || studentUserID == "" {
		return false
	}
	allowed, err := a.Identity.GuardianHasAccess(ctx, c.UserID, studentUserID, c.SchoolID)
	if err != nil {
		common.CaptureError(err)
		return false
	}
	return allowed
}

// requireChild resolves the parent claims and checks child ownership. On any
// failure it writes a 404 so a guardian cannot probe which student ids exist.
func (a *App) requireChild(w http.ResponseWriter, r *http.Request) (*common.Claims, string, bool) {
	c := common.RequireParent(w, r)
	if c == nil {
		return nil, "", false
	}
	studentID := param(r, "studentUserId")
	if !a.guardianCanAccess(r.Context(), c, studentID) {
		common.WriteError(w, http.StatusNotFound, "student not found")
		return nil, "", false
	}
	return c, studentID, true
}

// ---------------------------------------------------------------------------
// GET /parent/children/{studentUserId}/bills
// ---------------------------------------------------------------------------

// ListChildBills is GET /parent/children/{studentUserId}/bills.
func (a *App) ListChildBills(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireChild(w, r)
	if !ok {
		return
	}
	a.writeStudentBills(w, r, c.SchoolID, studentID)
}

// ListMyBills is GET /me/bills — the siswa's own bills (same shape as the
// parent list), scoped to the JWT's own user id + school.
func (a *App) ListMyBills(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil {
		return
	}
	if !a.checkStudentBillVisibility(w, r, c.SchoolID, c.UserID) {
		return
	}
	a.writeStudentBills(w, r, c.SchoolID, c.UserID)
}

func (a *App) writeStudentBills(w http.ResponseWriter, r *http.Request, schoolID, studentID string) {
	page, perPage := common.Page(r)

	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM student_bills WHERE school_id=$1 AND student_user_id::text=$2`,
		schoolID, studentID).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT b.id,b.scheme_id::text,COALESCE(s.name,''),b.status,b.period_year,b.period_month,
		        b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al
		                  JOIN bill_payments p ON p.id=al.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'),0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv
		                  JOIN bill_payments p ON p.id=rv.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id),0)
		 FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE b.school_id=$1 AND b.student_user_id::text=$2
		 ORDER BY b.period_year DESC,b.period_month DESC,b.created_at DESC
		 LIMIT $3 OFFSET $4`,
		schoolID, studentID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var (
			id, schemeID, schemeName, status string
			year, month                      int
			base, discount, net              int64
			verified, reversed               int64
			dueAt                            sql.NullTime
			createdAt                        time.Time
		)
		if err := rows.Scan(&id, &schemeID, &schemeName, &status, &year, &month,
			&base, &discount, &net, &dueAt, &createdAt, &verified, &reversed); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, billSummary(id, schemeID, schemeName, status, year, month,
			base, discount, net, verified, reversed, outstanding, dueAt, createdAt))
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, map[string]any{"page": page, "per_page": perPage, "total": total})
}

func billSummary(id, schemeID, schemeName, status string, year, month int,
	base, discount, net, verified, reversed, outstanding int64, dueAt sql.NullTime, createdAt time.Time) map[string]any {
	m := map[string]any{
		"id":              id,
		"scheme_id":       schemeID,
		"scheme_name":     schemeName,
		"status":          status,
		"period_year":     year,
		"period_month":    month,
		"base_idr":        base,
		"discount_idr":    discount,
		"net_idr":         net,
		"paid_idr":        verified - reversed,
		"outstanding_idr": outstanding,
		"created_at":      createdAt,
	}
	if dueAt.Valid {
		m["due_at"] = dueAt.Time
	}
	return m
}

// ---------------------------------------------------------------------------
// GET /parent/children/{studentUserId}/bills/{billId}
// ---------------------------------------------------------------------------

// GetChildBill is GET /parent/children/{studentUserId}/bills/{billId}.
func (a *App) GetChildBill(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireChild(w, r)
	if !ok {
		return
	}
	billID := param(r, "billId")

	var (
		id, schemeID, schemeName, status string
		year, month                      int
		base, discount, net              int64
		dueAt                            sql.NullTime
		createdAt                        time.Time
		snapshot                         []byte
	)
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT b.id,b.scheme_id::text,COALESCE(s.name,''),b.status,b.period_year,b.period_month,
		        b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at,b.discount_snapshot
		 FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE b.id=$1 AND b.school_id=$2 AND b.student_user_id::text=$3`,
		billID, c.SchoolID, studentID).
		Scan(&id, &schemeID, &schemeName, &status, &year, &month, &base, &discount, &net, &dueAt, &createdAt, &snapshot)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "bill not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	items, err := a.billItems(r.Context(), c.SchoolID, billID)
	if err != nil {
		writeErr(w, err)
		return
	}
	discounts, err := a.billDiscountSteps(r.Context(), c.SchoolID, billID)
	if err != nil {
		writeErr(w, err)
		return
	}
	payments, err := a.billPayments(r.Context(), c.SchoolID, billID)
	if err != nil {
		writeErr(w, err)
		return
	}
	_, verified, reversed, outstanding, err := billTotalsQ(r.Context(), a.DB, c.SchoolID, billID)
	if err != nil {
		writeErr(w, err)
		return
	}

	view := billSummary(id, schemeID, schemeName, status, year, month,
		base, discount, net, verified, reversed, outstanding, dueAt, createdAt)
	view["items"] = items
	view["discounts"] = discounts
	view["payments"] = payments
	if len(snapshot) > 0 {
		view["discount_snapshot"] = json.RawMessage(snapshot)
	}
	common.WriteJSON(w, http.StatusOK, view, nil)
}

func (a *App) billItems(ctx context.Context, schoolID, billID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,name,COALESCE(component_code,''),amount_idr,discount_idr,net_idr
		 FROM bill_items WHERE bill_id=$1 AND school_id=$2 ORDER BY name`, billID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, code    string
			amount, disc, net int64
		)
		if err := rows.Scan(&id, &name, &code, &amount, &disc, &net); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "component_code": code,
			"amount_idr": amount, "discount_idr": disc, "net_idr": net,
		})
	}
	return out, rows.Err()
}

func (a *App) billDiscountSteps(ctx context.Context, schoolID, billID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT sequence,source,label,priority,base_before_idr,requested_idr,applied_idr,base_after_idr
		 FROM bill_discount_steps WHERE bill_id=$1 AND school_id=$2 ORDER BY sequence`, billID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			seq, prio                                 int
			source, label                             string
			baseBefore, requested, applied, baseAfter int64
		)
		if err := rows.Scan(&seq, &source, &label, &prio, &baseBefore, &requested, &applied, &baseAfter); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"sequence": seq, "source": source, "label": label, "priority": prio,
			"base_before": baseBefore, "requested": requested, "applied": applied, "base_after": baseAfter,
		})
	}
	return out, rows.Err()
}

func (a *App) billPayments(ctx context.Context, schoolID, billID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,amount_idr,status,transfer_at,bank_name,account_name,reference,file_name,review_note,created_at,verified_at
		 FROM bill_payments WHERE student_bill_id=$1 AND school_id=$2 ORDER BY created_at DESC`, billID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPaymentRows(rows)
}

// ---------------------------------------------------------------------------
// GET /parent/children/{studentUserId}/payments
// ---------------------------------------------------------------------------

// ListChildPayments is GET /parent/children/{studentUserId}/payments.
func (a *App) ListChildPayments(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireChild(w, r)
	if !ok {
		return
	}
	a.writeStudentPayments(w, r, c.SchoolID, studentID)
}

// ListMyPayments is GET /me/payments — the siswa's own payment history.
func (a *App) ListMyPayments(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil {
		return
	}
	if !a.checkStudentBillVisibility(w, r, c.SchoolID, c.UserID) {
		return
	}
	a.writeStudentPayments(w, r, c.SchoolID, c.UserID)
}

func (a *App) writeStudentPayments(w http.ResponseWriter, r *http.Request, schoolID, studentID string) {
	page, perPage := common.Page(r)

	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.school_id=$1 AND b.student_user_id::text=$2`, schoolID, studentID).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT p.id,p.amount_idr,p.status,p.transfer_at,p.bank_name,p.account_name,p.reference,
		        p.file_name,p.review_note,p.created_at,p.verified_at
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.school_id=$1 AND b.student_user_id::text=$2
		 ORDER BY p.created_at DESC LIMIT $3 OFFSET $4`,
		schoolID, studentID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items, err := scanPaymentRows(rows)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, map[string]any{"page": page, "per_page": perPage, "total": total})
}

// GetChildPaymentProof is
// GET /parent/children/{studentUserId}/payments/{paymentId}/proof — the only
// way a transfer proof leaves the private store for a guardian.
func (a *App) GetChildPaymentProof(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireChild(w, r)
	if !ok {
		return
	}
	var key, mime, checksum string
	var size int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT p.object_key,p.mime,p.size_bytes,p.sha256
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.id=$1 AND p.school_id=$2 AND b.student_user_id::text=$3 AND p.object_key IS NOT NULL`,
		param(r, "paymentId"), c.SchoolID, studentID).Scan(&key, &mime, &size, &checksum)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "payment not found")
		return
	}
	a.streamObject(w, r, key, mime, checksum, size)
}
