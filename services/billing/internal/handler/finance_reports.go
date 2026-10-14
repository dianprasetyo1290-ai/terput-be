package handler

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// openBalanceCTE is the open balance of every non-void bill issued before
// the as-of instant $2, counting only money verified/reversed before it.
var openBalanceCTE = `WITH ob AS (
	SELECT b.student_user_id::text AS student, b.due_at,
	       b.net_idr
	     - COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
	                 WHERE p.student_bill_id=b.id AND p.status='verified' AND p.verified_at < $2),0)
	     + COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
	                 WHERE p.student_bill_id=b.id AND rv.created_at < $2),0) AS out
	  FROM student_bills b
	 WHERE b.school_id=$1 AND b.status<>'void' AND b.created_at < $2
), ab AS (
	SELECT student, out, ` + model.AgingBucketSQL("due_at", "$2::timestamptz") + ` AS bucket FROM ob WHERE out > 0
)`

type agingBucketTotal struct {
	Count     int   `json:"count"`
	AmountIDR int64 `json:"amount_idr"`
}

// agingTotals returns the open balance per aging bucket as of asOf.
func (a *App) agingTotals(r *http.Request, schoolID string, asOf time.Time) (map[string]agingBucketTotal, int64, error) {
	out := map[string]agingBucketTotal{}
	for _, k := range model.AgingBuckets {
		out[k] = agingBucketTotal{}
	}
	rows, err := a.DB.QueryContext(r.Context(), openBalanceCTE+`
		SELECT bucket, count(*), COALESCE(sum(out),0) FROM ab GROUP BY bucket`, schoolID, asOf)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var total int64
	for rows.Next() {
		var k string
		var b agingBucketTotal
		if err := rows.Scan(&k, &b.Count, &b.AmountIDR); err != nil {
			return nil, 0, err
		}
		out[k] = b
		total += b.AmountIDR
	}
	return out, total, rows.Err()
}

// FinanceAging is GET /admin/finance/aging?as_of=YYYY-MM-DD&bucket=&page=&per_page=
// (menu 23): receivable aging computed in SQL as of a date, the per-student
// breakdown (paginated, largest balance first) and the month-end trend of
// the last 12 months (BUG-138/175).
func (a *App) FinanceAging(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	loc := a.loc()
	asOf := a.now()
	if s := strings.TrimSpace(r.URL.Query().Get("as_of")); s != "" {
		d, err := time.ParseInLocation("2006-01-02", s, loc)
		if err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, "as_of harus berformat YYYY-MM-DD")
			return
		}
		if end := d.AddDate(0, 0, 1); end.Before(asOf) {
			asOf = end
		}
	}
	bucketFilter := strings.TrimSpace(r.URL.Query().Get("bucket"))
	if bucketFilter != "" {
		valid := false
		for _, k := range model.AgingBuckets {
			valid = valid || k == bucketFilter
		}
		if !valid {
			common.WriteError(w, http.StatusUnprocessableEntity, "bucket tidak dikenal")
			return
		}
	}
	page, perPage := common.Page(r)

	buckets, total, err := a.agingTotals(r, schoolID, asOf)
	if err != nil {
		writeErr(w, err)
		return
	}

	type studentAging struct {
		StudentUserID string           `json:"student_user_id"`
		StudentName   string           `json:"student_name"`
		Bills         int              `json:"bills"`
		TotalIDR      int64            `json:"total_idr"`
		Buckets       map[string]int64 `json:"buckets"`
		OldestBucket  string           `json:"oldest_bucket"`
	}
	rows, err := a.DB.QueryContext(r.Context(), openBalanceCTE+`, ps AS (
		SELECT student, count(*) AS bills, sum(out) AS total,
		       COALESCE(sum(out) FILTER (WHERE bucket='current'),0) AS b0,
		       COALESCE(sum(out) FILTER (WHERE bucket='d1_30'),0) AS b1,
		       COALESCE(sum(out) FILTER (WHERE bucket='d31_60'),0) AS b2,
		       COALESCE(sum(out) FILTER (WHERE bucket='d61_90'),0) AS b3,
		       COALESCE(sum(out) FILTER (WHERE bucket='d90_plus'),0) AS b4
		  FROM ab GROUP BY student
	)
	SELECT student, bills, total, b0, b1, b2, b3, b4, count(*) OVER () FROM ps
	 WHERE $3='' OR ($3='current' AND b0>0) OR ($3='d1_30' AND b1>0) OR ($3='d31_60' AND b2>0)
	    OR ($3='d61_90' AND b3>0) OR ($3='d90_plus' AND b4>0)
	 ORDER BY total DESC, student LIMIT $4 OFFSET $5`,
		schoolID, asOf, bucketFilter, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []studentAging{}
	count := 0
	for rows.Next() {
		var s studentAging
		var b [5]int64
		if err := rows.Scan(&s.StudentUserID, &s.Bills, &s.TotalIDR, &b[0], &b[1], &b[2], &b[3], &b[4], &count); err != nil {
			writeErr(w, err)
			return
		}
		s.Buckets = map[string]int64{}
		for i, k := range model.AgingBuckets {
			s.Buckets[k] = b[i]
			if b[i] > 0 {
				s.OldestBucket = k
			}
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].StudentUserID
	}
	names := a.userNames(r.Context(), ids)
	for i := range items {
		items[i].StudentName = names[items[i].StudentUserID]
	}

	type trendPoint struct {
		Year     int                         `json:"year"`
		Month    int                         `json:"month"`
		TotalIDR int64                       `json:"total_idr"`
		Buckets  map[string]agingBucketTotal `json:"buckets"`
	}
	trend := []trendPoint{}
	for _, p := range model.TrendPoints(asOf, 12, loc) {
		b, t, err := a.agingTotals(r, schoolID, p.At)
		if err != nil {
			writeErr(w, err)
			return
		}
		trend = append(trend, trendPoint{Year: p.Year, Month: p.Month, TotalIDR: t, Buckets: b})
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"as_of":     asOf,
		"total_idr": total,
		"buckets":   buckets,
		"students":  items,
		"trend":     trend,
	}, &common.Meta{Page: page, PerPage: perPage, Total: count})
}

