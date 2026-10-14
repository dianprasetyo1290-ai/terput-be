package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
)

func importRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Post("/auth/login", app.Login)
	r.Post("/auth/mfa/verify", app.VerifyMFA)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTSessionMiddleware(testSecret, app.ValidateSession))
		pr.Post("/users/import", app.ImportUsers)
		pr.Get("/users/import-batches", app.ListImportBatches)
		pr.Post("/users/import-batches/{id}/rollback", app.RollbackImportBatch)
	})
	return r
}

func postImport(t *testing.T, h http.Handler, token, fileName, content string, fields map[string]string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", fileName)
	_, _ = fw.Write([]byte(content))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/users/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestImportUsersFlow(t *testing.T) {
	db := openTestDB(t)
	h := importRouter(newTestApp(db))
	admin := seedUser(t, db, common.RoleAdmin)
	tok := loginAny(t, h, admin.email, admin.password)
	p := admin.userID[:8]
	good := fmt.Sprintf("Nama;Email;Peran;ID Sumber\nAni %s;ani-%s@test.local;siswa;OLD-1\nBudi %s;budi-%s@test.local;guru;OLD-2\n", p, p, p, p)
	mixed := good + fmt.Sprintf("Cici;cici-%s@test;siswa;\n", p)

	cases := []struct {
		name   string
		file   string
		body   string
		fields map[string]string
		status int
		check  string
	}{
		{"unsupported format", "data.xls", "x", nil, http.StatusBadRequest, "Format file tidak didukung"},
		{"missing column", "data.csv", "Nama,Telepon\nA,1\n", nil, http.StatusBadRequest, "Email, Peran"},
		{"too many rows", "data.csv", "Nama,Email,Peran\n" + strings.Repeat("A,a@b.id,siswa\n", 1001), nil, http.StatusBadRequest, "maksimal 1000"},
		{"admin role not allowed", "data.csv", good, map[string]string{"role": common.RoleKepalaSekolah}, http.StatusForbidden, "guru atau siswa"},
		{"dry run preview", "data.csv", mixed, map[string]string{"dry_run": "true"}, http.StatusOK, "Pratinjau"},
		{"invalid rows block commit", "data.csv", mixed, nil, http.StatusUnprocessableEntity, "tidak ada data yang disimpan"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, body := postImport(t, h, tok, c.file, c.body, c.fields)
			if st != c.status || !strings.Contains(fmt.Sprint(body["message"], body["error"]), c.check) {
				t.Fatalf("got %d %v", st, body)
			}
		})
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM users WHERE email LIKE $1 AND email NOT LIKE 'qa-%'`, "%-"+p+"@test.local").Scan(&n)
	if n != 0 {
		t.Fatalf("preview/rejected import saved %d users", n)
	}
	st, body := postImport(t, h, tok, "data.csv", mixed, map[string]string{"dry_run": "true"})
	if d := dataMap(body); d["valid"] != float64(2) || d["invalid"] != float64(1) {
		t.Fatalf("preview %d %v", st, body)
	}

	// commit skipping the invalid row
	st, body = postImport(t, h, tok, "data.csv", mixed, map[string]string{"skip_invalid": "true"})
	if st != http.StatusOK || dataMap(body)["created"] != float64(2) {
		t.Fatalf("commit %d %v", st, body)
	}
	batchID := dataMap(body)["batch_id"].(string)
	var ref, status string
	if err := db.QueryRow(`SELECT i.source_ref, usr.student_status FROM import_batch_items i
		JOIN user_school_roles usr ON usr.user_id=i.user_id WHERE i.batch_id=$1 AND i.role='siswa'`, batchID).Scan(&ref, &status); err != nil || ref != "OLD-1" || status != "aktif" {
		t.Fatalf("item %q %q %v", ref, status, err)
	}
	// same file again: emails now taken
	st, body = postImport(t, h, tok, "data.csv", good, map[string]string{"dry_run": "true"})
	if dataMap(body)["invalid"] != float64(2) {
		t.Fatalf("re-import preview %d %v", st, body)
	}

	st, body = doJSON(t, h, http.MethodGet, "/users/import-batches", tok, nil)
	if st != http.StatusOK || !strings.Contains(fmt.Sprint(body["data"]), batchID) {
		t.Fatalf("list %d %v", st, body)
	}

	// rollback refused while an account has signed in
	var aniID string
	_ = db.QueryRow(`SELECT user_id FROM import_batch_items WHERE batch_id=$1 AND role='siswa'`, batchID).Scan(&aniID)
	mustExec(t, db, `INSERT INTO login_events (user_id, email, success, reason) VALUES ($1,'x',true,'ok')`, aniID)
	path := "/users/import-batches/" + batchID + "/rollback"
	if st, body = doJSON(t, h, http.MethodPost, path, tok, map[string]string{"reason": "pendek"}); st != http.StatusBadRequest {
		t.Fatalf("short reason %d %v", st, body)
	}
	if st, body = doJSON(t, h, http.MethodPost, path, tok, map[string]string{"reason": "file salah unit, diulang"}); st != http.StatusConflict {
		t.Fatalf("blocked rollback %d %v", st, body)
	}
	mustExec(t, db, `DELETE FROM login_events WHERE user_id=$1`, aniID)
	if st, body = doJSON(t, h, http.MethodPost, path, tok, map[string]string{"reason": "file salah unit, diulang"}); st != http.StatusOK || dataMap(body)["removed"] != float64(2) {
		t.Fatalf("rollback %d %v", st, body)
	}
	if st, _ = doJSON(t, h, http.MethodPost, path, tok, map[string]string{"reason": "file salah unit, diulang"}); st != http.StatusConflict {
		t.Fatalf("second rollback %d", st)
	}
	// emails released: the corrected file can be imported again
	if st, body = postImport(t, h, tok, "data.csv", good, nil); st != http.StatusOK || dataMap(body)["created"] != float64(2) {
		t.Fatalf("re-import after rollback %d %v", st, body)
	}

	// another unit's admin cannot see or roll back this batch
	other := seedUser(t, db, common.RoleAdmin)
	otok := loginAny(t, h, other.email, other.password)
	if st, _ = doJSON(t, h, http.MethodPost, "/users/import-batches/"+dataMap(body)["batch_id"].(string)+"/rollback", otok, map[string]string{"reason": "bukan milik unit ini"}); st != http.StatusNotFound {
		t.Fatalf("cross-unit rollback %d", st)
	}
}
