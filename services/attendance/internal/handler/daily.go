package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

// Daily class attendance input, not tied to a schedule slot:
//   - FR-41-002: Academic Admin inputs/corrects a class's daily attendance
//     (before the lock; after it, corrections.go).
//   - FR-69-001 / AC-69-01: wali kelas records their own class's daily
//     attendance in bulk or per student, with notes, then submits it.

type dailyStudent struct {
	StudentID   string  `json:"student_id"`
	StudentName string  `json:"student_name"`
	Status      string  `json:"status"`
	Catatan     *string `json:"catatan"`
	LeaveStatus string  `json:"leave_status,omitempty"` // approved izin/sakit covering the day (BR-ATT-02)
}

type dailyAttendanceResponse struct {
	ClassroomID string                 `json:"classroom_id"`
	Date        string                 `json:"date"`
	Locked      bool                   `json:"locked"`
	LockAt      *time.Time             `json:"lock_at,omitempty"`
	Submission  *store.DailySubmission `json:"submission"`
	Students    []dailyStudent         `json:"students"`
}

// dailyScope enforces role + classroom ownership and returns the school.
func (h *Handler) dailyScope(w http.ResponseWriter, r *http.Request, claims *common.Claims, classroomID string, write bool) (string, bool) {
	roles := []string{"admin", "superadmin", "walikelas", "kepala_sekolah"}
	if write {
		roles = []string{"admin", "superadmin", "walikelas"}
	}
	if !requireRole(w, claims, roles...) {
		return "", false
	}
	if classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id wajib diisi")
		return "", false
	}
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, classroomID) {
		return "", false
	}
	schoolID := scopeFromRequest(claims, r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id wajib diisi")
		return "", false
	}
	return schoolID, true
}

func validDate(d string) bool {
	_, err := time.Parse("2006-01-02", d)
	return err == nil
}

func (h *Handler) buildDaily(r *http.Request, schoolID, classroomID, date string) (*dailyAttendanceResponse, error) {
	existing, err := h.Store.ListStudentAttendances(r.Context(), classroomID, date)
	if err != nil {
		return nil, err
	}
	byStudent := map[string]model.StudentAttendance{}
	for _, sa := range existing {
		byStudent[sa.StudentID] = sa
	}
	type rosterRow struct{ id, name string }
	var roster []rosterRow
	if rs, err := h.Client.GetRoster(r.Context(), classroomID); err == nil {
		for _, s := range rs {
			roster = append(roster, rosterRow{s.StudentID, s.Name})
		}
	} else {
		// academic unreachable: show what's recorded rather than nothing.
		for _, sa := range existing {
			roster = append(roster, rosterRow{sa.StudentID, ""})
		}
	}
	ids := make([]string, 0, len(roster))
	for _, s := range roster {
		ids = append(ids, s.id)
	}
	leaves, err := h.Store.ApprovedLeaveTypes(r.Context(), schoolID, date, ids)
	if err != nil {
		return nil, err
	}
	out := &dailyAttendanceResponse{ClassroomID: classroomID, Date: date, Students: make([]dailyStudent, 0, len(roster))}
	for _, s := range roster {
		row := dailyStudent{StudentID: s.id, StudentName: s.name, Status: string(model.StatusBelumPresensi)}
		if sa, ok := byStudent[s.id]; ok {
			row.Status, row.Catatan = sa.Status, sa.Catatan
		}
		if l, ok := leaves[s.id]; ok {
			row.LeaveStatus = l.Status
		}
		out.Students = append(out.Students, row)
	}
	if cfg, err := h.Store.GetLockSettings(r.Context(), schoolID); err == nil {
		out.Locked = cfg.IsLocked(date, h.clock(), wibLocation)
		if cfg.Enabled {
			if at, err := cfg.LockAt(date, wibLocation); err == nil {
				out.LockAt = &at
			}
		}
	} else {
		return nil, err
	}
	sub, err := h.Store.GetDailySubmission(r.Context(), classroomID, date)
	if err != nil {
		return nil, err
	}
	out.Submission = sub
	return out, nil
}

