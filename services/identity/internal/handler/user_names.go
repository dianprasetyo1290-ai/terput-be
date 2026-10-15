package handler

// Display names for account ids: GET /users/names?ids=a,b,c. The other
// services store users by id only (student_user_id, requester_user_id, ...),
// and several screens used to fall back to the raw id when their own name
// lookup missed. This endpoint lets the SPA resolve any id it shows in one
// batched call. Staff only (not students or parents), and scoped to the
// caller's school: an id outside it simply does not resolve.

import (
	"net/http"
	"strings"

	"skci/identity/internal/common"
	"skci/identity/internal/rules"
)

const maxNameIDs = 500

type userName struct {
	Name string  `json:"name"`
	NISN *string `json:"nisn,omitempty"`
}

// UserNames: GET /users/names?ids=<comma-separated ids>.
func (a *App) UserNames(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims.ActiveRole == common.RoleSiswa || claims.ActiveRole == common.RoleOrangTua {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	var ids []string
	seen := map[string]bool{}
	for _, id := range strings.Split(r.URL.Query().Get("ids"), ",") {
		id = strings.TrimSpace(id)
		if rules.IsUUID(id) && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > maxNameIDs {
		common.WriteError(w, http.StatusBadRequest, "Maksimal 500 id per permintaan")
		return
	}
	out := map[string]userName{}
	if len(ids) == 0 {
		common.WriteJSON(w, http.StatusOK, out, nil)
		return
	}
	// Superadmin sees every account; anyone else the accounts of their own
	// school (plus the cross-school superadmin accounts, which act in it).
	school := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin {
		if s := r.URL.Query().Get("school_id"); rules.IsUUID(s) {
			school = s
		} else {
			school = ""
		}
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT u.id::text, u.name, u.nisn FROM users u
		WHERE u.id::text = ANY(string_to_array($1, ',')) AND u.deleted_at IS NULL
		  AND ($2 = '' OR u.is_superadmin OR EXISTS (
		    SELECT 1 FROM user_school_roles usr WHERE usr.user_id = u.id AND usr.school_id::text = $2))`,
		strings.Join(ids, ","), school)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat nama pengguna")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n userName
		if err := rows.Scan(&id, &n.Name, &n.NISN); err != nil {
			common.WriteDBError(w, err, "Gagal memuat nama pengguna")
			return
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		common.WriteDBError(w, err, "Gagal memuat nama pengguna")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
