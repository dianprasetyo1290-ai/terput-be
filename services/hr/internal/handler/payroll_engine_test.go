package handler_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"skci/hr/internal/common"
)

func doJSON(t *testing.T, h http.Handler, method, token, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func dataOf(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode %s: %v", rr.Body.String(), err)
	}
	return env.Data
}

// Payroll end to end on the real schema: no mock salary, real BPJS/TER from
// the statutory tables, BR-PRL-04, duplicate regular run, final run
// immutability, payslip PDF whose hash matches finalisation, owner-only ESS
// PDF, accounting journal pending → retry posted, tax reports and
// reconciliation.
func TestPayrollEngineE2E(t *testing.T) {
	app := setupTestApp(t)
	ctx := context.Background()

	var posted atomic.Int32
	acct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		var d, c float64
		for _, l := range body["lines"].([]any) {
			m := l.(map[string]any)
			d += m["debit_idr"].(float64)
			c += m["credit_idr"].(float64)
		}
		if body["source"] != "payroll" || d != c || r.Header.Get("X-Internal-Key") != testIntKey {
			w.WriteHeader(422)
			_, _ = w.Write([]byte(`{"status":422,"message":"unbalanced"}`))
			return
		}
		posted.Add(1)
		_, _ = w.Write([]byte(`{"status":200,"trans_no":"PRL26-0001"}`))
	}))
	defer acct.Close()

	for _, q := range []string{
		`DELETE FROM payroll_inputs WHERE school_id = $1`,
		`DELETE FROM employee_components WHERE school_id = $1`,
		`DELETE FROM pay_component_accounts WHERE school_id = $1`,
		`DELETE FROM payroll_input_changes WHERE school_id = $1`,
		`DELETE FROM pay_components WHERE school_id = $1`,
		`DELETE FROM tax_remittances WHERE school_id = $1`,
		`DELETE FROM payroll_accounts WHERE school_id = $1`,
	} {
		if _, err := testDB.ExecContext(ctx, q, schoolID); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	const empUser = "20000000-0000-0000-0000-0000000000e1"
	var posID, empID, otherEmp string
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(testDB.QueryRowContext(ctx, `INSERT INTO positions (school_id, code, name) VALUES ($1, 'PAY-T', 'Guru') RETURNING id`, schoolID).Scan(&posID))
	must(testDB.QueryRowContext(ctx, `INSERT INTO employees (school_id, user_id, employee_no, full_name, ptkp_status, hire_date, status)
		VALUES ($1, $2, 'P-001', 'Ani Payroll', 'TK/0', '2025-01-01', 'active') RETURNING id`, schoolID, empUser).Scan(&empID))
	must(testDB.QueryRowContext(ctx, `INSERT INTO employees (school_id, employee_no, full_name, ptkp_status, hire_date, status)
		VALUES ($1, 'P-002', 'Budi Tanpa Gaji', 'TK/0', '2025-01-01', 'active') RETURNING id`, schoolID).Scan(&otherEmp))
	enc, err := app.Crypto.EncryptAmount(10_000_000, schoolID)
	must(err)
	_, err = testDB.ExecContext(ctx, `INSERT INTO employment_assignments (school_id, employee_id, position_id, employment_type, base_salary_enc, effective_from)
		VALUES ($1, $2, $3, 'tetap', $4, '2025-01-01')`, schoolID, empID, posID, enc)
	must(err)

	router := app.Router()
	po := createToken("20000000-0000-0000-0000-0000000000a1", common.RolePayroll)

	// Component without an account is refused (BR-PRL-04).
	rr := doJSON(t, router, http.MethodPost, po, "/payroll/components", map[string]any{
		"code": "TUNJ", "name": "Tunjangan Jabatan", "kind": "earning", "nature": "fixed", "taxable": true,
		"calc": "fixed_amount", "default_amount_idr": 500000,
	})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("component without account = %d %s", rr.Code, rr.Body.String())
	}
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/components", map[string]any{
		"code": "TUNJ", "name": "Tunjangan Jabatan", "kind": "earning", "nature": "fixed", "taxable": true,
		"calc": "fixed_amount", "default_amount_idr": 500000, "acc_code": "5103",
	})
	if rr.Code != http.StatusCreated || dataOf(t, rr)["status"] != "draft" {
		t.Fatalf("create component = %d %s", rr.Code, rr.Body.String())
	}
	_, err = testDB.ExecContext(ctx, `UPDATE pay_components SET status = 'active' WHERE school_id = $1 AND code = 'TUNJ'`, schoolID)
	must(err)

	// An active component with no account blocks a run, listing it.
	_, err = testDB.ExecContext(ctx, `INSERT INTO pay_components (school_id, code, name, kind, nature, calc, status) VALUES ($1, 'NOACC', 'Tanpa Akun', 'earning', 'variable', 'from_input', 'active')`, schoolID)
	must(err)
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/runs", map[string]any{"period": "2026-03", "kind": "regular"})
	if rr.Code != http.StatusUnprocessableEntity || !strings.Contains(rr.Body.String(), "NOACC") {
		t.Fatalf("run with unmapped component = %d %s", rr.Code, rr.Body.String())
	}
	_, err = testDB.ExecContext(ctx, `DELETE FROM pay_components WHERE school_id = $1 AND code = 'NOACC'`, schoolID)
	must(err)

	if rr = doJSON(t, router, http.MethodPost, po, "/payroll/runs", map[string]any{"period": "2026-3"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad period = %d", rr.Code)
	}
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/runs", map[string]any{"period": "2026-03", "kind": "regular"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create run = %d %s", rr.Code, rr.Body.String())
	}
	runID := dataOf(t, rr)["id"].(string)
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/runs", map[string]any{"period": "2026-03", "kind": "regular"})
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "run susulan") {
		t.Fatalf("duplicate regular = %d %s", rr.Code, rr.Body.String())
	}

	rr = doJSON(t, router, http.MethodPost, po, "/payroll/runs/"+runID+"/calculate", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("calculate = %d %s", rr.Code, rr.Body.String())
	}
	totals := dataOf(t, rr)["totals"].(map[string]any)
	if totals["lines"].(float64) != 1 || len(totals["skipped"].([]any)) != 1 {
		t.Fatalf("employee without salary must be skipped, not paid: %v", totals)
	}

	var gross, taxable, bpjsEmp, pph, net int64
	var details []byte
	must(testDB.QueryRowContext(ctx, `SELECT gross_idr, taxable_idr, bpjs_employee_idr, pph21_idr, net_idr, details FROM payroll_lines WHERE run_id = $1 AND employee_id = $2`,
		runID, empID).Scan(&gross, &taxable, &bpjsEmp, &pph, &net, &details))
	// 10jt base + 500rb allowance; BPJS on 10jt: kes 1% 100.000, JHT 2%
	// 200.000, JP 1% 100.000. PPh gross adds employer kes 400.000, JKK
	// 0,24% 24.000, JKM 0,30% 30.000 = 10.954.000 → TER A 3%.
	if gross != 10_500_000 || bpjsEmp != 400_000 || taxable != 10_954_000 {
		t.Fatalf("gross=%d bpjs=%d taxable=%d", gross, bpjsEmp, taxable)
	}
	if pph != 10_954_000*300/10000 || net != gross-bpjsEmp-pph {
		t.Fatalf("pph=%d net=%d details=%s", pph, net, details)
	}

	// Approve (core callback) and finalize with accounting not configured.
	_, err = testDB.ExecContext(ctx, `UPDATE payroll_runs SET status = 'approved' WHERE id = $1`, runID)
	must(err)
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/runs/"+runID+"/finalize", nil)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "belum dikonfigurasi") {
		t.Fatalf("finalize = %d %s", rr.Code, rr.Body.String())
	}
	journal := dataOf(t, rr)["journal"].(map[string]any)
	if journal["status"] != "pending" {
		t.Fatalf("journal without ACCOUNTING_URL must stay pending: %v", journal)
	}
	if rr = doJSON(t, router, http.MethodPost, po, "/payroll/runs/"+runID+"/calculate", nil); rr.Code != http.StatusConflict {
		t.Fatalf("recalculate final run = %d", rr.Code)
	}
	if _, err := testDB.ExecContext(ctx, `UPDATE payroll_lines SET net_idr = 1 WHERE run_id = $1`, runID); err == nil {
		t.Fatal("final payroll lines must be immutable")
	}

	// Payslip PDF re-renders to the hash stored at finalisation.
	var slipID, stored string
	must(testDB.QueryRowContext(ctx, `SELECT ps.id, ps.sha256 FROM payslips ps JOIN payroll_lines l ON l.id = ps.line_id WHERE l.run_id = $1`, runID).Scan(&slipID, &stored))
	rr = doGet(t, router, po, "/payroll/payslips/"+slipID+"/pdf")
	sum := sha256.Sum256(rr.Body.Bytes())
	if rr.Code != http.StatusOK || !bytes.HasPrefix(rr.Body.Bytes(), []byte("%PDF-1.4")) || hex.EncodeToString(sum[:]) != stored {
		t.Fatalf("payslip pdf = %d hash mismatch", rr.Code)
	}

	// ESS: owner sees and downloads; someone else gets 403.
	owner := createToken(empUser, common.RolePegawai)
	rr = doGet(t, router, owner, "/me/payslips")
	var list struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &list)
	if rr.Code != http.StatusOK || len(list.Data) != 1 {
		t.Fatalf("/me/payslips = %d %s", rr.Code, rr.Body.String())
	}
	if rr = doGet(t, router, owner, "/me/payslips/"+slipID+"/pdf"); rr.Code != http.StatusOK {
		t.Fatalf("owner pdf = %d", rr.Code)
	}
	_, err = testDB.ExecContext(ctx, `UPDATE employees SET user_id = '20000000-0000-0000-0000-0000000000e2' WHERE id = $1`, otherEmp)
	must(err)
	if rr = doGet(t, router, createToken("20000000-0000-0000-0000-0000000000e2", common.RolePegawai), "/me/payslips/"+slipID+"/pdf"); rr.Code != http.StatusForbidden {
		t.Fatalf("other employee pdf = %d, want 403", rr.Code)
	}

	// Configure accounting; retry posts the balanced journal once.
	app.Config.AccountingURL = acct.URL
	router = app.Router()
	jid := journal["id"].(string)
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/journals/"+jid+"/retry", nil)
	if rr.Code != http.StatusOK || posted.Load() != 1 {
		t.Fatalf("retry = %d %s", rr.Code, rr.Body.String())
	}
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/journals/"+jid+"/retry", nil)
	if rr.Code != http.StatusOK || posted.Load() != 1 {
		t.Fatalf("retry of posted journal must not repost: %d posted=%d", rr.Code, posted.Load())
	}

	// Reports come from the final lines; reconciliation against remittances.
	fm := createToken("20000000-0000-0000-0000-0000000000f1", common.RoleFinanceManager)
	rr = doGet(t, router, fm, "/payroll/reports/pph21?period=2026-03")
	if rr.Code != http.StatusOK || int64(dataOf(t, rr)["pph21_idr"].(float64)) != pph {
		t.Fatalf("pph21 report = %d %s", rr.Code, rr.Body.String())
	}
	if rr = doGet(t, router, fm, "/payroll/reports/bpjs?period=2026-03"); rr.Code != http.StatusOK {
		t.Fatalf("bpjs report = %d", rr.Code)
	}
	if rr = doGet(t, router, fm, "/payroll/reports/pph21/annual?year=2026"); rr.Code != http.StatusOK {
		t.Fatalf("annual report = %d", rr.Code)
	}
	rr = doJSON(t, router, http.MethodPost, fm, "/payroll/remittances", map[string]any{
		"period": "2026-03", "kind": "pph21", "amount_idr": pph, "paid_on": "2026-04-10", "ref": "NTPN-1",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("remittance = %d %s", rr.Code, rr.Body.String())
	}
	rr = doGet(t, router, fm, "/payroll/reports/reconciliation?period=2026-03")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"kind":"pph21","withheld_idr":`) || !strings.Contains(rr.Body.String(), `"status":"match"`) {
		t.Fatalf("reconciliation = %d %s", rr.Code, rr.Body.String())
	}
	rr = doGet(t, router, fm, "/payroll/statutory")
	if rr.Code != http.StatusOK || len(dataOf(t, rr)["missing"].([]any)) != 0 {
		t.Fatalf("statutory = %d %s", rr.Code, rr.Body.String())
	}

	// Pull attendance before HR sent the recap → 409.
	rr = doJSON(t, router, http.MethodPost, po, "/payroll/inputs/pull-attendance", map[string]any{"period": "2026-04"})
	if rr.Code != http.StatusConflict {
		t.Fatalf("pull attendance unsent = %d %s", rr.Code, rr.Body.String())
	}
}
