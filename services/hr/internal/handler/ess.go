package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/client"
	"skci/hr/internal/common"
	"skci/hr/internal/model"
	"skci/hr/internal/store"
)

// getMeEmployee finds the Employee record linked to the caller's JWT user ID.
func (a *App) getMeEmployee(r *http.Request) (*model.Employee, error) {
	claims := common.GetClaims(r.Context())
	if claims == nil || claims.Subject == "" {
		return nil, fmt.Errorf("sesi tidak valid")
	}
	schoolID := a.effectiveSchoolID(r)
	return store.GetEmployeeByUserID(r.Context(), a.DB, schoolID, claims.Subject)
}

// callerID is the JWT user id. ESS employees are looked up by it, so it is
// both the maker and the subject of their approvals.
func callerID(r *http.Request) string {
	if claims := common.GetClaims(r.Context()); claims != nil {
		return claims.Subject
	}
	return ""
}

func (a *App) MeEmployee(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai Anda tidak ditemukan dalam sistem HRIS")
		return
	}
	common.JSON(w, http.StatusOK, a.maskEmployee(emp, true))
}

func (a *App) MeDashboard(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	dashboard, err := store.GetESSDashboard(r.Context(), a.DB, schoolID, emp.ID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat dashboard pegawai")
		return
	}

	common.JSON(w, http.StatusOK, dashboard)
}

type CheckInRequest struct {
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
	Note *string `json:"note"`
}

func (a *App) MeCheckIn(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	var req CheckInRequest
	if err := decodeJSON(r, &req); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	today := time.Now().Format("2006-01-02")
	month := today[:7]

	if locked, _ := store.IsPeriodLocked(r.Context(), a.DB, schoolID, month); locked {
		common.Error(w, http.StatusConflict, "Periode presensi sudah dikunci")
		return
	}

	// Geofence check from HR settings
	settings, _ := store.GetHRSettings(r.Context(), a.DB, schoolID)
	if settings != nil && settings.GeolocationEnabled && len(settings.Geofence) > 0 {
		var geo model.GeofenceConfig
		if err := json.Unmarshal(settings.Geofence, &geo); err == nil && geo.RadiusM > 0 {
			dist := model.DistanceM(req.Lat, req.Lng, geo.Lat, geo.Lng)
			if dist > geo.RadiusM {
				common.Error(w, http.StatusBadRequest, fmt.Sprintf("Presensi ditolak: Anda berada di luar radius sekolah (jarak: %.0fm, maks: %.0fm)", dist, geo.RadiusM))
				return
			}
		}
	}

	loc := time.FixedZone("WIB", 7*3600)
	if settings != nil && settings.Timezone != "" {
		if l, err := time.LoadLocation(settings.Timezone); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)

	// Determine schedule
	scheds, _ := store.ListWorkSchedules(r.Context(), a.DB, schoolID)
	sched := model.WorkSchedule{
		StartTime:            "07:30",
		LateToleranceMinutes: 15,
	}
	if len(scheds) > 0 {
		sched = scheds[0]
	}
	status := model.AttendanceStatus(now, sched, loc)

	att := model.EmployeeAttendance{
		SchoolID:   schoolID,
		EmployeeID: emp.ID,
		WorkDate:   today,
		Status:     status,
		CheckInAt:  &now,
		Lat:        &req.Lat,
		Lng:        &req.Lng,
		Source:     "ess",
		Note:       req.Note,
	}

	if err := store.RecordAttendance(r.Context(), a.DB, &att); err != nil {
		common.InternalError(w, "Gagal mencatat presensi masuk", err)
		return
	}

	common.JSON(w, http.StatusOK, att)
}

