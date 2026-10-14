package handler

import (
	"net/http"
	"sort"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// FinanceSummary is GET /admin/finance/summary?period_year=&period_month= —
// the data behind the Finance Dashboard (menu 17) and Payment Dashboard
// (menu 26). Bill-side figures cover bills of that period; payment-side
// figures cover payments submitted/verified in that calendar period.
// ponytail: per-bill aggregation happens in Go over every bill of the period;
// move into SQL if a school ever has 100k+ bills a year.
func (a *App) FinanceSummary(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	year := common.IntParam(r, "period_year")
	if year == 0 {
		year = time.Now().Year()
	}
	month := common.IntParam(r, "period_month")
	ctx := r.Context()

	// --- bills of the period, with their ledger paid amount -------------
	rows, err := a.DB.QueryContext(ctx,
		`SELECT b.student_user_id::text,b.period_month,b.net_idr,b.due_at,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                  WHERE p.student_bill_id=b.id AND p.status='verified'),0)
		      - COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                  WHERE p.student_bill_id=b.id),0)
		 FROM student_bills b
		 WHERE b.school_id=$1 AND b.status<>'void' AND b.period_year=$2 AND ($3=0 OR b.period_month=$3)`,
		schoolID, year, month)
	if err != nil {
		writeErr(w, err)
		return
	}
	type bucket struct {
		Count     int   `json:"count"`
		AmountIDR int64 `json:"amount_idr"`
	}
	type trendPoint struct {
		Month       int   `json:"month"`
		BilledIDR   int64 `json:"billed_idr"`
		PaidIDR     int64 `json:"paid_idr"`
		Outstanding int64 `json:"outstanding_idr"`
	}
	type studentDebt struct {
		StudentUserID  string `json:"student_user_id"`
		BilledIDR      int64  `json:"billed_idr"`
		PaidIDR        int64  `json:"paid_idr"`
		OutstandingIDR int64  `json:"outstanding_idr"`
		Bills          int    `json:"bills"`
	}
	var billed, paid, outstanding int64
	status := map[string]*bucket{"paid": {}, "partial": {}, "unpaid": {}}
	aging := map[string]*bucket{"current": {}, "d1_30": {}, "d31_60": {}, "d61_90": {}, "d90_plus": {}}
	trend := make([]trendPoint, 12)
	for i := range trend {
		trend[i].Month = i + 1
	}
	students := map[string]*studentDebt{}
	now := time.Now()
	for rows.Next() {
		var (
			student string
			m       int
			net, pd int64
			dueAt   *time.Time
		)
		if err := rows.Scan(&student, &m, &net, &dueAt, &pd); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		out := max(net-pd, 0)
		billed += net
		paid += pd
		outstanding += out
		st := string(model.DeriveBillStatus(net, pd))
		status[st].Count++
		status[st].AmountIDR += net
		trend[m-1].BilledIDR += net
		trend[m-1].PaidIDR += pd
		trend[m-1].Outstanding += out
		if out > 0 {
			key := "current"
			if dueAt != nil && now.After(*dueAt) {
				switch days := int(now.Sub(*dueAt).Hours() / 24); {
				case days <= 30:
					key = "d1_30"
				case days <= 60:
					key = "d31_60"
				case days <= 90:
					key = "d61_90"
				default:
					key = "d90_plus"
				}
			}
			aging[key].Count++
			aging[key].AmountIDR += out
		}
		sd := students[student]
		if sd == nil {
			sd = &studentDebt{StudentUserID: student}
			students[student] = sd
		}
		sd.BilledIDR += net
		sd.PaidIDR += pd
		sd.OutstandingIDR += out
		sd.Bills++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	top := []studentDebt{}
	for _, s := range students {
		if s.OutstandingIDR > 0 {
			top = append(top, *s)
		}
	}
	sort.Slice(top, func(i, j int) bool { return top[i].OutstandingIDR > top[j].OutstandingIDR })
	if len(top) > 10 {
		top = top[:10]
	}

	// --- billed per component -------------------------------------------
	type named struct {
		Name      string `json:"name"`
		Count     int    `json:"count"`
		AmountIDR int64  `json:"amount_idr"`
	}
	scanNamed := func(query string, args ...any) ([]named, error) {
		rs, err := a.DB.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rs.Close()
		out := []named{}
		for rs.Next() {
			var n named
			if err := rs.Scan(&n.Name, &n.Count, &n.AmountIDR); err != nil {
				return nil, err
			}
			out = append(out, n)
		}
		return out, rs.Err()
	}
	components, err := scanNamed(
		`SELECT i.name,count(*),COALESCE(sum(i.amount_idr),0) FROM bill_items i JOIN student_bills b ON b.id=i.bill_id
		 WHERE b.school_id=$1 AND b.status<>'void' AND b.period_year=$2 AND ($3=0 OR b.period_month=$3)
		 GROUP BY i.name ORDER BY 3 DESC`, schoolID, year, month)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Receipts by destination bank (manual transfers are the only channel).
	methods, err := scanNamed(
		`SELECT COALESCE(NULLIF(btrim(bank_name),''),'Tanpa keterangan bank'),count(*),COALESCE(sum(amount_idr),0)
		 FROM bill_payments WHERE school_id=$1 AND status='verified'
		   AND extract(year FROM verified_at)=$2 AND ($3=0 OR extract(month FROM verified_at)=$3)
		 GROUP BY 1 ORDER BY 3 DESC`, schoolID, year, month)
	if err != nil {
		writeErr(w, err)
		return
	}
	payStatus, err := scanNamed(
		`SELECT status,count(*),COALESCE(sum(amount_idr),0) FROM bill_payments
		 WHERE school_id=$1 AND extract(year FROM created_at)=$2 AND ($3=0 OR extract(month FROM created_at)=$3)
		 GROUP BY status`, schoolID, year, month)
	if err != nil {
		writeErr(w, err)
		return
	}
	refunds, err := scanNamed(
		`SELECT status,count(*),COALESCE(sum(amount_idr),0) FROM bill_refunds
		 WHERE school_id=$1 AND extract(year FROM created_at)=$2 AND ($3=0 OR extract(month FROM created_at)=$3)
		 GROUP BY status`, schoolID, year, month)
	if err != nil {
		writeErr(w, err)
		return
	}

	// Payment submissions per month and status (Payment Dashboard trend).
	type payTrend struct {
		Month     int    `json:"month"`
		Status    string `json:"status"`
		Count     int    `json:"count"`
		AmountIDR int64  `json:"amount_idr"`
	}
	payTrends := []payTrend{}
	pt, err := a.DB.QueryContext(ctx,
		`SELECT extract(month FROM created_at)::int,status,count(*),COALESCE(sum(amount_idr),0) FROM bill_payments
		 WHERE school_id=$1 AND extract(year FROM created_at)=$2 GROUP BY 1,2 ORDER BY 1`, schoolID, year)
	if err != nil {
		writeErr(w, err)
		return
	}
	for pt.Next() {
		var p payTrend
		if err := pt.Scan(&p.Month, &p.Status, &p.Count, &p.AmountIDR); err != nil {
			pt.Close()
			writeErr(w, err)
			return
		}
		payTrends = append(payTrends, p)
	}
	pt.Close()

	var discountIDR, scholarshipIDR int64
	if err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(st.applied_idr),0),COALESCE(sum(st.applied_idr) FILTER (WHERE d.scholarship),0)
		 FROM bill_discount_steps st JOIN student_bills b ON b.id=st.bill_id
		 LEFT JOIN bill_discounts d ON d.id=st.discount_id
		 WHERE b.school_id=$1 AND b.status<>'void' AND b.period_year=$2 AND ($3=0 OR b.period_month=$3)`,
		schoolID, year, month).Scan(&discountIDR, &scholarshipIDR); err != nil {
		writeErr(w, err)
		return
	}

	// Latest payment activity.
	recent := []map[string]any{}
	rr, err := a.DB.QueryContext(ctx,
		`SELECT p.id::text,b.student_user_id::text,p.amount_idr,p.status,p.created_at,COALESCE(p.bank_name,''),b.period_year,b.period_month
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.school_id=$1 ORDER BY p.created_at DESC LIMIT 8`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rr.Next() {
		var id, student, st, bank string
		var amt int64
		var at time.Time
		var y, m int
		if err := rr.Scan(&id, &student, &amt, &st, &at, &bank, &y, &m); err != nil {
			rr.Close()
			writeErr(w, err)
			return
		}
		recent = append(recent, map[string]any{"id": id, "student_user_id": student, "amount_idr": amt,
			"status": st, "created_at": at, "bank_name": bank, "period_year": y, "period_month": m})
	}
	rr.Close()

	var rate float64
	if billed > 0 {
		rate = float64(paid) / float64(billed)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"period_year": year, "period_month": month,
		"billed_idr": billed, "paid_idr": paid, "outstanding_idr": outstanding, "collection_rate": rate,
		"bill_status": status, "aging": aging, "trend": trend, "top_outstanding": top,
		"components": components, "methods": methods, "payment_status": payStatus, "payment_trend": payTrends,
		"refunds": refunds, "discount_idr": discountIDR, "scholarship_idr": scholarshipIDR, "recent_payments": recent,
	}, nil)
}
