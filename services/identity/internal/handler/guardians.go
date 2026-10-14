package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/store"
)

type guardianLinkRequest struct {
	GuardianUserID string `json:"guardian_user_id"`
	StudentUserID  string `json:"student_user_id"`
	Relationship   string `json:"relationship"`
	IsPrimary      bool   `json:"is_primary"`
	Status         string `json:"status"`
}

func guardianSchool(r *http.Request) (string, bool) {
	claims := common.FromCtx(r)
	if claims.ActiveRole == common.RoleSuperadmin {
		schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
		return schoolID, schoolID != ""
	}
	return claims.SchoolID, claims.SchoolID != ""
}

func (a *App) ListGuardianLinks(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "guardians", "view") {
		return
	}
	schoolID, ok := guardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	rows, err := a.DB.Query(`SELECT `+store.GuardianLinkColumns+` FROM guardian_links WHERE school_id=$1 ORDER BY created_at DESC`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat relasi orang tua")
		return
	}
	defer rows.Close()
	out := []model.GuardianLink{}
	for rows.Next() {
		link, err := store.ScanGuardianLink(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat relasi orang tua")
			return
		}
		out = append(out, link)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func validateGuardianLink(tx *sql.Tx, schoolID, guardianID, studentID string) error {
	var valid bool
	return tx.QueryRow(`SELECT
		EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$2 AND school_id=$1 AND role='orang_tua' AND status='active') AND
		EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$3 AND school_id=$1 AND role='siswa' AND status='active')`,
		schoolID, guardianID, studentID).Scan(&valid)
}

func (a *App) CreateGuardianLink(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "guardians", "create") {
		return
	}
	schoolID, ok := guardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	var req guardianLinkRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.GuardianUserID == "" || req.StudentUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "Orang tua dan siswa wajib dipilih")
		return
	}
	if req.Relationship == "" {
		req.Relationship = "wali"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if req.Status != "pending" && req.Status != "active" && req.Status != "inactive" {
		common.WriteError(w, http.StatusBadRequest, "Status tidak valid")
		return
	}
	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghubungkan orang tua dengan siswa")
		return
	}
	defer tx.Rollback()
	var valid bool
	if err := tx.QueryRow(`SELECT
		EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$2 AND school_id=$1 AND role='orang_tua' AND status='active') AND
		EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$3 AND school_id=$1 AND role='siswa' AND status='active')`, schoolID, req.GuardianUserID, req.StudentUserID).Scan(&valid); err != nil || !valid {
		common.WriteError(w, http.StatusBadRequest, "Orang tua dan siswa harus memiliki peran aktif di unit ini")
		return
	}
	verified := interface{}(nil)
	if req.Status == "active" {
		verified = time.Now()
	}
	link, err := store.ScanGuardianLink(tx.QueryRow(`INSERT INTO guardian_links (school_id, guardian_user_id, student_user_id, relationship, is_primary, status, verified_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING `+store.GuardianLinkColumns,
		schoolID, req.GuardianUserID, req.StudentUserID, req.Relationship, req.IsPrimary, req.Status, verified))
	if err != nil {
		common.WriteError(w, http.StatusConflict, "Gagal menghubungkan orang tua dengan siswa")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghubungkan orang tua dengan siswa")
		return
	}
	common.WriteJSON(w, http.StatusCreated, link, nil)
}

func (a *App) UpdateGuardianLink(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "guardians", "edit") {
		return
	}
	schoolID, ok := guardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	var req guardianLinkRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Relationship == "" || (req.Status != "pending" && req.Status != "active" && req.Status != "inactive") {
		common.WriteError(w, http.StatusBadRequest, "Hubungan dan status yang valid wajib diisi")
		return
	}
	link, err := store.ScanGuardianLink(a.DB.QueryRow(`UPDATE guardian_links SET relationship=$1, is_primary=$2, status=$3,
		verified_at=CASE WHEN $3='active' THEN COALESCE(verified_at,now()) ELSE verified_at END, updated_at=now()
		WHERE id=$4 AND school_id=$5 RETURNING `+store.GuardianLinkColumns,
		req.Relationship, req.IsPrimary, req.Status, chi.URLParam(r, "id"), schoolID))
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Relasi orang tua–siswa tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui relasi orang tua")
		return
	}
	common.WriteJSON(w, http.StatusOK, link, nil)
}

func (a *App) DeleteGuardianLink(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "guardians", "delete") {
		return
	}
	schoolID, ok := guardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	res, err := a.DB.Exec(`DELETE FROM guardian_links WHERE id=$1 AND school_id=$2`, chi.URLParam(r, "id"), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus relasi orang tua")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "Relasi orang tua–siswa tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": chi.URLParam(r, "id")}, nil)
}

func (a *App) MyChildren(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleOrangTua) {
		return
	}
	claims := common.FromCtx(r)
	children, err := store.LinkedChildren(a.DB, claims.SchoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar anak")
		return
	}
	common.WriteJSON(w, http.StatusOK, children, nil)
}

// internalGuardianSchool validates the mandatory tenant scope on the internal
// guardian-read endpoints (see cmd/server/main.go). Unlike guardianSchool,
// which resolves the school from JWT claims, an X-Internal-Key caller has no
// claims at all, so the school must arrive as an explicit ?school_id=; a blank
// value is a 400 rather than an implicit cross-school read.
func internalGuardianSchool(r *http.Request) (string, bool) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	return schoolID, schoolID != ""
}

