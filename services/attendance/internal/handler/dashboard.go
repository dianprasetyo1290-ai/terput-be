package handler

import (
	"net/http"

	"skci/attendance/internal/common"
)

// ActiveTeachers backs GET /dashboard/active-teachers: the "guru sedang
// mengajar" monitor. A guru shows up here from the moment they self
// check-in for a currently-in-progress schedule slot until that slot's
// end-time passes (see store.ActiveTeachers for the exact window logic).
// Polling is fine per spec, no websockets.
func (h *Handler) ActiveTeachers(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	list, err := h.Store.ActiveTeachers(r.Context(), scopeFromRequest(claims, r))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// best-effort name enrichment
	for i := range list {
		if u, err := h.Client.GetUser(bg(r), list[i].GuruID); err == nil && u != nil {
			list[i].GuruName = u.Name
		}
		if c, err := h.Client.GetClassroom(bg(r), list[i].ClassroomID); err == nil && c != nil {
			list[i].ClassroomName = c.Name
		}
		if list[i].SubjectID != "" {
			if s, err := h.Client.GetSubject(bg(r), list[i].SubjectID); err == nil && s != nil {
				list[i].SubjectName = s.Name
			}
		}
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// StudentAttendanceSummaryDashboard backs
// GET /dashboard/student-attendance-summary?academic_year_id= — monthly
// counts by status, suitable for a chart.
func (h *Handler) StudentAttendanceSummaryDashboard(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	list, err := h.Store.DashboardStudentSummary(r.Context(), scopeFromRequest(claims, r), academicYearFromRequest(r))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// TeacherAttendanceSummaryDashboard backs
// GET /dashboard/teacher-attendance-summary?academic_year_id= — monthly
// check-in counts, suitable for a chart.
func (h *Handler) TeacherAttendanceSummaryDashboard(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	list, err := h.Store.DashboardTeacherSummary(r.Context(), scopeFromRequest(claims, r), academicYearFromRequest(r))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}
