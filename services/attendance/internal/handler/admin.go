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
	if !requireRole(w, claims, "admin", "superadmin", "walikelas") {
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
	if !requireRole(w, claims, "admin", "superadmin", "walikelas") {
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
