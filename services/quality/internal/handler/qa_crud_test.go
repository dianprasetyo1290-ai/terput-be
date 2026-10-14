package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"skci/quality/internal/config"
	"skci/quality/internal/govreport"
	"skci/quality/internal/qa"
	"skci/quality/internal/storage"
)

// TestQAEndpointsE2E exercises the endpoints the web calls (instrument tree
// CRUD, new version, evidence download/delete, EDS export, accreditation
// report, report-template PUT/DELETE, action-plan ownership) and the role
// gates of T23 §5 against a real database.
// QUALITY_TEST_DATABASE_URL must point to an empty database.
func TestQAEndpointsE2E(t *testing.T) {
	dsn := os.Getenv("QUALITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("QUALITY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_name = 'qa_instruments'`).Scan(&n)
	if n == 0 {
		files, _ := filepath.Glob("../../migrations/*.up.sql")
		sort.Strings(files)
		for _, f := range files {
			sql, _ := os.ReadFile(f)
			if _, err := pool.Exec(ctx, string(sql)); err != nil {
				t.Fatalf("migrate %s: %v", f, err)
			}
		}
	}

	if _, err := pool.Exec(ctx, `TRUNCATE qa_instruments, qa_evidence, regulatory_templates, audit_events CASCADE`); err != nil {
		t.Fatal(err)
	}
	st, _ := storage.NewLocalStorage(t.TempDir())
	qs := qa.NewStore(pool)
	gs := govreport.NewStore(pool)
	cfg := &config.Config{JWTSecret: "s", InternalKey: "k"}
	app := NewApp(cfg, qs, gs, qa.NewHandler(qs, nil, st), govreport.NewHandler(gs, nil, st, "h", 1000), nil, st)
	router := app.Router()

	school := "11111111-1111-1111-1111-111111111111"
	coord := "22222222-2222-2222-2222-222222222222"
	guru := "33333333-3333-3333-3333-333333333333"
	tok := func(user, role string) string { return createTestToken("s", school, user, role) }
	coordTok, kepsekTok, guruTok, adminTok := tok(coord, "qa_coordinator"), tok(coord, "kepala_sekolah"), tok(guru, "guru"), tok(coord, "admin")

	do := func(method, path, token string, body any) (int, map[string]any) {
		var rdr io.Reader
		ct := ""
		switch b := body.(type) {
		case nil:
		case *bytes.Buffer:
			rdr = b
		default:
			raw, _ := json.Marshal(b)
			rdr = bytes.NewReader(raw)
			ct = "application/json"
		}
		req := httptest.NewRequest(method, path, rdr)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var env map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return rec.Code, env
	}
	data := func(env map[string]any) map[string]any {
		m, _ := env["data"].(map[string]any)
		return m
	}
	want := func(step string, got, exp int, env map[string]any) {
		t.Helper()
		if got != exp {
			t.Fatalf("%s: status %d, want %d (%v)", step, got, exp, env)
		}
	}

	// Role gates.
	c, e := do("GET", "/qa/instruments", guruTok, nil)
	want("guru read instruments", c, 403, e)
	c, e = do("POST", "/qa/instruments", kepsekTok, map[string]any{"code": "X", "name": "X", "issuer": "BAN", "version_label": "v1", "score_min": 1, "score_max": 4})
	want("kepsek create instrument", c, 403, e)
	c, e = do("GET", "/qa/instruments", kepsekTok, nil)
	want("kepsek read instruments", c, 200, e)
	c, e = do("GET", "/qa/reports/comparison?instrument_id=x", coordTok, nil)
	want("coordinator comparison", c, 403, e)
	c, e = do("GET", "/gov/dapodik/logs", kepsekTok, nil)
	want("kepsek dapodik", c, 403, e)

	// Instrument tree.
	c, e = do("POST", "/qa/instruments", coordTok, map[string]any{"code": "BAN", "name": "IASP", "issuer": "BAN-PDM", "version_label": "2024", "score_min": 1, "score_max": 4, "status": "active"})
	want("create instrument", c, 201, e)
	inst := data(e)["id"].(string)
	base := "/qa/instruments/" + inst
	c, e = do("POST", base+"/standards", coordTok, map[string]any{"code": "S1", "name": "Mutu Lulusan", "weight_bp": 10000})
	want("create standard", c, 201, e)
	std := data(e)["id"].(string)
	c, e = do("PUT", base+"/standards/"+std, coordTok, map[string]any{"code": "S1", "name": "Mutu Lulusan (rev)", "weight_bp": 10000})
	want("update standard", c, 200, e)
	c, e = do("POST", base+"/items", coordTok, map[string]any{"standard_id": std, "code": "B1", "statement": "Lulusan berkarakter"})
	want("create item", c, 201, e)
	item := data(e)["id"].(string)
	c, e = do("PUT", base+"/items/"+item, coordTok, map[string]any{"code": "B1", "statement": "Lulusan berkarakter Pancasila"})
	want("update item", c, 200, e)
	c, e = do("POST", base+"/indicators", coordTok, map[string]any{"item_id": item, "code": "I1", "name": "Kehadiran", "weight_bp": 6000, "unit": "percent_bp", "direction": "higher_better", "source_kind": "manual", "rubric": []map[string]int{{"min_value": 0, "score": 1}, {"min_value": 9000, "score": 4}}})
	want("create indicator", c, 201, e)
	ind := data(e)["id"].(string)
	c, e = do("POST", base+"/indicators", coordTok, map[string]any{"item_id": item, "code": "I2", "name": "Prestasi", "weight_bp": 4000, "unit": "count", "direction": "higher_better", "source_kind": "manual"})
	want("create indicator 2", c, 201, e)
	ind2 := data(e)["id"].(string)
	c, e = do("POST", base+"/indicators", coordTok, map[string]any{"item_id": item, "code": "I3", "name": "x", "weight_bp": 1, "unit": "bogus", "direction": "higher_better", "source_kind": "manual"})
	want("invalid indicator unit", c, 400, e)
	c, e = do("PUT", base+"/indicators/"+ind2, coordTok, map[string]any{"code": "I2", "name": "Prestasi lomba", "weight_bp": 4000, "unit": "count", "direction": "higher_better", "source_kind": "manual"})
	want("update indicator", c, 200, e)

	// A node of another instrument is a 404.
	c, e = do("POST", "/qa/instruments", coordTok, map[string]any{"code": "OTHER", "name": "Lain", "issuer": "x", "version_label": "1", "score_min": 1, "score_max": 4})
	want("other instrument", c, 201, e)
	other := data(e)["id"].(string)
	c, e = do("DELETE", "/qa/instruments/"+other+"/indicators/"+ind2, coordTok, nil)
	want("cross-instrument delete", c, 404, e)

	// New version copies the tree.
	c, e = do("POST", base+"/new-version", coordTok, map[string]any{"version_label": "2025"})
	want("new version", c, 201, e)
	nv := data(e)
	stds, _ := nv["standards"].([]any)
	if len(stds) != 1 || nv["status"] != "draft" {
		t.Fatalf("new version tree: %v", nv)
	}
	c, e = do("POST", base+"/new-version", coordTok, map[string]any{"version_label": "2025"})
	want("duplicate version", c, 409, e)

	// Evidence: upload, download, owner-only delete.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("title", "Rapor")
	_ = mw.WriteField("indicator_ids", ind)
	_ = mw.WriteField("academic_year_id", "44444444-4444-4444-4444-444444444444")
	fw, _ := mw.CreateFormFile("file", "../../etc/rapor.pdf")
	_, _ = fw.Write([]byte("%PDF-1.4 bukti"))
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/qa/evidence", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+guruTok)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("upload evidence: %d %s", rec.Code, rec.Body.String())
	}
	var up map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &up)
	ev := data(up)["id"].(string)
	if key, _ := data(up)["object_key"].(string); strings.Contains(key, "..") {
		t.Fatalf("object key not sanitized: %s", key)
	}
	req = httptest.NewRequest("GET", "/qa/evidence/"+ev+"/download", nil)
	req.Header.Set("Authorization", "Bearer "+coordTok)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.String() != "%PDF-1.4 bukti" || !strings.Contains(rec.Header().Get("Content-Disposition"), "rapor.pdf") {
		t.Fatalf("download: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	c, e = do("DELETE", "/qa/evidence/"+ev, tok("55555555-5555-5555-5555-555555555555", "guru"), nil)
	want("other guru delete evidence", c, 403, e)
	c, e = do("DELETE", "/qa/evidence/"+ev, guruTok, nil)
	want("uploader delete evidence", c, 200, e)

	// Self evaluation, scoring, export, report.
	c, e = do("POST", "/qa/self-evaluations", kepsekTok, map[string]any{"instrument_id": inst, "academic_year_id": "44444444-4444-4444-4444-444444444444", "period_label": "2026 Ganjil", "period_from": "2026-07-01", "period_to": "2026-12-31"})
	want("create EDS", c, 201, e)
	eds := data(e)["id"].(string)
	c, e = do("PUT", "/qa/self-evaluations/"+eds+"/items/"+ind, coordTok, map[string]any{"actual_value": 9500, "score": 4})
	want("score I1", c, 200, e)
	c, e = do("PUT", "/qa/self-evaluations/"+eds+"/items/"+ind2, coordTok, map[string]any{"actual_value": 2, "score": 2})
	want("score I2", c, 200, e)
	c, e = do("POST", "/qa/self-evaluations/"+eds+"/refresh-metrics", coordTok, nil)
	want("refresh metrics", c, 200, e)
	c, e = do("GET", "/qa/reports/accreditation?instrument_id="+inst, kepsekTok, nil)
	want("accreditation report", c, 200, e)
	score := data(e)["score"].(map[string]any)
	// (4*6000 + 2*4000)/10000 = 3.2 → 3200 milli.
	if score["total_score_milli"] != float64(3200) {
		t.Fatalf("accreditation score: %v", score)
	}
	req = httptest.NewRequest("GET", "/qa/reports/self-evaluation/"+eds+"?format=xlsx", nil)
	req.Header.Set("Authorization", "Bearer "+kepsekTok)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 || !bytes.HasPrefix(rec.Body.Bytes(), []byte("PK")) {
		t.Fatalf("xlsx export: %d", rec.Code)
	}

	// Structural edits are blocked once the EDS left draft.
	if _, err := pool.Exec(ctx, `UPDATE qa_self_evaluations SET status = 'finalized' WHERE id = $1`, eds); err != nil {
		t.Fatal(err)
	}
	c, e = do("DELETE", base+"/indicators/"+ind2, coordTok, nil)
	want("edit used instrument", c, 409, e)

	// Action plans: owner-only updates.
	var seItem string
	_ = pool.QueryRow(ctx, `SELECT id FROM qa_self_eval_items WHERE self_evaluation_id = $1 LIMIT 1`, eds).Scan(&seItem)
	c, e = do("POST", "/qa/action-plans", coordTok, map[string]any{"self_eval_item_id": seItem, "action": "Remedial", "owner_user_id": guru, "due_date": "2026-11-30"})
	want("create RTL", c, 201, e)
	ap := data(e)["id"].(string)
	c, e = do("POST", "/qa/action-plans/"+ap+"/status", tok("55555555-5555-5555-5555-555555555555", "guru"), map[string]any{"status": "done"})
	want("non-owner status", c, 403, e)
	c, e = do("PUT", "/qa/action-plans/"+ap, guruTok, map[string]any{"action": "Remedial + tutor sebaya"})
	want("owner update RTL", c, 200, e)
	c, e = do("DELETE", "/qa/action-plans/"+ap, guruTok, nil)
	want("owner delete RTL", c, 403, e)
	c, e = do("DELETE", "/qa/action-plans/"+ap, coordTok, nil)
	want("coordinator delete RTL", c, 200, e)

	// Report templates.
	c, e = do("POST", "/gov/report-templates", adminTok, map[string]any{"code": "LPJ", "name": "Laporan Bulanan", "recipient": "Disdik", "frequency": "monthly", "columns": []map[string]string{{"label": "Siswa", "metric_key": "students.active"}}})
	want("create template", c, 201, e)
	tpl := data(e)["id"].(string)
	c, e = do("GET", "/gov/report-templates/"+tpl, kepsekTok, nil)
	want("get template", c, 200, e)
	c, e = do("PUT", "/gov/report-templates/"+tpl, adminTok, map[string]any{"code": "LPJ", "name": "Laporan Bulanan Dinas", "recipient": "Disdik", "frequency": "monthly", "export_format": "csv", "columns": []any{}})
	want("update template", c, 200, e)
	if data(e)["name"] != "Laporan Bulanan Dinas" {
		t.Fatalf("template not updated: %v", e)
	}
	c, e = do("PUT", "/gov/report-templates/"+tpl, kepsekTok, map[string]any{"code": "LPJ", "name": "x", "recipient": "x", "frequency": "monthly"})
	want("kepsek update template", c, 403, e)
	c, e = do("DELETE", "/gov/report-templates/"+tpl, adminTok, nil)
	want("delete template", c, 200, e)
	c, e = do("GET", "/gov/report-templates/"+tpl, adminTok, nil)
	want("deleted template", c, 404, e)

	// Superadmin without school scope is refused, not run unscoped.
	c, e = do("GET", "/qa/instruments", createTestToken("s", "", coord, "superadmin"), nil)
	want("superadmin without school_id", c, 400, e)
}
