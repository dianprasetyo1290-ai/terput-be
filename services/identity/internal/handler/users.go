package handler

import (
	"database/sql"
	"database/sql/driver"
	"encoding/csv"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
)

// Data Pengguna: superadmin-scoped user directory (list/filter by role and
// optionally by school), used as-is by superadmin and re-mounted with the
// school_id forced from JWT claims for admin (see router in cmd/server).
//
// Data Siswa / Data Guru reuse this same query (role=siswa / role=guru) —
// see ListStudents/ListTeachers below — and PUT /users/{id} +
// POST /users/{id}/reset-password cover "admin can edit account data" /
// "admin can reset a student's password" for both, rather than duplicating
// separate edit/reset handlers per entity (see README Assumptions).

const userCols = `u.id, u.name, u.email, u.phone, u.default_password, u.password_changed, u.photo, u.birth_date, u.birth_place, u.address, u.religion, u.is_superadmin, u.created_at, u.updated_at`

func scanUser(row interface{ Scan(...interface{}) error }) (model.User, error) {
	var u model.User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.DefaultPassword, &u.PasswordChanged, &u.Photo, &u.BirthDate, &u.BirthPlace, &u.Address, &u.Religion, &u.IsSuperadmin, &u.CreatedAt, &u.UpdatedAt)
	if err == nil && u.PasswordChanged {
		u.DefaultPassword = nil // hide once the user has changed it from the default
	}
	return u, err
}

// ListUsers implements Data Pengguna. role filter is optional; school_id
// filter is optional ONLY for superadmin (resolveSchoolScope enforces the
// multi-tenancy rule: non-superadmin is always forced to their own
// school_id, superadmin may omit it to see all schools).
//
// Non-admin exception (added after a real guru-role 403 was caught via
// browser testing, 2026-07-18): any authenticated user may browse the guru
// directory (role=guru only, not siswa/admin lists) — evaluation-service's
// peer-evaluator picker and similar "pick a colleague" UI need this, and a
// list of guru names isn't sensitive the way siswa data or account/security
// fields are. listUsersByRole strips default_password/phone/address/
// birth info from the response in that case — see directoryOnly there.
func (a *App) ListUsers(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	role := r.URL.Query().Get("role")
	privileged := claims != nil && (claims.ActiveRole == common.RoleAdmin || claims.ActiveRole == common.RoleSuperadmin)
	if !privileged {
		if claims == nil {
			common.WriteError(w, http.StatusUnauthorized, "missing token")
			return
		}
		if role != common.RoleGuru {
			common.WriteError(w, http.StatusForbidden, "forbidden: requires role admin")
			return
		}
	}
	a.listUsersByRole(w, r, role, !privileged)
}

func (a *App) ListStudents(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	a.listUsersByRole(w, r, common.RoleSiswa, false)
}

func (a *App) ListTeachers(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	a.listUsersByRole(w, r, common.RoleGuru, false)
}

// listUsersByRole executes the shared Data Pengguna/Siswa/Guru query.
// directoryOnly strips account/security fields from the response — see
// ListUsers' doc comment for when that's set.
func (a *App) listUsersByRole(w http.ResponseWriter, r *http.Request, role string, directoryOnly bool) {
	schoolID, filtered := resolveSchoolScope(r)
	status := r.URL.Query().Get("status") // aktif/nonaktif filter for guru, optional

	where := `u.deleted_at IS NULL`
	args := []interface{}{}
	joinRole := role != "" || filtered || status != ""

	if joinRole {
		where += ` AND EXISTS (SELECT 1 FROM user_school_roles usr WHERE usr.user_id = u.id`
		if role != "" {
			args = append(args, role)
			where += ` AND usr.role = $` + placeholder(len(args))
		}
		if filtered {
			args = append(args, schoolID)
			where += ` AND usr.school_id = $` + placeholder(len(args))
		}
		if status != "" {
			args = append(args, status)
			where += ` AND usr.status = $` + placeholder(len(args))
		}
		where += `)`
	}

	var total int
	if err := a.DB.QueryRow(`SELECT count(*) FROM users u WHERE `+where, args...).Scan(&total); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}

	page, perPage := common.ParsePage(r)
	args2 := append(append([]interface{}{}, args...), perPage, (page-1)*perPage)
	query := `SELECT ` + userCols + ` FROM users u WHERE ` + where + ` ORDER BY u.name LIMIT $` + placeholder(len(args2)-1) + ` OFFSET $` + placeholder(len(args2))

	rows, err := a.DB.Query(query, args2...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list users: "+err.Error())
		return
	}
	defer rows.Close()

	out := []model.UserWithRoles{}
	ids := []string{}
	byID := map[string]*model.UserWithRoles{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list users")
			return
		}
		uwr := model.UserWithRoles{User: u, Roles: []model.UserSchoolRole{}}
		out = append(out, uwr)
		ids = append(ids, u.ID)
	}
	rows.Close()
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	if len(ids) > 0 {
		roleRows, err := a.DB.Query(`SELECT id, user_id, school_id, role, status, academic_year_id FROM user_school_roles WHERE user_id = ANY($1::uuid[])`, pqStringArray(ids))
		if err == nil {
			defer roleRows.Close()
			for roleRows.Next() {
				var ro model.UserSchoolRole
				if err := roleRows.Scan(&ro.ID, &ro.UserID, &ro.SchoolID, &ro.Role, &ro.Status, &ro.AcademicYearID); err == nil {
					if u, ok := byID[ro.UserID]; ok {
						u.Roles = append(u.Roles, ro)
					}
				}
			}
		}
	}

	if directoryOnly {
		for i := range out {
			out[i].DefaultPassword = nil
			out[i].Phone = nil
			out[i].Address = nil
			out[i].BirthDate = nil
			out[i].BirthPlace = nil
			out[i].Religion = nil
			out[i].Photo = nil
			out[i].Roles = []model.UserSchoolRole{}
		}
	}

	common.WriteJSON(w, http.StatusOK, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

type createUserRequest struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Phone    *string `json:"phone"`
	Password string  `json:"password"` // optional; auto-generated default password if omitted
	Role     string  `json:"role"`
	SchoolID string  `json:"school_id"` // forced to caller's school unless superadmin
}

