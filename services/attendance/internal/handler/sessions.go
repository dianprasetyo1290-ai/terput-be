package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

// ---- 1. guru self check-in ------------------------------------------

type checkInRequest struct {
	ScheduleID  string  `json:"schedule_id"`
	ClassroomID string  `json:"classroom_id"` // fallback if academic-service lookup fails, see README Assumptions
	SubjectID   string  `json:"subject_id"`
	TermID      string  `json:"term_id"`
	SessionDate string  `json:"session_date"` // YYYY-MM-DD, defaults to today
	StartTime   string  `json:"slot_start_time"`
	EndTime     string  `json:"slot_end_time"`
}

// CheckIn handles step 1 of the guru daily flow: self presensi tied to a
// schedule slot for today. It tries to resolve the slot's classroom/time
// range from academic-service; if that call fails (service down, or not
// wired up yet in local dev) it falls back to whatever the client supplied
// in the request body, since the guru's own client already has the schedule
// detail on screen to build the check-in button. See README Assumptions.
func (h *Handler) CheckIn(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	var req checkInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ScheduleID == "" {
		common.WriteError(w, http.StatusBadRequest, "schedule_id is required")
		return
	}
	if req.SessionDate == "" {
		req.SessionDate = time.Now().Format("2006-01-02")
	}

	classroomID := req.ClassroomID
	startTime := ptrOrNil(req.StartTime)
	endTime := ptrOrNil(req.EndTime)
	subjectID := ptrOrNil(req.SubjectID)
	termID := ptrOrNil(req.TermID)

	if sched, err := h.Client.GetSchedule(bg(r), req.ScheduleID); err == nil && sched != nil {
		if sched.ClassroomID != "" {
			classroomID = sched.ClassroomID
		}
		if sched.StartTime != "" {
			startTime = &sched.StartTime
		}
		if sched.EndTime != "" {
			endTime = &sched.EndTime
		}
		if sched.SubjectID != "" {
			subjectID = &sched.SubjectID
		}
		if sched.TermID != "" {
			termID = &sched.TermID
		}
		if sched.GuruID != "" && sched.GuruID != claims.UserID {
			common.WriteError(w, http.StatusForbidden, "schedule slot is not assigned to this guru")
			return
		}
	}
	if classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id is required (could not resolve from academic-service, must be supplied)")
		return
	}

	academicYearID := r.URL.Query().Get("academic_year_id")
	// attendance-service doesn't own academic years; the client is expected
	// to pass the current one (fetched from academic-service). If omitted we
	// still store a zero-value-free placeholder is not possible (NOT NULL) —
	// require it explicitly. See README Assumptions.
	if academicYearID == "" {
		academicYearID = r.Header.Get("X-Academic-Year-Id")
	}
	if academicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id is required (query param or X-Academic-Year-Id header)")
		return
	}

	cs, err := h.Store.CheckIn(r.Context(), store.CheckInInput{
		SchoolID:       claims.SchoolID,
		AcademicYearID: academicYearID,
		TermID:         termID,
		ScheduleID:     req.ScheduleID,
		ClassroomID:    classroomID,
		SubjectID:      subjectID,
		GuruID:         claims.UserID,
		SessionDate:    req.SessionDate,
		SlotStartTime:  startTime,
		SlotEndTime:    endTime,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to check in: "+err.Error())
		return
	}
	if cs.GuruID != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "this schedule slot's session was already checked into by another guru")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, cs, nil, "Presensi berhasil dicatat")
}

// ---- session detail / ownership guard --------------------------------

func (h *Handler) loadSessionOrErr(w http.ResponseWriter, r *http.Request) *model.ClassSession {
	id := chi.URLParam(r, "id")
	cs, err := h.Store.GetClassSession(r.Context(), id)
	if err == store.ErrNotFound {
		common.WriteError(w, http.StatusNotFound, "session not found")
		return nil
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil
	}
	return cs
}

// guruOwnsSession enforces "reject writing for a schedule slot that isn't
// theirs" for guru-role callers; admins may act on any session.
func guruOwnsSession(w http.ResponseWriter, claims *common.Claims, cs *model.ClassSession) bool {
	if claims.ActiveRole == "admin" || claims.ActiveRole == "superadmin" {
		return true
	}
	if cs.GuruID != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "this session belongs to another guru")
		return false
	}
	return true
}

func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	cs := h.loadSessionOrErr(w, r)
	if cs == nil {
		return
	}
	if !guruOwnsSession(w, claims, cs) {
		return
	}
	common.WriteJSON(w, http.StatusOK, cs, nil)
}

// ---- 2. Agenda Pembelajaran Harian ------------------------------------

type agendaRequest struct {
	Pertemuan       string   `json:"pertemuan"`
	Pembahasan      string   `json:"pembahasan"`
	MateriPertemuan string   `json:"materi_pertemuan"`
	BentukKegiatan  []string `json:"bentuk_kegiatan"`
}

