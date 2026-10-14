package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestFinanceReports covers BUG-123 (range validation, receipts per period/
// unit/component/method), BUG-138 (SQL aggregation, paginated aging and
// bill lists) and BUG-175 (monthly aging trend).
func TestFinanceReports(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	jkt := time.FixedZone("WIB", 7*3600)
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, jkt)
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Config.Gateway.Location = jkt
	app.Clock = func() time.Time { return now }
	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Get("/admin/finance/summary", app.FinanceSummary)
	r.Get("/admin/finance/aging", app.FinanceAging)
	r.Get("/admin/finance/receipts", app.FinanceReceipts)
	r.Get("/admin/arrears", app.ListArrears)

	schoolID, schemeID := genUUID(), genUUID()
	s1, s2 := genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, grade_level, name, status, installment, due_day)
		VALUES ($1, $2, $3, '7', 'SPP QA', 'active', true, 10)`, schemeID, schoolID, genUUID())
	bill := func(student string, month int, net int64, due time.Time) (string, string, string) {
		id, spp, gedung := genUUID(), genUUID(), genUUID()
		mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at, created_at) VALUES ($1,$2,$3,$4,2026,$5,$6,0,$6,'unpaid',$7,$8)`,
			id, schoolID, student, schemeID, month, net, due, due.AddDate(0, 0, -20))
		mustExec(t, db, `INSERT INTO bill_items (id, school_id, bill_id, name, component_code, amount_idr, net_idr)
			VALUES ($1,$2,$3,'SPP','SPP',$4,$4)`, spp, schoolID, id, net*3/4)
		mustExec(t, db, `INSERT INTO bill_items (id, school_id, bill_id, name, component_code, amount_idr, net_idr)
			VALUES ($1,$2,$3,'Gedung','GDG',$4,$4)`, gedung, schoolID, id, net/4)
		return id, spp, gedung
	}
	pay := func(billID string, item any, amount int64, at time.Time, source, bank string) {
		pid := genUUID()
		mustExec(t, db, `INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, bank_name, reference, object_key, mime, size_bytes, sha256,
			status, idempotency_key, uploader_user_id, verified_at, source)
			VALUES ($1,$2,$3,$4,$5,$6,'R','k','image/png',1,'x','verified',$7,$8,$5,$9)`,
			pid, schoolID, billID, amount, at, bank, "bp-"+pid, genUUID(), source)
		mustExec(t, db, `INSERT INTO bill_payment_allocations (school_id, payment_id, bill_item_id, amount_idr) VALUES ($1,$2,$3,$4)`,
			schoolID, pid, item, amount)
	}
	// s1: July bill fully paid by transfer to the SPP item; August bill
	// (due 10 Aug, 65 days overdue) paid 400k against the whole bill.
	jul, julSPP, _ := bill(s1, 7, 1000000, time.Date(2026, 7, 10, 0, 0, 0, 0, jkt))
	pay(jul, julSPP, 750000, time.Date(2026, 7, 5, 9, 0, 0, 0, jkt), "upload", "BCA")
	julGedung := ""
	_ = db.QueryRow(`SELECT id::text FROM bill_items WHERE bill_id=$1 AND component_code='GDG'`, jul).Scan(&julGedung)
	pay(jul, julGedung, 250000, time.Date(2026, 7, 6, 9, 0, 0, 0, jkt), "upload", "BCA")
	aug, _, _ := bill(s1, 8, 1000000, time.Date(2026, 8, 10, 0, 0, 0, 0, jkt))
	pay(aug, nil, 400000, time.Date(2026, 9, 2, 9, 0, 0, 0, jkt), "upload", "Mandiri")
	// s2: October bill not yet due.
	bill(s2, 10, 2000000, time.Date(2026, 10, 20, 0, 0, 0, 0, jkt))

	api := apiCaller{t: t, r: r, token: signToken(t, genUUID(), schoolID, common.RoleKepalaYayasan)}

	for _, tc := range []struct {
		name, q string
		want    int
	}{
		{"reversed range", "?from=2026-12-01&to=2026-01-01", http.StatusUnprocessableEntity},
		{"half range", "?from=2026-01-01", http.StatusUnprocessableEntity},
		{"bad month", "?period_year=2026&period_month=13", http.StatusUnprocessableEntity},
		{"year", "?period_year=2026", http.StatusOK},
	} {
		if code, env := api.do(http.MethodGet, "/admin/finance/summary"+tc.q, nil); code != tc.want {
			t.Fatalf("summary %s: %d %v", tc.name, code, env)
		}
	}
	if code, _ := api.do(http.MethodGet, "/admin/finance/receipts?from=2026-12-01&to=2026-01-01", nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("receipts reversed range: %d", code)
	}

	_, env := api.do(http.MethodGet, "/admin/finance/summary?period_year=2026", nil)
	d := dataOf(env)
	if d["billed_idr"].(float64) != 4000000 || d["paid_idr"].(float64) != 1400000 || d["outstanding_idr"].(float64) != 2600000 {
		t.Fatalf("summary totals %v", d)
	}
	ag := d["aging"].(map[string]any)
	if ag["d61_90"].(map[string]any)["amount_idr"].(float64) != 600000 || ag["current"].(map[string]any)["amount_idr"].(float64) != 2000000 {
		t.Fatalf("summary aging %v", ag)
	}
	if len(d["trend"].([]any)) != 12 {
		t.Fatalf("trend %v", d["trend"])
	}
	// A range limited to July only sees the July bill.
	_, env = api.do(http.MethodGet, "/admin/finance/summary?from=2026-07-01&to=2026-07-31", nil)
	if d := dataOf(env); d["billed_idr"].(float64) != 1000000 || len(d["trend"].([]any)) != 1 {
		t.Fatalf("july summary %v", d)
	}

	// Receipts: 1.4M in Jul–Sep; the 400k whole-bill payment is spread
	// 300k SPP / 100k Gedung.
	_, env = api.do(http.MethodGet, "/admin/finance/receipts?from=2026-07-01&to=2026-09-30", nil)
	d = dataOf(env)
	if d["received_idr"].(float64) != 1400000 {
		t.Fatalf("receipts %v", d)
	}
	comp := map[string]float64{}
	for _, x := range d["by_component"].([]any) {
		row := x.(map[string]any)
		comp[row["key"].(string)] = row["amount_idr"].(float64)
	}
	if comp["SPP"] != 1050000 || comp["Gedung"] != 350000 {
		t.Fatalf("by component %v", comp)
	}
	if len(d["by_month"].([]any)) != 2 || len(d["by_method"].([]any)) != 2 {
		t.Fatalf("by month/method %v %v", d["by_month"], d["by_method"])
	}
	if u := d["by_unit"].([]any); len(u) != 1 || u[0].(map[string]any)["key"] != "7" {
		t.Fatalf("by unit %v", u)
	}

	// Aging as of today: s2 2M current, s1 600k in 61–90.
	code, env := api.do(http.MethodGet, "/admin/finance/aging?per_page=1", nil)
	if code != http.StatusOK {
		t.Fatalf("aging %d %v", code, env)
	}
	d = dataOf(env)
	if d["total_idr"].(float64) != 2600000 {
		t.Fatalf("aging total %v", d)
	}
	if meta := env["meta"].(map[string]any); meta["total"].(float64) != 2 {
		t.Fatalf("aging meta %v", meta)
	}
	st := d["students"].([]any)
	if len(st) != 1 || st[0].(map[string]any)["student_user_id"] != s2 {
		t.Fatalf("aging page %v", st)
	}
	_, env = api.do(http.MethodGet, "/admin/finance/aging?bucket=d61_90", nil)
	if st := dataOf(env)["students"].([]any); len(st) != 1 || st[0].(map[string]any)["oldest_bucket"] != "d61_90" {
		t.Fatalf("aging bucket filter %v", st)
	}
	// Trend: at end of August the August bill was 21 days overdue and unpaid.
	tr := dataOf(env)["trend"].([]any)
	if len(tr) != 12 {
		t.Fatalf("trend len %d", len(tr))
	}
	for _, x := range tr {
		p := x.(map[string]any)
		if p["month"].(float64) == 8 && p["total_idr"].(float64) != 1000000 {
			t.Fatalf("august trend %v", p)
		}
	}
	if code, _ := api.do(http.MethodGet, "/admin/finance/aging?as_of=14-10-2026", nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad as_of: %d", code)
	}

	// Arrears list carries pagination meta.
	_, env = api.do(http.MethodGet, "/admin/arrears?per_page=1", nil)
	if meta := env["meta"].(map[string]any); meta["total"].(float64) != 2 {
		t.Fatalf("arrears meta %v", meta)
	}
}
