package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

// ---- 1. guru self check-in ------------------------------------------

type checkInRequest struct {
	ScheduleID  string `json:"schedule_id"`
	ClassroomID string `json:"classroom_id"` // fallback if academic-service lookup fails, see README Assumptions
	SubjectID   string `json:"subject_id"`
	TermID      string `json:"term_id"`
	SessionDate string `json:"session_date"` // YYYY-MM-DD, defaults to today
	StartTime   string `json:"slot_start_time"`
	EndTime     string `json:"slot_end_time"`
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
		req.SessionDate = todayWIB()
	}

	classroomID := req.ClassroomID
	startTime := ptrOrNil(req.StartTime)
	endTime := ptrOrNil(req.EndTime)
	subjectID := ptrOrNil(req.SubjectID)
	termID := ptrOrNil(req.TermID)

	if sched, err := h.Client.GetSchedule(bg(r), req.ScheduleID, claims.SchoolID); err == nil && sched != nil {
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
			// Not the guru academic-service has on record for this slot --
			// but a "pergantian guru" (substitute) request may already have
			// been accepted (see store.AcceptSubstituteRequest), naming this
			// caller as the stand-in for this exact (schedule_id,
			// session_date). In that case class_sessions.guru_id already
			// equals claims.UserID, so this caller is pre-authorized to
			// check in as the substitute.
			existing, _ := h.Store.FindSessionByScheduleAndDate(bg(r), req.ScheduleID, req.SessionDate)
			if existing == nil || existing.GuruID != claims.UserID {
				common.WriteError(w, http.StatusForbidden, "schedule slot is not assigned to this guru")
				return
			}
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
	// Seed every roster student as belum_presensi so a fresh session starts
	// with a real "not yet marked" status instead of callers each defaulting
	// unmarked students to hadir. Best-effort: academic-service being down
	// shouldn't fail the check-in itself (Roster()/frontend still fall back
	// sensibly for students that don't have a row yet).
	initSubjectID := ""
	if cs.SubjectID != nil {
		initSubjectID = *cs.SubjectID
	}
	if roster, err := h.Client.GetRosterForSubject(bg(r), cs.ClassroomID, initSubjectID, cs.SchoolID); err == nil {
		ids := make([]string, len(roster))
		for i, s := range roster {
			ids[i] = s.StudentID
		}
		_ = h.Store.InitStudentAttendances(r.Context(), cs, ids)
	}
	common.WriteJSONMsg(w, http.StatusOK, cs, nil, "Presensi berhasil dicatat")
}

// ---- pergantian guru (substitute teacher) request/response ------------
//
// Bug Report SEKOCI (GPM 5 Agustus 2026): a guru who can't attend no longer
// reassigns a slot directly -- they send a request (Alasan + Guru
// Pengganti, "Kirim Permintaan") to a named colleague. Nothing on
// class_sessions moves until that colleague accepts (see
// store.AcceptSubstituteRequest); a reject just resolves the request so
// the requester can pick someone else -- the slot stays theirs either way
// until an accept actually happens.

type createSubstituteRequestBody struct {
	ScheduleID  string `json:"schedule_id"`
	ClassroomID string `json:"classroom_id"` // fallback if academic-service lookup fails, same as CheckIn
	SubjectID   string `json:"subject_id"`
	TermID      string `json:"term_id"`
	SessionDate string `json:"session_date"` // YYYY-MM-DD, defaults to today
	StartTime   string `json:"slot_start_time"`
	EndTime     string `json:"slot_end_time"`
	RequestedTo string `json:"requested_to"` // Guru B
	Reason      string `json:"reason"`       // sakit | izin | cuti
}