func (a *App) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Email == "" || req.Role == "" {
		common.WriteError(w, http.StatusBadRequest, "name, email, and role are required")
		return
	}
	if !common.IsValidRole(req.Role) {
		common.WriteError(w, http.StatusBadRequest, "invalid role")
		return
	}
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin && req.SchoolID != "" {
		schoolID = req.SchoolID
	}
	if req.Role != common.RoleSuperadmin && schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required for non-superadmin roles")
		return
	}

	plainPassword := req.Password
	if plainPassword == "" {
		plainPassword = common.GenDefaultPassword()
	}
	hash, err := common.HashPassword(plainPassword)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	defer tx.Rollback()

	var u model.User
	row := tx.QueryRow(`INSERT INTO users AS u (name, email, phone, password_hash, default_password, password_changed, is_superadmin)
		VALUES ($1,$2,$3,$4,$5,false,$6) RETURNING `+userCols,
		req.Name, req.Email, req.Phone, hash, plainPassword, req.Role == common.RoleSuperadmin)
	u, err = scanUser(row)
	if err != nil {
		common.WriteError(w, http.StatusConflict, "failed to create user (email may already be in use): "+err.Error())
		return
	}
	var schoolIDArg interface{}
	if schoolID != "" {
		schoolIDArg = schoolID
	}
	if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)`, u.ID, schoolIDArg, req.Role); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign role: "+err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	u.DefaultPassword = &plainPassword // return the plaintext once, at creation time
	common.WriteJSON(w, http.StatusCreated, u, nil)
}

func (a *App) GetUser(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	id := chi.URLParam(r, "id")
	row := a.DB.QueryRow(`SELECT `+userCols+` FROM users u WHERE u.id=$1 AND u.deleted_at IS NULL`, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	roles, _ := aRolesFor(a, id)
	common.WriteJSON(w, http.StatusOK, model.UserWithRoles{User: u, Roles: roles}, nil)
}

func aRolesFor(a *App, userID string) ([]model.UserSchoolRole, error) {
	rows, err := a.DB.Query(`SELECT id, user_id, school_id, role, status, academic_year_id FROM user_school_roles WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.UserSchoolRole{}
	for rows.Next() {
		var ro model.UserSchoolRole
		if err := rows.Scan(&ro.ID, &ro.UserID, &ro.SchoolID, &ro.Role, &ro.Status, &ro.AcademicYearID); err != nil {
			return nil, err
		}
		out = append(out, ro)
	}
	return out, nil
}

type updateUserRequest struct {
	Name       string  `json:"name"`
	Phone      *string `json:"phone"`
	Photo      *string `json:"photo"`
	BirthDate  *string `json:"birth_date"`
	BirthPlace *string `json:"birth_place"`
	Address    *string `json:"address"`
	Religion   *string `json:"religion"`
}

