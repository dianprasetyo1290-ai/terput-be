package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"skci/attendance/internal/common"
	"skci/attendance/internal/store"
)

// ---- Presensi Siswa (admin) --------------------------------------------

// StudentAttendanceAdmin serves the "Presensi Siswa" admin view's per-record
// detail table: per classroom, filterable by academic year (including past
// years), bulan, tanggal. Shown only when an admin has picked a specific
// Tanggal — the "hari" filter was removed per client (V4 revision: redundant
// once Tanggal exists) and the no-filter default view is now
// StudentAttendanceSummaryAdmin below (one row per student, not one row per
// record).
func (h *Handler) StudentAttendanceAdmin(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	// kepala_sekolah: the principal gets a read-only, school-wide presensi
	// recap for oversight; falls through to the admin branch below (no
	// walikelas classroom scoping).
	if !requireRole(w, claims, "admin", "superadmin", "walikelas", "kepala_sekolah") {
		return
	}
	q := r.URL.Query()
	// Wali kelas's read-only "Presensi Siswa" menu reuses this same admin
	// endpoint, forced to their own classroom (see requireOwnClassroom) —
	// admin/superadmin behavior (optional/unfiltered classroom_id) is
	// unchanged.
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, q.Get("classroom_id")) {
		return
	}
	page, perPage := pageParams(r)
	list, total, err := h.Store.ListStudentAttendanceForAdmin(r.Context(), store.StudentAttendanceFilter{
		SchoolID:       scopeFromRequest(claims, r),
		AcademicYearID: academicYearFromRequest(r),
		ClassroomID:    q.Get("classroom_id"),
		Date:           q.Get("date"),
		Month:          q.Get("month"),
		Page:           page,
		PerPage:        perPage,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, map[string]interface{}{"page": page, "per_page": perPage, "total": total})
}

