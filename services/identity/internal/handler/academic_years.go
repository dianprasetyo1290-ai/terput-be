package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/rules"
)

// Tahun Akademik (Academic Year). identity owns this table (label, current
// flag, date range and period lock); Semester 1/2 sub-records and all
// scheduling live in academic-service, which resolves academic_year_id by
// calling this API over HTTP (GET /academic-years[/{id}]), per CONVENTIONS.md.
//
// QA-IDN (migration 0020):
//   - BUG-070: start_date/end_date, required on create (defaulted from a
//     "YYYY/YYYY+1" label to 1 Jul - 30 Jun when omitted), end > start.
//   - BUG-015: a locked year cannot be renamed, re-dated, deleted or made
//     current; academic-service refuses semester edits in it. Making a new
//     year current auto-locks earlier years that ended before it starts
//     (BR-GEN-02). Unlock needs superadmin + reason (audited).
//   - BUG-150: kepala_yayasan gets no academic-year data.

const ayCols = `id, school_id, label, is_current, created_at, updated_at,
	to_char(start_date, 'YYYY-MM-DD'), to_char(end_date, 'YYYY-MM-DD'), locked_at, locked_by::text, lock_reason`

func scanAY(row interface{ Scan(...interface{}) error }) (model.AcademicYear, error) {
	var ay model.AcademicYear
	err := row.Scan(&ay.ID, &ay.SchoolID, &ay.Label, &ay.IsCurrent, &ay.CreatedAt, &ay.UpdatedAt,
		&ay.StartDate, &ay.EndDate, &ay.LockedAt, &ay.LockedBy, &ay.LockReason)
	return ay, err
}

const msgYearLocked = "Tahun ajaran sudah dikunci; perubahan hanya dapat dilakukan setelah dibuka oleh superadmin"

type academicYearRequest struct {
	SchoolID  string `json:"school_id"`
	Label     string `json:"label"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// yearDates parses and validates the requested range; when both are empty
// and useLabelDefault is set, the range is derived from the label.
func yearDates(label, start, end string, useLabelDefault bool) (*time.Time, *time.Time, rules.FieldErrors) {
	s, okS := rules.ParseDate(start)
	e, okE := rules.ParseDate(end)
	errs := rules.FieldErrors{}
	if !okS {
		errs["start_date"] = "Format tanggal mulai harus YYYY-MM-DD"
	}
	if !okE {
		errs["end_date"] = "Format tanggal selesai harus YYYY-MM-DD"
	}
	if len(errs) > 0 {
		return nil, nil, errs
	}
	if s == nil && e == nil && useLabelDefault {
		if ds, de, ok := rules.DefaultYearRange(label); ok {
			s, e = &ds, &de
		}
	}
	return s, e, rules.ValidateYearDates(s, e)
}

func dateArg(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}

func (a *App) ListAcademicYears(w http.ResponseWriter, r *http.Request) {
	if c := common.FromCtx(r); c != nil && c.ActiveRole == common.RoleKepalaYayasan { // BUG-150
		common.WriteError(w, http.StatusForbidden, "Peran ini tidak memiliki akses ke data tahun ajaran")
		return
	}
	schoolID, filtered := resolveSchoolScope(r)
	query := `SELECT ` + ayCols + ` FROM academic_years WHERE deleted_at IS NULL`
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
		ay, err := scanAY(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list academic years")
			return
		}
		out = append(out, ay)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// GetAcademicYear: GET /academic-years/{id} (internal or JWT). Used by
// academic-service to validate semester dates against the year's range and
// lock.
func (a *App) GetAcademicYear(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if c != nil && c.ActiveRole == common.RoleKepalaYayasan {
		common.WriteError(w, http.StatusForbidden, "Peran ini tidak memiliki akses ke data tahun ajaran")
		return
	}
	ay, err := scanAY(a.DB.QueryRow(`SELECT `+ayCols+` FROM academic_years WHERE id=$1 AND deleted_at IS NULL`, chi.URLParam(r, "id")))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "academic year not found")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "failed to get academic year")
		return
	}
	if c != nil && c.ActiveRole != common.RoleSuperadmin && c.SchoolID != ay.SchoolID {
		common.WriteError(w, http.StatusNotFound, "academic year not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, ay, nil)
}

func (a *App) CreateAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	var req academicYearRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Label) == "" {
		common.WriteValidationError(w, http.StatusBadRequest, "Label tahun ajaran wajib diisi", map[string]string{"label": "Label tahun ajaran wajib diisi"})
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	// Root cause of "Gagal membuat tahun akademik" firing on a genuinely
	// unused label: this only ever checked req.SchoolID (JSON body), but
	// the axios client (web/src/api/client.ts) injects the superadmin's
	// impersonated school as a QUERY param (?school_id=), same as every
	// GET here goes through resolveSchoolScope for. Check the query param
	// first (matches resolveSchoolScope), body as a fallback.
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
	start, end, errs := yearDates(req.Label, req.StartDate, req.EndDate, true)
	if len(errs) > 0 {
		writeFieldErrors(w, errs)
		return
	}
	ay, err := scanAY(a.DB.QueryRow(`INSERT INTO academic_years (school_id, label, start_date, end_date) VALUES ($1, $2, $3, $4)
		RETURNING `+ayCols, schoolID, req.Label, dateArg(start), dateArg(end)))
	if isUniqueViolation(err) {
		common.WriteValidationError(w, http.StatusConflict, "Label tahun ajaran sudah digunakan di unit ini", map[string]string{"label": "Label tahun ajaran sudah digunakan di unit ini"})
		return
	}
	if err != nil {
		common.WriteDBError(w, err, "failed to create academic year")
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

// loadYearForWrite fetches a live year the caller may modify (own school
// unless superadmin). It writes the 404/403 itself and returns ok=false.
func (a *App) loadYearForWrite(w http.ResponseWriter, r *http.Request, id string) (model.AcademicYear, bool) {
	claims := common.FromCtx(r)
	ay, err := scanAY(a.DB.QueryRow(`SELECT `+ayCols+` FROM academic_years WHERE id=$1 AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "academic year not found")
		return ay, false
	} else if err != nil {
		common.WriteDBError(w, err, "failed to load academic year")
		return ay, false
	}
	if claims.ActiveRole != common.RoleSuperadmin && claims.SchoolID != ay.SchoolID {
		common.WriteError(w, http.StatusForbidden, "forbidden")
		return ay, false
	}
	return ay, true
}