// UpdateUser: "admin can edit account data" for Data Siswa / Data Guru /
// Data Pengguna alike.
func (a *App) UpdateUser(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	id := chi.URLParam(r, "id")
	var req updateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	row := a.DB.QueryRow(`UPDATE users AS u SET name=$1, phone=$2, photo=$3, birth_date=$4, birth_place=$5, address=$6, religion=$7, updated_at=now()
		WHERE id=$8 AND deleted_at IS NULL RETURNING `+userCols,
		req.Name, req.Phone, req.Photo, req.BirthDate, req.BirthPlace, req.Address, req.Religion, id)
	u, err := scanUser(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update user")
		return
	}
	common.WriteJSON(w, http.StatusOK, u, nil)
}

// DeleteUser: soft-delete (mirrors DeleteSchool's pattern in schools.go) —
// sets deleted_at rather than removing the row, so history (e.g. exam
// attempts, evaluation records) referencing this user_id stays intact.
func (a *App) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	id := chi.URLParam(r, "id")
	res, err := a.DB.Exec(`UPDATE users SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// ResetPassword: admin/superadmin resets any (in-scope) user's password to
// a freshly generated default password, returned once in the response.
func (a *App) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	id := chi.URLParam(r, "id")
	newPassword := common.GenDefaultPassword()
	hash, err := common.HashPassword(newPassword)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	res, err := a.DB.Exec(`UPDATE users SET password_hash=$1, default_password=$2, password_changed=false, updated_at=now()
		WHERE id=$3 AND deleted_at IS NULL`, hash, newPassword, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to reset password")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "new_password": newPassword}, nil)
}

type assignRoleRequest struct {
	Role     string `json:"role"`
	SchoolID string `json:"school_id"`
}

// AssignUserRole: "Admin can assign additional roles/jabatan to a user here."
func (a *App) AssignUserRole(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req assignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !common.IsValidRole(req.Role) {
		common.WriteError(w, http.StatusBadRequest, "valid role is required")
		return
	}
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin && req.SchoolID != "" {
		schoolID = req.SchoolID
	}
	var schoolIDArg interface{}
	if schoolID != "" {
		schoolIDArg = schoolID
	}
	if _, err := a.DB.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)
		ON CONFLICT (user_id, school_id, role) DO NOTHING`, id, schoolIDArg, req.Role); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign role: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]string{"user_id": id, "role": req.Role}, nil)
}

func (a *App) setTeacherStatus(w http.ResponseWriter, r *http.Request, status string) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	query := `UPDATE user_school_roles SET status=$1 WHERE user_id=$2 AND role='guru'`
	args := []interface{}{status, id}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$3`
		args = append(args, claims.SchoolID)
	}
	// Deactivating does NOT delete the account — other services filter
	// "guru aktif" lists by checking this status via GET /teachers.
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update teacher status")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "guru role assignment not found for user")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "status": status}, nil)
}

func (a *App) ActivateTeacher(w http.ResponseWriter, r *http.Request) {
	a.setTeacherStatus(w, r, "active")
}
func (a *App) DeactivateTeacher(w http.ResponseWriter, r *http.Request) {
	a.setTeacherStatus(w, r, "inactive")
}

// ExportUsers: CSV export of the filtered Data Pengguna list.
func (a *App) ExportUsers(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	schoolID, filtered := resolveSchoolScope(r)
	role := r.URL.Query().Get("role")

	where := `u.deleted_at IS NULL`
	args := []interface{}{}
	if role != "" || filtered {
		where += ` AND EXISTS (SELECT 1 FROM user_school_roles usr WHERE usr.user_id = u.id`
		if role != "" {
			args = append(args, role)
			where += ` AND usr.role = $` + placeholder(len(args))
		}
		if filtered {
			args = append(args, schoolID)
			where += ` AND usr.school_id = $` + placeholder(len(args))
		}
		where += `)`
	}
	rows, err := a.DB.Query(`SELECT `+userCols+` FROM users u WHERE `+where+` ORDER BY u.name`, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to export users")
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=users.csv")
	cw := csv.NewWriter(w)
	cw.Write([]string{"id", "name", "email", "phone", "password_changed", "is_superadmin", "created_at"})
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			break
		}
		phone := ""
		if u.Phone != nil {
			phone = *u.Phone
		}
		cw.Write([]string{u.ID, u.Name, u.Email, phone, boolStr(u.PasswordChanged), boolStr(u.IsSuperadmin), u.CreatedAt.Format("2006-01-02T15:04:05Z07:00")})
	}
	cw.Flush()
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func placeholder(n int) string {
	// small helper to keep dynamic-where-clause building above readable
	digits := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func pqStringArray(ids []string) interface{} {
	return stringArray(ids)
}

// stringArray implements driver.Valuer to pass a Go []string as a Postgres
// text[] literal without adding a second SQL-array dependency.
type stringArray []string

func (a stringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	out := "{"
	for i, s := range a {
		if i > 0 {
			out += ","
		}
		out += `"` + s + `"`
	}
	out += "}"
	return out, nil
}
