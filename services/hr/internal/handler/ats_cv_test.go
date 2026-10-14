package handler_test

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"skci/hr/internal/common"
)

// The public apply form uploads the CV; recruiters download it through
// /ats/applications/{id}/cv. A client-sent cv_object_key is ignored.
func TestATS_CVUploadAndDownload(t *testing.T) {
	router := setupTestApp(t).Router()
	ctx := context.Background()
	n := time.Now().UnixNano()
	var posID, reqID string
	if err := testDB.QueryRowContext(ctx, `INSERT INTO positions (school_id, code, name) VALUES ($1, $2, 'Guru CV') RETURNING id`, schoolID, fmt.Sprintf("CV-%d", n)).Scan(&posID); err != nil {
		t.Fatal(err)
	}
	if err := testDB.QueryRowContext(ctx, `
		INSERT INTO requisitions (school_id, position_id, quantity, reason, status, exceeds_headcount, requested_by, idempotency_key)
		VALUES ($1, $2, 1, 'addition', 'approved', false, '40000000-0000-0000-0000-000000000001', $3) RETURNING id`,
		schoolID, posID, fmt.Sprintf("cv-%d", n)).Scan(&reqID); err != nil {
		t.Fatal(err)
	}
	slug := fmt.Sprintf("guru-cv-%d", n)
	if _, err := testDB.ExecContext(ctx, `
		INSERT INTO job_postings (school_id, requisition_id, slug, title, status) VALUES ($1, $2, $3, 'Guru CV', 'open')`,
		schoolID, reqID, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.ExecContext(ctx, `
		INSERT INTO pipeline_stages (school_id, code, name, sort, kind) VALUES ($1, $2, 'Masuk', 0, 'normal')`,
		schoolID, fmt.Sprintf("cv%d", n%100000)); err != nil {
		t.Fatal(err)
	}

	apply := func(file string, data []byte) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("full_name", "Kandidat CV")
		_ = mw.WriteField("email", fmt.Sprintf("cv-%d-%s@example.com", n, file))
		_ = mw.WriteField("cv_object_key", "hr/documents/someone-else.pdf")
		if data != nil {
			fw, _ := mw.CreateFormFile("cv", file)
			_, _ = fw.Write(data)
		}
		_ = mw.Close()
		req := httptest.NewRequest(http.MethodPost, "/public/careers/"+schoolID+"/"+slug+"/apply", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}

	if rr := apply("cv.exe", []byte("MZ")); rr.Code != http.StatusBadRequest {
		t.Fatalf("non-PDF CV: want 400, got %d %s", rr.Code, rr.Body.String())
	}
	pdf := []byte("%PDF-1.4\n%test cv\n")
	if rr := apply("cv.pdf", pdf); rr.Code != http.StatusCreated {
		t.Fatalf("apply with CV: %d %s", rr.Code, rr.Body.String())
	}
	if rr := apply("none.pdf", nil); rr.Code != http.StatusCreated {
		t.Fatalf("apply without CV: %d %s", rr.Code, rr.Body.String())
	}

	var withCV, withoutCV, key string
	if err := testDB.QueryRowContext(ctx, `
		SELECT a.id, a.cv_object_key FROM applications a JOIN candidates c ON c.id = a.candidate_id
		WHERE c.email_norm = $1`, fmt.Sprintf("cv-%d-cv.pdf@example.com", n)).Scan(&withCV, &key); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "ats/cv/"+schoolID+"/") {
		t.Fatalf("CV key must be server-assigned, got %q", key)
	}
	if err := testDB.QueryRowContext(ctx, `
		SELECT a.id FROM applications a JOIN candidates c ON c.id = a.candidate_id
		WHERE c.email_norm = $1`, fmt.Sprintf("cv-%d-none.pdf@example.com", n)).Scan(&withoutCV); err != nil {
		t.Fatal(err)
	}

	recruiter := createToken("50000000-0000-0000-0000-000000000002", common.RoleRecruiter)
	rr := doGet(t, router, recruiter, "/ats/applications/"+withCV+"/cv")
	if rr.Code != http.StatusOK || !bytes.Equal(rr.Body.Bytes(), pdf) || rr.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("download CV: %d %q", rr.Code, rr.Body.String())
	}
	if rr := doGet(t, router, recruiter, "/ats/applications/"+withoutCV+"/cv"); rr.Code != http.StatusNotFound {
		t.Fatalf("no CV: want 404, got %d", rr.Code)
	}
	if rr := doGet(t, router, createToken("50000000-0000-0000-0000-000000000003", common.RolePegawai), "/ats/applications/"+withCV+"/cv"); rr.Code != http.StatusForbidden {
		t.Fatalf("pegawai: want 403, got %d", rr.Code)
	}
}