func (a *App) MeCheckOut(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	today := time.Now().Format("2006-01-02")
	month := today[:7]

	if locked, _ := store.IsPeriodLocked(r.Context(), a.DB, schoolID, month); locked {
		common.Error(w, http.StatusConflict, "Periode presensi sudah dikunci")
		return
	}

	att, err := store.GetAttendanceByEmployeeAndDate(r.Context(), a.DB, schoolID, emp.ID, today)
	if err != nil || att == nil {
		common.Error(w, http.StatusBadRequest, "Anda belum melakukan presensi masuk hari ini")
		return
	}

	now := time.Now()
	att.CheckOutAt = &now

	if err := store.RecordAttendance(r.Context(), a.DB, att); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mencatat presensi pulang")
		return
	}

	common.JSON(w, http.StatusOK, att)
}

func (a *App) MeListLeaveRequests(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)
	page, perPage := parsePagination(r)
	status := r.URL.Query().Get("status")

	reqs, total, err := store.ListLeaveRequests(r.Context(), a.DB, schoolID, emp.ID, status, page, perPage)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat permohonan cuti")
		return
	}

	common.JSONWithMeta(w, http.StatusOK, reqs, common.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: (total + perPage - 1) / perPage,
	})
}

type CreateLeaveRequestBody struct {
	LeaveTypeID    string  `json:"leave_type_id"`
	StartDate      string  `json:"start_date"`
	EndDate        string  `json:"end_date"`
	Reason         *string `json:"reason"`
	AttachmentKey  *string `json:"attachment_key"`
	IdempotencyKey string  `json:"idempotency_key"`
}

func (a *App) MeCreateLeaveRequest(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	var body CreateLeaveRequestBody
	if err := decodeJSON(r, &body); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if body.LeaveTypeID == "" || body.StartDate == "" || body.EndDate == "" {
		common.Error(w, http.StatusBadRequest, "Jenis cuti, tanggal mulai, dan tanggal selesai wajib diisi")
		return
	}

	start, err := time.Parse("2006-01-02", body.StartDate)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "Format tanggal mulai tidak valid (harus YYYY-MM-DD)")
		return
	}
	end, err := time.Parse("2006-01-02", body.EndDate)
	if err != nil {
		common.Error(w, http.StatusBadRequest, "Format tanggal selesai tidak valid (harus YYYY-MM-DD)")
		return
	}

	// Calculate working days
	holidays, _ := store.ListHolidays(r.Context(), a.DB, schoolID, start.Year())
	holidayMap := make(map[string]bool)
	for _, h := range holidays {
		holidayMap[h.Date] = true
	}

	scheds, _ := store.ListWorkSchedules(r.Context(), a.DB, schoolID)
	schedDays := []int16{1, 2, 3, 4, 5}
	if len(scheds) > 0 && len(scheds[0].Days) > 0 {
		schedDays = scheds[0].Days
	}

	days, err := model.WorkingDays(start, end, schedDays, holidayMap)
	if err != nil || days <= 0 {
		common.Error(w, http.StatusBadRequest, "Rentang tanggal cuti tidak memiliki hari kerja aktif")
		return
	}

	// Quota check
	lt, err := store.GetLeaveType(r.Context(), a.DB, schoolID, body.LeaveTypeID)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Jenis cuti tidak ditemukan")
		return
	}

	if lt.HasQuota {
		bal, err := store.GetLeaveBalance(r.Context(), a.DB, schoolID, emp.ID, lt.ID, start.Year())
		if err != nil {
			common.Error(w, http.StatusBadRequest, "Saldo cuti belum terbentuk untuk tahun ini")
			return
		}
		if ok, reason := model.CanTakeLeave(*bal, days, lt.HasQuota); !ok {
			common.Error(w, http.StatusBadRequest, "Pengajuan cuti ditolak: "+reason)
			return
		}
	}

	idemKey := body.IdempotencyKey
	if idemKey == "" {
		idemKey = fmt.Sprintf("leave-%s-%s-%s", emp.ID, body.StartDate, body.EndDate)
	}

	lr := model.LeaveRequest{
		SchoolID:       schoolID,
		EmployeeID:     emp.ID,
		LeaveTypeID:    body.LeaveTypeID,
		StartDate:      body.StartDate,
		EndDate:        body.EndDate,
		Days:           float64(days),
		Reason:         body.Reason,
		AttachmentKey:  body.AttachmentKey,
		IdempotencyKey: idemKey,
	}

	// The request and its core approval are created together: if core refuses
	// (e.g. flow not configured, 422) nothing is stored and the message is
	// returned to the user, never auto-approved (T2 §approval).
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.InternalError(w, "Gagal membuat permohonan cuti", err)
		return
	}
	defer tx.Rollback()
	if err := store.CreateLeaveRequest(r.Context(), tx, &lr); err != nil {
		common.InternalError(w, "Gagal membuat permohonan cuti", err)
		return
	}
	appID, err := a.Client.CreateApprovalRequest(r.Context(), schoolID, callerID(r), callerID(r),
		client.ReqTypeLeave, lr.ID, fmt.Sprintf("Pengajuan cuti %g hari oleh %s", days, emp.FullName))
	if err != nil {
		log.Printf("leave request %s: create approval: %v", lr.ID, err)
		status, msg := client.ApprovalErrorMessage(err)
		common.Error(w, status, msg)
		return
	}
	if err := store.UpdateLeaveRequestApproval(r.Context(), tx, lr.ID, appID); err != nil {
		common.InternalError(w, "Gagal menyimpan approval cuti", err)
		return
	}
	if err := tx.Commit(); err != nil {
		common.InternalError(w, "Gagal menyimpan permohonan cuti", err)
		return
	}
	lr.ApprovalID = &appID

	common.JSON(w, http.StatusCreated, lr)
}