// GetDailyAttendance: GET /admin/student-attendance/daily?classroom_id=&date=
func (h *Handler) GetDailyAttendance(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	q := r.URL.Query()
	classroomID, date := q.Get("classroom_id"), q.Get("date")
	if date == "" {
		date = todayWIB()
	}
	if !validDate(date) {
		common.WriteError(w, http.StatusBadRequest, "date harus berformat YYYY-MM-DD")
		return
	}
	schoolID, ok := h.dailyScope(w, r, claims, classroomID, false)
	if !ok {
		return
	}
	out, err := h.buildDaily(r, schoolID, classroomID, date)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type dailyEntry struct {
	StudentID string  `json:"student_id"`
	Status    string  `json:"status"`
	Catatan   *string `json:"catatan"`
}

type saveDailyBody struct {
	ClassroomID    string       `json:"classroom_id"`
	Date           string       `json:"date"`
	AcademicYearID string       `json:"academic_year_id"`
	TermID         string       `json:"term_id"`
	Entries        []dailyEntry `json:"entries"`
	Submit         bool         `json:"submit"`
	Catatan        *string      `json:"catatan"` // class-level note for the day
}

// SaveDailyAttendance: POST /admin/student-attendance/daily — save a draft
// (submit=false) or submit (submit=true) a class's daily attendance.
func (h *Handler) SaveDailyAttendance(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	var body saveDailyBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validDate(body.Date) {
		common.WriteError(w, http.StatusBadRequest, "date harus berformat YYYY-MM-DD")
		return
	}
	schoolID, ok := h.dailyScope(w, r, claims, body.ClassroomID, true)
	if !ok {
		return
	}
	if len(body.Entries) == 0 {
		common.WriteError(w, http.StatusBadRequest, "entries wajib diisi")
		return
	}
	if body.Date > todayWIB() {
		common.WriteError(w, http.StatusBadRequest, "tidak dapat mengisi presensi untuk tanggal yang akan datang")
		return
	}
	for _, e := range body.Entries {
		if !model.AttendanceStatus(e.Status).Valid() {
			common.WriteError(w, http.StatusBadRequest, "status tidak valid untuk siswa "+e.StudentID)
			return
		}
		if body.Submit && e.Status == string(model.StatusBelumPresensi) {
			common.WriteError(w, http.StatusBadRequest, "semua siswa harus diberi status sebelum dikirim")
			return
		}
	}
	if !h.ensureUnlocked(w, r, schoolID, body.Date) { // FR-41-003
		return
	}

	// Every student must be in this classroom's roster (academic is the
	// source of truth); and the academic year comes from the classroom.
	roster, err := h.Client.GetRoster(r.Context(), body.ClassroomID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "gagal memuat daftar siswa kelas")
		return
	}
	inRoster := map[string]bool{}
	ids := make([]string, 0, len(roster))
	for _, s := range roster {
		inRoster[s.StudentID] = true
		ids = append(ids, s.StudentID)
	}
	for _, e := range body.Entries {
		if !inRoster[e.StudentID] {
			common.WriteError(w, http.StatusBadRequest, "siswa "+e.StudentID+" tidak terdaftar di kelas ini")
			return
		}
	}
	academicYearID := body.AcademicYearID
	if c, err := h.Client.GetClassroom(r.Context(), body.ClassroomID); err == nil && c != nil && c.AcademicYearID != "" {
		academicYearID = c.AcademicYearID
	}
	if academicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id tidak dapat ditentukan")
		return
	}
	leaves, err := h.Store.ApprovedLeaveTypes(r.Context(), schoolID, body.Date, ids)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memeriksa izin: "+err.Error())
		return
	}
	sessionID, err := h.Store.FindOrCreateManualSession(r.Context(), schoolID, academicYearID, body.TermID, body.ClassroomID, body.Date, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var termID *string
	if body.TermID != "" {
		termID = &body.TermID
	}
	var alpa alpaCollector
	for _, e := range body.Entries {
		status, catatan := e.Status, e.Catatan
		if catatan != nil && strings.TrimSpace(*catatan) == "" {
			catatan = nil
		}
		// BR-ATT-02: an approved izin/sakit wins over hadir/terlambat/alpa.
		if l, ok := leaves[e.StudentID]; ok && status != "sakit" && status != "izin" {
			status, catatan = l.Status, &l.Catatan
		}
		if _, err := h.Store.UpsertStudentAttendance(r.Context(), body.Date, store.MarkInput{
			ClassSessionID: sessionID, SchoolID: schoolID, AcademicYearID: academicYearID, TermID: termID,
			ClassroomID: body.ClassroomID, StudentID: e.StudentID, Status: status, Catatan: catatan, MarkedBy: claims.UserID,
		}); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan presensi siswa "+e.StudentID+": "+err.Error())
			return
		}
		alpa.add(schoolID, e.StudentID, body.Date, status)
	}
	if _, err := h.Store.SaveDailySubmission(r.Context(), schoolID, body.ClassroomID, body.Date, claims.UserID, body.Submit, body.Catatan); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan status kirim: "+err.Error())
		return
	}
	h.notifyAlpaAsync(alpa) // FR-41-004

	out, err := h.buildDaily(r, schoolID, body.ClassroomID, body.Date)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	msg := "Presensi harian disimpan sebagai draf"
	if body.Submit {
		msg = "Presensi harian berhasil dikirim"
	}
	common.WriteJSONMsg(w, http.StatusOK, out, nil, msg)
}