// InternalGuardianChildren backs the service-to-service endpoint
//
//	GET /internal/guardians/{guardianUserId}/children?school_id=<uuid>
//
// returning the minimal {student_user_id,name,nisn} projection of the students
// the guardian is actively linked to at that one school, so sibling services
// (report/academic) can resolve a parent's children without touching identity's
// tables. Tenant-scoped by the mandatory school_id (400 when absent); only
// links active on both sides count (see store.LinkedGuardianChildren).
func (a *App) InternalGuardianChildren(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalGuardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	children, err := store.LinkedGuardianChildren(a.DB, schoolID, chi.URLParam(r, "guardianUserId"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar anak")
		return
	}
	common.WriteJSON(w, http.StatusOK, children, nil)
}

// InternalGuardianStudentAccess backs the service-to-service endpoint
//
//	GET /internal/guardians/{guardianUserId}/students/{studentUserId}/access?school_id=<uuid>
//
// returning 200 {data:{"allowed":true}} when an active link connects the
// guardian to that student in the given school, and 404 when it does not.
// Used by siblings that must authorize one specific parent-facing action.
// Tenant-scoped by the mandatory school_id (400 when absent).
func (a *App) InternalGuardianStudentAccess(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalGuardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	allowed, err := store.GuardianStudentAccess(a.DB, schoolID, chi.URLParam(r, "guardianUserId"), chi.URLParam(r, "studentUserId"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memeriksa akses orang tua")
		return
	}
	if !allowed {
		common.WriteError(w, http.StatusNotFound, "Akses orang tua tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]bool{"allowed": true}, nil)
}

// InternalStudentGuardians backs the service-to-service endpoint
//
//	GET /internal/students/{studentUserId}/guardians?school_id=<uuid>
//
// returning [{guardian_user_id, relationship, is_primary}] for the student's
// active guardian links at that school (T9 §4.1, used by T10/T19/T20/T21).
// Tenant-scoped by the mandatory school_id (400 when absent).
func (a *App) InternalStudentGuardians(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalGuardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	guardians, err := store.StudentGuardians(a.DB, schoolID, chi.URLParam(r, "studentUserId"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat orang tua siswa")
		return
	}
	common.WriteJSON(w, http.StatusOK, guardians, nil)
}

type claimRequest struct {
	NISN string `json:"nisn"`
	Code string `json:"code"`
}

func (a *App) ClaimChild(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleOrangTua) {
		return
	}
	claims := common.FromCtx(r)
	var req claimRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || strings.TrimSpace(req.NISN) == "" || req.Code == "" {
		common.WriteValidationError(w, http.StatusBadRequest, "NISN dan kode klaim wajib diisi", map[string]string{"nisn": "Wajib diisi", "code": "Wajib diisi"})
		return
	}
	link, err := store.ConsumeGuardianClaim(a.DB, claims.SchoolID, claims.UserID, strings.TrimSpace(req.NISN), time.Now(), func(hash string) bool {
		return common.CheckPassword(hash, req.Code)
	})
	switch {
	case err != nil:
		status, msg := claimErrorMessage(err)
		if status == http.StatusInternalServerError {
			common.CaptureError(err)
		}
		common.WriteError(w, status, msg)
	default:
		common.WriteJSON(w, http.StatusCreated, link, nil)
	}
}

type issueClaimRequest struct {
	StudentUserID  string `json:"student_user_id"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

func (a *App) IssueGuardianClaim(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "guardians", "create") {
		return
	}
	schoolID, ok := guardianSchool(r)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "Unit (school_id) wajib diisi")
		return
	}
	var req issueClaimRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.StudentUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "Siswa wajib dipilih")
		return
	}
	if req.ExpiresInHours <= 0 || req.ExpiresInHours > 168 {
		req.ExpiresInHours = 24
	}
	var exists bool
	if err := a.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$1 AND school_id=$2 AND role='siswa' AND status='active')`, req.StudentUserID, schoolID).Scan(&exists); err != nil || !exists {
		common.WriteError(w, http.StatusNotFound, "Siswa tidak ditemukan di unit ini")
		return
	}
	code := common.GenClaimCode()
	hash, err := common.HashPassword(code)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat kode klaim")
		return
	}
	claims := common.FromCtx(r)
	expires := time.Now().Add(time.Duration(req.ExpiresInHours) * time.Hour)
	if _, err := a.DB.Exec(`UPDATE guardian_claim_codes SET revoked_at=now() WHERE school_id=$1 AND student_user_id=$2 AND used_at IS NULL AND revoked_at IS NULL`, schoolID, req.StudentUserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat kode klaim")
		return
	}
	if _, err := a.DB.Exec(`INSERT INTO guardian_claim_codes (school_id, student_user_id, code_hash, expires_at, created_by) VALUES ($1,$2,$3,$4,$5)`, schoolID, req.StudentUserID, hash, expires, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat kode klaim")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]interface{}{"code": code, "expires_at": expires}, nil)
}

// claimErrorMessage maps claim failures to readable answers; internal
// errors never reach the client (BUG-045).
func claimErrorMessage(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrClaimRateLimit):
		return http.StatusTooManyRequests, "Terlalu banyak percobaan klaim. Coba lagi beberapa saat lagi"
	case errors.Is(err, store.ErrClaimUsed):
		return http.StatusUnprocessableEntity, "Kode klaim sudah pernah dipakai"
	case errors.Is(err, store.ErrClaimInvalid):
		return http.StatusUnprocessableEntity, "NISN atau kode klaim tidak cocok, atau kode sudah kedaluwarsa"
	}
	return http.StatusInternalServerError, "Gagal menghubungkan anak. Coba lagi"
}
