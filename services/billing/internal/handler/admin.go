package handler

import (
	"database/sql"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Bill generation, listing and arrears
// ---------------------------------------------------------------------------

// GenerateBills is POST /admin/bills/generate. It is idempotent per
// (school, scheme, student, period).
func (a *App) GenerateBills(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireFinanceStaff(w, r)
	if claims == nil {
		return
	}
	var in generateRequest
	if !common.Decode(w, r, &in) {
		return
	}
	in.ActorType, in.ActorID = claims.ActiveRole, claims.UserID
	result, err := a.generateBills(r.Context(), schoolID, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

// ListBills is GET /admin/bills.
func (a *App) ListBills(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireBillReader(w, r)
	if c == nil {
		return
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	studentID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	if !a.teacherMaySee(w, r, c, schoolID, studentID) {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	schemeID := strings.TrimSpace(r.URL.Query().Get("scheme_id"))
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT b.id,b.student_user_id::text,COALESCE(b.scheme_id::text,''),COALESCE(s.name,b.source_module,''),b.status,b.period_year,b.period_month,
		        b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'),0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id),0),
		        count(*) OVER ()
		 FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE b.school_id=$1
		   AND ($2='' OR b.student_user_id::text=$2)
		   AND ($3='' OR b.status=$3)
		   AND ($4='' OR b.scheme_id::text=$4)
		   AND ($5=0 OR b.period_year=$5)
		   AND ($6=0 OR b.period_month=$6)
		 ORDER BY b.period_year DESC,b.period_month DESC,b.created_at DESC
		 LIMIT $7 OFFSET $8`,
		schoolID, studentID, status, schemeID, year, month, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	total := 0
	for rows.Next() {
		var (
			id, student, scheme, schemeName, billStatus string
			y, m                                        int
			base, discount, net, verified, reversed     int64
			dueAt                                       sql.NullTime
			createdAt                                   time.Time
		)
		if err := rows.Scan(&id, &student, &scheme, &schemeName, &billStatus, &y, &m,
			&base, &discount, &net, &dueAt, &createdAt, &verified, &reversed, &total); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		item := a.billSummary(id, scheme, schemeName, billStatus, y, m, base, discount, net, verified, reversed, outstanding, dueAt, createdAt)
		item["student_user_id"] = student
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, &common.Meta{Page: page, PerPage: perPage, Total: total})
}

// ListArrears is GET /admin/arrears — bills that still owe money.
func (a *App) ListArrears(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")
	schemeID := strings.TrimSpace(r.URL.Query().Get("scheme_id"))
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT t.*, count(*) OVER () FROM (
		   SELECT b.id,b.student_user_id::text,COALESCE(b.scheme_id::text,''),COALESCE(s.name,b.source_module,''),b.status,b.period_year,b.period_month,
		          b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at,
		          COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                    WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'),0) AS verified,
		          COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                    WHERE p.student_bill_id=b.id AND p.school_id=b.school_id),0) AS reversed
		   FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		   WHERE b.school_id=$1 AND b.status<>'void'
		     AND ($2=0 OR b.period_year=$2) AND ($3=0 OR b.period_month=$3) AND ($4='' OR b.scheme_id::text=$4)
		 ) t
		 WHERE (t.net_idr - t.verified + t.reversed) > 0
		 ORDER BY t.due_at ASC NULLS LAST, t.period_year, t.period_month
		 LIMIT $5 OFFSET $6`,
		schoolID, year, month, schemeID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	total := 0
	for rows.Next() {
		var (
			id, student, scheme, schemeName, billStatus string
			y, m                                        int
			base, discount, net, verified, reversed     int64
			dueAt                                       sql.NullTime
			createdAt                                   time.Time
		)
		if err := rows.Scan(&id, &student, &scheme, &schemeName, &billStatus, &y, &m,
			&base, &discount, &net, &dueAt, &createdAt, &verified, &reversed, &total); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		item := a.billSummary(id, scheme, schemeName, billStatus, y, m, base, discount, net, verified, reversed, outstanding, dueAt, createdAt)
		item["student_user_id"] = student
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, &common.Meta{Page: page, PerPage: perPage, Total: total})
}

// ---------------------------------------------------------------------------
// Reconciliation export
// ---------------------------------------------------------------------------

// ExportReconciliation is GET /admin/reconciliation.csv.
func (a *App) ExportReconciliation(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT p.id,p.student_bill_id::text,b.student_user_id::text,b.period_year,b.period_month,
		        p.amount_idr,p.status,COALESCE(p.bank_name,''),COALESCE(p.account_name,''),COALESCE(p.reference,''),
		        p.transfer_at,p.verified_at,p.created_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al WHERE al.payment_id=p.id),0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv WHERE rv.payment_id=p.id),0),
		        b.net_idr
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.school_id=$1 AND ($2=0 OR b.period_year=$2) AND ($3=0 OR b.period_month=$3) AND ($4='' OR p.status=$4)
		 ORDER BY p.created_at`,
		schoolID, year, month, status)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="rekonsiliasi-pembayaran.csv"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{
		"payment_id", "bill_id", "student_user_id", "period_year", "period_month",
		"amount_idr", "status", "bank_name", "account_name", "reference",
		"transfer_at", "verified_at", "created_at", "allocated_idr", "reversed_idr",
		"net_idr", "outstanding_idr",
	})
	for rows.Next() {
		var (
			id, billID, studentID, payStatus string
			bank, account, reference         string
			year, month                      int
			amount, allocated, reversed, net int64
			transferAt, verifiedAt           sql.NullTime
			createdAt                        time.Time
		)
		if err := rows.Scan(&id, &billID, &studentID, &year, &month, &amount, &payStatus,
			&bank, &account, &reference, &transferAt, &verifiedAt, &createdAt,
			&allocated, &reversed, &net); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{allocated}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		_ = cw.Write([]string{
			id, billID, studentID, strconv.Itoa(year), strconv.Itoa(month),
			strconv.FormatInt(amount, 10), payStatus, bank, account, reference,
			csvTime(transferAt), csvTime(verifiedAt), createdAt.Format(time.RFC3339),
			strconv.FormatInt(allocated, 10), strconv.FormatInt(reversed, 10),
			strconv.FormatInt(net, 10), strconv.FormatInt(outstanding, 10),
		})
	}
	if err := rows.Err(); err != nil {
		common.CaptureError(err)
	}
}

func csvTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}

// teacherMaySee enforces that a guru/walikelas only reads bills of one
// student at a time, and only a student of a class they teach or are wali
// kelas of (resolved via academic-service). Admins pass straight through.
// Fails closed: a roster lookup error denies.
func (a *App) teacherMaySee(w http.ResponseWriter, r *http.Request, c *common.Claims, schoolID, studentID string) bool {
	if c.ActiveRole != common.RoleGuru && c.ActiveRole != common.RoleWalikelas {
		return true
	}
	if studentID == "" {
		common.WriteError(w, http.StatusForbidden, "student_user_id is required for teachers")
		return false
	}
	if a.Roster == nil {
		common.WriteError(w, http.StatusForbidden, "class roster unavailable")
		return false
	}
	allowed, err := a.Roster.TeacherStudentIDs(r.Context(), schoolID, c.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusForbidden, "class roster unavailable")
		return false
	}
	if !allowed[studentID] {
		common.WriteError(w, http.StatusForbidden, "student is not in your classes")
		return false
	}
	return true
}
