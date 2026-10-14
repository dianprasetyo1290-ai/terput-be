package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/client"
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
	if _, ok := h.termYear(w, academicYearID); !ok {
		return
	}
	terms, err := h.Store.BootstrapTerms(r.Context(), schoolID, academicYearID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat semester")
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
// QA-IDN: end > start (BUG-071), inside the academic year's range and not
// overlapping the other semester (BUG-072), and refused when identity has
// locked the year (BUG-015). When identity cannot be reached the edit is
// refused (503) rather than skipping the lock check.
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
	cur, err := h.Store.GetTerm(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat semester")
		return
	}
	if cur == nil {
		common.WriteError(w, http.StatusNotFound, "term not found")
		return
	}
	start, okS := parseDatePtr(req.StartDate)
	end, okE := parseDatePtr(req.EndDate)
	if !okS || !okE {
		common.WriteValidationError(w, http.StatusBadRequest, "Format tanggal harus YYYY-MM-DD", map[string]string{"start_date": "Format tanggal harus YYYY-MM-DD"})
		return
	}
	if start == nil {
		start = dateOnly(cur.StartDate)
	}
	if end == nil {
		end = dateOnly(cur.EndDate)
	}
	year, ok := h.termYear(w, cur.AcademicYearID)
	if !ok {
		return
	}
	var ys, ye *time.Time
	if year != nil {
		ys, _ = parseDatePtr(year.StartDate)
		ye, _ = parseDatePtr(year.EndDate)
	}
	all, err := h.Store.ListTerms(r.Context(), cur.SchoolID, cur.AcademicYearID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat semester")
		return
	}
	var siblings []termSibling
	for _, t := range all {
		if t.ID != cur.ID {
			siblings = append(siblings, termSibling{Order: t.Order, StartDate: dateOnly(t.StartDate), EndDate: dateOnly(t.EndDate)})
		}
	}
	if errs := validateTermDates(cur.Order, start, end, ys, ye, siblings); len(errs) > 0 {
		msg := errs["end_date"]
		if msg == "" {
			msg = errs["start_date"]
		}
		common.WriteValidationError(w, http.StatusBadRequest, msg, errs)
		return
	}
	t, err := h.Store.UpdateTerm(r.Context(), schoolID, chi.URLParam(r, "id"), req.Name, req.StartDate, req.EndDate)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan semester")
		return
	}
	if t == nil {
		common.WriteError(w, http.StatusNotFound, "term not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// termYear loads the academic year from identity and refuses locked years.
// A year identity no longer knows (deleted) is treated as having no range.
func (h *Handler) termYear(w http.ResponseWriter, academicYearID string) (*client.AcademicYear, bool) {
	if h.Identity == nil {
		return nil, true
	}
	year, err := h.Identity.GetAcademicYear(academicYearID)
	if errors.Is(err, client.ErrAcademicYearNotFound) {
		return nil, true
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusServiceUnavailable, "Tidak dapat memverifikasi tahun ajaran; coba lagi nanti")
		return nil, false
	}
	if year.LockedAt != nil {
		common.WriteError(w, http.StatusConflict, "Tahun ajaran sudah dikunci; semester tidak dapat diubah")
		return nil, false
	}
	return year, true
}
