package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/rules"
)

func changed(req, stored *string) bool {
	if req == nil {
		return false
	}
	if stored == nil {
		return *req != ""
	}
	return *req != *stored
}

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
	// GovernmentLogo: SEKOCI V10 item C -- the "Logo Pemerintah" printed at
	// the rapor letterhead's top-left, beside the school's own logo.
	GovernmentLogo *string `json:"government_logo"`
	// QA-IDN (migration 0020). Partial-update semantics: omitted (null)
	// keeps the stored value, "" clears it -- older forms that do not send
	// these fields must not wipe them.
	PrincipalName           *string `json:"principal_name"`
	PrincipalUserID         *string `json:"principal_user_id"`
	AccountingEntityCode    *string `json:"accounting_entity_code"`
	SettlementBankName      *string `json:"settlement_bank_name"`
	SettlementAccountNumber *string `json:"settlement_account_number"`
	SettlementAccountName   *string `json:"settlement_account_name"`
}

func (q *schoolRequest) trim() {
	for _, p := range []**string{&q.Code, &q.Level, &q.Email, &q.PrincipalName, &q.PrincipalUserID, &q.AccountingEntityCode,
		&q.SettlementBankName, &q.SettlementAccountNumber, &q.SettlementAccountName} {
		if *p != nil {
			v := strings.TrimSpace(**p)
			*p = &v
		}
	}
	q.Name = strings.TrimSpace(q.Name)
	if q.Code != nil {
		v := strings.ToUpper(*q.Code)
		q.Code = &v
	}
}

// validate checks the payload merged with the stored values (update) so a
// settlement account is judged as a whole.
func (q schoolRequest) validate(cur *model.School) rules.FieldErrors {
	pick := func(req, stored *string) *string {
		if req != nil {
			return req
		}
		return stored
	}
	in := rules.School{Name: q.Name, Code: q.Code, Level: q.Level, Email: q.Email,
		SettlementAccountNumber: q.SettlementAccountNumber, SettlementBankName: q.SettlementBankName, SettlementAccountName: q.SettlementAccountName}
	if cur != nil {
		in.SettlementAccountNumber = pick(q.SettlementAccountNumber, cur.SettlementAccountNumber)
		in.SettlementBankName = pick(q.SettlementBankName, cur.SettlementBankName)
		in.SettlementAccountName = pick(q.SettlementAccountName, cur.SettlementAccountName)
	}
	errs := rules.ValidateSchool(in)
	if q.PrincipalUserID != nil && *q.PrincipalUserID != "" && !rules.IsUUID(*q.PrincipalUserID) {
		errs["principal_user_id"] = "Kepala unit tidak valid"
	}
	return errs
}

func writeFieldErrors(w http.ResponseWriter, errs rules.FieldErrors) {
	msg := "Data belum valid"
	for _, k := range []string{"name", "code", "level"} {
		if errs[k] != "" {
			msg = errs[k]
			break
		}
	}
	common.WriteValidationError(w, http.StatusBadRequest, msg, errs)
}

const schoolCols = `id, foundation_id, name, code, level, address, subdistrict, district, city, province,
	registration_number, year_founded, phone_number, email, website, logo, government_logo, is_active, current_academic_year_id, created_at, updated_at,
	principal_name, principal_user_id, accounting_entity_code, settlement_bank_name, settlement_account_number, settlement_account_name`

