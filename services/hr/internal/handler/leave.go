package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/common"
	"skci/hr/internal/model"
	"skci/hr/internal/store"
)

// --- LEAVE TYPES ---

func (a *App) ListLeaveTypes(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	types, err := store.ListLeaveTypes(r.Context(), a.DB, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat jenis cuti")
		return
	}
	common.JSON(w, http.StatusOK, types)
}

func (a *App) CreateLeaveType(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var lt model.LeaveType
	if err := decodeJSON(r, &lt); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if strings.TrimSpace(lt.Code) == "" || strings.TrimSpace(lt.Name) == "" {
		common.Error(w, http.StatusBadRequest, "Kode dan nama jenis cuti wajib diisi")
		return
	}
	lt.SchoolID = schoolID

	if err := store.CreateLeaveType(r.Context(), a.DB, &lt); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membuat jenis cuti: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, lt)
}

func (a *App) UpdateLeaveType(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var lt model.LeaveType
	if err := decodeJSON(r, &lt); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	lt.ID = id
	lt.SchoolID = schoolID

	if err := store.UpdateLeaveType(r.Context(), a.DB, &lt); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui jenis cuti")
		return
	}
	common.JSON(w, http.StatusOK, lt)
}

func (a *App) DeleteLeaveType(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteLeaveType(r.Context(), a.DB, schoolID, id); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus jenis cuti")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Jenis cuti berhasil dihapus"})
}

// --- LEAVE BALANCES ---

func (a *App) ListLeaveBalances(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empID := r.URL.Query().Get("employee_id")
	year := 0
	if y := r.URL.Query().Get("year"); y != "" {
		if val, err := strconv.Atoi(y); err == nil {
			year = val
		}
	}
	if year == 0 {
		year = time.Now().Year()
	}

	balances, err := store.ListLeaveBalances(r.Context(), a.DB, schoolID, empID, year)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat saldo cuti")
		return
	}
	common.JSON(w, http.StatusOK, balances)
}

// --- LEAVE REQUESTS ---

func (a *App) ListLeaveRequests(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	page, perPage := parsePagination(r)

	empID := r.URL.Query().Get("employee_id")
	status := r.URL.Query().Get("status")

	reqs, total, err := store.ListLeaveRequests(r.Context(), a.DB, schoolID, empID, status, page, perPage)
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

func (a *App) AdminApproveLeaveRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	// Apply approval decision (BR-HR-04, AC-86-02)
	if err := store.ApplyLeaveDecisionTx(r.Context(), a.DB, id, "approved"); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyetujui permohonan cuti: "+err.Error())
		return
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	schoolID := a.effectiveSchoolID(r)
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "leave_requests", id, "approve", nil, nil, "Persetujuan cuti oleh Admin", nil)

	common.JSON(w, http.StatusOK, map[string]string{"message": "Permohonan cuti berhasil disetujui"})
}

func (a *App) AdminRejectLeaveRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	if err := store.ApplyLeaveDecisionTx(r.Context(), a.DB, id, "rejected"); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menolak permohonan cuti: "+err.Error())
		return
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	schoolID := a.effectiveSchoolID(r)
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "leave_requests", id, "reject", nil, nil, "Penolakan cuti oleh Admin", nil)

	common.JSON(w, http.StatusOK, map[string]string{"message": "Permohonan cuti telah ditolak"})
}

// --- OVERTIME REQUESTS ---

func (a *App) ListOvertimeRequests(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	page, perPage := parsePagination(r)

	empID := r.URL.Query().Get("employee_id")
	status := r.URL.Query().Get("status")

	reqs, total, err := store.ListOvertimeRequests(r.Context(), a.DB, schoolID, empID, status, page, perPage)
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

func (a *App) AdminApproveOvertimeRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	if err := store.ApplyOvertimeDecisionTx(r.Context(), a.DB, id, "approved"); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyetujui lembur: "+err.Error())
		return
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	schoolID := a.effectiveSchoolID(r)
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "overtime_requests", id, "approve", nil, nil, "Persetujuan lembur oleh Admin", nil)

	common.JSON(w, http.StatusOK, map[string]string{"message": "Pengajuan lembur berhasil disetujui"})
}

func (a *App) AdminRejectOvertimeRequest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	if err := store.ApplyOvertimeDecisionTx(r.Context(), a.DB, id, "rejected"); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menolak lembur: "+err.Error())
		return
	}

	var actorID *string
	if claims != nil && claims.Subject != "" {
		actorID = &claims.Subject
	}
	schoolID := a.effectiveSchoolID(r)
	_ = store.WriteAuditEvent(r.Context(), a.DB, schoolID, actorID, "overtime_requests", id, "reject", nil, nil, "Penolakan lembur oleh Admin", nil)

	common.JSON(w, http.StatusOK, map[string]string{"message": "Pengajuan lembur telah ditolak"})
}
