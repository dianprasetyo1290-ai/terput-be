package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestImportPaymentHistory covers BUG-122: history from the previous
// aggregator settles bills as verified 'import' payments, all or nothing,
// idempotent on re-import, and is not journaled.
func TestImportPaymentHistory(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	schoolID, schemeID, year, citra, dodi := genUUID(), genUUID(), genUUID(), genUUID(), genUUID()
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.Roster = enrollmentRoster{grades: map[string]string{citra: "7", dodi: "7"}}
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Post("/admin/bills/generate", app.GenerateBills)
		pr.Post("/admin/payments/import", app.ImportPayments)
		pr.Get("/admin/payments/imports", app.ListPaymentImports)
	})
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, grade_level, name, status, installment, due_day, effective_from, effective_to)
		VALUES ($1, $2, $3, '7', 'SMP', 'active', false, 10, '2026-07-01', '2027-06-30')`, schemeID, schoolID, year)
	mustExec(t, db, `INSERT INTO billing_scheme_components (school_id, scheme_id, code, name, kind, amount_idr) VALUES ($1, $2, 'SPP', 'SPP', 'monthly', 500000)`,
		schoolID, schemeID)
	mustExec(t, db, `INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc, receivable_acc) VALUES ($1, 'SPP', 'SPP', '4-100', '1-300')`, schoolID)
	user := genUUID()
	token := signToken(t, user, schoolID, common.RoleFinanceManager)
	fm := apiCaller{t: t, r: r, token: token}
	for _, m := range []int{7, 8} {
		if code, env := fm.do(http.MethodPost, "/admin/bills/generate", map[string]any{"scheme_id": schemeID, "period_year": 2026,
			"period_month": m, "student_user_ids": []string{citra, dodi}}); code != http.StatusOK {
			t.Fatalf("generate: %d %v", code, env)
		}
	}
	upload := func(csv string, dry bool) (int, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("source_name", "Aggregator Lama")
		if dry {
			_ = mw.WriteField("dry_run", "true")
		}
		fw, _ := mw.CreateFormFile("file", "riwayat.csv")
		_, _ = fw.Write([]byte(csv))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/admin/payments/import", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var env map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return rec.Code, env
	}
	statusOf := func(student string, month int) string {
		var s string
		_ = db.QueryRow(`SELECT status FROM student_bills WHERE school_id=$1 AND student_user_id=$2 AND period_month=$3`, schoolID, student, month).Scan(&s)
		return s
	}
	hdr := "student_user_id,period,amount_idr,paid_at,reference\n"
	good := hdr + citra + ",2026-07,500000,2026-07-05,A1\n" + citra + ",2026-08,200000,2026-08-05,A2\n" + dodi + ",2026-07,500000,2026-07-06,A3\n"

	// Over-payment and unknown period reject the whole file.
	bad := good + dodi + ",2026-08,600000,2026-08-05,A4\n" + dodi + ",2026-09,1,2026-08-05,A5\n"
	code, env := upload(bad, false)
	if msg, _ := env["message"].(string); code != http.StatusUnprocessableEntity || !strings.Contains(msg, "baris 5") || !strings.Contains(msg, "baris 6") {
		t.Fatalf("bad file: %d %v", code, env)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM bill_payments WHERE school_id=$1`, schoolID).Scan(&n)
	if n != 0 {
		t.Fatalf("rejected file left %d payments", n)
	}
	if code, env := upload(good, true); code != http.StatusOK || dataOf(env)["imported"].(float64) != 3 {
		t.Fatalf("dry run: %d %v", code, env)
	}
	_ = db.QueryRow(`SELECT count(*) FROM bill_payments WHERE school_id=$1`, schoolID).Scan(&n)
	if n != 0 {
		t.Fatal("dry run saved payments")
	}
	code, env = upload(good, false)
	if code != http.StatusCreated || dataOf(env)["imported"].(float64) != 3 || dataOf(env)["total_idr"].(float64) != 1200000 {
		t.Fatalf("import: %d %v", code, env)
	}
	if statusOf(citra, 7) != "paid" || statusOf(citra, 8) != "partial" || statusOf(dodi, 7) != "paid" || statusOf(dodi, 8) != "unpaid" {
		t.Fatalf("statuses %s %s %s %s", statusOf(citra, 7), statusOf(citra, 8), statusOf(dodi, 7), statusOf(dodi, 8))
	}
	// Re-import: same lines skipped; a new line in the same file is added.
	code, env = upload(good+citra+",2026-08,300000,2026-08-20,A6\n", false)
	if code != http.StatusCreated || dataOf(env)["imported"].(float64) != 1 || dataOf(env)["skipped"].(float64) != 3 {
		t.Fatalf("re-import: %d %v", code, env)
	}
	if statusOf(citra, 8) != "paid" {
		t.Fatalf("citra aug %s", statusOf(citra, 8))
	}
	var src string
	_ = db.QueryRow(`SELECT string_agg(DISTINCT source, ',') FROM bill_payments WHERE school_id=$1`, schoolID).Scan(&src)
	if src != "import" {
		t.Fatalf("sources %q", src)
	}
	code, env = fm.do(http.MethodGet, "/admin/payments/imports", nil)
	if items, _ := env["data"].([]any); code != http.StatusOK || len(items) != 2 {
		t.Fatalf("imports list: %d %v", code, env)
	}
	var audits int
	_ = db.QueryRow(`SELECT count(*) FROM billing_audit_events WHERE school_id=$1 AND action='payment_history_imported'`, schoolID).Scan(&audits)
	if audits != 2 {
		t.Fatalf("audits %d", audits)
	}
}
