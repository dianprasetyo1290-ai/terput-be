package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/attendance/internal/common"
)

// StudentAttendanceSummary backs
// GET /students/{id}/attendance-summary?term_id=&academic_year_id= — the
// report-service integration point: hadir/sakit/izin/tanpa_keterangan counts
// for one student's e-raport. Reachable two ways (see
// common.InternalOrJWTMiddleware): report-service calling internally with
// X-Internal-Key (claims will be nil), or a logged-in admin/guru/the student
// themself calling with a normal JWT.
func (h *Handler) StudentAttendanceSummary(w http.ResponseWriter, r *http.Request) {
	studentID := chi.URLParam(r, "id")
	claims := common.FromCtx(r)

	schoolID := ""
	if claims != nil {
		switch claims.ActiveRole {
		case "siswa":
			if claims.UserID != studentID {
				common.WriteError(w, http.StatusForbidden, "siswa may only view their own attendance summary")
				return
			}
			schoolID = claims.SchoolID
		case "superadmin":
			schoolID = r.URL.Query().Get("school_id")
		default: // admin, guru
			schoolID = claims.SchoolID
		}
	}
	// claims == nil means this came in via X-Internal-Key (report-service);
	// trusted, unscoped by school on purpose since the caller already knows
	// which student/school it's asking about.

	q := r.URL.Query()
	summary, err := h.Store.StudentAttendanceSummary(r.Context(), studentID, q.Get("term_id"), q.Get("academic_year_id"), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, summary, nil)
}