func (a *App) MeCancelLeaveRequest(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	id := chi.URLParam(r, "id")
	schoolID := a.effectiveSchoolID(r)

	lr, err := store.GetLeaveRequest(r.Context(), a.DB, schoolID, id)
	if err != nil || lr.EmployeeID != emp.ID {
		common.Error(w, http.StatusNotFound, "Permohonan cuti tidak ditemukan")
		return
	}

	if lr.Status != "pending" && lr.Status != "approved" {
		common.Error(w, http.StatusBadRequest, "Permohonan cuti dengan status ini tidak dapat dibatalkan")
		return
	}

	if err := store.CancelLeaveRequestTx(r.Context(), a.DB, id); err != nil {
		common.InternalError(w, "Gagal membatalkan cuti", err)
		return
	}

	common.JSON(w, http.StatusOK, map[string]string{"message": "Permohonan cuti berhasil dibatalkan"})
}

func (a *App) MeListLeaveBalances(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)
	year := time.Now().Year()
	if y := r.URL.Query().Get("year"); y != "" {
		if val, err := strconv.Atoi(y); err == nil {
			year = val
		}
	}

	balances, err := store.ListLeaveBalances(r.Context(), a.DB, schoolID, emp.ID, year)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat saldo cuti")
		return
	}
	common.JSON(w, http.StatusOK, balances)
}

func (a *App) MeListOvertimeRequests(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)
	page, perPage := parsePagination(r)
	status := r.URL.Query().Get("status")

	reqs, total, err := store.ListOvertimeRequests(r.Context(), a.DB, schoolID, emp.ID, status, page, perPage)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat pengajuan lembur")
		return
	}

	common.JSONWithMeta(w, http.StatusOK, reqs, common.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: (total + perPage - 1) / perPage,
	})
}

