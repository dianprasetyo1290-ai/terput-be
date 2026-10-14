package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"skci/identity/internal/common"
	"skci/identity/internal/rules"
)

// BUG-082 Matriks hak akses per modul. Each module declares its actions and
// the roles allowed by default (what the code did before the matrix existed).
// A role_permissions row (unit-specific first, then global) overrides the
// defaults for that role+module; superadmin is never restricted, so the
// matrix can't lock the platform out.
//
//	GET /security/permissions?role=&school_id=   (superadmin)
//	PUT /security/permissions                     (superadmin) {school_id?, items:[{role,module,can_view,...,reset?}]}
//	GET /me/permissions                           (any user, caller's active role)
//	GET /internal/permissions?role=&school_id=    (other services, X-Internal-Key)

var permissionActions = []string{"view", "create", "edit", "delete", "approve"}

type permissionModule struct {
	Key      string              `json:"key"`
	Label    string              `json:"label"`
	Actions  []string            `json:"actions"`
	Defaults map[string][]string `json:"defaults"` // action -> roles
}

var permissionModules = []permissionModule{
	{Key: "announcements", Label: "Informasi / Pengumuman", Actions: []string{"view", "create", "edit", "delete", "approve"},
		Defaults: map[string][]string{"view": common.AllRoles, "create": {common.RoleAdmin}, "edit": {common.RoleAdmin}, "delete": {common.RoleAdmin}, "approve": {common.RoleAdmin}}},
	{Key: "guardians", Label: "Relasi Orang Tua–Siswa", Actions: []string{"view", "create", "edit", "delete"},
		Defaults: map[string][]string{"view": {common.RoleAdmin}, "create": {common.RoleAdmin}, "edit": {common.RoleAdmin}, "delete": {common.RoleAdmin}}},
	{Key: "academic_years", Label: "Tahun Akademik", Actions: []string{"create", "edit", "delete"},
		Defaults: map[string][]string{"create": {common.RoleAdmin}, "edit": {common.RoleAdmin}, "delete": {common.RoleAdmin}}},
	{Key: "references", Label: "Data Referensi", Actions: []string{"create", "edit"},
		Defaults: map[string][]string{"create": {common.RoleAdmin}, "edit": {common.RoleAdmin}}},
	{Key: "privacy_requests", Label: "Permintaan Data Pribadi (UU PDP)", Actions: []string{"view", "approve"},
		Defaults: map[string][]string{"view": {common.RoleAdmin}, "approve": {common.RoleAdmin}}},
}

var actionLabels = map[string]string{"view": "melihat", "create": "menambah", "edit": "mengubah", "delete": "menghapus", "approve": "menyetujui/menerbitkan"}

