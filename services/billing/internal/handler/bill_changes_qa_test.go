package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestBillTermsAnnualAndChanges covers FR-18-001 (monthly/annual/one-time
// components), BUG-126/052 (default installment plan per component, manual
// terms), BUG-128 (a year of bills at once) and BUG-053 (cancel/adjust via
// the Approval Center keeping the original value).
func TestBillTermsAnnualAndChanges(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	core := &fakeCore{}
	srv := httptest.NewServer(core.handler())
	defer srv.Close()

	schoolID, schemeID, year := genUUID(), genUUID(), genUUID()
	citra, dodi := genUUID(), genUUID()
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Core = client.New(srv.URL, "k")
	app.Notification = client.New(srv.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8080"
	app.Roster = enrollmentRoster{grades: map[string]string{citra: "7", dodi: "7"}}
	r := chi.NewRouter()
	r.Post("/internal/bills/change-requests/{id}/callback", app.BillChangeCallback)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Post("/admin/bills/generate", app.GenerateBills)
		pr.Post("/admin/bills/generate-annual", app.GenerateAnnualBills)
		pr.Get("/admin/bills/{id}", app.GetBill)
		pr.Put("/admin/bills/{id}/installments", app.SetBillInstallments)
		pr.Post("/admin/bills/{id}/change-requests", app.CreateBillChangeRequest)
		pr.Get("/admin/bill-change-requests", app.ListBillChangeRequests)
	})
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, grade_level, name, status, installment, due_day, effective_from, effective_to)
		VALUES ($1, $2, $3, '7', 'SMP 2026/2027', 'active', true, 10, '2026-07-01', '2027-06-30')`, schemeID, schoolID, year)
	for _, c := range []struct {
		code, kind string
		amount     int64
		terms      int
	}{{"SPP", "monthly", 500000, 1}, {"KEG", "annual", 300000, 1}, {"PGK", "one_time", 9000000, 3}} {
		mustExec(t, db, `INSERT INTO billing_scheme_components (school_id, scheme_id, code, name, kind, amount_idr, installment_terms, installment_interval_months)
			VALUES ($1, $2, $3, $3, $4, $5, $6, 1)`, schoolID, schemeID, c.code, c.kind, c.amount, c.terms)
		mustExec(t, db, `INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc, receivable_acc) VALUES ($1, $2, $2, '4-100', '1-300')`,
			schoolID, c.code)
	}
	maker, checker := genUUID(), genUUID()
	fm := apiCaller{t: t, r: r, token: signToken(t, maker, schoolID, common.RoleFinanceManager)}
	internal := apiCaller{t: t, r: r}
	billOf := func(student string, month int) (id string, net int64, status string, due time.Time) {
		_ = db.QueryRow(`SELECT id::text, net_idr, status, due_at FROM student_bills WHERE school_id=$1 AND student_user_id=$2 AND period_month=$3`,
			schoolID, student, month).Scan(&id, &net, &status, &due)
		return
	}

	// July: every component; the 9M one-time fee is split over 3 months.
	if code, env := fm.do(http.MethodPost, "/admin/bills/generate", map[string]any{"scheme_id": schemeID, "period_year": 2026, "period_month": 7,
		"student_user_ids": []string{citra}}); code != http.StatusOK || dataOf(env)["generated"].(float64) != 1 {
		t.Fatalf("generate july: %d %v", code, env)
	}
	jul, net, _, due := billOf(citra, 7)
	if net != 9800000 || !due.Equal(time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("july bill net=%d due=%v", net, due)
	}
	var terms []int64
	rows, _ := db.Query(`SELECT amount_idr FROM bill_installments WHERE bill_id=$1 ORDER BY seq`, jul)
	for rows.Next() {
		var v int64
		_ = rows.Scan(&v)
		terms = append(terms, v)
	}
	rows.Close()
	if len(terms) != 3 || terms[0] != 3800000 || terms[1] != 3000000 || terms[2] != 3000000 {
		t.Fatalf("july terms %v", terms)
	}
	// August: only the monthly component.
	fm.do(http.MethodPost, "/admin/bills/generate", map[string]any{"scheme_id": schemeID, "period_year": 2026, "period_month": 8, "student_user_ids": []string{citra}})
	aug, net, _, _ := billOf(citra, 8)
	if net != 500000 {
		t.Fatalf("august net %d", net)
	}

	// Annual run for a student activated in September: Sep..Nov at once.
	code, env := fm.do(http.MethodPost, "/admin/bills/generate-annual", map[string]any{"scheme_id": schemeID, "from": "2026-09", "to": "2026-11",
		"student_user_ids": []string{dodi}})
	if code != http.StatusOK || dataOf(env)["generated"].(float64) != 3 {
		t.Fatalf("annual: %d %v", code, env)
	}
	if _, net, _, _ := billOf(dodi, 9); net != 9800000 {
		t.Fatalf("dodi september %d", net)
	}
	if _, net, _, _ := billOf(dodi, 10); net != 500000 {
		t.Fatalf("dodi october %d", net)
	}
	if code, _ := fm.do(http.MethodPost, "/admin/bills/generate-annual", map[string]any{"scheme_id": schemeID, "from": "2026-01", "to": "2027-06",
		"student_user_ids": []string{dodi}}); code != http.StatusUnprocessableEntity {
		t.Fatalf("18-month annual run: %d", code)
	}

	// Paying the first term moves the due date to the second term.
	payID := genUUID()
	mustExec(t, db, `INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, reference, object_key, mime, size_bytes, sha256,
		status, idempotency_key, uploader_user_id, verified_at, source)
		VALUES ($1, $2, $3, 4000000, now(), 'T', 'k', 'image/png', 1, 'x', 'verified', $4, $5, now(), 'upload')`, payID, schoolID, jul, "bp-"+payID, genUUID())
	mustExec(t, db, `INSERT INTO bill_payment_allocations (school_id, payment_id, amount_idr) VALUES ($1, $2, 4000000)`, schoolID, payID)
	if err := recomputeBillStatusQ(context.Background(), db, schoolID, jul); err != nil {
		t.Fatal(err)
	}
	if _, _, status, due := billOf(citra, 7); status != "partial" || due.Month() != time.August {
		t.Fatalf("after first term: %s %v", status, due)
	}

	// Manual terms on the August bill.
	for _, tc := range []struct {
		body any
		want int
	}{
		{map[string]any{"installments": []map[string]any{{"due_date": "2026-08-10", "amount_idr": 200000}, {"due_date": "2026-09-10", "amount_idr": 200000}}}, 422},
		{map[string]any{"installments": []map[string]any{{"due_date": "2026-09-10", "amount_idr": 250000}, {"due_date": "2026-08-10", "amount_idr": 250000}}}, 422},
		{map[string]any{"installments": []map[string]any{{"due_date": "2026-08-10", "amount_idr": 250000}, {"due_date": "2026-09-10", "amount_idr": 250000}}}, 200},
	} {
		if code, env := fm.do(http.MethodPut, "/admin/bills/"+aug+"/installments", tc.body); code != tc.want {
			t.Fatalf("installments %v: %d %v", tc.body, code, env)
		}
	}

	// Cancel August: waits for approval, bill stays active.
	code, env = fm.do(http.MethodPost, "/admin/bills/"+aug+"/change-requests", map[string]any{"kind": "cancel", "reason": "Salah terbit"})
	if code != http.StatusAccepted {
		t.Fatalf("cancel request: %d %v", code, env)
	}
	cancelID := dataOf(env)["id"].(string)
	if appr := core.lastApproval(t); appr.RequestType != "bill_adjustment" || appr.MakerUserID != maker || *appr.AmountIDR != 500000 {
		t.Fatalf("approval %+v", appr)
	}
	if code, _ := fm.do(http.MethodPost, "/admin/bills/"+aug+"/change-requests", map[string]any{"kind": "adjust", "new_net_idr": 400000, "reason": "x"}); code != http.StatusConflict {
		t.Fatalf("second pending request: %d", code)
	}
	if _, _, status, _ := billOf(citra, 8); status != "unpaid" {
		t.Fatalf("bill changed before approval: %s", status)
	}
	cb := func(id, by, decision string) (int, map[string]any) {
		return internal.do(http.MethodPost, "/internal/bills/change-requests/"+id+"/callback",
			map[string]any{"approval_id": "appr-" + id, "decision": decision, "decided_by": by})
	}
	if code, _ := cb(cancelID, maker, "approved"); code != http.StatusUnprocessableEntity {
		t.Fatalf("self approval: %d", code)
	}
	if code, env := cb(cancelID, checker, "approved"); code != http.StatusOK || dataOf(env)["status"] != "applied" {
		t.Fatalf("approve cancel: %d %v", code, env)
	}
	if code, env := cb(cancelID, checker, "approved"); code != http.StatusOK || dataOf(env)["status"] != "applied" {
		t.Fatalf("replay: %d %v", code, env)
	}
	if _, net, status, _ := billOf(citra, 8); status != "void" || net != 500000 {
		t.Fatalf("cancelled bill must keep its value: %s %d", status, net)
	}
	var lineSum int64
	_ = db.QueryRow(`SELECT sum(amount_idr) FROM bill_items WHERE bill_id=$1`, aug).Scan(&lineSum)
	if lineSum != 0 {
		t.Fatalf("cancel lines must reverse the bill: %d", lineSum)
	}

	// Adjust July (paid 4M) down to 3.5M: 500k to the deposit, terms rescaled.
	code, env = fm.do(http.MethodPost, "/admin/bills/"+jul+"/change-requests", map[string]any{"kind": "adjust", "new_net_idr": 3500000, "reason": "Potongan khusus"})
	if code != http.StatusAccepted {
		t.Fatalf("adjust request: %d %v", code, env)
	}
	adjID := dataOf(env)["id"].(string)
	if code, env := cb(adjID, checker, "approved"); code != http.StatusOK || dataOf(env)["status"] != "applied" {
		t.Fatalf("approve adjust: %d %v", code, env)
	}
	var deposit, termSum int64
	_ = db.QueryRow(`SELECT balance_idr FROM student_deposits WHERE school_id=$1 AND student_user_id=$2`, schoolID, citra).Scan(&deposit)
	_ = db.QueryRow(`SELECT COALESCE(sum(amount_idr),0) FROM bill_installments WHERE bill_id=$1`, jul).Scan(&termSum)
	if _, net, status, _ := billOf(citra, 7); net != 3500000 || status != "paid" || deposit != 500000 || termSum != 3500000 {
		t.Fatalf("adjust: net=%d status=%s deposit=%d terms=%d", net, status, deposit, termSum)
	}

	// Rejection leaves the bill untouched.
	_, env = fm.do(http.MethodPost, "/admin/bills/"+jul+"/change-requests", map[string]any{"kind": "cancel", "reason": "uji tolak"})
	rejID := dataOf(env)["id"].(string)
	if code, env := cb(rejID, checker, "rejected"); code != http.StatusOK || dataOf(env)["status"] != "rejected" {
		t.Fatalf("reject: %d %v", code, env)
	}
	if _, _, status, _ := billOf(citra, 7); status != "paid" {
		t.Fatalf("rejected change touched the bill: %s", status)
	}

	// Detail shows terms, adjustments and requests; list is paginated.
	code, env = fm.do(http.MethodGet, "/admin/bills/"+jul, nil)
	d := dataOf(env)
	if code != http.StatusOK || len(d["installments"].([]any)) != 3 || len(d["adjustments"].([]any)) != 1 || len(d["change_requests"].([]any)) != 2 {
		t.Fatalf("detail: %d %v", code, d)
	}
	_, env = fm.do(http.MethodGet, "/admin/bill-change-requests?per_page=2", nil)
	if meta := env["meta"].(map[string]any); meta["total"].(float64) != 3 {
		t.Fatalf("list meta %v", meta)
	}

	// Journals: the cancelled August bill nets to zero receivable in the
	// bill-issued journal (original line + cancellation lines).
	mustExec(t, db, `INSERT INTO billing_journal_settings (school_id, unit_div_code, receivable_acc, cash_acc, bank_acc, discount_acc, scholarship_acc, refund_acc, deposit_acc)
	        VALUES ($1, 'SMP', '1-1300', '1-1100', '1-1200', '5-1100', '5-1200', '4-1900', '2-1500')`, schoolID)
	var sppNet int64
	_ = db.QueryRow(`SELECT COALESCE(sum(bi.amount_idr),0) FROM bill_items bi JOIN student_bills b ON b.id=bi.bill_id
		WHERE b.id=$1 AND `+journalBillLive, aug).Scan(&sppNet)
	draft, err := app.buildBillIssued(context.Background(), schoolID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || draft == nil || sppNet != 0 {
		t.Fatalf("bill issued journal: %v %v net=%d", draft, err, sppNet)
	}
	var debit, credit int64
	for _, l := range draft.lines {
		debit += l.DebitIDR
		credit += l.CreditIDR
	}
	if debit != credit || debit == 0 {
		t.Fatalf("unbalanced journal %d/%d", debit, credit)
	}
}
