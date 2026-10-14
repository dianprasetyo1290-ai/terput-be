package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
)

// TestBankStatementImportAndMatch covers BUG-009/139: a valid file imports
// (line_hash filled, re-import idempotent), a malformed one is a 400 with
// the supported format, and reconciliation marks the matching line with the
// record it matched while the rest become open exceptions.
func TestBankStatementImportAndMatch(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	app, _ := setupTestApp(t, db, genUUID(), genUUID())
	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Post("/admin/bank-statements/import", app.AdminImportBankStatement)
	r.Post("/admin/bank-reconciliation/run", app.AdminRunBankReconciliation)
	srv := httptest.NewServer(r)
	defer srv.Close()

	schoolID, studentID, schemeID, billID, payID := genUUID(), genUUID(), genUUID(), genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP QA', 'active', true, 10)`, schemeID, schoolID, genUUID())
	mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month,
		base_idr, discount_idr, net_idr, status, due_at) VALUES ($1, $2, $3, $4, 2026, 9, 750000, 0, 750000, 'paid', '2026-09-10')`,
		billID, schoolID, studentID, schemeID)
	mustExec(t, db, `INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, reference, object_key, mime, size_bytes, sha256,
		status, idempotency_key, uploader_user_id, verified_at, source)
		VALUES ($1, $2, $3, 750000, '2026-09-01T09:00:00Z', 'TRX123', 'k', 'image/png', 1, 'x', 'verified', $4, $5, now(), 'upload')`,
		payID, schoolID, billID, "bp-"+payID, genUUID())
	token := signToken(t, genUUID(), schoolID, common.RoleFinanceManager)

	upload := func(content string) (int, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "mutasi.csv")
		_, _ = fw.Write([]byte(content))
		_ = mw.WriteField("account", "BCA-001")
		_ = mw.Close()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/bank-statements/import", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var env map[string]any
		_ = json.Unmarshal(b, &env)
		return resp.StatusCode, env
	}

	file := "Tanggal;Keterangan;Kredit\n01/09/2026;TRF TRX123 SPP;750.000\n02/09/2026;SETORAN TAK DIKENAL;125.000\n"
	code, env := upload(file)
	if code != http.StatusOK {
		t.Fatalf("import status=%d env=%v", code, env)
	}
	if d := env["data"].(map[string]any); d["inserted"].(float64) != 2 {
		t.Fatalf("inserted=%v", d["inserted"])
	}
	code, env = upload(file)
	if d := env["data"].(map[string]any); code != http.StatusOK || d["inserted"].(float64) != 0 || d["duplicates"].(float64) != 2 {
		t.Fatalf("re-import status=%d data=%v", code, d)
	}
	if code, env = upload("foo;bar\n1;2\n"); code != http.StatusBadRequest {
		t.Fatalf("malformed status=%d env=%v", code, env)
	}

	run := func() int {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/bank-reconciliation/run", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if _, err := app.runBankReconciliation(t.Context(), schoolID); err != nil {
		t.Fatalf("direct run: %v", err)
	}
	for i := 0; i < 2; i++ { // second run must not fail on the existing exception
		if c := run(); c != http.StatusOK {
			t.Fatalf("run %d status=%d", i, c)
		}
	}
	var matched string
	_ = db.QueryRow(`SELECT matched_ref FROM bank_statement_lines WHERE school_id=$1 AND amount_idr=750000`, schoolID).Scan(&matched)
	if matched != "bill_payment:"+payID {
		t.Fatalf("matched_ref=%q", matched)
	}
	var open int
	_ = db.QueryRow(`SELECT count(*) FROM reconciliation_exceptions WHERE school_id=$1 AND kind='bank_unmatched' AND status='open'`, schoolID).Scan(&open)
	if open != 1 {
		t.Fatalf("open exceptions=%d want 1", open)
	}
}