// StudentAttendanceSummaryAdmin serves the "Presensi Siswa" admin view's
// DEFAULT display (client V4 revision, item 1): one row per student per
// classroom, Hadir/Sakit/Izin/Tanpa Keterangan counts aggregated over one
// month (defaults to the current month server-side if omitted) — no
// duplicate student rows, re-aggregates when the month filter changes.
func (h *Handler) StudentAttendanceSummaryAdmin(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	// kepala_sekolah: the principal gets a read-only, school-wide presensi
	// recap for oversight; falls through to the admin branch below (no
	// walikelas classroom scoping).
	if !requireRole(w, claims, "admin", "superadmin", "walikelas", "kepala_sekolah") {
		return
	}
	q := r.URL.Query()
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, q.Get("classroom_id")) {
		return
	}
	list, err := h.Store.StudentAttendanceMonthlySummary(r.Context(),
		scopeFromRequest(claims, r), academicYearFromRequest(r), q.Get("classroom_id"), q.Get("month"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// StudentAttendanceMonthlyGridAdmin serves the "Presensi Siswa" admin
// monthly grid print view (client revision, GPM School 2026-08-24, item 1):
// replaces the day-by-day print flow with one printable table per month
// (columns = day 1..N). Requires a specific classroom_id (unlike the
// counts-only summary above, which can aggregate "Semua Kelas") because the
// roster merge below needs one classroom to enumerate students from.
func (h *Handler) StudentAttendanceMonthlyGridAdmin(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	// kepala_sekolah: the principal gets a read-only, school-wide presensi
	// recap for oversight; falls through to the admin branch below (no
	// walikelas classroom scoping).
	if !requireRole(w, claims, "admin", "superadmin", "walikelas", "kepala_sekolah") {
		return
	}
	q := r.URL.Query()
	classroomID := q.Get("classroom_id")
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, classroomID) {
		return
	}
	if classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id is required")
		return
	}
	grid, err := h.Store.StudentAttendanceMonthlyGrid(r.Context(), scopeFromRequest(claims, r), academicYearFromRequest(r), classroomID, q.Get("month"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	roster, err := h.Client.GetRoster(r.Context(), classroomID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "could not load classroom roster")
		return
	}
	byStudent := map[string]map[string]string{}
	for _, g := range grid {
		byStudent[g.StudentID] = g.Days
	}
	type gridRow struct {
		StudentID   string            `json:"student_id"`
		StudentName string            `json:"student_name"`
		Days        map[string]string `json:"days"`
	}
	out := make([]gridRow, 0, len(roster))
	for _, s := range roster {
		days := byStudent[s.StudentID]
		if days == nil {
			days = map[string]string{}
		}
		out = append(out, gridRow{StudentID: s.StudentID, StudentName: s.Name, Days: days})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// StudentAttendanceExport returns the same filtered set as
// StudentAttendanceAdmin as CSV, for the admin's export/print flow (actual
// print formatting is left to the frontend, per the task spec).
func (h *Handler) StudentAttendanceExport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	q := r.URL.Query()
	list, _, err := h.Store.ListStudentAttendanceForAdmin(r.Context(), store.StudentAttendanceFilter{
		SchoolID:       scopeFromRequest(claims, r),
		AcademicYearID: academicYearFromRequest(r),
		ClassroomID:    q.Get("classroom_id"),
		Date:           q.Get("date"),
		Month:          q.Get("month"),
		Page:           1,
		PerPage:        500,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="presensi_siswa.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"student_id", "classroom_id", "academic_year_id", "term_id", "status", "catatan", "created_at"})
	for _, sa := range list {
		term := ""
		if sa.TermID != nil {
			term = *sa.TermID
		}
		catatan := ""
		if sa.Catatan != nil {
			catatan = *sa.Catatan
		}
		_ = cw.Write([]string{sa.StudentID, sa.ClassroomID, sa.AcademicYearID, term, sa.Status, catatan, sa.CreatedAt.Format("2006-01-02 15:04:05")})
	}
	cw.Flush()
}

// ---- Presensi Guru (admin) ---------------------------------------------

// TeacherAttendanceAdmin serves the "Presensi Guru" admin view: per-guru
// attendance across sessions they taught, alongside their daily agenda
// entries for context (pertemuan keberapa, materi, catatan) — this is
// exactly what class_sessions already stores, so it's a filtered list of
// sessions scoped by guru_id.
func (h *Handler) TeacherAttendanceAdmin(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	// kepala_sekolah: principal views the school-wide teacher attendance
	// recap (read-only oversight), same as admin.
	if !requireRole(w, claims, "admin", "superadmin", "kepala_sekolah") {
		return
	}
	page, perPage := pageParams(r)
	q := r.URL.Query()
	list, total, err := h.Store.ListSessions(r.Context(), store.ListSessionsFilter{
		SchoolID:       scopeFromRequest(claims, r),
		AcademicYearID: academicYearFromRequest(r),
		TermID:         q.Get("term_id"),
		GuruID:         q.Get("guru_id"),
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

func (h *Handler) TeacherAttendanceExport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	q := r.URL.Query()
	list, _, err := h.Store.ListSessions(r.Context(), store.ListSessionsFilter{
		SchoolID:       scopeFromRequest(claims, r),
		AcademicYearID: academicYearFromRequest(r),
		TermID:         q.Get("term_id"),
		GuruID:         q.Get("guru_id"),
		Date:           q.Get("date"),
		Month:          q.Get("month"),
		Day:            normalizeDay(q.Get("day")),
		Page:           1,
		PerPage:        500,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="presensi_guru.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"guru_id", "classroom_id", "session_date", "checked_in_at", "pertemuan", "pembahasan", "materi_pertemuan", "bentuk_kegiatan"})
	for _, cs := range list {
		checkedIn := ""
		if cs.CheckedInAt != nil {
			checkedIn = cs.CheckedInAt.Format("2006-01-02 15:04:05")
		}
		pertemuan, pembahasan, materi := "", "", ""
		if cs.Pertemuan != nil {
			pertemuan = *cs.Pertemuan
		}
		if cs.Pembahasan != nil {
			pembahasan = *cs.Pembahasan
		}
		if cs.MateriPertemuan != nil {
			materi = *cs.MateriPertemuan
		}
		_ = cw.Write([]string{cs.GuruID, cs.ClassroomID, cs.SessionDate, checkedIn, pertemuan, pembahasan, materi, fmt.Sprintf("%v", cs.BentukKegiatan)})
	}
	cw.Flush()
}
