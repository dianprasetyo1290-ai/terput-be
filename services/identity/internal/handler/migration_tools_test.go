package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/client"
	"skci/identity/internal/common"
)

func TestUnexplainedDifferences(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	s := func(v string) *string { return &v }
	cases := []struct {
		name string
		in   []migrationTotal
		want int
	}{
		{"match", []migrationTotal{{SourceValue: f(10), TargetValue: f(10)}}, 0},
		{"no source yet", []migrationTotal{{TargetValue: f(10)}}, 1},
		{"diff without note", []migrationTotal{{SourceValue: f(10), TargetValue: f(9), Diff: -1}}, 1},
		{"diff short note", []migrationTotal{{SourceValue: f(10), TargetValue: f(9), Diff: -1, Explanation: s("dobel")}}, 1},
		{"diff explained", []migrationTotal{{SourceValue: f(10), TargetValue: f(9), Diff: -1, Explanation: s("1 siswa ganda di sistem lama")}}, 0},
	}
	for _, c := range cases {
		if got := unexplainedDifferences(c.in); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}

func TestValidateTotalInput(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		in   totalInput
		want string
	}{
		{totalInput{Category: "siswa", SourceValue: f(10)}, ""},
		{totalInput{Category: "Tagihan_2024", Metric: "sum_amount", Label: "Tagihan", SourceValue: f(10.5)}, ""},
		{totalInput{Category: "x"}, "Kategori"},
		{totalInput{Category: "custom"}, "Label kategori wajib"},
		{totalInput{Category: "siswa", Metric: "avg"}, "Metrik"},
		{totalInput{Category: "siswa", SourceValue: f(-1)}, "negatif"},
		{totalInput{Category: "siswa", SourceValue: f(1.5)}, "bulat"},
		{totalInput{Category: "custom", Remove: true}, ""},
	}
	for i, c := range cases {
		got := validateTotalInput(&c.in)
		if (c.want == "" && got != "") || (c.want != "" && !strings.Contains(got, c.want)) {
			t.Errorf("case %d: got %q want %q", i, got, c.want)
		}
	}
}

func TestArchiveRows(t *testing.T) {
	cases := []struct {
		name     string
		table    [][]string
		titleCol string
		rows     int
		title    string
		err      string
	}{
		{"empty", [][]string{{"A"}}, "", 0, "", "tidak berisi data"},
		{"title from Nama", [][]string{{"NIS", "Nama", "ID Sumber"}, {"1", "Ani", "S1"}, {"", "", ""}}, "", 1, "Ani", ""},
		{"explicit title column", [][]string{{"NIS", "Nama"}, {"1", "Ani"}}, "NIS", 1, "1", ""},
		{"unknown title column", [][]string{{"NIS"}, {"1"}}, "Judul", 0, "", "tidak ditemukan"},
		{"duplicate header", [][]string{{"A", "a"}, {"1", "2"}}, "", 0, "", "lebih dari sekali"},
	}
	for _, c := range cases {
		rows, msg := archiveRows(c.table, c.titleCol)
		if c.err != "" {
			if !strings.Contains(msg, c.err) {
				t.Errorf("%s: msg %q", c.name, msg)
			}
			continue
		}
		if msg != "" || len(rows) != c.rows || rows[0].Title != c.title {
			t.Errorf("%s: got %v %q", c.name, rows, msg)
		}
	}
	rows, _ := archiveRows([][]string{{"NIS", "Nama", "ID Sumber"}, {"1", "Ani", "S1"}}, "")
	if rows[0].SourceRef != "S1" || rows[0].Payload["NIS"] != "1" {
		t.Fatalf("payload/source ref: %+v", rows[0])
	}
}

func migrationRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(claimsMiddleware)
		pr.Post("/users/import", app.ImportUsers)
		pr.Get("/users/import-batches", app.ListImportBatches)
		pr.Post("/users/import-batches/{id}/rollback", app.RollbackImportBatch)
		pr.Get("/migration-runs", app.ListMigrationRuns)
		pr.Post("/migration-runs", app.CreateMigrationRun)
		pr.Get("/migration-runs/{id}", app.GetMigrationRun)
		pr.Put("/migration-runs/{id}/totals", app.UpsertMigrationTotals)
		pr.Post("/migration-runs/{id}/recompute", app.RecomputeMigrationRun)
		pr.Post("/migration-runs/{id}/sign-off", app.SignOffMigrationRun)
		pr.Post("/migration-runs/{id}/discard", app.DiscardMigrationRun)
		pr.Post("/archive/import", app.ImportArchive)
		pr.Get("/archive/categories", app.ListArchiveCategories)
		pr.Get("/archive", app.SearchArchive)
		pr.Get("/archive/{id}", app.GetArchiveRecord)
	})
	return r
}

