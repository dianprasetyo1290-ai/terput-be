package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/hr/internal/common"
	"skci/hr/internal/model"
	"skci/hr/internal/store"
)

// --- WORK SCHEDULES ---

func (a *App) ListWorkSchedules(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	schedules, err := store.ListWorkSchedules(r.Context(), a.DB, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat jadwal kerja")
		return
	}
	common.JSON(w, http.StatusOK, schedules)
}

func (a *App) CreateWorkSchedule(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var ws model.WorkSchedule
	if err := decodeJSON(r, &ws); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if strings.TrimSpace(ws.Name) == "" {
		common.Error(w, http.StatusBadRequest, "Nama jadwal kerja wajib diisi")
		return
	}
	ws.SchoolID = schoolID

	if err := store.CreateWorkSchedule(r.Context(), a.DB, &ws); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membuat jadwal kerja: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, ws)
}

func (a *App) UpdateWorkSchedule(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var ws model.WorkSchedule
	if err := decodeJSON(r, &ws); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	ws.ID = id
	ws.SchoolID = schoolID

	if err := store.UpdateWorkSchedule(r.Context(), a.DB, &ws); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui jadwal kerja")
		return
	}
	common.JSON(w, http.StatusOK, ws)
}

func (a *App) DeleteWorkSchedule(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteWorkSchedule(r.Context(), a.DB, schoolID, id); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus jadwal kerja")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Jadwal kerja berhasil dihapus"})
}

// --- HOLIDAYS ---

func (a *App) ListHolidays(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	year := 0
	if y := r.URL.Query().Get("year"); y != "" {
		if val, err := strconv.Atoi(y); err == nil {
			year = val
		}
	}
	if year == 0 {
		year = time.Now().Year()
	}

	holidays, err := store.ListHolidays(r.Context(), a.DB, schoolID, year)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat hari libur")
		return
	}
	common.JSON(w, http.StatusOK, holidays)
}

func (a *App) CreateHoliday(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var h model.Holiday
	if err := decodeJSON(r, &h); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if strings.TrimSpace(h.Date) == "" || strings.TrimSpace(h.Name) == "" {
		common.Error(w, http.StatusBadRequest, "Tanggal dan nama hari libur wajib diisi")
		return
	}
	h.SchoolID = schoolID

	if err := store.CreateHoliday(r.Context(), a.DB, &h); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyimpan hari libur: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, h)
}

func (a *App) DeleteHoliday(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteHoliday(r.Context(), a.DB, schoolID, id); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus hari libur")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Hari libur berhasil dihapus"})
}

// --- ATTENDANCE ---

func (a *App) ListAttendance(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	page, perPage := parsePagination(r)

	month := r.URL.Query().Get("month")
	empID := r.URL.Query().Get("employee_id")

	list, total, err := store.ListAttendance(r.Context(), a.DB, schoolID, month, empID, page, perPage)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat riwayat presensi")
		return
	}

	common.JSONWithMeta(w, http.StatusOK, list, common.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: (total + perPage - 1) / perPage,
	})
}

type UpdateAttendanceRequest struct {
	Status     string  `json:"status"`
	CheckInAt  *string `json:"check_in_at"`
	CheckOutAt *string `json:"check_out_at"`
	Reason     string  `json:"reason"`
}

