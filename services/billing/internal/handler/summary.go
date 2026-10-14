package handler

import (
	"context"
	"net/http"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// billLedgerCTE is every non-void bill of the school whose period falls in
// [$2::date, $3::date] with its ledger paid amount (verified allocations
// minus reversals) and open balance. $4 is the aging as-of instant.
var billLedgerCTE = `WITH bl AS (
	SELECT b.student_user_id::text AS student, b.period_year AS y, b.period_month AS m, b.net_idr AS net, b.due_at,
	       COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
	                 WHERE p.student_bill_id=b.id AND p.status='verified'),0)
	     - COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
	                 WHERE p.student_bill_id=b.id),0) AS paid
	  FROM student_bills b
	 WHERE b.school_id=$1 AND b.status<>'void'
	   AND make_date(b.period_year,b.period_month,1) BETWEEN $2::date AND $3::date
), bo AS (
	SELECT bl.*, GREATEST(net-paid,0) AS out, ` + model.AgingBucketSQL("due_at", "$4::timestamptz") + ` AS bucket FROM bl
)`

// paymentMethodSQL labels a bill_payments row (alias p) by the channel the
// money came through; gt/cp are the joined gateway and cashier rows.
const paymentMethodSQL = `CASE p.source
	WHEN 'gateway' THEN 'Online: ' || COALESCE(pm.label, 'gateway')
	WHEN 'cashier' THEN CASE cp.method WHEN 'cash' THEN 'Kasir: tunai' WHEN 'transfer' THEN 'Kasir: transfer' ELSE 'Kasir' END
	WHEN 'deposit' THEN 'Deposit siswa'
	ELSE 'Transfer: ' || COALESCE(NULLIF(btrim(p.bank_name),''),'tanpa keterangan bank') END`

const paymentMethodJoins = `
	LEFT JOIN gateway_transactions gt ON p.source='gateway' AND gt.id=p.source_id
	LEFT JOIN payment_methods pm ON pm.id=gt.method_id
	LEFT JOIN cashier_payments cp ON p.source='cashier' AND cp.id=p.source_id`

// reportRange reads either from/to (validated) or period_year/period_month.
func (a *App) reportRange(r *http.Request) (model.ReportRange, error) {
	loc := a.loc()
	rg, ok, err := model.ParseReportRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"), loc)
	if err != nil {
		return rg, &httpError{status: http.StatusUnprocessableEntity, msg: err.Error()}
	}
	if ok {
		return rg, nil
	}
	year := common.IntParam(r, "period_year")
	if year == 0 {
		year = a.now().In(loc).Year()
	}
	month := common.IntParam(r, "period_month")
	if year < 2000 || year > 2100 || month < 0 || month > 12 {
		return rg, &httpError{status: http.StatusUnprocessableEntity, msg: "period_year/period_month tidak valid"}
	}
	return model.PeriodRange(year, month, loc), nil
}

// FinanceSummary is GET /admin/finance/summary — the data behind the Finance
// Dashboard (menu 17) and Payment Dashboard (menu 26). The window is either
// ?from=&to= (YYYY-MM-DD, validated) or ?period_year=&period_month=.
// Bill-side figures cover bills whose period lies in the window;
// payment-side figures cover money verified/submitted inside it. Every
// aggregate is computed in SQL.
func (a *App) FinanceSummary(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	rg, err := a.reportRange(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	ctx := r.Context()
	now := a.now()
	periodFrom := time.Date(rg.From.Year(), rg.From.Month(), 1, 0, 0, 0, 0, rg.From.Location()).Format("2006-01-02")
	periodTo := rg.To.Format("2006-01-02")
	billArgs := []any{schoolID, periodFrom, periodTo, now}

	type bucket struct {
		Count     int   `json:"count"`
		AmountIDR int64 `json:"amount_idr"`
	}
	var billed, paid, outstanding int64
	status := map[string]*bucket{"paid": {}, "partial": {}, "unpaid": {}}
	aging := map[string]*bucket{}
	for _, k := range model.AgingBuckets {
		aging[k] = &bucket{}
	}
	if err := a.DB.QueryRowContext(ctx, billLedgerCTE+`
		SELECT COALESCE(sum(net),0), COALESCE(sum(paid),0), COALESCE(sum(out),0),
		       count(*) FILTER (WHERE paid<=0), COALESCE(sum(net) FILTER (WHERE paid<=0),0),
		       count(*) FILTER (WHERE paid>0 AND paid<net), COALESCE(sum(net) FILTER (WHERE paid>0 AND paid<net),0),
		       count(*) FILTER (WHERE paid>0 AND paid>=net), COALESCE(sum(net) FILTER (WHERE paid>0 AND paid>=net),0)
		  FROM bo`, billArgs...).Scan(&billed, &paid, &outstanding,
		&status["unpaid"].Count, &status["unpaid"].AmountIDR,
		&status["partial"].Count, &status["partial"].AmountIDR,
		&status["paid"].Count, &status["paid"].AmountIDR); err != nil {
		writeErr(w, err)
		return
	}
	ar, err := a.DB.QueryContext(ctx, billLedgerCTE+`
		SELECT bucket, count(*), COALESCE(sum(out),0) FROM bo WHERE out>0 GROUP BY bucket`, billArgs...)
	if err != nil {
		writeErr(w, err)
		return
	}
	for ar.Next() {
		var k string
		var b bucket
		if err := ar.Scan(&k, &b.Count, &b.AmountIDR); err != nil {
			ar.Close()
			writeErr(w, err)
			return
		}
		aging[k] = &b
	}
	ar.Close()

	type trendPoint struct {
		Year        int   `json:"year"`
		Month       int   `json:"month"`
		BilledIDR   int64 `json:"billed_idr"`
		PaidIDR     int64 `json:"paid_idr"`
		Outstanding int64 `json:"outstanding_idr"`
	}
	months := rg.Months()
	trend := make([]trendPoint, len(months))
	idx := map[[2]int]int{}
	for i, ym := range months {
		trend[i] = trendPoint{Year: ym[0], Month: ym[1]}
		idx[ym] = i
	}
	tr, err := a.DB.QueryContext(ctx, billLedgerCTE+`
		SELECT y, m, COALESCE(sum(net),0), COALESCE(sum(paid),0), COALESCE(sum(out),0) FROM bo GROUP BY y, m`, billArgs...)
	if err != nil {
		writeErr(w, err)
		return
	}
	for tr.Next() {
		var p trendPoint
		if err := tr.Scan(&p.Year, &p.Month, &p.BilledIDR, &p.PaidIDR, &p.Outstanding); err != nil {
			tr.Close()
			writeErr(w, err)
			return
		}
		if i, ok := idx[[2]int{p.Year, p.Month}]; ok {
			trend[i] = p
		}
	}
	tr.Close()

	type studentDebt struct {
		StudentUserID  string `json:"student_user_id"`
		StudentName    string `json:"student_name"`
		BilledIDR      int64  `json:"billed_idr"`
		PaidIDR        int64  `json:"paid_idr"`
		OutstandingIDR int64  `json:"outstanding_idr"`
		Bills          int    `json:"bills"`
	}
	top := []studentDebt{}
	ts, err := a.DB.QueryContext(ctx, billLedgerCTE+`
		SELECT student, sum(net), sum(paid), sum(out), count(*) FROM bo
		 GROUP BY student HAVING sum(out)>0 ORDER BY sum(out) DESC, student LIMIT 10`, billArgs...)
	if err != nil {
		writeErr(w, err)
		return
	}
	for ts.Next() {
		var s studentDebt
		if err := ts.Scan(&s.StudentUserID, &s.BilledIDR, &s.PaidIDR, &s.OutstandingIDR, &s.Bills); err != nil {
			ts.Close()
			writeErr(w, err)
			return
		}
		top = append(top, s)
	}
	ts.Close()
	ids := make([]string, len(top))
	for i := range top {
		ids[i] = top[i].StudentUserID
	}
	names := a.userNames(ctx, ids)
	for i := range top {
		top[i].StudentName = names[top[i].StudentUserID]
	}

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
		`SELECT i.name,count(*),COALESCE(sum(i.net_idr),0) FROM bill_items i JOIN student_bills b ON b.id=i.bill_id
		 WHERE b.school_id=$1 AND b.status<>'void' AND make_date(b.period_year,b.period_month,1) BETWEEN $2::date AND $3::date
		 GROUP BY i.name ORDER BY 3 DESC`, schoolID, periodFrom, periodTo)
	if err != nil {
		writeErr(w, err)
		return
	}
	methods, err := scanNamed(
		`SELECT `+paymentMethodSQL+`,count(*),COALESCE(sum(p.amount_idr),0)
		 FROM bill_payments p`+paymentMethodJoins+`
		 WHERE p.school_id=$1 AND p.status='verified' AND p.verified_at >= $2 AND p.verified_at < $3
		 GROUP BY 1 ORDER BY 3 DESC`, schoolID, rg.Start, rg.End)
	if err != nil {
		writeErr(w, err)
		return
	}
	payStatus, err := scanNamed(
		`SELECT status,count(*),COALESCE(sum(amount_idr),0) FROM bill_payments
		 WHERE school_id=$1 AND created_at >= $2 AND created_at < $3 GROUP BY status`, schoolID, rg.Start, rg.End)
	if err != nil {
		writeErr(w, err)
		return
	}
	refunds, err := scanNamed(
		`SELECT status,count(*),COALESCE(sum(amount_idr),0) FROM bill_refunds
		 WHERE school_id=$1 AND created_at >= $2 AND created_at < $3 GROUP BY status`, schoolID, rg.Start, rg.End)
	if err != nil {
		writeErr(w, err)
		return
	}

	type payTrend struct {
		Year      int    `json:"year"`
		Month     int    `json:"month"`
		Status    string `json:"status"`
		Count     int    `json:"count"`
		AmountIDR int64  `json:"amount_idr"`
	}
	payTrends := []payTrend{}
	pt, err := a.DB.QueryContext(ctx,
		`SELECT extract(year FROM `+localTS("created_at", "$4")+`)::int, extract(month FROM `+localTS("created_at", "$4")+`)::int,
		        status, count(*), COALESCE(sum(amount_idr),0) FROM bill_payments
		  WHERE school_id=$1 AND created_at >= $2 AND created_at < $3 GROUP BY 1,2,3 ORDER BY 1,2`,
		schoolID, rg.Start, rg.End, a.tzOffsetSeconds())
	if err != nil {
		writeErr(w, err)
		return
	}
	for pt.Next() {
		var p payTrend
		if err := pt.Scan(&p.Year, &p.Month, &p.Status, &p.Count, &p.AmountIDR); err != nil {
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
		 WHERE b.school_id=$1 AND b.status<>'void' AND make_date(b.period_year,b.period_month,1) BETWEEN $2::date AND $3::date`,
		schoolID, periodFrom, periodTo).Scan(&discountIDR, &scholarshipIDR); err != nil {
		writeErr(w, err)
		return
	}

	recent := []map[string]any{}
	rr, err := a.DB.QueryContext(ctx,
		`SELECT p.id::text,b.student_user_id::text,p.amount_idr,p.status,p.created_at,COALESCE(p.bank_name,''),b.period_year,b.period_month
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.school_id=$1 AND p.created_at >= $2 AND p.created_at < $3 ORDER BY p.created_at DESC LIMIT 8`,
		schoolID, rg.Start, rg.End)
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

	// Collection rate in basis points (paid / billed), integer only.
	var rateBP int64
	if billed > 0 {
		rateBP = paid * 10000 / billed
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"period_year": rg.From.Year(), "period_month": common.IntParam(r, "period_month"),
		"from": rg.From.Format("2006-01-02"), "to": rg.To.Format("2006-01-02"),
		"billed_idr": billed, "paid_idr": paid, "outstanding_idr": outstanding,
		"collection_rate": float64(rateBP) / 10000, "collection_rate_bp": rateBP,
		"bill_status": status, "aging": aging, "trend": trend, "top_outstanding": top,
		"components": components, "methods": methods, "payment_status": payStatus, "payment_trend": payTrends,
		"refunds": refunds, "discount_idr": discountIDR, "scholarship_idr": scholarshipIDR, "recent_payments": recent,
	}, nil)
}

// localTS renders a timestamptz column as the school's wall-clock time,
// given the zone's UTC offset in seconds bound to placeholder ph.
func localTS(col, ph string) string {
	return `((` + col + ` AT TIME ZONE 'UTC') + make_interval(secs => ` + ph + `::int))`
}

// tzOffsetSeconds is the school zone's current UTC offset (Indonesian zones
// have no daylight saving, so one offset covers any report range).
func (a *App) tzOffsetSeconds() int {
	_, off := a.now().In(a.loc()).Zone()
	return off
}

// userNames resolves display names through identity, best effort: an id
// identity cannot resolve maps to "".
func (a *App) userNames(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if a.IdentitySvc == nil || !a.IdentitySvc.Configured() {
		return out
	}
	for _, id := range ids {
		if _, seen := out[id]; seen || id == "" {
			continue
		}
		out[id] = ""
		if u, err := a.IdentitySvc.User(ctx, id); err == nil {
			out[id] = u.Name
		}
	}
	return out
}
