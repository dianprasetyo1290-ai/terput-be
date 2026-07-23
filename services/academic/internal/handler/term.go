package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
)

// BootstrapTerms: POST /academic-years/{academic_year_id}/terms/bootstrap
// Creates Semester 1 + Semester 2 for a newly-created academic year.
// Per spec this can be triggered by admin explicitly (our choice, documented
// in README) rather than identity-service calling us automatically on
// academic-year creation, to avoid tight coupling between the two services'
// write paths in local dev.
func (h *Handler) BootstrapTerms(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID := chi.URLParam(r, "academic_year_id")
	terms, err := h.Store.BootstrapTerms(r.Context(), schoolID, academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, terms, nil)
}

// ListTerms: GET /terms?academic_year_id=
func (h *Handler) ListTerms(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	terms, err := h.Store.ListTerms(r.Context(), schoolID, academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, terms, nil)
}

// GetTerm: GET /terms/{id}
func (h *Handler) GetTerm(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	t, err := h.Store.GetTerm(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t == nil {
		common.WriteError(w, http.StatusNotFound, "term not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

type updateTermRequest struct {
	Name      *string `json:"name"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

// UpdateTerm: PUT /terms/{id} — editable start/end dates per spec.
func (h *Handler) UpdateTerm(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req updateTermRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t, err := h.Store.UpdateTerm(r.Context(), schoolID, chi.URLParam(r, "id"), req.Name, req.StartDate, req.EndDate)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t == nil {
		common.WriteError(w, http.StatusNotFound, "term not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}
