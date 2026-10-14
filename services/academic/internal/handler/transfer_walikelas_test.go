package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
)

// TestMenu78WaliKelasOwnClassOnly (QA-PAY round 2): a wali kelas lists,
// opens and looks up finances only for requests of their own class.
func TestMenu78WaliKelasOwnClassOnly(t *testing.T) {
	f := newMenu78Fixture(t)
	ctx := context.Background()
	const (
		wali  = "f7800000-0000-4000-8000-0000000000a1"
		other = "f7800000-0000-4000-8000-0000000000a2"
	)
	classID := "f7800000-0000-4000-8000-000000000005"
	if _, err := f.h.Store.DB.ExecContext(ctx, `UPDATE classrooms SET homeroom_teacher_user_id = $1 WHERE id = $2`, wali, classID); err != nil {
		t.Fatal(err)
	}

	code, env := f.create("walikelas", wali, "2026-08-01")
	if code != http.StatusCreated {
		t.Fatalf("create by own wali: %d %v", code, env)
	}
	id := dataField(env, "id")

	listCount := func(user string) int {
		code, env := f.do(http.MethodGet, "/student-status-requests", "walikelas", user, nil)
		if code != http.StatusOK {
			t.Fatalf("list %s: %d %v", user, code, env)
		}
		rows, _ := env["data"].([]any)
		return len(rows)
	}
	if n := listCount(wali); n != 1 {
		t.Fatalf("own wali sees %d rows, want 1", n)
	}
	if n := listCount(other); n != 0 {
		t.Fatalf("other wali sees %d rows, want 0", n)
	}
	if code, env := f.do(http.MethodGet, "/student-status-requests", common.RoleAdmin, f.admin, nil); code != http.StatusOK || len(env["data"].([]any)) != 1 {
		t.Fatalf("admin list: %d %v", code, env)
	}

	if code, _ := f.do(http.MethodGet, "/student-status-requests/"+id, "walikelas", wali, nil); code != http.StatusOK {
		t.Fatalf("detail own wali: %d", code)
	}
	if code, _ := f.do(http.MethodGet, "/student-status-requests/"+id, "walikelas", other, nil); code != http.StatusForbidden {
		t.Fatalf("detail other wali: %d want 403", code)
	}
	fin := "/student-status-requests/financial-summary?student_user_id=" + f.student
	if code, _ := f.do(http.MethodGet, fin, "walikelas", wali, nil); code != http.StatusOK {
		t.Fatalf("finance own wali: %d", code)
	}
	if code, _ := f.do(http.MethodGet, fin, "walikelas", other, nil); code != http.StatusForbidden {
		t.Fatalf("finance other wali: %d want 403", code)
	}
}

// TestMenu78Attachments: real upload, own keys only, download via the
// request's access rule.
func TestMenu78Attachments(t *testing.T) {
	t.Setenv("STORAGE_ROOT", t.TempDir())
	f := newMenu78Fixture(t)
	f.router.(*chi.Mux).Post("/student-status-request-attachments", f.h.UploadStatusRequestAttachment)
	f.router.(*chi.Mux).Get("/student-status-requests/{id}/files", f.h.DownloadStatusRequestAttachment)

	upload := func(role, user, name string) (int, map[string]any) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write([]byte("%PDF-1.4 surat"))
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/student-status-request-attachments", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req = req.WithContext(common.WithClaims(req.Context(), &common.Claims{UserID: user, ActiveRole: role, SchoolID: f.schoolID}))
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		var env map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		return rec.Code, env
	}
	if code, _ := upload("kepala_sekolah", f.admin, "a.pdf"); code != http.StatusForbidden {
		t.Fatalf("kepala_sekolah upload: %d want 403", code)
	}
	if code, _ := upload(common.RoleAdmin, f.admin, "a.exe"); code != http.StatusUnprocessableEntity {
		t.Fatalf("exe upload: %d want 422", code)
	}
	code, env := upload(common.RoleAdmin, f.admin, "surat.pdf")
	if code != http.StatusCreated {
		t.Fatalf("upload: %d %v", code, env)
	}
	key := dataField(env, "object_key")

	body := func(k string) map[string]any {
		return map[string]any{
			"student_user_id": f.student, "kind": "withdrawal", "effective_date": "2026-08-01", "reason": "Pindah domisili",
			"attachments": []map[string]any{{"name": "surat.pdf", "object_key": k}},
		}
	}
	foreign := f.schoolID + "/transfers/someone-else/x.pdf"
	if code, _ := f.do(http.MethodPost, "/student-status-requests", common.RoleAdmin, f.admin, body(foreign)); code != http.StatusUnprocessableEntity {
		t.Fatalf("foreign key: %d want 422", code)
	}
	code, env = f.do(http.MethodPost, "/student-status-requests", common.RoleAdmin, f.admin, body(key))
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, env)
	}
	id := dataField(env, "id")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/student-status-requests/"+id+"/files?key="+url.QueryEscape(key), nil)
	req = req.WithContext(common.WithClaims(req.Context(), &common.Claims{UserID: f.admin, ActiveRole: "kepala_sekolah", SchoolID: f.schoolID}))
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "surat") {
		t.Fatalf("download: %d %s", rec.Code, rec.Body.String())
	}
	if code, _ := f.do(http.MethodGet, "/student-status-requests/"+id+"/files?key=x", common.RoleAdmin, f.admin, nil); code != http.StatusNotFound {
		t.Fatalf("unknown key: %d want 404", code)
	}
	if code, _ := f.do(http.MethodGet, "/student-status-requests/"+id+"/files?key="+url.QueryEscape(key), "walikelas", "f7800000-0000-4000-8000-0000000000a2", nil); code != http.StatusForbidden {
		t.Fatalf("other wali download: %d want 403", code)
	}
}