func findPermissionModule(key string) (permissionModule, bool) {
	for _, m := range permissionModules {
		if m.Key == key {
			return m, true
		}
	}
	return permissionModule{}, false
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// permissionRow is the effective permission set of one role on one module.
type permissionRow struct {
	Role       string          `json:"role"`
	Module     string          `json:"module"`
	Allowed    map[string]bool `json:"allowed"`
	Overridden bool            `json:"overridden"`
	Scope      string          `json:"scope"` // default | global | unit
}

func defaultPermissions(m permissionModule, role string) map[string]bool {
	out := map[string]bool{}
	for _, act := range m.Actions {
		out[act] = contains(m.Defaults[act], role)
	}
	return out
}

// effectivePermissions returns one row per module for role in schoolID.
func (a *App) effectivePermissions(ctx context.Context, schoolID, role string) ([]permissionRow, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT DISTINCT ON (module) module, school_id IS NOT NULL,
			can_view, can_create, can_edit, can_delete, can_approve
		FROM role_permissions
		WHERE role=$1 AND (school_id IS NULL OR school_id::text=$2)
		ORDER BY module, school_id NULLS LAST`, role, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	overrides := map[string]permissionRow{}
	for rows.Next() {
		var mod string
		var unit bool
		var v, c, e, d, ap bool
		if err := rows.Scan(&mod, &unit, &v, &c, &e, &d, &ap); err != nil {
			return nil, err
		}
		scope := "global"
		if unit {
			scope = "unit"
		}
		overrides[mod] = permissionRow{Module: mod, Overridden: true, Scope: scope,
			Allowed: map[string]bool{"view": v, "create": c, "edit": e, "delete": d, "approve": ap}}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]permissionRow, 0, len(permissionModules))
	for _, m := range permissionModules {
		row := permissionRow{Role: role, Module: m.Key, Allowed: defaultPermissions(m, role), Scope: "default"}
		if ov, ok := overrides[m.Key]; ok {
			row.Overridden, row.Scope = true, ov.Scope
			for _, act := range m.Actions {
				row.Allowed[act] = ov.Allowed[act]
			}
		}
		if role == common.RoleSuperadmin {
			for _, act := range m.Actions {
				row.Allowed[act] = true
			}
			row.Overridden, row.Scope = false, "default"
		}
		out = append(out, row)
	}
	return out, nil
}

// allowedTo reports whether role may perform action on module in schoolID.
func (a *App) allowedTo(ctx context.Context, schoolID, role, module, action string) (bool, error) {
	if role == common.RoleSuperadmin {
		return true, nil
	}
	m, ok := findPermissionModule(module)
	if !ok || !contains(m.Actions, action) {
		return false, errors.New("unknown permission " + module + "." + action)
	}
	var allowed bool
	err := a.DB.QueryRowContext(ctx, `SELECT CASE $3
			WHEN 'view' THEN can_view WHEN 'create' THEN can_create WHEN 'edit' THEN can_edit
			WHEN 'delete' THEN can_delete ELSE can_approve END
		FROM role_permissions
		WHERE role=$1 AND module=$2 AND (school_id IS NULL OR school_id::text=$4)
		ORDER BY school_id NULLS LAST LIMIT 1`, role, module, action, schoolID).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return contains(m.Defaults[action], role), nil
	}
	return allowed, err
}

// permit is the matrix-aware replacement for requireRole on identity's own
// modules. Internal-key callers pass, like requireRole.
func (a *App) permit(w http.ResponseWriter, r *http.Request, module, action string) bool {
	claims := common.FromCtx(r)
	if claims == nil {
		if common.IsInternal(r) {
			return true
		}
		common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
		return false
	}
	ok, err := a.allowedTo(r.Context(), claims.SchoolID, claims.ActiveRole, module, action)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa hak akses")
		return false
	}
	if !ok {
		m, _ := findPermissionModule(module)
		common.WriteError(w, http.StatusForbidden, "Peran Anda tidak memiliki izin "+actionLabels[action]+" pada modul "+m.Label)
		return false
	}
	return true
}

// GetPermissionMatrix: GET /security/permissions?role=&school_id=.
func (a *App) GetPermissionMatrix(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r) {
		return
	}
	q := r.URL.Query()
	school := q.Get("school_id")
	if school != "" && !rules.IsUUID(school) {
		common.WriteValidationError(w, http.StatusBadRequest, "Unit tidak valid", map[string]string{"school_id": "Tidak valid"})
		return
	}
	roles := []string{}
	if role := q.Get("role"); role != "" {
		if !common.IsValidRole(role) {
			common.WriteValidationError(w, http.StatusBadRequest, "Peran tidak dikenal", map[string]string{"role": "Tidak dikenal"})
			return
		}
		roles = append(roles, role)
	} else {
		for _, role := range common.AllRoles {
			if role != common.RoleSuperadmin {
				roles = append(roles, role)
			}
		}
		sort.Strings(roles)
	}
	out := []permissionRow{}
	for _, role := range roles {
		rows, err := a.effectivePermissions(r.Context(), school, role)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat matriks hak akses")
			return
		}
		out = append(out, rows...)
	}
	common.WriteJSON(w, http.StatusOK, out, map[string]any{"modules": permissionModules, "actions": permissionActions})
}

type permissionUpdate struct {
	Role    string          `json:"role"`
	Module  string          `json:"module"`
	Allowed map[string]bool `json:"allowed"`
	Reset   bool            `json:"reset"`
}

// validatePermissionUpdate checks one matrix edit.
func validatePermissionUpdate(u permissionUpdate) string {
	if !common.IsValidRole(u.Role) {
		return "Peran tidak dikenal: " + u.Role
	}
	if u.Role == common.RoleSuperadmin {
		return "Hak akses Super Admin tidak dapat dibatasi"
	}
	m, ok := findPermissionModule(u.Module)
	if !ok {
		return "Modul tidak dikenal: " + u.Module
	}
	if u.Reset {
		return ""
	}
	for act := range u.Allowed {
		if !contains(m.Actions, act) {
			return "Aksi " + act + " tidak tersedia pada modul " + m.Label
		}
	}
	return ""
}

// UpdatePermissionMatrix: PUT /security/permissions {school_id?, items}.
func (a *App) UpdatePermissionMatrix(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r) {
		return
	}
	claims := common.FromCtx(r)
	var in struct {
		SchoolID string             `json:"school_id"`
		Items    []permissionUpdate `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	if in.SchoolID != "" && !rules.IsUUID(in.SchoolID) {
		common.WriteValidationError(w, http.StatusBadRequest, "Unit tidak valid", map[string]string{"school_id": "Tidak valid"})
		return
	}
	if len(in.Items) == 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Tidak ada perubahan yang dikirim", map[string]string{"items": "Wajib diisi"})
		return
	}
	for _, it := range in.Items {
		if msg := validatePermissionUpdate(it); msg != "" {
			common.WriteValidationError(w, http.StatusBadRequest, msg, map[string]string{"items": msg})
			return
		}
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan matriks hak akses")
		return
	}
	defer tx.Rollback()
	school := nullIfEmpty(in.SchoolID)
	var actor any
	if claims != nil {
		actor = nullIfEmpty(claims.UserID)
	}
	for _, it := range in.Items {
		if it.Reset {
			if _, err := tx.ExecContext(r.Context(), `DELETE FROM role_permissions WHERE role=$1 AND module=$2
				AND school_id IS NOT DISTINCT FROM $3::uuid`, it.Role, it.Module, school); err != nil {
				common.WriteDBError(w, err, "Gagal menyimpan matriks hak akses")
				return
			}
			continue
		}
		m, _ := findPermissionModule(it.Module)
		// Start from the current effective values so a partial edit keeps
		// the other actions as they are.
		cur := defaultPermissions(m, it.Role)
		for act, v := range it.Allowed {
			cur[act] = v
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO role_permissions
				(school_id, role, module, can_view, can_create, can_edit, can_delete, can_approve, updated_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (COALESCE(school_id, '00000000-0000-0000-0000-000000000000'::uuid), role, module)
			DO UPDATE SET can_view=EXCLUDED.can_view, can_create=EXCLUDED.can_create, can_edit=EXCLUDED.can_edit,
				can_delete=EXCLUDED.can_delete, can_approve=EXCLUDED.can_approve,
				updated_by=EXCLUDED.updated_by, updated_at=now()`,
			school, it.Role, it.Module, cur["view"], cur["create"], cur["edit"], cur["delete"], cur["approve"], actor); err != nil {
			common.WriteDBError(w, err, "Gagal menyimpan matriks hak akses")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan matriks hak akses")
		return
	}
	var actorID, actorRole *string
	if claims != nil {
		actorID, actorRole = &claims.UserID, &claims.ActiveRole
	}
	var sp *string
	if in.SchoolID != "" {
		sp = &in.SchoolID
	}
	a.RecordSecurityAudit(sp, "permission_matrix_updated", "role_permissions", actorID, actorRole, nil, in.Items, "", r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusOK, map[string]int{"updated": len(in.Items)}, nil, "Matriks hak akses disimpan")
}

// MyPermissions: GET /me/permissions — what the SPA uses to hide buttons.
func (a *App) MyPermissions(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
		return
	}
	rows, err := a.effectivePermissions(r.Context(), claims.SchoolID, claims.ActiveRole)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat hak akses")
		return
	}
	out := map[string]map[string]bool{}
	for _, row := range rows {
		out[row.Module] = row.Allowed
	}
	common.WriteJSON(w, http.StatusOK, out, map[string]any{"role": claims.ActiveRole})
}

// InternalPermissions: GET /internal/permissions?role=&school_id= for other
// services adopting the matrix (contract in the QA-IDN handoff).
func (a *App) InternalPermissions(w http.ResponseWriter, r *http.Request) {
	role := r.URL.Query().Get("role")
	if !common.IsValidRole(role) {
		common.WriteValidationError(w, http.StatusBadRequest, "Peran tidak dikenal", map[string]string{"role": "Tidak dikenal"})
		return
	}
	rows, err := a.effectivePermissions(r.Context(), r.URL.Query().Get("school_id"), role)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat hak akses")
		return
	}
	common.WriteJSON(w, http.StatusOK, rows, nil)
}
