package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
)

type issueTokenReq struct {
	UserID      string `json:"user_id"`
	ClassroomID string `json:"classroom_id"`
	ExpiresAt   string `json:"expires_at"` // RFC3339; defaults to exam.end_at if omitted
}

func (h *Handler) IssueToken(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	var req issueTokenReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.UserID == "" {
		common.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	classroomID := req.ClassroomID
	if classroomID == "" {
		classroomID = exam.ClassroomID
	}
	expiresAt := exam.EndAt
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "expires_at must be RFC3339")
			return
		}
		expiresAt = t
	}
	tok, err := h.Store.IssueToken(r.Context(), exam.SchoolID, exam.AcademicYearID, examID, req.UserID, classroomID, c.UserID, expiresAt)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, tok, nil)
}

type bulkIssueTokenReq struct {
	ClassroomID string   `json:"classroom_id"`
	StudentIDs  []string `json:"student_ids"` // simplification: caller (admin-web) sources these from academic-service's roster; see README Assumptions
	ExpiresAt   string   `json:"expires_at"`
}

func (h *Handler) BulkIssueTokens(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	var req bulkIssueTokenReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.ClassroomID == "" || len(req.StudentIDs) == 0 {
		common.WriteError(w, http.StatusBadRequest, "classroom_id and a non-empty student_ids are required")
		return
	}
	expiresAt := exam.EndAt
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "expires_at must be RFC3339")
			return
		}
		expiresAt = t
	}
	var issued []interface{}
	for _, sid := range req.StudentIDs {
		tok, err := h.Store.IssueToken(r.Context(), exam.SchoolID, exam.AcademicYearID, examID, sid, req.ClassroomID, c.UserID, expiresAt)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		issued = append(issued, tok)
	}
	common.WriteJSON(w, http.StatusCreated, issued, nil)
}

func (h *Handler) ListTokens(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	list, err := h.Store.ListTokens(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

type regenTokenReq struct {
	ExpiresAt string `json:"expires_at"`
}

func (h *Handler) RegenerateToken(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	tokenID := chi.URLParam(r, "tokenId")
	var req regenTokenReq
	if r.ContentLength > 0 {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&req) // body optional; malformed/empty body just means "no new expiry"
	}
	var expiresAt *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "expires_at must be RFC3339")
			return
		}
		expiresAt = &t
	}
	tok, err := h.Store.RegenerateToken(r.Context(), tokenID, common.EffectiveSchoolID(r, c), expiresAt)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if tok == nil {
		common.WriteError(w, http.StatusNotFound, "token not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, tok, nil)
}
