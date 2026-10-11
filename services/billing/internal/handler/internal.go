package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// POST /internal/bills/generate
// ---------------------------------------------------------------------------

type internalGenerateRequest struct {
	SchoolID string `json:"school_id"`
	generateRequest
}

// InternalGenerateBills is POST /internal/bills/generate. It lets the scheduler
// or a sibling service trigger generation for an explicit set of students; the
// operation is idempotent per (school, scheme, student, period).
func (a *App) InternalGenerateBills(w http.ResponseWriter, r *http.Request) {
	var in internalGenerateRequest
	if !common.Decode(w, r, &in) {
		return
	}
	schoolID := strings.TrimSpace(in.SchoolID)
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	result, err := a.generateBills(r.Context(), schoolID, in.generateRequest)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

// ---------------------------------------------------------------------------
// GET /internal/students/{studentUserId}/outstanding
// ---------------------------------------------------------------------------

// InternalStudentOutstanding is GET /internal/students/{studentUserId}/outstanding.
func (a *App) InternalStudentOutstanding(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	studentID := param(r, "studentUserId")
	if studentID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "student_user_id is required")
		return
	}
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT b.id,b.scheme_id::text,COALESCE(s.name,''),b.status,b.period_year,b.period_month,
		        b.base_idr,b.discount_idr,b.net_idr,b.due_at,b.created_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'),0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id),0)
		 FROM student_bills b LEFT JOIN billing_schemes s ON s.id=b.scheme_id
		 WHERE b.school_id=$1 AND b.student_user_id::text=$2
		   AND ($3=0 OR b.period_year=$3) AND ($4=0 OR b.period_month=$4)
		 ORDER BY b.period_year, b.period_month, b.created_at`,
		schoolID, studentID, year, month)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	var total int64
	bills := []map[string]any{}
	for rows.Next() {
		var (
			id, scheme, schemeName, status          string
			y, m                                    int
			base, discount, net, verified, reversed int64
			dueAt                                   sql.NullTime
			createdAt                               time.Time
		)
		if err := rows.Scan(&id, &scheme, &schemeName, &status, &y, &m,
			&base, &discount, &net, &dueAt, &createdAt, &verified, &reversed); err != nil {
			writeErr(w, err)
			return
		}
		outstanding, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			writeErr(w, err)
			return
		}
		total += outstanding
		bills = append(bills, billSummary(id, scheme, schemeName, status, y, m, base, discount, net, verified, reversed, outstanding, dueAt, createdAt))
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"student_user_id": studentID,
		"school_id":       schoolID,
		"outstanding_idr": total,
		"bills":           bills,
	}, nil)
}
