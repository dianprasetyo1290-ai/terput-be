package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
	"skci/academic/internal/model"
)

// isAgamaSubject reuses classroom_subject.go's normalizeGroup (same package)
// — the one place that already knows real Data Mata Pelajaran subject_group
// values are per-religion strings ("Agama Islam", "Agama Kristen", ...)
// rather than one shared "Agama" value.
func isAgamaSubject(subjectGroup *string) bool {
	group := normalizeGroup(subjectGroup)
	return group != nil && *group == "agama"
}

// checkScheduleSlotConflict enforces "a jam pelajaran slot may hold more
// than one subject ONLY when every subject in it is an Agama variant" —
// per spec, one classroom's students split by religion into parallel Agama
// sessions at the same hour, but a school must still not be able to
// double-book e.g. Matematika into an already-occupied slot. excludeID is
// the schedule row being edited (UpdateSchedule), so it doesn't conflict
// with itself; pass "" from CreateSchedule (new row, nothing to exclude).
func (h *Handler) checkScheduleSlotConflict(r *http.Request, schoolID, periodID string, newCS *model.ClassroomSubject, excludeID string) (string, error) {
	existing, err := h.Store.ListSchedulesByPeriod(r.Context(), schoolID, periodID)
	if err != nil {
		return "", err
	}
	newIsAgama := isAgamaSubject(newCS.SubjectGroup)
	for _, e := range existing {
		if e.ID == excludeID || e.ClassroomSubjectID == newCS.ID {
			continue // itself, or an idempotent re-post of the same subject
		}
		if !newIsAgama {
			return "jam pelajaran ini sudah terisi mata pelajaran lain di hari dan jam yang sama", nil
		}
		occupant, err := h.Store.GetClassroomSubject(r.Context(), schoolID, e.ClassroomSubjectID)
		if err != nil {
			return "", err
		}
		if occupant == nil || !isAgamaSubject(occupant.SubjectGroup) {
			return "jam pelajaran ini sudah terisi mata pelajaran non-Agama; hanya varian Agama yang boleh digabung pada jam yang sama", nil
		}
	}
	return "", nil
}

func parseDayParam(r *http.Request) *int {
	v := r.URL.Query().Get("day")
	if v == "" {
		return nil
	}
	d, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &d
}