// CreateSubstituteRequest implements "Kirim Permintaan": Guru A asks Guru B
// to cover a slot. Mirrors the old direct-assign endpoint's academic-service
// resolution/fallback and ownership rule (only the guru actually assigned to
// the slot may request a substitute for it) but only inserts a pending
// substitute_requests row -- class_sessions.guru_id is untouched here.
func (h *Handler) CreateSubstituteRequest(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	var req createSubstituteRequestBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ScheduleID == "" {
		common.WriteError(w, http.StatusBadRequest, "schedule_id is required")
		return
	}
	if req.RequestedTo == "" {
		common.WriteError(w, http.StatusBadRequest, "requested_to is required")
		return
	}
	if req.RequestedTo == claims.UserID {
		common.WriteError(w, http.StatusBadRequest, "cannot request yourself as your own substitute")
		return
	}
	if !model.ValidSubstituteReason[req.Reason] {
		common.WriteError(w, http.StatusBadRequest, "invalid reason, must be one of sakit|izin|cuti")
		return
	}
	if req.SessionDate == "" {
		req.SessionDate = todayWIB()
	}

	classroomID := req.ClassroomID
	startTime := ptrOrNil(req.StartTime)
	endTime := ptrOrNil(req.EndTime)
	subjectID := ptrOrNil(req.SubjectID)
	termID := ptrOrNil(req.TermID)

	originalGuruID := claims.UserID // who is being replaced, from the caller's own JWT
	sched, schedErr := h.Client.GetSchedule(bg(r), req.ScheduleID, claims.SchoolID)
	if schedErr == nil && sched != nil {
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
		if sched.GuruID != "" {
			originalGuruID = sched.GuruID
		}
	}
	// Only the guru currently on the hook for this slot may request a
	// substitute for it -- either per academic-service's schedule record,
	// or (if that's unreachable/inconclusive) per an existing
	// class_session's guru_id (e.g. they were already substituted in once
	// today and now need to hand it off again).
	if originalGuruID != claims.UserID {
		existing, _ := h.Store.FindSessionByScheduleAndDate(bg(r), req.ScheduleID, req.SessionDate)
		if existing == nil || existing.GuruID != claims.UserID {
			common.WriteError(w, http.StatusForbidden, "only the guru assigned to this slot may request a substitute")
			return
		}
	}
	if classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id is required (could not resolve from academic-service, must be supplied)")
		return
	}

	if pending, perr := h.Store.HasPendingSubstituteRequest(bg(r), req.ScheduleID, req.SessionDate); perr == nil && pending {
		common.WriteError(w, http.StatusConflict, "sudah ada permintaan pergantian guru yang menunggu konfirmasi untuk jadwal ini")
		return
	}

	academicYearID := r.URL.Query().Get("academic_year_id")
	if academicYearID == "" {
		academicYearID = r.Header.Get("X-Academic-Year-Id")
	}
	if academicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id is required (query param or X-Academic-Year-Id header)")
		return
	}

	sr, err := h.Store.CreateSubstituteRequest(r.Context(), store.CreateSubstituteRequestInput{
		SchoolID:       claims.SchoolID,
		AcademicYearID: academicYearID,
		TermID:         termID,
		ScheduleID:     req.ScheduleID,
		ClassroomID:    classroomID,
		SubjectID:      subjectID,
		SessionDate:    req.SessionDate,
		SlotStartTime:  startTime,
		SlotEndTime:    endTime,
		RequestedBy:    claims.UserID,
		RequestedTo:    req.RequestedTo,
		Reason:         req.Reason,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create substitute request: "+err.Error())
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, sr, nil, "Permintaan pergantian guru berhasil dikirim")
}

