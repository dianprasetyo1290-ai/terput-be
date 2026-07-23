package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
)

// Data Jabatan (Position) / Peran — same concept, implemented once. Admin
// creates jabatan/role labels for their school and assigns a guru to hold
// one; that feeds the read-only "Organisasi Sekolah" list (Jabatan | Nama).

type positionRequest struct {
	Name     string `json:"name"`
	SchoolID string `json:"school_id"` // only honored for superadmin; admin is forced to own school
}

func (a *App) ListPositions(w http.ResponseWriter, r *http.Request) {
	schoolID, filtered := resolveSchoolScope(r)
	if !filtered {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	rows, err := a.DB.Query(`SELECT id, school_id, name, created_at, updated_at FROM positions
		WHERE school_id = $1 AND deleted_at IS NULL ORDER BY name`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list positions")
		return
	}
	defer rows.Close()
	out := []model.Position{}
	for rows.Next() {
		var p model.Position
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.Name, &p.CreatedAt, &p.UpdatedAt); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list positions")
			return
		}
		out = append(out, p)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (a *App) CreatePosition(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	var req positionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin && req.SchoolID != "" {
		schoolID = req.SchoolID
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	var p model.Position
	err := a.DB.QueryRow(`INSERT INTO positions (school_id, name) VALUES ($1, $2)
		ON CONFLICT (school_id, name) DO UPDATE SET name = EXCLUDED.name, deleted_at = NULL, updated_at = now()
		RETURNING id, school_id, name, created_at, updated_at`, schoolID, req.Name).
		Scan(&p.ID, &p.SchoolID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create position: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, p, nil)
}

func (a *App) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req positionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	query := `UPDATE positions SET name=$1, updated_at=now() WHERE id=$2 AND deleted_at IS NULL`
	args := []interface{}{req.Name, id}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$3`
		args = append(args, claims.SchoolID)
	}
	query += ` RETURNING id, school_id, name, created_at, updated_at`
	var p model.Position
	err := a.DB.QueryRow(query, args...).Scan(&p.ID, &p.SchoolID, &p.Name, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "position not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update position")
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

func (a *App) DeletePosition(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	query := `UPDATE positions SET deleted_at=now() WHERE id=$1 AND deleted_at IS NULL`
	args := []interface{}{id}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$2`
		args = append(args, claims.SchoolID)
	}
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete position")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "position not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

type assignPositionRequest struct {
	UserID string `json:"user_id"`
}

// AssignPosition assigns a guru to hold a jabatan. If the position's name
// canonicalizes to one of the 3 jabatan-only system roles (Walikelas,
// Kepala Sekolah, Kepala Yayasan), it also mirrors that into
// user_school_roles so it appears in the JWT `roles` claim (see README
// Assumptions for why this mirroring exists).
func (a *App) AssignPosition(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	positionID := chi.URLParam(r, "id")
	var req assignPositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		common.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	var schoolID, posName string
	query := `SELECT school_id, name FROM positions WHERE id=$1 AND deleted_at IS NULL`
	args := []interface{}{positionID}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$2`
		args = append(args, claims.SchoolID)
	}
	if err := a.DB.QueryRow(query, args...).Scan(&schoolID, &posName); err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "position not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign position")
		return
	}

	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign position")
		return
	}
	defer tx.Rollback()

	var pa model.PositionAssignment
	err = tx.QueryRow(`INSERT INTO position_assignments (position_id, user_id, school_id) VALUES ($1,$2,$3)
		ON CONFLICT (position_id, user_id) DO UPDATE SET created_at = position_assignments.created_at
		RETURNING id, position_id, user_id, school_id, created_at`,
		positionID, req.UserID, schoolID).Scan(&pa.ID, &pa.PositionID, &pa.UserID, &pa.SchoolID, &pa.CreatedAt)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign position: "+err.Error())
		return
	}

	if role, ok := common.JabatanOnlyRoleForPosition(posName); ok {
		if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)
			ON CONFLICT (user_id, school_id, role) DO NOTHING`, req.UserID, schoolID, role); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to mirror jabatan role: "+err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign position")
		return
	}
	pa.PositionName = posName
	common.WriteJSON(w, http.StatusCreated, pa, nil)
}

func (a *App) UnassignPosition(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	assignmentID := chi.URLParam(r, "assignmentId")
	query := `DELETE FROM position_assignments WHERE id=$1`
	args := []interface{}{assignmentID}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$2`
		args = append(args, claims.SchoolID)
	}
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to unassign position")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "assignment not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": assignmentID}, nil)
}

// Organization returns the "Organisasi Sekolah" list: Jabatan | Nama, e.g.
// "Kepala Yayasan | Ridwan".
func (a *App) Organization(w http.ResponseWriter, r *http.Request) {
	schoolID, filtered := resolveSchoolScope(r)
	if !filtered {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	rows, err := a.DB.Query(`
		SELECT pa.id, pa.position_id, p.name, pa.user_id, u.name, pa.school_id, pa.created_at
		FROM position_assignments pa
		JOIN positions p ON p.id = pa.position_id AND p.deleted_at IS NULL
		JOIN users u ON u.id = pa.user_id AND u.deleted_at IS NULL
		WHERE pa.school_id = $1
		ORDER BY p.name, u.name`, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load organization")
		return
	}
	defer rows.Close()
	out := []model.PositionAssignment{}
	for rows.Next() {
		var pa model.PositionAssignment
		if err := rows.Scan(&pa.ID, &pa.PositionID, &pa.PositionName, &pa.UserID, &pa.UserName, &pa.SchoolID, &pa.CreatedAt); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to load organization")
			return
		}
		out = append(out, pa)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