type createSchedulePeriodRequest struct {
	DayOfWeek int    `json:"day_of_week"`
	PeriodNo  int    `json:"period_no"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// CreateSchedulePeriod: POST /classrooms/{id}/schedule-periods — "edit jam
// pelajaran". Flexible per classroom: each classroom has its own set of
// period rows, so different classes can have different period lengths.
func (h *Handler) CreateSchedulePeriod(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	classroomID := chi.URLParam(r, "id")
	classroom, err := h.Store.GetClassroom(r.Context(), schoolID, classroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if classroom == nil {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}
	var req createSchedulePeriodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.StartTime == "" || req.EndTime == "" || req.DayOfWeek < 1 || req.DayOfWeek > 7 {
		common.WriteError(w, http.StatusBadRequest, "day_of_week (1-7), period_no, start_time, end_time are required")
		return
	}
	p, err := h.Store.CreateSchedulePeriod(r.Context(), model.SchedulePeriod{
		SchoolID:       schoolID,
		AcademicYearID: classroom.AcademicYearID,
		ClassroomID:    classroomID,
		DayOfWeek:      req.DayOfWeek,
		PeriodNo:       req.PeriodNo,
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, p, nil)
}

// ListSchedulePeriods: GET /classrooms/{id}/schedule-periods?day= — same
// wali-kelas ownership guard as ListClassroomSchedules, since the Jadwal
// Kelas view loads both for the same classroom.
func (h *Handler) ListSchedulePeriods(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	classroomID := chi.URLParam(r, "id")
	if !h.requireOwnClassroomIfWaliKelas(w, r, schoolID, classroomID) {
		return
	}
	items, err := h.Store.ListSchedulePeriods(r.Context(), schoolID, classroomID, parseDayParam(r))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

type updateSchedulePeriodRequest struct {
	StartTime *string `json:"start_time"`
	EndTime   *string `json:"end_time"`
}

// UpdateSchedulePeriod: PUT /schedule-periods/{id}
func (h *Handler) UpdateSchedulePeriod(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req updateSchedulePeriodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := h.Store.UpdateSchedulePeriod(r.Context(), schoolID, chi.URLParam(r, "id"), req.StartTime, req.EndTime)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		common.WriteError(w, http.StatusNotFound, "schedule period not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

// DeleteSchedulePeriod: DELETE /schedule-periods/{id}
func (h *Handler) DeleteSchedulePeriod(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteSchedulePeriod(r.Context(), schoolID, chi.URLParam(r, "id")); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

type createScheduleRequest struct {
	SchedulePeriodID   string `json:"schedule_period_id"`
	ClassroomSubjectID string `json:"classroom_subject_id"`
}

// CreateSchedule: POST /classrooms/{id}/schedules — "edit jadwal". Assigns a
// subject (via its classroom_subjects mapping, which already carries the
// guru) into a period slot. Per spec, the classroom_subject_id offered here
// MUST already belong to this classroom (i.e. be a subject assigned to it
// via Data Kelas) — enforced below rather than trusting the client to only
// send valid options, since the "IPA classroom must not offer Ekonomi"
// constraint is a correctness rule, not just a UI nicety.
func (h *Handler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	classroomID := chi.URLParam(r, "id")
	classroom, err := h.Store.GetClassroom(r.Context(), schoolID, classroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if classroom == nil {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}
	var req createScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SchedulePeriodID == "" || req.ClassroomSubjectID == "" {
		common.WriteError(w, http.StatusBadRequest, "schedule_period_id and classroom_subject_id are required")
		return
	}
	period, err := h.Store.GetSchedulePeriod(r.Context(), schoolID, req.SchedulePeriodID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if period == nil || period.ClassroomID != classroomID {
		common.WriteError(w, http.StatusBadRequest, "schedule_period_id does not belong to this classroom")
		return
	}
	cs, err := h.Store.GetClassroomSubject(r.Context(), schoolID, req.ClassroomSubjectID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cs == nil || cs.ClassroomID != classroomID {
		common.WriteError(w, http.StatusBadRequest, "classroom_subject_id is not a subject assigned to this classroom (Data Kelas); assign it there first")
		return
	}
	if msg, err := h.checkScheduleSlotConflict(r, schoolID, req.SchedulePeriodID, cs, ""); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	} else if msg != "" {
		common.WriteError(w, http.StatusConflict, msg)
		return
	}
	sc, err := h.Store.CreateSchedule(r.Context(), model.Schedule{
		SchoolID:           schoolID,
		AcademicYearID:     classroom.AcademicYearID,
		ClassroomID:        classroomID,
		SchedulePeriodID:   req.SchedulePeriodID,
		ClassroomSubjectID: req.ClassroomSubjectID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, sc, nil)
}

// ListClassroomSchedules: GET /classrooms/{id}/schedules?day= — also backs
// wali kelas's read-only "Jadwal Kelas" menu (guarded to their own classroom
// below; every other role's behavior is unchanged).
func (h *Handler) ListClassroomSchedules(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	classroomID := chi.URLParam(r, "id")
	if !h.requireOwnClassroomIfWaliKelas(w, r, schoolID, classroomID) {
		return
	}
	items, err := h.Store.ListSchedulesByClassroom(r.Context(), schoolID, classroomID, parseDayParam(r))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

type updateScheduleRequest struct {
	ClassroomSubjectID *string `json:"classroom_subject_id"`
}

// UpdateSchedule: PUT /schedules/{id}
func (h *Handler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req updateScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	existing, err := h.Store.GetSchedule(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		common.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if req.ClassroomSubjectID != nil {
		cs, err := h.Store.GetClassroomSubject(r.Context(), schoolID, *req.ClassroomSubjectID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if cs == nil || cs.ClassroomID != existing.ClassroomID {
			common.WriteError(w, http.StatusBadRequest, "classroom_subject_id is not a subject assigned to this classroom")
			return
		}
		if msg, err := h.checkScheduleSlotConflict(r, schoolID, existing.SchedulePeriodID, cs, existing.ID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		} else if msg != "" {
			common.WriteError(w, http.StatusConflict, msg)
			return
		}
	}
	sc, err := h.Store.UpdateSchedule(r.Context(), schoolID, chi.URLParam(r, "id"), req.ClassroomSubjectID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, sc, nil)
}

// DeleteSchedule: DELETE /schedules/{id}
func (h *Handler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteSchedule(r.Context(), schoolID, chi.URLParam(r, "id")); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// GetScheduleByID: GET /schedules/{id}?school_id= — internal-consumable
// (see main.go's InternalOrJWTMiddleware group). attendance-service's
// check-in flow calls this to resolve classroom/subject/teacher/time AND
// the current term for the schedule's academic year, since a guru's own
// check-in request only ever carries schedule_id + whatever it happened to
// already have on screen (previously nothing resolved server-side at all,
// since this route didn't exist).
func (h *Handler) GetScheduleByID(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	sc, err := h.Store.GetScheduleEnriched(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sc == nil {
		common.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	termID, err := h.Store.CurrentTermID(r.Context(), sc.SchoolID, sc.AcademicYearID)
	if err != nil {
		termID = "" // best-effort: an unresolved term shouldn't block check-in
	}
	teacherID := ""
	if sc.TeacherUserID != nil {
		teacherID = *sc.TeacherUserID
	}
	subjectID := ""
	if sc.SubjectID != nil {
		subjectID = *sc.SubjectID
	}
	startTime, endTime := "", ""
	if sc.StartTime != nil {
		startTime = *sc.StartTime
	}
	if sc.EndTime != nil {
		endTime = *sc.EndTime
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":           sc.ID,
		"classroom_id": sc.ClassroomID,
		"subject_id":   subjectID,
		"teacher_id":   teacherID,
		"start_time":   startTime,
		"end_time":     endTime,
		"term_id":      termID,
	}, nil)
}

// ActiveSchedulesNow: GET /schedules/active-now?school_id= — every schedule
// slot in the school whose day/time window covers this exact moment, across
// ALL classrooms/guru. Internal-consumable (see main.go's InternalOrJWTMiddleware
// group) — attendance-service's "Guru Sedang Mengajar" widget is the caller.
func (h *Handler) ActiveSchedulesNow(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	items, err := h.Store.ListSchedulesActiveNow(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// MySchedule: GET /me/schedule?day= — guru's own teaching schedule.
func (h *Handler) MySchedule(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	claims := common.FromCtx(r)
	items, err := h.Store.ListSchedulesForTeacher(r.Context(), schoolID, claims.UserID, parseDayParam(r))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}