// ListIncomingSubstituteRequests: Guru B's pending list, shown on their own
// "Jadwal Saya" page. This list IS the "notification" the bug report asks
// for -- no generic in-app notification/push mechanism exists elsewhere in
// this codebase to reuse (identity-service's announcements are school-wide
// broadcasts, not a fit for a 1:1 pending action), so a queryable pending
// row is the pragmatic minimum that satisfies "sistem mengirim notifikasi
// ke akun Guru B".
func (h *Handler) ListIncomingSubstituteRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	list, err := h.Store.ListIncomingSubstituteRequests(r.Context(), claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// ListOutgoingSubstituteRequests: Guru A's own requests (any status) --
// lets them see a rejection ("Guru A mendapat notifikasi penolakan") and
// confirm the slot is still theirs to hand off to someone else.
func (h *Handler) ListOutgoingSubstituteRequests(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	list, err := h.Store.ListOutgoingSubstituteRequests(r.Context(), claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// substituteResponseBundle bundles the resolved request and the
// class_session it just reassigned, so the frontend can update both the
// pending list and "Jadwal Saya" from one accept response.
type substituteResponseBundle struct {
	Request *model.SubstituteRequest `json:"request"`
	Session *model.ClassSession      `json:"session"`
}

// loadOwnIncomingRequestOrErr enforces "only the addressed Guru B, and only
// while still pending, may accept/reject" -- shared by both handlers below.
func (h *Handler) loadOwnIncomingRequestOrErr(w http.ResponseWriter, r *http.Request, claims *common.Claims) *model.SubstituteRequest {
	id := chi.URLParam(r, "id")
	sr, err := h.Store.GetSubstituteRequest(r.Context(), id)
	if err == store.ErrNotFound {
		common.WriteError(w, http.StatusNotFound, "substitute request not found")
		return nil
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return nil
	}
	if sr.RequestedTo != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "this request was not sent to you")
		return nil
	}
	if sr.Status != string(model.SubstituteStatusPending) {
		common.WriteError(w, http.StatusConflict, "this request has already been responded to")
		return nil
	}
	return sr
}

// AcceptSubstituteRequestHandler: Guru B clicks "Terima" -- the teaching
// slot transfers immediately and now shows up on Guru B's own "Jadwal
// Saya" (class_sessions.guru_id reassigned inside the store call).
func (h *Handler) AcceptSubstituteRequestHandler(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	if h.loadOwnIncomingRequestOrErr(w, r, claims) == nil {
		return
	}
	sr, cs, err := h.Store.AcceptSubstituteRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to accept substitute request: "+err.Error())
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, substituteResponseBundle{Request: sr, Session: cs}, nil, "Permintaan diterima, jadwal berhasil dipindahkan")
}