func scanSchool(row interface{ Scan(...interface{}) error }) (model.School, error) {
	var s model.School
	err := row.Scan(&s.ID, &s.FoundationID, &s.Name, &s.Code, &s.Level, &s.Address, &s.Subdistrict, &s.District, &s.City, &s.Province,
		&s.RegistrationNumber, &s.YearFounded, &s.PhoneNumber, &s.Email, &s.Website, &s.Logo, &s.GovernmentLogo, &s.IsActive, &s.CurrentAcademicYearID, &s.CreatedAt, &s.UpdatedAt,
		&s.PrincipalName, &s.PrincipalUserID, &s.AccountingEntityCode, &s.SettlementBankName, &s.SettlementAccountNumber, &s.SettlementAccountName)
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
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
		return
	}
	args = append(args, perPage, (page-1)*perPage)
	query += " ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args)-1) + " OFFSET $" + strconv.Itoa(len(args))

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
		return
	}
	defer rows.Close()
	out := []model.School{}
	for rows.Next() {
		s, err := scanSchool(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar unit")
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format data tidak valid")
		return
	}
	req.trim()
	if errs := req.validate(nil); len(errs) > 0 {
		writeFieldErrors(w, errs)
		return
	}
	row := a.DB.QueryRow(`INSERT INTO schools (foundation_id, name, code, level, address, subdistrict, district, city, province,
			registration_number, year_founded, phone_number, email, website, logo, government_logo,
			principal_name, principal_user_id, accounting_entity_code, settlement_bank_name, settlement_account_number, settlement_account_name)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,
			NULLIF($17,''), NULLIF($18,'')::uuid, NULLIF($19,''), NULLIF($20,''), NULLIF($21,''), NULLIF($22,''))
		RETURNING `+schoolCols,
		req.FoundationID, req.Name, req.Code, req.Level, req.Address, req.Subdistrict, req.District, req.City, req.Province,
		req.RegistrationNumber, req.YearFounded, req.PhoneNumber, req.Email, req.Website, req.Logo, req.GovernmentLogo,
		req.PrincipalName, req.PrincipalUserID, req.AccountingEntityCode, req.SettlementBankName, req.SettlementAccountNumber, req.SettlementAccountName)
	s, err := scanSchool(row)
	if err != nil {
		common.WriteDBError(w, err, "Gagal membuat unit")
		return
	}
	common.WriteJSON(w, http.StatusCreated, s, nil)
}

func (a *App) GetSchool(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	if claims == nil && !common.IsInternal(r) {
		common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
		return
	}
	if claims != nil && claims.ActiveRole != common.RoleSuperadmin && claims.SchoolID != id {
		common.WriteError(w, http.StatusForbidden, "Tidak dapat mengakses unit lain")
		return
	}
	row := a.DB.QueryRow(`SELECT `+schoolCols+` FROM schools WHERE id = $1 AND deleted_at IS NULL`, id)
	s, err := scanSchool(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Unit tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat unit")
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
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	var req schoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format data tidak valid")
		return
	}
	req.trim()
	cur, err := scanSchool(a.DB.QueryRow(`SELECT `+schoolCols+` FROM schools WHERE id=$1 AND deleted_at IS NULL`, id))
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Unit tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memperbarui unit")
		return
	}
	if errs := req.validate(&cur); len(errs) > 0 {
		writeFieldErrors(w, errs)
		return
	}
	// Settlement account and accounting entity feed billing/accounting:
	// only superadmin (yayasan level) may change them.
	if claims.ActiveRole != common.RoleSuperadmin && (changed(req.AccountingEntityCode, cur.AccountingEntityCode) ||
		changed(req.SettlementBankName, cur.SettlementBankName) || changed(req.SettlementAccountNumber, cur.SettlementAccountNumber) ||
		changed(req.SettlementAccountName, cur.SettlementAccountName)) {
		common.WriteError(w, http.StatusForbidden, "Entitas akuntansi dan rekening settlement hanya dapat diubah superadmin")
		return
	}
	row := a.DB.QueryRow(`UPDATE schools SET foundation_id=$1, name=$2, code=$3, level=$4, address=$5, subdistrict=$6, district=$7,
			city=$8, province=$9, registration_number=$10, year_founded=$11, phone_number=$12, email=$13, website=$14, logo=$15, government_logo=$16,
			principal_name            = CASE WHEN $18::text IS NULL THEN principal_name ELSE NULLIF($18,'') END,
			principal_user_id         = CASE WHEN $19::text IS NULL THEN principal_user_id ELSE NULLIF($19,'')::uuid END,
			accounting_entity_code    = CASE WHEN $20::text IS NULL THEN accounting_entity_code ELSE NULLIF($20,'') END,
			settlement_bank_name      = CASE WHEN $21::text IS NULL THEN settlement_bank_name ELSE NULLIF($21,'') END,
			settlement_account_number = CASE WHEN $22::text IS NULL THEN settlement_account_number ELSE NULLIF($22,'') END,
			settlement_account_name   = CASE WHEN $23::text IS NULL THEN settlement_account_name ELSE NULLIF($23,'') END,
			updated_at=now()
		WHERE id=$17 AND deleted_at IS NULL
		RETURNING `+schoolCols,
		req.FoundationID, req.Name, req.Code, req.Level, req.Address, req.Subdistrict, req.District, req.City, req.Province,
		req.RegistrationNumber, req.YearFounded, req.PhoneNumber, req.Email, req.Website, req.Logo, req.GovernmentLogo, id,
		req.PrincipalName, req.PrincipalUserID, req.AccountingEntityCode, req.SettlementBankName, req.SettlementAccountNumber, req.SettlementAccountName)
	s, err := scanSchool(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "Unit tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memperbarui unit")
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
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus unit")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "Unit tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (a *App) setSchoolActive(w http.ResponseWriter, r *http.Request, active bool) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	id := chi.URLParam(r, "id")
	claims := common.FromCtx(r)
	var body struct {
		Confirm bool   `json:"confirm"`
		Reason  string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	var impact schoolImpact
	if !active {
		// BUG-014: show what deactivating locks out before doing it. Data is
		// never deleted (BR-GEN-07); only is_active flips.
		err := a.DB.QueryRow(`SELECT
				count(DISTINCT user_id) FILTER (WHERE role='siswa'),
				count(DISTINCT user_id) FILTER (WHERE role IN ('guru','walikelas')),
				count(DISTINCT user_id)
			FROM user_school_roles usr JOIN users u ON u.id = usr.user_id AND u.deleted_at IS NULL
			WHERE usr.school_id=$1 AND usr.status='active'`, id).Scan(&impact.ActiveStudents, &impact.ActiveTeachers, &impact.ActiveUsers)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memperbarui status unit")
			return
		}
		if impact.ActiveUsers > 0 && (!body.Confirm || strings.TrimSpace(body.Reason) == "") {
			common.WriteJSONMsg(w, http.StatusConflict, map[string]any{"requires_confirmation": true, "impact": impact},
				nil, "Unit masih memiliki pengguna aktif. Menonaktifkan unit akan memblokir akses mereka (data tetap tersimpan). Kirim confirm=true dan alasan untuk melanjutkan.")
			return
		}
	}
	res, err := a.DB.Exec(`UPDATE schools SET is_active=$1, updated_at=now() WHERE id=$2 AND deleted_at IS NULL`, active, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui status unit")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "Unit tidak ditemukan")
		return
	}
	action := "school_activated"
	if !active {
		action = "school_deactivated"
	}
	a.RecordSecurityAudit(&id, action, id, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"is_active": active, "impact": impact}, body.Reason, r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"id": id, "is_active": active, "impact": impact}, nil)
}