// UpdateAcademicYear: PUT /academic-years/{id} — label and date range.
// Omitted dates keep the stored ones. is_current is managed exclusively via
// SetCurrentAcademicYear. Locked years are read-only.
func (a *App) UpdateAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	id := chi.URLParam(r, "id")
	var req academicYearRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Label) == "" {
		common.WriteValidationError(w, http.StatusBadRequest, "Label tahun ajaran wajib diisi", map[string]string{"label": "Label tahun ajaran wajib diisi"})
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	cur, ok := a.loadYearForWrite(w, r, id)
	if !ok {
		return
	}
	if cur.LockedAt != nil {
		common.WriteError(w, http.StatusConflict, msgYearLocked)
		return
	}
	start, end := req.StartDate, req.EndDate
	if start == "" && cur.StartDate != nil {
		start = *cur.StartDate
	}
	if end == "" && cur.EndDate != nil {
		end = *cur.EndDate
	}
	var sArg, eArg interface{}
	if start != "" || end != "" {
		s, e, errs := yearDates(req.Label, start, end, false)
		if len(errs) > 0 {
			writeFieldErrors(w, errs)
			return
		}
		sArg, eArg = dateArg(s), dateArg(e)
	}
	ay, err := scanAY(a.DB.QueryRow(`UPDATE academic_years SET label=$1, start_date=$2, end_date=$3, updated_at=now()
		WHERE id=$4 AND deleted_at IS NULL AND locked_at IS NULL RETURNING `+ayCols, req.Label, sArg, eArg, id))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusConflict, msgYearLocked)
		return
	}
	if isUniqueViolation(err) {
		common.WriteValidationError(w, http.StatusConflict, "Label tahun ajaran sudah digunakan di unit ini", map[string]string{"label": "Label tahun ajaran sudah digunakan di unit ini"})
		return
	}
	if err != nil {
		common.WriteDBError(w, err, "failed to update academic year")
		return
	}
	common.WriteJSON(w, http.StatusOK, ay, nil)
}