// RejectSubstituteRequestHandler: Guru B clicks "Tolak Permintaan" -- Guru
// A gets a rejection notification (visible via ListOutgoingSubstituteRequests)
// and the slot stays stuck with them until they request someone else.
func (h *Handler) RejectSubstituteRequestHandler(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	if h.loadOwnIncomingRequestOrErr(w, r, claims) == nil {
		return
	}
	sr, err := h.Store.RejectSubstituteRequest(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to reject substitute request: "+err.Error())
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, sr, nil, "Permintaan pergantian guru ditolak")
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

// CheckOut records jam keluar (clock-out) for a pertemuan -- the
// counterpart to CheckIn's jam masuk (checked_in_at). Client request: each
// pertemuan should record both a start and end time, not just one
// timestamp.
func (h *Handler) CheckOut(w http.ResponseWriter, r *http.Request) {
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
	updated, err := h.Store.CheckOut(r.Context(), cs.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to check out: "+err.Error())
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, updated, nil, "Jam keluar berhasil dicatat")
}

// ---- 2. Agenda Pembelajaran Harian ------------------------------------

type agendaRequest struct {
	Pertemuan       string   `json:"pertemuan"`
	Pembahasan      string   `json:"pembahasan"`
	MateriPertemuan string   `json:"materi_pertemuan"`
	BentukKegiatan  []string `json:"bentuk_kegiatan"`
	// Catatan: session-level notes (client example: "ada tugas yang harus
	// diselesaikan hari itu juga"). TugasDiberikan/TugasJenis: the "tugas"
	// checkbox -- whether homework was given, and Individu vs Kelompok.
	Catatan        string `json:"catatan"`
	TugasDiberikan bool   `json:"tugas_diberikan"`
	TugasJenis     string `json:"tugas_jenis"`
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
	// tugas_jenis is only meaningful when tugas_diberikan is checked; a
	// stale/mismatched value from the client is dropped rather than
	// rejected, since "no task given" makes the type moot either way.
	tugasJenis := ""
	if req.TugasDiberikan {
		if req.TugasJenis != "" && !model.ValidTugasJenis[req.TugasJenis] {
			common.WriteError(w, http.StatusBadRequest, "invalid tugas_jenis, must be individu or kelompok")
			return
		}
		tugasJenis = req.TugasJenis
	}
	updated, err := h.Store.UpdateAgenda(r.Context(), cs.ID, store.AgendaInput{
		Pertemuan:       ptrOrNil(req.Pertemuan),
		Pembahasan:      ptrOrNil(req.Pembahasan),
		MateriPertemuan: ptrOrNil(req.MateriPertemuan),
		BentukKegiatan:  req.BentukKegiatan,
		Catatan:         ptrOrNil(req.Catatan),
		TugasDiberikan:  req.TugasDiberikan,
		TugasJenis:      ptrOrNil(tugasJenis),
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
	existing, err := h.Store.ListStudentAttendances(r.Context(), cs.ClassroomID, cs.SessionDate)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	statusByStudent := map[string]model.StudentAttendance{}
	for _, sa := range existing {
		statusByStudent[sa.StudentID] = sa
	}

	// Subject-filtered, not the full classroom: two Agama variants (Islam/
	// Kristen/...) can share the same classroom+period, each with its own
	// class_session — a guru must only ever see/mark their OWN religion's
	// students here, never the whole classroom, or two guru's roster screens
	// for the same slot silently collide over the same (classroom_id,
	// session_date) rows.
	subjectID := ""
	if cs.SubjectID != nil {
		subjectID = *cs.SubjectID
	}
	roster, err := h.Client.GetRosterForSubject(bg(r), cs.ClassroomID, subjectID, cs.SchoolID)
	if err != nil {
		// academic-service unreachable: fall back to whatever we already
		// have recorded rather than failing the whole screen.
		common.WriteJSONMsg(w, http.StatusOK, existing, nil, "roster tidak tersedia dari academic-service, menampilkan data presensi yang sudah tercatat")
		return
	}

	type rosterEntry struct {
		StudentID   string  `json:"student_id"`
		StudentName string  `json:"student_name"`
		Status      string  `json:"status"`
		Catatan     *string `json:"catatan,omitempty"`
	}
	out := make([]rosterEntry, 0, len(roster))
	for _, s := range roster {
		status := string(model.StatusBelumPresensi)
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
	list, err := h.Store.ListStudentAttendances(r.Context(), cs.ClassroomID, cs.SessionDate)
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
	// BR-ATT-02: "pilih semua" must not overwrite an approved izin/sakit.
	leaves, err := h.Store.ApprovedLeaveTypes(r.Context(), cs.SchoolID, cs.SessionDate, req.StudentIDs)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to check approved leave: "+err.Error())
		return
	}
	results := make([]model.StudentAttendance, 0, len(req.StudentIDs))
	for _, sid := range req.StudentIDs {
		status, catatan := req.Status, (*string)(nil)
		if l, ok := leaves[sid]; ok {
			status, catatan = l.Status, &l.Catatan
		}
		sa, err := h.Store.UpsertStudentAttendance(r.Context(), cs.SessionDate, store.MarkInput{
			ClassSessionID: cs.ID,
			SchoolID:       cs.SchoolID,
			AcademicYearID: cs.AcademicYearID,
			TermID:         cs.TermID,
			ClassroomID:    cs.ClassroomID,
			StudentID:      sid,
			Status:         status,
			Catatan:        catatan,
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
	sa, err := h.Store.UpsertStudentAttendance(r.Context(), cs.SessionDate, store.MarkInput{
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

// MySessionsToday: GET /me/sessions/today (guru) -- the calling guru's own
// class_sessions rows for today, keyed by schedule_id, so the guru's own
// dashboard can tell whether "Mulai Pelajaran" has already been done for
// the currently-showing slot (and its agenda filled in) even after a page
// reload, instead of only knowing it within the current browser session.
func (h *Handler) MySessionsToday(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "guru") {
		return
	}
	today := todayWIB()
	list, _, err := h.Store.ListSessions(r.Context(), store.ListSessionsFilter{
		SchoolID: claims.SchoolID,
		GuruID:   claims.UserID,
		Date:     today,
		PerPage:  100,
		Page:     1,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
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
