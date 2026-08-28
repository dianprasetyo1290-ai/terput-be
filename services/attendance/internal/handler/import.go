// services/attendance/internal/handler/import.go
//
// Admin Excel import of historical attendance (client revision, GPM School
// 2026-08-24, item 1): "does the school have to import every day?" -- no.
// AdminAttendanceImportTemplate hands back a ready-to-fill .xlsx for one
// classroom/month (one row per roster student, one column per day);
// AdminAttendanceImport reads it back, matching students by exact name
// against the same roster, and upserts one student_attendances row per
// non-empty day cell via the store functions from Task 7.
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"skci/attendance/internal/common"
)

// attendanceStatusCodes: the single-letter codes schools fill into the
// template, matching the Indonesian school convention (Hadir/Sakit/Izin/
// Alpa) rather than asking them to type the full enum value.
var attendanceStatusCodes = map[string]string{
	"H": "hadir", "S": "sakit", "I": "izin", "A": "tanpa_keterangan",
}

func daysInMonth(month string) (int, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return 0, fmt.Errorf("month must be YYYY-MM")
	}
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day(), nil
}

// AdminAttendanceImportTemplate: GET /admin/student-attendance/import-template
// ?classroom_id=&month=YYYY-MM
func (h *Handler) AdminAttendanceImportTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	q := r.URL.Query()
	classroomID := q.Get("classroom_id")
	month := q.Get("month")
	if classroomID == "" || month == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id and month are required")
		return
	}
	days, err := daysInMonth(month)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	roster, err := h.Client.GetRoster(r.Context(), classroomID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "could not load classroom roster")
		return
	}

	f := excelize.NewFile()
	sheet := "Presensi"
	f.SetSheetName(f.GetSheetName(0), sheet)
	f.SetCellValue(sheet, "A1", "Nama Siswa")
	for d := 1; d <= days; d++ {
		col, _ := excelize.ColumnNumberToName(d + 1)
		f.SetCellValue(sheet, col+"1", strconv.Itoa(d))
	}
	for i, s := range roster {
		row := i + 2
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), s.Name)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=presensi_%s_%s.xlsx", classroomID, month))
	_ = f.Write(w)
}

// AdminAttendanceImport: POST /admin/student-attendance/import -- multipart
// form fields classroom_id, academic_year_id, month=YYYY-MM, term_id
// (optional), file=.xlsx matching the template above. Aborts on the first
// unrecognized student name or status code (matches exam-service's
// ImportQuestions convention: fail fast with a row-numbered message rather
// than partially importing).
func (h *Handler) AdminAttendanceImport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}
	classroomID := r.FormValue("classroom_id")
	month := r.FormValue("month")
	termID := r.FormValue("term_id")
	academicYearID := r.FormValue("academic_year_id")
	if classroomID == "" || month == "" || academicYearID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id, academic_year_id and month are required")
		return
	}
	days, err := daysInMonth(month)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required (multipart field 'file')")
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "could not parse xlsx: "+err.Error())
		return
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		common.WriteError(w, http.StatusBadRequest, "file has no sheets")
		return
	}
	records, err := f.GetRows(sheets[0])
	if err != nil || len(records) < 2 {
		common.WriteError(w, http.StatusBadRequest, "file must contain a header row plus at least one data row")
		return
	}

	roster, err := h.Client.GetRoster(r.Context(), classroomID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "could not load classroom roster")
		return
	}
	byName := map[string]string{}       // lower(trim(name)) -> student_id
	ambiguousNames := map[string]bool{} // names shared by 2+ students in this classroom
	for _, s := range roster {
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if _, seen := byName[key]; seen {
			ambiguousNames[key] = true
		}
		byName[key] = s.StudentID
	}

	header := records[0]
	dayCol := map[int]int{} // day-of-month -> column index
	for i, cell := range header {
		d, err := strconv.Atoi(strings.TrimSpace(cell))
		if err == nil && d >= 1 && d <= days {
			dayCol[d] = i
		}
	}

	schoolID := scopeFromRequest(claims, r)
	imported, skipped := 0, 0
	sessionIDCache := map[int]string{} // day -> class_session id, one find-or-create per day not per row

	for rowNum, row := range records[1:] {
		if len(row) == 0 || strings.TrimSpace(row[0]) == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(row[0]))
		if ambiguousNames[key] {
			common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("row %d: ada lebih dari satu siswa bernama %q di kelas ini, import berdasarkan nama tidak bisa dipakai -- hubungi admin", rowNum+2, row[0]))
			return
		}
		studentID, ok := byName[key]
		if !ok {
			common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("row %d: siswa %q tidak ditemukan di kelas ini", rowNum+2, row[0]))
			return
		}
		for day, col := range dayCol {
			if col >= len(row) {
				continue
			}
			code := strings.ToUpper(strings.TrimSpace(row[col]))
			if code == "" {
				skipped++
				continue
			}
			status, ok := attendanceStatusCodes[code]
			if !ok {
				common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("row %d, tanggal %d: kode %q tidak dikenal (pakai H/S/I/A)", rowNum+2, day, code))
				return
			}
			sessionDate := fmt.Sprintf("%s-%02d", month, day)
			sessionID, cached := sessionIDCache[day]
			if !cached {
				sessionID, err = h.Store.FindOrCreateImportSession(r.Context(), schoolID, academicYearID, termID, classroomID, sessionDate, claims.UserID)
				if err != nil {
					common.WriteError(w, http.StatusInternalServerError, err.Error())
					return
				}
				sessionIDCache[day] = sessionID
			}
			if err := h.Store.UpsertStudentAttendanceImport(r.Context(), sessionID, schoolID, academicYearID, termID, classroomID, studentID, sessionDate, status, claims.UserID); err != nil {
				common.WriteError(w, http.StatusInternalServerError, err.Error())
				return
			}
			imported++
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"imported_count": imported,
		"skipped_count":  skipped,
	}, nil)
}