func (a *App) UpdateAttendance(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	var req UpdateAttendanceRequest
	if err := decodeJSON(r, &req); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if strings.TrimSpace(req.Reason) == "" {
		common.Error(w, http.StatusBadRequest, "Alasan koreksi presensi wajib diisi")
		return
	}

	rec, err := store.GetAttendanceRecord(r.Context(), a.DB, schoolID, id)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data presensi tidak ditemukan")
		return
	}

	// Verify period is not locked (AC-86-04)
	month := rec.WorkDate
	if len(month) >= 7 {
		month = month[:7]
	}
	locked, _ := store.IsPeriodLocked(r.Context(), a.DB, schoolID, month)
	if locked {
		common.Error(w, http.StatusConflict, "Periode presensi ini sudah dikunci dan dikirim ke payroll")
		return
	}

	beforeBytes, _ := json.Marshal(rec)

	rec.Status = req.Status
	if req.CheckInAt != nil && *req.CheckInAt != "" {
		if t, err := time.Parse(time.RFC3339, *req.CheckInAt); err == nil {
			rec.CheckInAt = &t
		}
	}
	if req.CheckOutAt != nil && *req.CheckOutAt != "" {
		if t, err := time.Parse(time.RFC3339, *req.CheckOutAt); err == nil {
			rec.CheckOutAt = &t
		}
	}
	rec.Source = "manual"
	note := fmt.Sprintf("Koreksi manual: %s", req.Reason)
	rec.Note = &note

	if err := store.RecordAttendance(r.Context(), a.DB, rec); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui presensi")
		return
	}

	afterBytes, _ := json.Marshal(rec)
	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "employee_attendance", rec.ID, "correction", beforeBytes, afterBytes, req.Reason, nil)

	common.JSON(w, http.StatusOK, rec)
}