func (h *Handler) UpdateAgenda(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	cs := h.loadSessionOrErr(w, r)
	if cs == nil {
		return
	}
	if !guruOwnsSession(w, claims, cs) {
		return
	}
	var req agendaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, bk := range req.BentukKegiatan {
		if !model.ValidBentukKegiatan[bk] {
			common.WriteError(w, http.StatusBadRequest, "invalid bentuk_kegiatan value: "+bk)
			return
		}
	}
	updated, err := h.Store.UpdateAgenda(r.Context(), cs.ID, store.AgendaInput{
		Pertemuan:       ptrOrNil(req.Pertemuan),
		Pembahasan:      ptrOrNil(req.Pembahasan),
		MateriPertemuan: ptrOrNil(req.MateriPertemuan),
		BentukKegiatan:  req.BentukKegiatan,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save agenda: "+err.Error())
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, updated, nil, "Agenda pembelajaran berhasil disimpan")
}

// ---- 3. roster + student attendance -----------------------------------

// Roster proxies academic-service's classroom roster (best-effort) merged
// with whatever attendance has already been recorded for this session, so
// the guru's "Presensi" screen can render the full student list with
// current statuses (defaulting to hadir for anyone not yet marked).
func (h *Handler) Roster(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	cs := h.loadSessionOrErr(w, r)
	if cs == nil {
		return
	}
	if !guruOwnsSession(w, claims, cs) {
		return
	}
	existing, err := h.Store.ListStudentAttendances(r.Context(), cs.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	statusByStudent := map[string]model.StudentAttendance{}
	for _, sa := range existing {
		statusByStudent[sa.StudentID] = sa
	}

	roster, err := h.Client.GetRoster(bg(r), cs.ClassroomID)
	if err != nil {
		// academic-service unreachable: fall back to whatever we already
		// have recorded rather than failing the whole screen.
		common.WriteJSONMsg(w, http.StatusOK, existing, nil, "roster tidak tersedia dari academic-service, menampilkan data presensi yang sudah tercatat")
		return
	}

	type rosterEntry struct {
		StudentID   string `json:"student_id"`
		StudentName string `json:"student_name"`
		Status      string `json:"status"`
		Catatan     *string `json:"catatan,omitempty"`
	}
	out := make([]rosterEntry, 0, len(roster))
	for _, s := range roster {
		status := string(model.StatusHadir)
		var catatan *string
		if sa, ok := statusByStudent[s.StudentID]; ok {
			status = sa.Status
			catatan = sa.Catatan
		}
		out = append(out, rosterEntry{StudentID: s.StudentID, StudentName: s.Name, Status: status, Catatan: catatan})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (h *Handler) ListSessionStudents(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	cs := h.loadSessionOrErr(w, r)
	if cs == nil {
		return
	}
	if !guruOwnsSession(w, claims, cs) {
		return
	}
	list, err := h.Store.ListStudentAttendances(r.Context(), cs.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// bulkMarkRequest implements "Pilih semua" — mark every listed student
// hadir (or, in principle, any single status) in one call. The frontend
// supplies student_ids from the roster it already fetched via Roster().
type bulkMarkRequest struct {
	StudentIDs []string `json:"student_ids"`
	Status     string   `json:"status"` // defaults to hadir
}

func (h *Handler) BulkMarkStudents(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	cs := h.loadSessionOrErr(w, r)
	if cs == nil {
		return
	}
	if !guruOwnsSession(w, claims, cs) {
		return
	}
	var req bulkMarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Status == "" {
		req.Status = string(model.StatusHadir)
	}
	if !model.AttendanceStatus(req.Status).Valid() {
		common.WriteError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if len(req.StudentIDs) == 0 {
		common.WriteError(w, http.StatusBadRequest, "student_ids is required")
		return
	}
	results := make([]model.StudentAttendance, 0, len(req.StudentIDs))
	for _, sid := range req.StudentIDs {
		sa, err := h.Store.UpsertStudentAttendance(r.Context(), store.MarkInput{
			ClassSessionID: cs.ID,
			SchoolID:       cs.SchoolID,
			AcademicYearID: cs.AcademicYearID,
			TermID:         cs.TermID,
			ClassroomID:    cs.ClassroomID,
			StudentID:      sid,
			Status:         req.Status,
			MarkedBy:       claims.UserID,
		})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed marking student "+sid+": "+err.Error())
			return
		}
		results = append(results, *sa)
	}
	common.WriteJSONMsg(w, http.StatusOK, results, nil, "Presensi siswa berhasil disimpan")
}

// markOneRequest implements the per-student "aksi -> ubah status" override.
type markOneRequest struct {
	Status  string  `json:"status"`
	Catatan *string `json:"catatan"`
}

func (h *Handler) MarkOneStudent(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	cs := h.loadSessionOrErr(w, r)
	if cs == nil {
		return
	}
	if !guruOwnsSession(w, claims, cs) {
		return
	}
	studentID := chi.URLParam(r, "student_id")
	var req markOneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !model.AttendanceStatus(req.Status).Valid() {
		common.WriteError(w, http.StatusBadRequest, "invalid status, must be one of hadir|sakit|izin|tanpa_keterangan")
		return
	}
	sa, err := h.Store.UpsertStudentAttendance(r.Context(), store.MarkInput{
		ClassSessionID: cs.ID,
		SchoolID:       cs.SchoolID,
		AcademicYearID: cs.AcademicYearID,
		TermID:         cs.TermID,
		ClassroomID:    cs.ClassroomID,
		StudentID:      studentID,
		Status:         req.Status,
		Catatan:        req.Catatan,
		MarkedBy:       claims.UserID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to mark student: "+err.Error())
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, sa, nil, "Status kehadiran siswa berhasil diubah")
}

// ---- admin: list sessions (also serves as "Presensi Guru" raw feed) ---

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	page, perPage := pageParams(r)
	q := r.URL.Query()
	list, total, err := h.Store.ListSessions(r.Context(), store.ListSessionsFilter{
		SchoolID:       scopeFromRequest(claims, r),
		AcademicYearID: academicYearFromRequest(r),
		TermID:         q.Get("term_id"),
		GuruID:         q.Get("guru_id"),
		ClassroomID:    q.Get("classroom_id"),
		Date:           q.Get("date"),
		Month:          q.Get("month"),
		Day:            normalizeDay(q.Get("day")),
		Page:           page,
		PerPage:        perPage,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, map[string]interface{}{"page": page, "per_page": perPage, "total": total})
}