type CreateOvertimeRequestBody struct {
	WorkDate       string `json:"work_date"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
	Minutes        int    `json:"minutes"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (a *App) MeCreateOvertimeRequest(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	var body CreateOvertimeRequestBody
	if err := decodeJSON(r, &body); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if body.WorkDate == "" || body.StartAt == "" || body.EndAt == "" || body.Minutes <= 0 {
		common.Error(w, http.StatusBadRequest, "Data pengajuan lembur tidak lengkap")
		return
	}

	idemKey := body.IdempotencyKey
	if idemKey == "" {
		idemKey = fmt.Sprintf("ot-%s-%s-%d", emp.ID, body.WorkDate, time.Now().Unix())
	}

	startAt, err := time.Parse(time.RFC3339, body.StartAt)
	if err != nil {
		startAt, err = time.Parse("2006-01-02 15:04:05", body.StartAt)
		if err != nil {
			startAt, _ = time.Parse("2006-01-02 15:04", body.StartAt)
		}
	}
	endAt, err := time.Parse(time.RFC3339, body.EndAt)
	if err != nil {
		endAt, err = time.Parse("2006-01-02 15:04:05", body.EndAt)
		if err != nil {
			endAt, _ = time.Parse("2006-01-02 15:04", body.EndAt)
		}
	}

	ot := model.OvertimeRequest{
		SchoolID:       schoolID,
		EmployeeID:     emp.ID,
		WorkDate:       body.WorkDate,
		StartAt:        startAt,
		EndAt:          endAt,
		Minutes:        body.Minutes,
		Reason:         body.Reason,
		IdempotencyKey: idemKey,
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.InternalError(w, "Gagal mengajukan lembur", err)
		return
	}
	defer tx.Rollback()
	if err := store.CreateOvertimeRequest(r.Context(), tx, &ot); err != nil {
		common.InternalError(w, "Gagal mengajukan lembur", err)
		return
	}
	appID, err := a.Client.CreateApprovalRequest(r.Context(), schoolID, callerID(r), callerID(r),
		client.ReqTypeOvertime, ot.ID, fmt.Sprintf("Pengajuan lembur %d menit (%s) oleh %s", ot.Minutes, ot.WorkDate, emp.FullName))
	if err != nil {
		log.Printf("overtime request %s: create approval: %v", ot.ID, err)
		status, msg := client.ApprovalErrorMessage(err)
		common.Error(w, status, msg)
		return
	}
	if err := store.UpdateOvertimeRequestApproval(r.Context(), tx, ot.ID, appID); err != nil {
		common.InternalError(w, "Gagal menyimpan approval lembur", err)
		return
	}
	if err := tx.Commit(); err != nil {
		common.InternalError(w, "Gagal menyimpan pengajuan lembur", err)
		return
	}
	ot.ApprovalID = &appID

	common.JSON(w, http.StatusCreated, ot)
}

func (a *App) MeListProfileChangeRequests(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	reqs, err := store.ListProfileChangeRequests(r.Context(), a.DB, schoolID, emp.ID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat riwayat pengajuan profil")
		return
	}
	common.JSON(w, http.StatusOK, reqs)
}

type CreateProfileChangeRequestBody struct {
	Field       string `json:"field"`
	NewValue    string `json:"new_value"`
	Reason      string `json:"reason"`
	DocumentKey string `json:"document_key"`
}

func (a *App) MeCreateProfileChangeRequest(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	var body CreateProfileChangeRequestBody
	if err := decodeJSON(r, &body); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if body.Field != "address" && body.Field != "phone" && body.Field != "bank_account" && body.Field != "ptkp_status" {
		common.Error(w, http.StatusBadRequest, "Bidang yang diubah tidak didukung (hanya address, phone, bank_account, ptkp_status)")
		return
	}

	var valEnc []byte
	if body.Field == "ptkp_status" {
		valEnc = []byte(body.NewValue)
	} else {
		valEnc, _ = a.Crypto.Encrypt(body.NewValue, schoolID)
	}

	pcr := model.ProfileChangeRequest{
		SchoolID:    schoolID,
		EmployeeID:  emp.ID,
		Field:       body.Field,
		NewValueEnc: valEnc,
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.InternalError(w, "Gagal menyimpan pengajuan perubahan profil", err)
		return
	}
	defer tx.Rollback()
	if err := store.CreateProfileChangeRequest(r.Context(), tx, &pcr); err != nil {
		common.InternalError(w, "Gagal menyimpan pengajuan perubahan profil", err)
		return
	}
	appID, err := a.Client.CreateApprovalRequest(r.Context(), schoolID, callerID(r), callerID(r),
		client.ReqTypeProfileChange, pcr.ID, fmt.Sprintf("Perubahan data %s oleh %s", body.Field, emp.FullName))
	if err != nil {
		log.Printf("profile change %s: create approval: %v", pcr.ID, err)
		status, msg := client.ApprovalErrorMessage(err)
		common.Error(w, status, msg)
		return
	}
	if err := store.UpdateProfileChangeRequestApproval(r.Context(), tx, pcr.ID, appID); err != nil {
		common.InternalError(w, "Gagal menyimpan approval perubahan profil", err)
		return
	}
	if err := tx.Commit(); err != nil {
		common.InternalError(w, "Gagal menyimpan pengajuan perubahan profil", err)
		return
	}
	pcr.ApprovalID = &appID

	common.JSON(w, http.StatusCreated, pcr)
}

func (a *App) MeListDocuments(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	docs, err := store.ListEmployeeDocuments(r.Context(), a.DB, schoolID, emp.ID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat dokumen Anda")
		return
	}
	common.JSON(w, http.StatusOK, docs)
}

func (a *App) MeUploadDocument(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai tidak ditemukan")
		return
	}
	schoolID := a.effectiveSchoolID(r)

	if err := r.ParseMultipartForm(15 << 20); err != nil {
		common.Error(w, http.StatusBadRequest, "Gagal membaca form multipart")
		return
	}

	docTypeID := r.FormValue("document_type_id")
	if docTypeID == "" {
		common.Error(w, http.StatusBadRequest, "Jenis dokumen wajib dipilih")
		return
	}
	expiryDate := r.FormValue("expiry_date")

	file, header, err := r.FormFile("file")
	if err != nil {
		common.Error(w, http.StatusBadRequest, "File dokumen wajib diunggah")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membaca konten file")
		return
	}

	storageKey := fmt.Sprintf("docs/%s/%s/%d-%s", schoolID, emp.ID, time.Now().Unix(), header.Filename)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if _, err := a.Storage.Put(r.Context(), storageKey, contentType, bytes.NewReader(data), header.Size); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyimpan file ke penyimpanan")
		return
	}

	doc := model.EmployeeDocument{
		SchoolID:       schoolID,
		EmployeeID:     emp.ID,
		DocumentTypeID: docTypeID,
		ObjectKey:      storageKey,
		FileName:       header.Filename,
		SizeBytes:      header.Size,
		MIME:           contentType,
	}
	if expiryDate != "" {
		doc.ExpiresOn = &expiryDate
	}
	claims := common.GetClaims(r.Context())
	if claims != nil && claims.Subject != "" {
		doc.UploadedBy = &claims.Subject
	}

	if err := store.CreateEmployeeDocument(r.Context(), a.DB, &doc); err != nil {
		_ = a.Storage.Delete(r.Context(), storageKey)
		common.Error(w, http.StatusInternalServerError, "Gagal mencatat dokumen di database")
		return
	}

	common.JSON(w, http.StatusCreated, doc)
}

// MeListAttendance lists the caller's own attendance records. ESS users must
// not hit the HR-admin /attendance list (which takes any employee_id).
func (a *App) MeListAttendance(w http.ResponseWriter, r *http.Request) {
	emp, err := a.getMeEmployee(r)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Data pegawai Anda tidak ditemukan dalam sistem HRIS")
		return
	}
	page, perPage := parsePagination(r)
	list, total, err := store.ListAttendance(r.Context(), a.DB, emp.SchoolID, r.URL.Query().Get("month"), emp.ID, page, perPage)
	if err != nil {
		common.InternalError(w, "Gagal memuat riwayat presensi", err)
		return
	}
	common.JSONWithMeta(w, http.StatusOK, list, common.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: (total + perPage - 1) / perPage,
	})
}
