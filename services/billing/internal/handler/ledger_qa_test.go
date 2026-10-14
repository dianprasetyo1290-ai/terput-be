package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestLedgerUnallocatedInvoice covers BUG-130 (receivable card with running
// and closing balance), BUG-132 (unallocated funds with age) and BUG-129
// (invoice PDF per bill).
func TestLedgerUnallocatedInvoice(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	app := New(db, config.Config{}, stubIdentity{}, nil)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Get("/admin/students/{id}/ledger", app.StudentLedger)
		pr.Get("/admin/finance/unallocated", app.UnallocatedFunds)
		pr.Get("/admin/bills/{id}/invoice.pdf", app.AdminBillInvoice)
	})
	school, student, fm := genUUID(), genUUID(), genUUID()
	api := apiCaller{t: t, r: r, token: signToken(t, fm, school, common.RoleFinanceManager)}

	bill := genUUID()
	scheme := genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day) VALUES ($1,$2,$3,'SPP','active',false,10)`,
		scheme, school, genUUID())
	mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month, base_idr, discount_idr, net_idr, status, due_at, created_at)
		VALUES ($1,$2,$3,$4,2026,7,1000000,200000,800000,'partial','2026-07-10','2026-07-01T08:00:00Z')`, bill, school, student, scheme)
	mustExec(t, db, `INSERT INTO bill_items (school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr) VALUES ($1,$2,'SPP (Juli)','SPP',1000000,200000,800000)`, school, bill)
	pay := genUUID()
	mustExec(t, db, `INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, status, idempotency_key, uploader_user_id, verified_at, source, source_id)
		VALUES ($1,$2,$3,500000,'verified',$4,$5,'2026-07-05T08:00:00Z','cashier',$6)`, pay, school, bill, "k-"+pay, fm, genUUID())
	mustExec(t, db, `INSERT INTO student_deposits (school_id, student_user_id, balance_idr) VALUES ($1,$2,150000)`, school, student)
	mustExec(t, db, `INSERT INTO deposit_movements (school_id, student_user_id, amount_idr, balance_after_idr, reason, ref, created_at)
		VALUES ($1,$2,200000,200000,'lebih bayar','x','2026-06-01T00:00:00Z'), ($1,$2,-50000,150000,'dipakai','y','2026-06-10T00:00:00Z')`, school, student)
	mustExec(t, db, `INSERT INTO bank_statement_lines (school_id, account, posted_on, amount_idr, description, line_hash, import_id)
		VALUES ($1,'BCA','2026-07-02',300000,'TRF TANPA KET',$2,$3)`, school, genUUID(), genUUID())

	code, env := api.do(http.MethodGet, "/admin/students/"+student+"/ledger", nil)
	d := dataOf(env)
	tot, _ := d["totals"].(map[string]any)
	entries, _ := d["entries"].([]any)
	if code != 200 || tot["closing_idr"] != float64(300000) || tot["closing_deposit_idr"] != float64(150000) ||
		tot["net_position_idr"] != float64(150000) || len(entries) != 4 {
		t.Fatalf("ledger: %d %v", code, env)
	}
	code, env = api.do(http.MethodGet, "/admin/students/"+student+"/ledger?from=2026-07-02", nil)
	tot, _ = dataOf(env)["totals"].(map[string]any)
	if code != 200 || tot["opening_idr"] != float64(800000) || tot["opening_deposit_idr"] != float64(150000) {
		t.Fatalf("ledger from: %d %v", code, env)
	}
	if code, _ := api.do(http.MethodGet, "/admin/students/"+student+"/ledger?from=2026-08-01&to=2026-07-01", nil); code != 422 {
		t.Fatalf("reversed range: %d", code)
	}

	code, env = api.do(http.MethodGet, "/admin/finance/unallocated", nil)
	d = dataOf(env)
	rows, _ := d["rows"].([]any)
	sum, _ := d["summary"].(map[string]any)
	if code != 200 || len(rows) != 2 || sum["deposit_idr"] != float64(150000) || sum["bank_unmatched_idr"] != float64(300000) {
		t.Fatalf("unallocated: %d %v", code, env)
	}
	first, _ := rows[0].(map[string]any)
	if first["kind"] != "deposit" || first["amount_idr"] != float64(150000) || first["age_days"].(float64) < 100 {
		t.Fatalf("oldest first: %v", first)
	}
	if code, _ := api.do(http.MethodGet, "/admin/finance/unallocated?kind=x", nil); code != 422 {
		t.Fatalf("bad kind: %d", code)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/bills/"+bill+"/invoice.pdf", nil)
	req.Header.Set("Authorization", "Bearer "+api.token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	body := rec.Body.Bytes()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(body, []byte("%PDF-1.4")) ||
		!bytes.Contains(body, []byte("Sisa yang harus dibayar: Rp")) || !strings.Contains(rec.Header().Get("Content-Disposition"), "INV-202607-") {
		t.Fatalf("invoice: %d %s", rec.Code, rec.Header())
	}
	req = httptest.NewRequest(http.MethodGet, "/admin/bills/"+genUUID()+"/invoice.pdf", nil)
	req.Header.Set("Authorization", "Bearer "+api.token)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("missing invoice: %d", rec.Code)
	}
}

func TestBuildTextPDFPaging(t *testing.T) {
	cases := []struct {
		lines, pages int
	}{{1, 1}, {40, 1}, {120, 3}}
	for _, tc := range cases {
		ls := make([]pdfLine, tc.lines)
		for i := range ls {
			ls[i] = pdfLine{Text: "Baris (uji) é"}
		}
		pdf := string(buildTextPDF(ls))
		if got := strings.Count(pdf, "/Type /Page "); got != tc.pages {
			t.Errorf("%d lines: %d pages", tc.lines, got)
		}
		if !strings.Contains(pdf, `Baris \(uji\) ?`) || !strings.HasSuffix(pdf, "%%EOF\n") {
			t.Errorf("escaping/trailer: %q", pdf[len(pdf)-40:])
		}
	}
	if got := InvoiceNumber(2026, 7, "abcd1234-ffff"); got != "INV/202607/ABCD1234" {
		t.Errorf("invoice no %s", got)
	}
}