type schoolImpact struct {
	ActiveStudents int `json:"active_students"`
	ActiveTeachers int `json:"active_teachers"`
	ActiveUsers    int `json:"active_users"`
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
		common.WriteError(w, http.StatusBadRequest, "Pengguna (user_id) wajib diisi")
		return
	}
	_, err := a.DB.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1, $2, 'admin')
		ON CONFLICT (user_id, school_id, role) DO NOTHING`, req.UserID, schoolID)
	if err != nil {
		common.WriteDBError(w, err, "Gagal menetapkan admin")
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
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat riwayat penghapusan unit")
		return
	}
	rows, err := a.DB.Query(`SELECT id, name, code, deleted_at FROM schools WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC LIMIT $1 OFFSET $2`, perPage, (page-1)*perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat riwayat penghapusan unit")
		return
	}
	defer rows.Close()
	out := []deletedSchool{}
	for rows.Next() {
		var d deletedSchool
		if err := rows.Scan(&d.ID, &d.Name, &d.Code, &d.DeletedAt); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat riwayat penghapusan unit")
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
		common.WriteError(w, http.StatusNotFound, "Unit ini belum memiliki admin untuk diakses")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk sebagai admin unit")
		return
	}

	token, _, err := a.issueToken(r, adminUserID, schoolID, common.RoleAdmin, []string{common.RoleAdmin}, "", false)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat sesi. Coba lagi")
		return
	}
	c := common.FromCtx(r)
	a.RecordSecurityAudit(&schoolID, "impersonation_started", adminUserID, &c.UserID, &c.ActiveRole, nil, map[string]string{"school_id": schoolID}, "superadmin impersonated school admin", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"token": token, "school_id": schoolID, "impersonated_user_id": adminUserID}, nil)
}
