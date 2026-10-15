package handler

// Multi-unit workspace (sidebar "Multi-Unit Workspace"): a yayasan runs
// several units (SD, SMP, SMK), each a row in schools. GET /auth/units lists
// the units the caller can work in and POST /auth/switch-unit reissues the
// token scoped to one of them. Superadmin works in every unit through the
// ?school_id= override instead, so it only needs the list.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/store"
)

type unitItem struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Code  *string  `json:"code"`
	Level *string  `json:"level"`
	Roles []string `json:"roles"`
}

// MyUnits lists the caller's units: GET /auth/units.
func (a *App) MyUnits(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	var out []unitItem
	if claims.ActiveRole == common.RoleSuperadmin {
		rows, err := a.DB.QueryContext(r.Context(), `SELECT id, name, code, level FROM schools
			WHERE deleted_at IS NULL AND is_active ORDER BY CASE upper(level) WHEN 'TK' THEN 1 WHEN 'SD' THEN 2 WHEN 'SMP' THEN 3 WHEN 'SMA' THEN 4 WHEN 'SMK' THEN 5 ELSE 9 END, name`)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var u unitItem
			if err := rows.Scan(&u.ID, &u.Name, &u.Code, &u.Level); err != nil {
				common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
				return
			}
			u.Roles = []string{common.RoleSuperadmin}
			out = append(out, u)
		}
		common.WriteJSON(w, http.StatusOK, out, nil)
		return
	}

	roles, err := store.UserRoles(a.DB, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
		return
	}
	bySchool := map[string][]string{}
	for _, ro := range activeRoleRows(roles) {
		if ro.SchoolID != nil {
			bySchool[*ro.SchoolID] = append(bySchool[*ro.SchoolID], ro.Role)
		}
	}
	if len(bySchool) == 0 {
		common.WriteJSON(w, http.StatusOK, out, nil)
		return
	}
	ids := make([]string, 0, len(bySchool))
	for id := range bySchool {
		ids = append(ids, id)
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT id, name, code, level FROM schools
		WHERE deleted_at IS NULL AND is_active AND id::text = ANY(string_to_array($1, ',')) ORDER BY CASE upper(level) WHEN 'TK' THEN 1 WHEN 'SD' THEN 2 WHEN 'SMP' THEN 3 WHEN 'SMA' THEN 4 WHEN 'SMK' THEN 5 ELSE 9 END, name`, strings.Join(ids, ","))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var u unitItem
		if err := rows.Scan(&u.ID, &u.Name, &u.Code, &u.Level); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
			return
		}
		u.Roles = bySchool[u.ID]
		out = append(out, u)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type switchUnitRequest struct {
	SchoolID string `json:"school_id"`
}

// unitRole picks the role the caller works as in a unit: the current active
// role when it is held there too, otherwise the first active role there.
func unitRole(rows []model.UserSchoolRole, schoolID, current string) string {
	first := ""
	for _, ro := range rows {
		if ro.SchoolID == nil || *ro.SchoolID != schoolID {
			continue
		}
		if ro.Role == current {
			return current
		}
		if first == "" {
			first = ro.Role
		}
	}
	return first
}

// SwitchUnit reissues the token scoped to another unit: POST /auth/switch-unit.
func (a *App) SwitchUnit(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	var req switchUnitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SchoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "Unit wajib dipilih")
		return
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		common.WriteError(w, http.StatusBadRequest, "Superadmin memilih unit lewat school_id, tanpa berpindah sesi")
		return
	}
	if user, err := store.FindUserByID(a.DB, claims.UserID); err != nil || (user.DeactivateAt != nil && !user.DeactivateAt.After(time.Now())) {
		common.WriteError(w, http.StatusUnauthorized, "Akun dinonaktifkan. Hubungi admin unit")
		return
	}
	roles, err := store.UserRoles(a.DB, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal berpindah unit")
		return
	}
	roles = activeRoleRows(roles)
	role := unitRole(roles, req.SchoolID, claims.ActiveRole)
	if role == "" {
		common.WriteError(w, http.StatusForbidden, "Akun Anda tidak memiliki akses ke unit ini")
		return
	}
	var allRoles []string
	for _, ro := range roles {
		allRoles = append(allRoles, ro.Role)
	}
	token, _, err := a.issueToken(r, claims.UserID, req.SchoolID, role, allRoles, claims.SessionID, false)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat sesi. Coba lagi")
		return
	}
	sID := req.SchoolID
	a.RecordSecurityAudit(&sID, "unit_switched", claims.UserID, &claims.UserID, &role,
		map[string]any{"from_school_id": claims.SchoolID, "from_role": claims.ActiveRole},
		map[string]any{"to_school_id": req.SchoolID, "to_role": role}, "switched active unit", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, loginResponse{Token: token}, nil)
}
