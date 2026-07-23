package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
)

// Data Sekolah (School) CRUD + activate/deactivate/assign-admin/impersonate.

type schoolRequest struct {
	FoundationID       *string `json:"foundation_id"`
	Name               string  `json:"name"`
	Code               *string `json:"code"`
	Level              *string `json:"level"`
	Address            *string `json:"address"`
	Subdistrict        *string `json:"subdistrict"`
	District           *string `json:"district"`
	City               *string `json:"city"`
	Province           *string `json:"province"`
	RegistrationNumber *string `json:"registration_number"`
	YearFounded        *string `json:"year_founded"`
	PhoneNumber        *string `json:"phone_number"`
	Email              *string `json:"email"`
	Website            *string `json:"website"`
	Logo               *string `json:"logo"`
}

const schoolCols = `id, foundation_id, name, code, level, address, subdistrict, district, city, province,
	registration_number, year_founded, phone_number, email, website, logo, is_active, current_academic_year_id, created_at, updated_at`

func scanSchool(row interface{ Scan(...interface{}) error }) (model.School, error) {
	var s model.School
	err := row.Scan(&s.ID, &s.FoundationID, &s.Name, &s.Code, &s.Level, &s.Address, &s.Subdistrict, &s.District, &s.City, &s.Province,
		&s.RegistrationNumber, &s.YearFounded, &s.PhoneNumber, &s.Email, &s.Website, &s.Logo, &s.IsActive, &s.CurrentAcademicYearID, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// ListSchools: superadmin sees all (optionally filtered by ?foundation_id=);
// non-superadmin is hard-scoped to their own school via JWT (per
// resolveSchoolScope), so this effectively returns a single-item list.
func (a *App) ListSchools(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	page, perPage := common.ParsePage(r)

	query := `SELECT ` + schoolCols + ` FROM schools WHERE deleted_at IS NULL`
	countQuery := `SELECT count(*) FROM schools WHERE deleted_at IS NULL`
	args := []interface{}{}

	if claims.ActiveRole != common.RoleSuperadmin {
		args = append(args, claims.SchoolID)
		query += ` AND id = $1`
		countQuery += ` AND id = $1`
	} else if fid := r.URL.Query().Get("foundation_id"); fid != "" {
		args = append(args, fid)
		query += ` AND foundation_id = $1`
		countQuery += ` AND foundation_id = $1`
	}

	var total int
	if err := a.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list schools")
		return
	}
	args = append(args, perPage, (page-1)*perPage)
	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list schools")
		return
	}
	defer rows.Close()
	out := []model.School{}
	for rows.Next() {
		s, err := scanSchool(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list schools")
			return
		}
		out = append(out, s)
	}
	common.WriteJSON(w, http.StatusOK, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

func (a *App) CreateSchool(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	var req schoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	row := a.DB.QueryRow(`INSERT INTO schools (foundation_id, name, code, level, address, subdistrict, district, city, province,
			registration_number, year_founded, phone_number, email, website, logo)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING `+schoolCols,
		req.FoundationID, req.Name, req.Code, req.Level, req.Address, req.Subdistrict, req.District, req.City, req.Province,
		req.RegistrationNumber, req.YearFounded, req.PhoneNumber, req.Email, req.Website, req.Logo)
	s, err := scanSchool(row)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create school: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, s, nil)
}

func (a *App) GetSchool(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	if claims == nil && !common.IsInternal(r) {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	if claims != nil && claims.ActiveRole != common.RoleSuperadmin && claims.SchoolID != id {
		common.WriteError(w, http.StatusForbidden, "cannot access another school")
		return
	}
	row := a.DB.QueryRow(`SELECT `+schoolCols+` FROM schools WHERE id = $1 AND deleted_at IS NULL`, id)
	s, err := scanSchool(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "school not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get school")
		return
	}
	common.WriteJSON(w, http.StatusOK, s, nil)
}

// UpdateSchool: superadmin may edit any school; admin may only edit their
// own school's identity fields ("Identitas Sekolah").
func (a *App) UpdateSchool(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	if claims.ActiveRole == common.RoleSuperadmin {
		// ok
	} else if claims.ActiveRole == common.RoleAdmin && claims.SchoolID == id {
		// ok
	} else {
		common.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req schoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	row := a.DB.QueryRow(`UPDATE schools SET foundation_id=$1, name=$2, code=$3, level=$4, address=$5, subdistrict=$6, district=$7,
			city=$8, province=$9, registration_number=$10, year_founded=$11, phone_number=$12, email=$13, website=$14, logo=$15, updated_at=now()
		WHERE id=$16 AND deleted_at IS NULL
		RETURNING `+schoolCols,
		req.FoundationID, req.Name, req.Code, req.Level, req.Address, req.Subdistrict, req.District, req.City, req.Province,
		req.RegistrationNumber, req.YearFounded, req.PhoneNumber, req.Email, req.Website, req.Logo, id)
	s, err := scanSchool(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "school not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update school: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, s, nil)
}

func (a *App) DeleteSchool(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	id := chi.URLParam(r, "id")
	res, err := a.DB.Exec(`UPDATE schools SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete school")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "school not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (a *App) setSchoolActive(w http.ResponseWriter, r *http.Request, active bool) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	id := chi.URLParam(r, "id")
	res, err := a.DB.Exec(`UPDATE schools SET is_active=$1, updated_at=now() WHERE id=$2 AND deleted_at IS NULL`, active, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update school status")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "school not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"id": id, "is_active": active}, nil)
}

func (a *App) ActivateSchool(w http.ResponseWriter, r *http.Request) { a.setSchoolActive(w, r, true) }
func (a *App) DeactivateSchool(w http.ResponseWriter, r *http.Request) {
	a.setSchoolActive(w, r, false)
}

type assignAdminRequest struct {
	UserID string `json:"user_id"`
}

// AssignAdmin gives an existing user account the 'admin' role scoped to this
// school (upsert — safe to call again to re-point ownership).
func (a *App) AssignAdmin(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	schoolID := chi.URLParam(r, "id")
	var req assignAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		common.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	_, err := a.DB.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1, $2, 'admin')
		ON CONFLICT (user_id, school_id, role) DO NOTHING`, req.UserID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign admin: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"school_id": schoolID, "user_id": req.UserID, "role": "admin"}, nil)
}

// deletedSchool is the minimal shape for the Riwayat Penghapusan (deletion
// history) list — name + deleted_at is the stated requirement; id/code
// riding along for a future restore action costs nothing extra here.
type deletedSchool struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      *string   `json:"code"`
	DeletedAt time.Time `json:"deleted_at"`
}

// ListDeletedSchools: Data Sekolah's "riwayat penghapusan" — soft-deleted
// schools, most recently deleted first. Superadmin only, same as the rest
// of Data Sekolah's lifecycle actions.
func (a *App) ListDeletedSchools(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	page, perPage := common.ParsePage(r)

	var total int
	if err := a.DB.QueryRow(`SELECT count(*) FROM schools WHERE deleted_at IS NOT NULL`).Scan(&total); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list deleted schools")
		return
	}
	rows, err := a.DB.Query(`SELECT id, name, code, deleted_at FROM schools WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC LIMIT $1 OFFSET $2`, perPage, (page-1)*perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list deleted schools")
		return
	}
	defer rows.Close()
	out := []deletedSchool{}
	for rows.Next() {
		var d deletedSchool
		if err := rows.Scan(&d.ID, &d.Name, &d.Code, &d.DeletedAt); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list deleted schools")
			return
		}
		out = append(out, d)
	}
	common.WriteJSON(w, http.StatusOK, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// Impersonate issues a JWT scoped to this school with the target admin's
// identity, so a superadmin can "act as" that school's admin.
func (a *App) Impersonate(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	schoolID := chi.URLParam(r, "id")

	var adminUserID string
	err := a.DB.QueryRow(`SELECT user_id FROM user_school_roles WHERE school_id=$1 AND role='admin' ORDER BY created_at LIMIT 1`, schoolID).Scan(&adminUserID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "school has no assigned admin to impersonate")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "impersonation failed")
		return
	}

	token, err := a.issueToken(adminUserID, schoolID, common.RoleAdmin, []string{common.RoleAdmin})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"token": token, "school_id": schoolID, "impersonated_user_id": adminUserID}, nil)
}