func postMultipart(t *testing.T, h http.Handler, path, claims, fileName, content string, fields map[string]string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", fileName)
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-Claims", claims)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func countRows(t *testing.T, app *App, q string, args ...any) int {
	t.Helper()
	var n int
	if err := app.DB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestMigrationRunTrialSignOffAndImports(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := migrationRouter(app)
	maker := seedUser(t, db, common.RoleAdmin)
	school := maker.schoolID
	checker := seedAccount(t, app, school, common.RoleAdmin)
	asMaker := claimsFor(maker.userID, common.RoleAdmin, school)
	asChecker := claimsFor(checker, common.RoleAdmin, school)
	p := maker.userID[:8]
	t.Cleanup(func() {
		mustExec(t, db, `DELETE FROM archive_records WHERE school_id=$1`, school)
		mustExec(t, db, `DELETE FROM source_id_map WHERE school_id=$1`, school)
		mustExec(t, db, `DELETE FROM import_batches WHERE school_id=$1`, school)
		mustExec(t, db, `DELETE FROM migration_runs WHERE school_id=$1`, school)
	})

	// Production needs a signed-off trial run.
	st, body := govCall(t, h, http.MethodPost, "/migration-runs", asMaker, map[string]any{"title": "Produksi", "mode": "production"})
	if st != http.StatusBadRequest {
		t.Fatalf("production without trial: %d %v", st, body)
	}
	st, body = govCall(t, h, http.MethodPost, "/migration-runs", asMaker, map[string]any{"title": "Percobaan 1", "mode": "trial"})
	if st != http.StatusCreated {
		t.Fatalf("create trial: %d %v", st, body)
	}
	trialID := dataMap(body)["id"].(string)
	st, body = govCall(t, h, http.MethodPost, "/migration-runs", asMaker, map[string]any{"title": "Produksi", "mode": "production", "trial_run_id": trialID})
	if st != http.StatusConflict {
		t.Fatalf("production from open trial: %d %v", st, body)
	}

	// Trial account import stores nothing but records the counts.
	students := fmt.Sprintf("Nama,Email,Peran,ID Sumber\nAni %s,ani-%s@test.local,siswa,LS-%s-1\nBudi %s,budi-%s@test.local,siswa,LS-%s-2\n", p, p, p, p, p, p)
	st, body = postMultipart(t, h, "/users/import", asMaker, "s.csv", students, map[string]string{"run_id": trialID})
	if st != http.StatusOK || !strings.Contains(fmt.Sprint(body["message"]), "percobaan") {
		t.Fatalf("trial import: %d %v", st, body)
	}
	if n := countRows(t, app, `SELECT count(*) FROM users WHERE email IN ($1,$2)`, "ani-"+p+"@test.local", "budi-"+p+"@test.local"); n != 0 {
		t.Fatalf("trial import stored %d accounts", n)
	}
	// A trial parent import resolves students created by the trial above.
	parents := fmt.Sprintf("Nama,Email,Hubungan,ID Sumber Siswa\nIbu Ani,ibu-%s@test.local,ibu,LS-%s-1\nIbu Ani,ibu-%s@test.local,ibu,LS-%s-2\n", p, p, p, p)
	st, body = postMultipart(t, h, "/users/import", asMaker, "o.csv", parents, map[string]string{"run_id": trialID, "kind": "orang_tua"})
	if st != http.StatusOK || dataMap(body)["trial"] != true {
		t.Fatalf("trial parents: %d %v", st, body)
	}
	if n := countRows(t, app, `SELECT count(*) FROM users WHERE email=$1`, "ibu-"+p+"@test.local"); n != 0 {
		t.Fatal("trial parent import stored an account")
	}

	// Control totals: target computed from the trial batches.
	st, body = govCall(t, h, http.MethodPut, "/migration-runs/"+trialID+"/totals", asMaker, map[string]any{"items": []map[string]any{
		{"category": "siswa", "source_value": 3},
		{"category": "relasi_ortu", "source_value": 2},
		{"category": "orang_tua", "source_value": 1},
	}})
	if st != http.StatusOK || dataMap(body)["unexplained_differences"].(float64) < 1 {
		t.Fatalf("totals: %d %v", st, body)
	}
	cases := []struct {
		name   string
		claims string
		code   int
		msg    string
	}{
		{"maker cannot sign", asMaker, http.StatusConflict, "maker-checker"},
		{"difference unexplained", asChecker, http.StatusConflict, "belum cocok"},
	}
	for _, c := range cases {
		st, body = govCall(t, h, http.MethodPost, "/migration-runs/"+trialID+"/sign-off", c.claims, map[string]any{})
		if st != c.code || !strings.Contains(fmt.Sprint(body["message"]), c.msg) {
			t.Fatalf("%s: %d %v", c.name, st, body)
		}
	}
	st, body = govCall(t, h, http.MethodPut, "/migration-runs/"+trialID+"/totals", asMaker, map[string]any{"items": []map[string]any{
		{"category": "siswa", "source_value": 3, "explanation": "1 siswa lulus belum dihapus di sistem lama"},
	}})
	if st != http.StatusOK {
		t.Fatalf("explain: %d %v", st, body)
	}
	st, body = govCall(t, h, http.MethodPost, "/migration-runs/"+trialID+"/sign-off", asChecker, map[string]any{"note": "cocok"})
	if st != http.StatusOK || dataMap(body)["status"] != "signed_off" {
		t.Fatalf("sign-off: %d %v", st, body)
	}
	st, _ = govCall(t, h, http.MethodPut, "/migration-runs/"+trialID+"/totals", asMaker, map[string]any{"items": []map[string]any{{"category": "siswa", "source_value": 1}}})
	if st != http.StatusConflict {
		t.Fatalf("totals after sign-off: %d", st)
	}

	// Production run, real imports.
	st, body = govCall(t, h, http.MethodPost, "/migration-runs", asMaker, map[string]any{"title": "Produksi", "mode": "production", "trial_run_id": trialID})
	if st != http.StatusCreated {
		t.Fatalf("production: %d %v", st, body)
	}
	prodID := dataMap(body)["id"].(string)
	st, body = postMultipart(t, h, "/users/import", asMaker, "s.csv", students, map[string]string{"run_id": prodID})
	if st != http.StatusOK || dataMap(body)["created"].(float64) != 2 {
		t.Fatalf("prod import: %d %v", st, body)
	}
	if n := countRows(t, app, `SELECT count(*) FROM source_id_map WHERE school_id=$1 AND entity_type='siswa'`, school); n != 2 {
		t.Fatalf("source_id_map siswa = %d", n)
	}
	// Re-importing the same legacy ids is refused per row (BUG-077).
	again := fmt.Sprintf("Nama,Email,Peran,ID Sumber\nAni Lagi,ani2-%s@test.local,siswa,LS-%s-1\n", p, p)
	st, body = postMultipart(t, h, "/users/import", asMaker, "s.csv", again, map[string]string{"dry_run": "true"})
	if st != http.StatusOK || dataMap(body)["invalid"].(float64) != 1 || !strings.Contains(fmt.Sprint(dataMap(body)["errors"]), "sudah pernah dimigrasikan") {
		t.Fatalf("re-import: %d %v", st, body)
	}
	// Parents: one account, two links.
	unknown := parents + fmt.Sprintf("Ayah X,ayah-%s@test.local,ayah,TIDAK-ADA\n", p)
	st, body = postMultipart(t, h, "/users/import", asMaker, "o.csv", unknown, map[string]string{"run_id": prodID, "kind": "orang_tua", "dry_run": "true"})
	if st != http.StatusOK || dataMap(body)["invalid"].(float64) != 1 || !strings.Contains(fmt.Sprint(dataMap(body)["errors"]), "tidak ditemukan") {
		t.Fatalf("parent preview: %d %v", st, body)
	}
	st, body = postMultipart(t, h, "/users/import", asMaker, "o.csv", parents, map[string]string{"run_id": prodID, "kind": "orang_tua"})
	if st != http.StatusOK {
		t.Fatalf("parent import: %d %v", st, body)
	}
	batchID := dataMap(body)["batch_id"].(string)
	if n := countRows(t, app, `SELECT count(*) FROM guardian_links g JOIN users u ON u.id=g.guardian_user_id WHERE u.email=$1`, "ibu-"+p+"@test.local"); n != 2 {
		t.Fatalf("links = %d", n)
	}
	// Totals of the production run come from live data.
	st, body = govCall(t, h, http.MethodPost, "/migration-runs/"+prodID+"/recompute", asMaker, nil)
	if st != http.StatusOK {
		t.Fatalf("recompute: %d %v", st, body)
	}
	if n := countRows(t, app, `SELECT target_value::int FROM migration_control_totals WHERE run_id=$1 AND category='relasi_ortu'`, prodID); n != 2 {
		t.Fatalf("relasi_ortu target = %d", n)
	}
	// Rolling back the parent batch removes links and the new account.
	st, body = govCall(t, h, http.MethodPost, "/users/import-batches/"+batchID+"/rollback", asMaker, map[string]any{"reason": "salah hubungan orang tua"})
	if st != http.StatusOK || dataMap(body)["links_removed"].(float64) != 2 || dataMap(body)["removed"].(float64) != 1 {
		t.Fatalf("rollback: %d %v", st, body)
	}
	// Classes cannot be imported without a current academic year.
	classes := fmt.Sprintf("Nama Kelas,ID Sumber Siswa\nX-A,LS-%s-1\n", p)
	st, body = postMultipart(t, h, "/users/import", asMaker, "k.csv", classes, map[string]string{"kind": "kelas"})
	if st != http.StatusBadRequest || !strings.Contains(fmt.Sprint(body["message"]), "Tahun ajaran") {
		t.Fatalf("class without year: %d %v", st, body)
	}

	// With a current year the classes go to the academic service; a
	// re-run is idempotent there (FindOrCreate + upsert enrollment).
	mustExec(t, db, `INSERT INTO academic_years (school_id, label, is_current) VALUES ($1,'2026/2027',true)`, school)
	t.Cleanup(func() { mustExec(t, db, `DELETE FROM academic_years WHERE school_id=$1`, school) })
	var mu sync.Mutex
	created, enrolled := 0, 0
	acad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/classrooms":
			common.WriteJSON(w, http.StatusOK, []any{}, nil)
		case r.Method == http.MethodPost && r.URL.Path == "/classrooms":
			created++
			common.WriteJSON(w, http.StatusCreated, map[string]string{"id": newIdentityTestUUID(), "name": "X-A"}, nil)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/students"):
			enrolled++
			common.WriteJSON(w, http.StatusCreated, map[string]string{}, nil)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(acad.Close)
	app.Academic = &client.AcademicClient{BaseURL: acad.URL, InternalKey: "internal-key", HTTPClient: acad.Client()}
	classes2 := classes + fmt.Sprintf("X-A,LS-%s-2\n", p)
	st, body = postMultipart(t, h, "/users/import", asMaker, "k.csv", classes2, map[string]string{"kind": "kelas"})
	if st != http.StatusOK || created != 1 || enrolled != 2 {
		t.Fatalf("class import: %d created=%d enrolled=%d %v", st, created, enrolled, body)
	}
	classBatch := dataMap(body)["batch_id"].(string)
	if st, body = govCall(t, h, http.MethodPost, "/users/import-batches/"+classBatch+"/rollback", asMaker, map[string]any{"reason": "kelas salah semua"}); st != http.StatusConflict ||
		!strings.Contains(fmt.Sprint(body["message"]), "menu Kelas") {
		t.Fatalf("class rollback: %d %v", st, body)
	}

	// Archive: import, search, read-only.
	arch := "NIS,Nama,Nilai Akhir,ID Sumber\n1001,Ani Rapor,88,R-1\n1002,Budi Rapor,79,R-2\n"
	st, body = postMultipart(t, h, "/archive/import", asMaker, "rapor.csv", arch, map[string]string{"category": "rapor_2019"})
	if st != http.StatusCreated {
		t.Fatalf("archive import: %d %v", st, body)
	}
	st, body = govCall(t, h, http.MethodGet, "/archive?category=rapor_2019&q=budi", asMaker, nil)
	if st != http.StatusOK || len(dataList(body)) != 1 {
		t.Fatalf("archive search: %d %v", st, body)
	}
	recID := dataList(body)[0].(map[string]any)["id"].(string)
	if _, err := db.Exec(`UPDATE archive_records SET title='x' WHERE id=$1`, recID); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("archive update allowed: %v", err)
	}
	asGuru := claimsFor(checker, common.RoleGuru, school)
	if st, _ = govCall(t, h, http.MethodGet, "/archive/"+recID, asGuru, nil); st != http.StatusForbidden {
		t.Fatalf("guru archive: %d", st)
	}
	otherSchool := claimsFor(checker, common.RoleKepalaSekolah, newIdentityTestUUID())
	if st, _ = govCall(t, h, http.MethodGet, "/archive/"+recID, otherSchool, nil); st != http.StatusNotFound {
		t.Fatalf("other unit archive: %d", st)
	}
}
