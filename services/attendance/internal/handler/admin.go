package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"

	"skci/attendance/internal/common"
	"skci/attendance/internal/store"
)

// ---- Presensi Siswa (admin) --------------------------------------------

// StudentAttendanceAdmin serves the "Presensi Siswa" admin view: per
// classroom, filterable by academic year (including past years), bulan,
// hari, tanggal.
func (h *Handler) StudentAttendanceAdmin(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	page, perPage := pageParams(r)
	q := r.URL.Query()
	list, total, err := h.Store.ListStudentAttendanceForAdmin(r.Context(), store.StudentAttendanceFilter{
		SchoolID:       scopeFromRequest(claims, r),
		AcademicYearID: academicYearFromRequest(r),
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
		Day:            normalizeDay(q.Get("day")),
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
