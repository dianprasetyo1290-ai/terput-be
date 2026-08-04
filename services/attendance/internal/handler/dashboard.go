package handler

import (
	"net/http"
	"time"

	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
)

// activeTeachersNow is the shared source of truth behind both the "Guru
// Sedang Mengajar" list widget (ActiveTeachers) and the admin dashboard's
// "Guru Aktif Mengajar" stat card (TeachingStats) -- was previously two
// different (and differently wrong) computations. It starts from
// academic-service's real SCHEDULE for this exact moment (every guru who is
// SUPPOSED to be teaching right now, across every classroom), not from
// attendance's own class_sessions table -- class_sessions only gets a row
// once a guru actually checks in, so a schedule-only slot used to be
// invisible here entirely (the "0 sedang mengajar" bug even when guru were
// genuinely scheduled). Each schedule row is then matched against any
// existing class_session (same schedule_id + today's date, mirroring
// CheckIn's own upsert key) to derive real status: "aktif" only once that
// specific session's Agenda Pembelajaran Harian has actually been filled
// in, "tidak_aktif" by default (scheduled but not yet checked in, or
// checked in but agenda not filled yet).
func (h *Handler) activeTeachersNow(r *http.Request, schoolID string) ([]model.ActiveTeacher, error) {
	schedules, err := h.Client.ListActiveSchedulesNow(r.Context(), schoolID)
	if err != nil {
		return nil, err
	}
	today := time.Now().Format("2006-01-02")
	out := make([]model.ActiveTeacher, 0, len(schedules))
	for _, sc := range schedules {
		if sc.TeacherUserID == nil || *sc.TeacherUserID == "" {
			continue // slot with no assigned guru yet -- nothing to show
		}
		at := model.ActiveTeacher{
			GuruID:      *sc.TeacherUserID,
			ClassroomID: sc.ClassroomID,
			ScheduleID:  sc.ID,
			Status:      "tidak_aktif",
		}
		if sc.ClassroomName != nil {
			at.ClassroomName = *sc.ClassroomName
		}
		if sc.SubjectID != nil {
			at.SubjectID = *sc.SubjectID
		}
		if sc.SubjectName != nil {
			at.SubjectName = *sc.SubjectName
		}
		if sc.StartTime != nil {
			at.SlotStartTime = *sc.StartTime
		}
		if sc.EndTime != nil {
			at.SlotEndTime = *sc.EndTime
		}
		if cs, err := h.Store.FindSessionByScheduleAndDate(r.Context(), sc.ID, today); err == nil && cs != nil {
			at.SessionID = cs.ID
			if cs.CheckedInAt != nil {
				at.CheckedInAt = cs.CheckedInAt.Format(time.RFC3339)
			}
			if cs.AgendaFilledAt != nil {
				at.Status = "aktif"
			}
		}
		if u, err := h.Client.GetUser(bg(r), at.GuruID); err == nil && u != nil {
			at.GuruName = u.Name
		}
		out = append(out, at)
	}
	return out, nil
}

// ActiveTeachers backs GET /dashboard/active-teachers: the "guru sedang
// mengajar" monitor -- every guru scheduled to teach right now, whether or
// not they've checked in yet (see activeTeachersNow).
func (h *Handler) ActiveTeachers(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	list, err := h.activeTeachersNow(r, scopeFromRequest(claims, r))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// TeachingStats backs GET /dashboard/teaching-stats: the admin dashboard's
// "Total Guru" / "Guru Aktif Mengajar" stat cards. Total Guru is a live
// identity-service count (never cached), Guru Aktif Mengajar reuses
// activeTeachersNow's same live schedule+status computation as the list
// widget above, so the two can never disagree with each other again.
func (h *Handler) TeachingStats(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	schoolID := scopeFromRequest(claims, r)
	list, err := h.activeTeachersNow(r, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	activeCount := 0
	for _, at := range list {
		if at.Status == "aktif" {
			activeCount++
		}
	}
	totalGuru := 0
	if n, err := h.Client.CountUsersByRole(bg(r), schoolID, "guru"); err == nil {
		totalGuru = n
	}
	common.WriteJSON(w, http.StatusOK, map[string]int{
		"total_guru":          totalGuru,
		"guru_aktif_mengajar": activeCount,
	}, nil)
}

// resolveTermLabels swaps each row's raw term_id/"unknown" key (what the
// store's GROUP BY term_id produces) for a human label ("Semester 1", ...)
// via academic-service, so the chart keeps rendering a plain label string
// per row with zero frontend changes needed. Best-effort: an unresolvable
// term_id (lookup failure, or the literal "unknown" bucket) just keeps
// showing as-is rather than erroring the whole chart.
func (h *Handler) resolveTermLabels(r *http.Request, list []model.MonthlyStatusCount) {
	cache := map[string]string{}
	for i := range list {
		key := list[i].Month
		if key == "" || key == "unknown" {
			list[i].Month = "Lainnya"
			continue
		}
		if label, ok := cache[key]; ok {
			list[i].Month = label
			continue
		}
		if t, err := h.Client.GetTerm(bg(r), key); err == nil && t != nil && t.Name != "" {
			cache[key] = t.Name
			list[i].Month = t.Name
		}
	}
}

// StudentAttendanceSummaryDashboard backs
// GET /dashboard/student-attendance-summary?academic_year_id= — per-semester
// (term) counts by status, suitable for a chart. Was previously grouped by
// calendar month, which made the chart grow unboundedly tall across a
// school year.
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
	h.resolveTermLabels(r, list)
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// TeacherAttendanceSummaryDashboard backs
// GET /dashboard/teacher-attendance-summary?academic_year_id= — per-semester
// (term) check-in counts, suitable for a chart.
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
	h.resolveTermLabels(r, list)
	common.WriteJSON(w, http.StatusOK, list, nil)
}
