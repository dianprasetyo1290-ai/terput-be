package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/rules"
)

// QA-BILL hand-off: finance and oversight roles need to read students of
// their own unit (student detail, refund names, scholarship picker). They get
// a read-only, school-scoped view: no password, contact, birth, religion or
// NIK — only what identifies the student (name, email, NISN, status).
var studentOversightRoles = map[string]bool{
	common.RoleFinanceManager: true,
	common.RoleKepalaYayasan:  true,
	common.RoleKepalaSekolah:  true,
	common.RoleAuditor:        true,
}

func isStudentOversightRole(role string) bool { return studentOversightRoles[role] }

// stripToIdentifiers keeps name/email/NISN/status, drops everything private.
func stripToIdentifiers(u *model.User) {
	u.DefaultPassword = nil
	u.Phone = nil
	u.Address = nil
	u.BirthDate = nil
	u.BirthPlace = nil
	u.Religion = nil
	u.NIK = nil
}

// getUserForOversight serves GET /users/{id} to a studentOversightRoles
// caller: only a student of the caller's own unit, identifiers only.
func (a *App) getUserForOversight(w http.ResponseWriter, r *http.Request, claims *common.Claims) {
	id := chi.URLParam(r, "id")
	if !rules.IsUUID(id) || claims.SchoolID == "" {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	}
	var ok bool
	if err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM user_school_roles
		WHERE user_id::text=$1 AND school_id::text=$2 AND role='siswa')`, id, claims.SchoolID).Scan(&ok); err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengguna")
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "Siswa tidak ditemukan di unit Anda")
		return
	}
	u, err := scanUser(a.DB.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id=$1 AND u.deleted_at IS NULL`, id))
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengguna")
		return
	}
	stripToIdentifiers(&u)
	all, _ := aRolesFor(a, id)
	roles := []model.UserSchoolRole{}
	for _, ro := range all {
		if ro.SchoolID != nil && *ro.SchoolID == claims.SchoolID {
			roles = append(roles, ro)
		}
	}
	common.WriteJSON(w, http.StatusOK, model.UserWithRoles{User: u, Roles: roles}, nil)
}

type internalRoleUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	SchoolID string `json:"school_id"`
}

// InternalUsersByRole: GET /internal/users?role=<role>&school_id=<uuid>
// (X-Internal-Key). Active holders of a role in a unit, so core and billing
// can send in-app alerts to e.g. akuntan / finance_manager. school_id is
// mandatory (tenant scope); a global role holder (school_id NULL, such as a
// kepala_yayasan) is included when include_global=true.
func (a *App) InternalUsersByRole(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	role := strings.TrimSpace(q.Get("role"))
	school := strings.TrimSpace(q.Get("school_id"))
	errs := map[string]string{}
	if !common.IsValidRole(role) {
		errs["role"] = "Peran tidak dikenal"
	}
	if !rules.IsUUID(school) {
		errs["school_id"] = "school_id wajib diisi"
	}
	if len(errs) > 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Parameter tidak valid", errs)
		return
	}
	includeGlobal := q.Get("include_global") == "true"
	rows, err := a.DB.QueryContext(r.Context(), `SELECT DISTINCT u.id::text, u.name, u.email, COALESCE(usr.school_id::text, '')
		FROM user_school_roles usr JOIN users u ON u.id = usr.user_id
		WHERE usr.role = $1 AND usr.status = 'active' AND u.deleted_at IS NULL
		  AND (usr.school_id::text = $2 OR ($3 AND usr.school_id IS NULL))
		ORDER BY u.name`, role, school, includeGlobal)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengguna")
		return
	}
	defer rows.Close()
	out := []internalRoleUser{}
	for rows.Next() {
		var u internalRoleUser
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.SchoolID); err != nil {
			common.WriteDBError(w, err, "Gagal memuat pengguna")
			return
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengguna")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
