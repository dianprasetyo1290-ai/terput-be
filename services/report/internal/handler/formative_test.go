package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/report/internal/common"
)

func TestUpsertFormativeGrades_RoleForbidden(t *testing.T) {
	h := &Handler{d: &Deps{}}

	// Siswa role should be forbidden
	req := httptest.NewRequest("PUT", "/formative", bytes.NewBufferString(`{
		"term_id": "term-1",
		"classroom_subject_id": "cs-1",
		"student_id": "stu-1",
		"assessment_type_id": "type-1",
		"score": 85
	}`))
	claims := &common.Claims{
		UserID:     "user-siswa",
		SchoolID:   "school-1",
		ActiveRole: "siswa",
	}
	ctx := context.WithValue(req.Context(), common.ClaimsCtxKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.UpsertFormativeGrades(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("want status 403 Forbidden for siswa role, got %d", rec.Code)
	}
}

func TestUpsertFormativeGrades_MissingFields(t *testing.T) {
	h := &Handler{d: &Deps{}}

	req := httptest.NewRequest("PUT", "/formative", bytes.NewBufferString(`{
		"term_id": "term-1",
		"score": 85
	}`))
	claims := &common.Claims{
		UserID:     "user-guru",
		SchoolID:   "school-1",
		ActiveRole: "guru",
	}
	ctx := context.WithValue(req.Context(), common.ClaimsCtxKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.UpsertFormativeGrades(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want status 400 Bad Request for missing fields, got %d", rec.Code)
	}
	var env common.Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err == nil {
		if env.Message == "" {
			t.Errorf("want error message in envelope, got empty")
		}
	}
}