func (a *App) ImportAttendance(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	claims := common.GetClaims(r.Context())

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.Error(w, http.StatusBadRequest, "Gagal membaca form multipart")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		common.Error(w, http.StatusBadRequest, "File spreadsheet wajib diunggah")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membaca isi file")
		return
	}

	xl, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		common.Error(w, http.StatusBadRequest, "File bukan format Excel (.xlsx) yang valid")
		return
	}
	defer xl.Close()

	sheetList := xl.GetSheetList()
	if len(sheetList) == 0 {
		common.Error(w, http.StatusBadRequest, "Sheet Excel kosong")
		return
	}

	rows, err := xl.GetRows(sheetList[0])
	if err != nil {
		common.Error(w, http.StatusBadRequest, "Gagal membaca baris sheet")
		return
	}

	if len(rows) < 2 {
		common.Error(w, http.StatusBadRequest, "File harus memiliki header dan minimal 1 baris data")
		return
	}

	// Columns expected: Employee No (A), Date YYYY-MM-DD (B), Status (C), Check-In HH:mm (D), Check-Out HH:mm (E)
	type RowError struct {
		Row   int    `json:"row"`
		Error string `json:"error"`
	}

	var rowErrors []RowError
	successCount := 0

	for idx, row := range rows {
		if idx == 0 {
			continue // skip header
		}
		rowNum := idx + 1
		if len(row) < 3 {
			rowErrors = append(rowErrors, RowError{Row: rowNum, Error: "Kolom tidak lengkap (minimal No. Pegawai, Tanggal, Status)"})
			continue
		}

		empNo := strings.TrimSpace(row[0])
		dateStr := strings.TrimSpace(row[1])
		statusStr := strings.TrimSpace(strings.ToLower(row[2]))

		if empNo == "" || dateStr == "" || statusStr == "" {
			rowErrors = append(rowErrors, RowError{Row: rowNum, Error: "No. Pegawai, Tanggal, dan Status tidak boleh kosong"})
			continue
		}

		// Find employee
		var empID string
		err := a.DB.QueryRowContext(r.Context(), `SELECT id FROM employees WHERE school_id = $1 AND employee_no = $2 AND deleted_at IS NULL`, schoolID, empNo).Scan(&empID)
		if err != nil {
			rowErrors = append(rowErrors, RowError{Row: rowNum, Error: fmt.Sprintf("Pegawai no '%s' tidak ditemukan", empNo)})
			continue
		}

		// Check period lock
		month := dateStr
		if len(month) >= 7 {
			month = month[:7]
		}
		if locked, _ := store.IsPeriodLocked(r.Context(), a.DB, schoolID, month); locked {
			rowErrors = append(rowErrors, RowError{Row: rowNum, Error: fmt.Sprintf("Periode %s sudah dikunci", month)})
			continue
		}

		var checkIn, checkOut *time.Time
		if len(row) > 3 && strings.TrimSpace(row[3]) != "" {
			if t, err := time.Parse("15:04", strings.TrimSpace(row[3])); err == nil {
				wD, _ := time.Parse("2006-01-02", dateStr)
				full := time.Date(wD.Year(), wD.Month(), wD.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
				checkIn = &full
			}
		}
		if len(row) > 4 && strings.TrimSpace(row[4]) != "" {
			if t, err := time.Parse("15:04", strings.TrimSpace(row[4])); err == nil {
				wD, _ := time.Parse("2006-01-02", dateStr)
				full := time.Date(wD.Year(), wD.Month(), wD.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
				checkOut = &full
			}
		}

		note := "Import mesin presensi"
		att := model.EmployeeAttendance{
			SchoolID:   schoolID,
			EmployeeID: empID,
			WorkDate:   dateStr,
			Status:     statusStr,
			CheckInAt:  checkIn,
			CheckOutAt: checkOut,
			Source:     "import",
			Note:       &note,
		}

		if err := store.RecordAttendance(r.Context(), a.DB, &att); err != nil {
			rowErrors = append(rowErrors, RowError{Row: rowNum, Error: err.Error()})
			continue
		}
		successCount++
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "employee_attendance", "", "import", nil, nil, fmt.Sprintf("Import presensi: %d berhasil, %d galat", successCount, len(rowErrors)), nil)

	common.JSON(w, http.StatusOK, map[string]any{
		"imported": successCount,
		"errors":   rowErrors,
	})
}

func (a *App) GetAttendanceRecap(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	month := r.URL.Query().Get("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}

	recap, err := store.GetAttendanceRecap(r.Context(), a.DB, schoolID, month)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat rekap presensi")
		return
	}
	common.JSON(w, http.StatusOK, recap)
}

func (a *App) SendAttendancePeriod(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	period := chi.URLParam(r, "period")
	claims := common.GetClaims(r.Context())

	if strings.TrimSpace(period) == "" {
		common.Error(w, http.StatusBadRequest, "Periode bulan wajib diisi (YYYY-MM)")
		return
	}

	var actorID string
	if claims != nil && claims.Subject != "" {
		actorID = claims.Subject
	}

	if err := store.SendAttendancePeriod(r.Context(), a.DB, schoolID, period, actorID); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mengunci periode presensi: "+err.Error())
		return
	}

	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, &actorID, "attendance_periods", period, "lock", nil, nil, "Kunci periode presensi dan kirim ke payroll", nil)

	common.JSON(w, http.StatusOK, map[string]string{
		"message": "Periode presensi berhasil dikunci dan dikirim ke payroll",
		"period":  period,
	})
}

// --- HR SETTINGS ---

func (a *App) GetHRSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	settings, err := store.GetHRSettings(r.Context(), a.DB, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat pengaturan HR")
		return
	}
	common.JSON(w, http.StatusOK, settings)
}

func (a *App) UpdateHRSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var body struct {
		model.HRSettings
		SchoolLat       *float64 `json:"school_lat"`
		SchoolLng       *float64 `json:"school_lng"`
		GeofenceRadiusM *float64 `json:"geofence_radius_m"`
	}
	if err := decodeJSON(r, &body); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	settings := body.HRSettings
	settings.SchoolID = schoolID

	if body.SchoolLat != nil && body.SchoolLng != nil && body.GeofenceRadiusM != nil && *body.GeofenceRadiusM > 0 {
		geo := model.GeofenceConfig{
			Lat:     *body.SchoolLat,
			Lng:     *body.SchoolLng,
			RadiusM: *body.GeofenceRadiusM,
		}
		geoBytes, _ := json.Marshal(geo)
		settings.Geofence = geoBytes
		settings.GeolocationEnabled = true
	}

	if err := store.UpdateHRSettings(r.Context(), a.DB, &settings); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui pengaturan HR")
		return
	}
	common.JSON(w, http.StatusOK, settings)
}