// DeleteAcademicYear: DELETE /academic-years/{id} — soft delete (see
// migration 0004). Refuses the school's current year (pick another current
// year first) and locked years.
func (a *App) DeleteAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	id := chi.URLParam(r, "id")
	ay, ok := a.loadYearForWrite(w, r, id)
	if !ok {
		return
	}
	if ay.IsCurrent {
		common.WriteError(w, http.StatusBadRequest, "tidak dapat menghapus tahun akademik yang sedang berjalan; jadikan tahun lain sebagai tahun berjalan terlebih dahulu")
		return
	}
	if ay.LockedAt != nil {
		common.WriteError(w, http.StatusConflict, msgYearLocked)
		return
	}
	if _, err := a.DB.Exec(`UPDATE academic_years SET deleted_at = now() WHERE id = $1 AND locked_at IS NULL`, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete academic year")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// SetCurrentAcademicYear marks this academic year as the school's current
// one (unsets any previous current year for that school), points
// schools.current_academic_year_id at it and locks earlier years that ended
// before this one starts (BR-GEN-02).
func (a *App) SetCurrentAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	ay, ok := a.loadYearForWrite(w, r, id)
	if !ok {
		return
	}
	if ay.LockedAt != nil {
		common.WriteError(w, http.StatusConflict, "Tahun ajaran yang terkunci tidak dapat dijadikan tahun berjalan")
		return
	}
	schoolID := ay.SchoolID

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
	var locked []string
	if ay.StartDate != nil {
		rows, err := tx.Query(`UPDATE academic_years SET locked_at=now(), locked_by=$3, lock_reason='Otomatis: tahun ajaran baru diaktifkan', updated_at=now()
			WHERE school_id=$1 AND id<>$2 AND deleted_at IS NULL AND locked_at IS NULL AND end_date IS NOT NULL AND end_date < $4::date
			RETURNING id`, schoolID, id, nullUUID(claims.UserID), *ay.StartDate)
		if err != nil {
			common.WriteDBError(w, err, "failed to set current academic year")
			return
		}
		for rows.Next() {
			var lid string
			if err := rows.Scan(&lid); err == nil {
				locked = append(locked, lid)
			}
		}
		rows.Close()
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to set current academic year")
		return
	}
	if len(locked) > 0 {
		a.RecordSecurityAudit(&schoolID, "academic_years_auto_locked", id, &claims.UserID, &claims.ActiveRole, nil,
			map[string]any{"locked_year_ids": locked}, "new academic year activated", r.Header.Get("X-Correlation-ID"))
	}
	if locked == nil {
		locked = []string{}
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "school_id": schoolID, "locked_year_ids": locked}, nil)
}

func nullUUID(s string) interface{} {
	if rules.IsUUID(s) {
		return s
	}
	return nil
}

// LockAcademicYear: POST /academic-years/{id}/lock {reason} — admin of the
// unit or superadmin.
func (a *App) LockAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ay, ok := a.loadYearForWrite(w, r, id)
	if !ok {
		return
	}
	if ay.LockedAt != nil {
		common.WriteError(w, http.StatusConflict, "Tahun ajaran sudah terkunci")
		return
	}
	if ay.IsCurrent {
		common.WriteError(w, http.StatusConflict, "Tahun ajaran berjalan tidak dapat dikunci; aktifkan tahun ajaran berikutnya terlebih dahulu")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = "Dikunci manual"
	}
	ay, err := scanAY(a.DB.QueryRow(`UPDATE academic_years SET locked_at=now(), locked_by=$2, lock_reason=$3, updated_at=now()
		WHERE id=$1 AND locked_at IS NULL RETURNING `+ayCols, id, nullUUID(claims.UserID), reason))
	if err != nil {
		common.WriteDBError(w, err, "failed to lock academic year")
		return
	}
	a.RecordSecurityAudit(&ay.SchoolID, "academic_year_locked", id, &claims.UserID, &claims.ActiveRole, nil, map[string]any{"label": ay.Label}, reason, r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, ay, nil)
}

// UnlockAcademicYear: POST /academic-years/{id}/unlock {reason} — the
// "otorisasi" path for changing a closed period: superadmin only, reason
// of at least 10 characters, audited.
func (a *App) UnlockAcademicYear(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if len(strings.TrimSpace(body.Reason)) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan membuka kunci wajib diisi (minimal 10 karakter)", map[string]string{"reason": "Alasan wajib diisi (minimal 10 karakter)"})
		return
	}
	ay, ok := a.loadYearForWrite(w, r, id)
	if !ok {
		return
	}
	if ay.LockedAt == nil {
		common.WriteError(w, http.StatusConflict, "Tahun ajaran tidak terkunci")
		return
	}
	before := map[string]any{"locked_at": ay.LockedAt, "lock_reason": ay.LockReason}
	ay, err := scanAY(a.DB.QueryRow(`UPDATE academic_years SET locked_at=NULL, locked_by=NULL, lock_reason=NULL, updated_at=now()
		WHERE id=$1 RETURNING `+ayCols, id))
	if err != nil {
		common.WriteDBError(w, err, "failed to unlock academic year")
		return
	}
	a.RecordSecurityAudit(&ay.SchoolID, "academic_year_unlocked", id, &claims.UserID, &claims.ActiveRole, before, map[string]any{"label": ay.Label}, body.Reason, r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, ay, nil)
}
