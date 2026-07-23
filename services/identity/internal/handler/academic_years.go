package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
)

// Tahun Akademik (Academic Year). identity owns this table (id/label/current
// flag only); Semester 1/2 sub-records and all scheduling live in
// academic-service, which resolves academic_year_id by calling this API
// over HTTP (GET/POST /academic-years), per CONVENTIONS.md.

type academicYearRequest struct {
	SchoolID string `json:"school_id"`
	Label    string `json:"label"`
}

func (a *App) ListAcademicYears(w http.ResponseWriter, r *http.Request) {
	schoolID, filtered := resolveSchoolScope(r)
	query := `SELECT id, school_id, label, is_current, created_at, updated_at FROM academic_years WHERE deleted_at IS NULL`
	args := []interface{}{}
	if filtered {
		query += ` AND school_id = $1`
		args = append(args, schoolID)
	}
	query += ` ORDER BY label DESC`
	rows, err := a.DB.Query(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list academic years")
		return
	}
	defer rows.Close()
	out := []model.AcademicYear{}
	for rows.Next() {
		var ay model.AcademicYear
		if err := rows.Scan(&ay.ID, &ay.SchoolID, &ay.Label, &ay.IsCurrent, &ay.CreatedAt, &ay.UpdatedAt); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list academic years")
			return
		}
		out = append(out, ay)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (a *App) CreateAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	var req academicYearRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Label == "" {
		common.WriteError(w, http.StatusBadRequest, "label is required")
		return
	}
	// Root cause of "Gagal membuat tahun akademik" firing on a genuinely
	// unused label: this only ever checked req.SchoolID (JSON body), but
	// the axios client (web/src/api/client.ts) injects the superadmin's
	// impersonated school as a QUERY param (?school_id=), same as every
	// GET here goes through resolveSchoolScope for. A superadmin managing
	// a school via the picker therefore always sent schoolID="" and hit
	// the 400 below regardless of the label — the frontend then papered
	// over that 400 with a hardcoded "label already used" message. Check
	// the query param first (matches resolveSchoolScope), body as a
	// fallback for any other caller shape.
	schoolID := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin {
		if q := r.URL.Query().Get("school_id"); q != "" {
			schoolID = q
		} else if req.SchoolID != "" {
			schoolID = req.SchoolID
		}
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	var ay model.AcademicYear
	err := a.DB.QueryRow(`INSERT INTO academic_years (school_id, label) VALUES ($1, $2)
		RETURNING id, school_id, label, is_current, created_at, updated_at`, schoolID, req.Label).
		Scan(&ay.ID, &ay.SchoolID, &ay.Label, &ay.IsCurrent, &ay.CreatedAt, &ay.UpdatedAt)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "label tahun akademik sudah digunakan di sekolah ini")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create academic year: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, ay, nil)
}

// isUniqueViolation checks a database/sql error against pgx's error type —
// this service's driver is pgx/v5/stdlib (see internal/store/db.go), not
// lib/pq, so the error unwraps to *pgconn.PgError, not *pq.Error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

type updateAcademicYearRequest struct {
	Label string `json:"label"`
}

// UpdateAcademicYear: PUT /academic-years/{id} — rename only (is_current is
// managed exclusively via SetCurrentAcademicYear, which also fixes up
// schools.current_academic_year_id).
func (a *App) UpdateAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req updateAcademicYearRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Label == "" {
		common.WriteError(w, http.StatusBadRequest, "label is required")
		return
	}

	query := `UPDATE academic_years SET label = $1, updated_at = now()
		WHERE id = $2 AND deleted_at IS NULL`
	args := []interface{}{req.Label, id}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id = $3`
		args = append(args, claims.SchoolID)
	}
	query += ` RETURNING id, school_id, label, is_current, created_at, updated_at`

	var ay model.AcademicYear
	err := a.DB.QueryRow(query, args...).Scan(&ay.ID, &ay.SchoolID, &ay.Label, &ay.IsCurrent, &ay.CreatedAt, &ay.UpdatedAt)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "academic year not found")
		return
	}
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "label tahun akademik sudah digunakan di sekolah ini")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update academic year: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, ay, nil)
}

// DeleteAcademicYear: DELETE /academic-years/{id} — soft delete (see
// migration 0004). Refuses to delete the school's current year: deleting it
// would silently null out schools.current_academic_year_id (ON DELETE SET
// NULL is for the FK's hard-delete case, but soft-delete bypasses that, so
// this is an explicit application-level guard) and leave the school with no
// current year until an admin picks a new one manually anyway — better to
// make them do that first.
func (a *App) DeleteAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")

	var schoolID string
	var isCurrent bool
	err := a.DB.QueryRow(`SELECT school_id, is_current FROM academic_years WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&schoolID, &isCurrent)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "academic year not found")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete academic year")
		return
	}
	if claims.ActiveRole != common.RoleSuperadmin && claims.SchoolID != schoolID {
		common.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	if isCurrent {
		common.WriteError(w, http.StatusBadRequest, "tidak dapat menghapus tahun akademik yang sedang berjalan; jadikan tahun lain sebagai tahun berjalan terlebih dahulu")
		return
	}

	if _, err := a.DB.Exec(`UPDATE academic_years SET deleted_at = now() WHERE id = $1`, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete academic year")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// SetCurrentAcademicYear marks this academic year as the school's current
// one (unsets any previous current year for that school) and points
// schools.current_academic_year_id at it.
func (a *App) SetCurrentAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")

	var schoolID string
	if err := a.DB.QueryRow(`SELECT school_id FROM academic_years WHERE id=$1`, id).Scan(&schoolID); err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "academic year not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to set current academic year")
		return
	}
	if claims.ActiveRole != common.RoleSuperadmin && claims.SchoolID != schoolID {
		common.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to set current academic year")
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE academic_years SET is_current=false WHERE school_id=$1`, schoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to set current academic year")
		return
	}
	if _, err := tx.Exec(`UPDATE academic_years SET is_current=true, updated_at=now() WHERE id=$1`, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to set current academic year")
		return
	}
	if _, err := tx.Exec(`UPDATE schools SET current_academic_year_id=$1, updated_at=now() WHERE id=$2`, id, schoolID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to set current academic year")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to set current academic year")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "school_id": schoolID}, nil)
}
