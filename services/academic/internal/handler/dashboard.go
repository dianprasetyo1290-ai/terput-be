package handler

import (
	"net/http"

	"skci/academic/internal/common"
)

// DashboardCounts: GET /dashboard/counts?academic_year_id= — jumlah siswa,
// jumlah guru, jumlah kelas, scoped to school_id + academic_year_id.
func (h *Handler) DashboardCounts(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	counts, err := h.Store.DashboardCountsFor(r.Context(), schoolID, academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, counts, nil)
}