// receiptRow is one aggregated slice of the receipts report.
type receiptRow struct {
	Key       string `json:"key"`
	Count     int    `json:"count"`
	AmountIDR int64  `json:"amount_idr"`
}

// FinanceReceipts is GET /admin/finance/receipts?from=&to= (menu 17,
// BUG-123): money received (verified ledger allocations) in the range per
// month, unit (scheme grade level), fee component and payment method. A
// payment allocated to a whole bill is spread over the bill's components in
// proportion to their net amounts (model.SplitByWeights).
func (a *App) FinanceReceipts(w http.ResponseWriter, r *http.Request) {
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
	rows, err := a.DB.QueryContext(ctx,
		`SELECT al.id::text, al.amount_idr, to_char(`+localTS("p.verified_at", "$4")+`, 'YYYY-MM'),
		        COALESCE(NULLIF(btrim(s.grade_level),''), 'Semua tingkat'), `+paymentMethodSQL+`,
		        (al.bill_item_id IS NOT NULL), COALESCE(i.name,''), COALESCE(i.net_idr,0)
		   FROM bill_payments p
		   JOIN bill_payment_allocations al ON al.payment_id=p.id
		   JOIN student_bills b ON b.id=p.student_bill_id
		   LEFT JOIN billing_schemes s ON s.id=b.scheme_id`+paymentMethodJoins+`
		   LEFT JOIN bill_items i ON (al.bill_item_id IS NOT NULL AND i.id=al.bill_item_id)
		                          OR (al.bill_item_id IS NULL AND i.bill_id=b.id)
		  WHERE p.school_id=$1 AND p.status='verified' AND p.verified_at >= $2 AND p.verified_at < $3
		  ORDER BY al.id, i.created_at, i.id`,
		schoolID, rg.Start, rg.End, a.tzOffsetSeconds())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type alloc struct {
		amount              int64
		month, unit, method string
		direct              bool
		names               []string
		weights             []int64
	}
	var allocs []*alloc
	byID := map[string]*alloc{}
	for rows.Next() {
		var id, month, unit, method, name string
		var amount, weight int64
		var direct bool
		if err := rows.Scan(&id, &amount, &month, &unit, &method, &direct, &name, &weight); err != nil {
			writeErr(w, err)
			return
		}
		al := byID[id]
		if al == nil {
			al = &alloc{amount: amount, month: month, unit: unit, method: method, direct: direct}
			byID[id] = al
			allocs = append(allocs, al)
		}
		if name != "" {
			al.names = append(al.names, name)
			al.weights = append(al.weights, weight)
		}
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}

	agg := map[string]map[string]*receiptRow{"month": {}, "unit": {}, "component": {}, "method": {}}
	add := func(dim, key string, amount int64, count int) {
		row := agg[dim][key]
		if row == nil {
			row = &receiptRow{Key: key}
			agg[dim][key] = row
		}
		row.AmountIDR += amount
		row.Count += count
	}
	var total int64
	for _, al := range allocs {
		total += al.amount
		add("month", al.month, al.amount, 1)
		add("unit", al.unit, al.amount, 1)
		add("method", al.method, al.amount, 1)
		switch {
		case len(al.names) == 0:
			add("component", "Tanpa komponen", al.amount, 1)
		case al.direct:
			add("component", al.names[0], al.amount, 1)
		default:
			for i, part := range model.SplitByWeights(al.amount, al.weights) {
				if part > 0 {
					add("component", al.names[i], part, 1)
				}
			}
		}
	}

	var reversed int64
	if err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(rv.amount_idr),0) FROM bill_payment_reversals rv
		  WHERE rv.school_id=$1 AND rv.created_at >= $2 AND rv.created_at < $3`,
		schoolID, rg.Start, rg.End).Scan(&reversed); err != nil {
		writeErr(w, err)
		return
	}

	list := func(dim string, byKey bool) []receiptRow {
		out := []receiptRow{}
		for _, v := range agg[dim] {
			out = append(out, *v)
		}
		sort.Slice(out, func(i, j int) bool {
			if byKey || out[i].AmountIDR == out[j].AmountIDR {
				return out[i].Key < out[j].Key
			}
			return out[i].AmountIDR > out[j].AmountIDR
		})
		return out
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"from":         rg.From.Format("2006-01-02"),
		"to":           rg.To.Format("2006-01-02"),
		"received_idr": total,
		"reversed_idr": reversed,
		"net_idr":      total - reversed,
		"by_month":     list("month", true),
		"by_unit":      list("unit", false),
		"by_component": list("component", false),
		"by_method":    list("method", false),
	}, nil)
}
