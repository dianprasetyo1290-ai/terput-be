package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
	"skci/academic/internal/model"
)

type createSubjectRequest struct {
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	SubjectGroup *string `json:"subject_group"`
	Kelompok     *string `json:"kelompok"`
}

// CreateSubject: POST /subjects
func (h *Handler) CreateSubject(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req createSubjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	sub, err := h.Store.CreateSubject(r.Context(), model.Subject{
		SchoolID:     schoolID,
		Name:         req.Name,
		Description:  req.Description,
		SubjectGroup: req.SubjectGroup,
		Kelompok:     req.Kelompok,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, sub, nil)
}

// ListSubjects: GET /subjects?page=&per_page=
func (h *Handler) ListSubjects(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	page, perPage := common.ParsePage(r)
	items, total, err := h.Store.ListSubjects(r.Context(), schoolID, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// GetSubject: GET /subjects/{id}
func (h *Handler) GetSubject(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	sub, err := h.Store.GetSubject(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sub == nil {
		common.WriteError(w, http.StatusNotFound, "subject not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, sub, nil)
}

type updateSubjectRequest struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	SubjectGroup *string `json:"subject_group"`
	Kelompok     *string `json:"kelompok"`
}

// UpdateSubject: PUT /subjects/{id}
func (h *Handler) UpdateSubject(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req updateSubjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sub, err := h.Store.UpdateSubject(r.Context(), schoolID, chi.URLParam(r, "id"), req.Name, req.Description, req.SubjectGroup, req.Kelompok)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sub == nil {
		common.WriteError(w, http.StatusNotFound, "subject not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, sub, nil)
}

// DeleteSubject: DELETE /subjects/{id}
func (h *Handler) DeleteSubject(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteSubject(r.Context(), schoolID, chi.URLParam(r, "id")); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}
