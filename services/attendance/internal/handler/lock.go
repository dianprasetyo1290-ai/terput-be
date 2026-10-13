package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
)

// FR-41-003 / BR-ATT-01 [per klien]: per-school daily attendance lock.
// Every direct write path calls ensureUnlocked before touching
// student_attendances; once a date is locked, a change has to go through a
// correction request (corrections.go) approved in core's Approval Center.

type lockSettingsReader interface {
	GetLockSettings(ctx context.Context, schoolID string) (*model.LockSettings, error)
}

// lockedMessage is the user-facing refusal (409) for a locked date.
func lockedMessage(date string) string {
	return "Presensi tanggal " + date + " sudah dikunci. Ajukan koreksi presensi untuk mengubahnya (perlu persetujuan)."
}

// dateLocked reports whether attendance for date at schoolID is locked now.
// schoolID "" (superadmin without scope) never locks — there's no client
// configuration to apply.
func (h *Handler) dateLocked(ctx context.Context, schoolID, date string) (bool, error) {
	if schoolID == "" || h.lockSettings == nil {
		return false, nil
	}
	cfg, err := h.lockSettings.GetLockSettings(ctx, schoolID)
	if err != nil {
		return false, err
	}
	return cfg.IsLocked(date, h.clock(), wibLocation), nil
}

// ensureUnlocked writes 409 (locked) / 500 and returns false when the write
// must not proceed.
func (h *Handler) ensureUnlocked(w http.ResponseWriter, r *http.Request, schoolID, date string) bool {
	locked, err := h.dateLocked(r.Context(), schoolID, date)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membaca pengaturan kunci presensi: "+err.Error())
		return false
	}
	if locked {
		common.WriteError(w, http.StatusConflict, lockedMessage(model.DateOnly(date)))
		return false
	}
	return true
}

func (h *Handler) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}

type lockSettingsResponse struct {
	model.LockSettings
	// Convenience for the settings screen: is "today" already locked?
	TodayLocked bool   `json:"today_locked"`
	Today       string `json:"today"`
}

// GetLockSettings: GET /admin/attendance-lock-settings (admin, superadmin
// with ?school_id=, kepala_sekolah read-only).
func (h *Handler) GetLockSettingsHandler(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin", "kepala_sekolah") {
		return
	}
	schoolID := scopeFromRequest(claims, r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id wajib diisi")
		return
	}
	cfg, err := h.Store.GetLockSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	today := todayWIB()
	common.WriteJSON(w, http.StatusOK, lockSettingsResponse{LockSettings: *cfg, Today: today,
		TodayLocked: cfg.IsLocked(today, h.clock(), wibLocation)}, nil)
}

type putLockSettingsBody struct {
	Enabled    bool   `json:"enabled"`
	CutoffDays int    `json:"cutoff_days"`
	CutoffTime string `json:"cutoff_time"`
}

// PutLockSettings: PUT /admin/attendance-lock-settings (admin, superadmin).
func (h *Handler) PutLockSettingsHandler(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, "admin", "superadmin") {
		return
	}
	schoolID := scopeFromRequest(claims, r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id wajib diisi")
		return
	}
	var body putLockSettingsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.CutoffTime == "" {
		body.CutoffTime = "23:59"
	}
	in := model.LockSettings{SchoolID: schoolID, Enabled: body.Enabled, CutoffDays: body.CutoffDays, CutoffTime: body.CutoffTime}
	if err := in.Validate(); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := h.Store.UpsertLockSettings(r.Context(), in, claims.UserID, claims.ActiveRole)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan pengaturan: "+err.Error())
		return
	}
	today := todayWIB()
	common.WriteJSONMsg(w, http.StatusOK, lockSettingsResponse{LockSettings: *cfg, Today: today,
		TodayLocked: cfg.IsLocked(today, h.clock(), wibLocation)}, nil, "Pengaturan kunci presensi disimpan")
}
