package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
)

// BUG-073 Data Referensi: master lists used by forms instead of free text.
// Global rows (school_id NULL) are maintained by superadmin; a unit admin
// adds unit rows (e.g. its own program/jurusan) and can switch them off.
//
//	GET  /references?category=&include_inactive=true
//	POST /references        {category, code, label, sort_order, school_id?}
//	PUT  /references/{id}   {label, sort_order, active}

var referenceCategories = map[string]string{
	"agama":          "Agama",
	"jenis_kelamin":  "Jenis Kelamin",
	"pekerjaan_ortu": "Pekerjaan Orang Tua",
	"program":        "Program / Jurusan",
}

var reRefCode = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

type referenceValue struct {
	ID        string    `json:"id"`
	SchoolID  *string   `json:"school_id"`
	Category  string    `json:"category"`
	Code      string    `json:"code"`
	Label     string    `json:"label"`
	SortOrder int       `json:"sort_order"`
	Active    bool      `json:"active"`
	Global    bool      `json:"global"`
	UpdatedAt time.Time `json:"updated_at"`
}

const refCols = `id, school_id::text, category, code, label, sort_order, active, updated_at`

func scanRef(row interface{ Scan(...any) error }) (referenceValue, error) {
	var v referenceValue
	err := row.Scan(&v.ID, &v.SchoolID, &v.Category, &v.Code, &v.Label, &v.SortOrder, &v.Active, &v.UpdatedAt)
	v.Global = v.SchoolID == nil
	return v, err
}

// referenceLabel resolves value (code or label, case-insensitive) in an
// active row of category visible to schoolID; returns the label.
func (a *App) referenceLabel(ctx context.Context, category, value, schoolID string) (string, bool, error) {
	var label string
	err := a.DB.QueryRowContext(ctx, `SELECT label FROM reference_values
		WHERE category=$1 AND active AND (school_id IS NULL OR school_id::text=$3)
		  AND (lower(code)=lower($2) OR lower(label)=lower($2))
		ORDER BY school_id NULLS LAST LIMIT 1`, category, strings.TrimSpace(value), schoolID).Scan(&label)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return label, err == nil, err
}

// ListReferences: any signed-in user (forms need the lists).
func (a *App) ListReferences(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	school := claims.SchoolID
	if claims.ActiveRole == common.RoleSuperadmin {
		school = r.URL.Query().Get("school_id")
	}
	category := r.URL.Query().Get("category")
	if category != "" {
		if _, ok := referenceCategories[category]; !ok {
			common.WriteValidationError(w, http.StatusBadRequest, "Kategori referensi tidak dikenal", map[string]string{"category": "Tidak dikenal"})
			return
		}
	}
	inactive := r.URL.Query().Get("include_inactive") == "true" &&
		(claims.ActiveRole == common.RoleAdmin || claims.ActiveRole == common.RoleSuperadmin)
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+refCols+` FROM reference_values
		WHERE ($1='' OR category=$1) AND (school_id IS NULL OR school_id::text=$2) AND ($3 OR active)
		ORDER BY category, sort_order, label`, category, school, inactive)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat data referensi")
		return
	}
	defer rows.Close()
	out := []referenceValue{}
	for rows.Next() {
		v, err := scanRef(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat data referensi")
			return
		}
		out = append(out, v)
	}
	common.WriteJSON(w, http.StatusOK, out, map[string]any{"categories": referenceCategories})
}

type referenceInput struct {
	Category  string `json:"category"`
	Code      string `json:"code"`
	Label     string `json:"label"`
	SortOrder *int   `json:"sort_order"`
	Active    *bool  `json:"active"`
	SchoolID  string `json:"school_id"`
}

func validateReference(in *referenceInput, create bool) map[string]string {
	errs := map[string]string{}
	in.Code = strings.ToLower(strings.TrimSpace(in.Code))
	in.Label = strings.TrimSpace(in.Label)
	if create {
		if _, ok := referenceCategories[in.Category]; !ok {
			errs["category"] = "Pilih kategori"
		}
		if !reRefCode.MatchString(in.Code) {
			errs["code"] = "Kode 1–40 karakter: huruf kecil, angka atau garis bawah"
		}
	}
	if in.Label == "" || len([]rune(in.Label)) > 100 {
		errs["label"] = "Label wajib diisi (maksimal 100 karakter)"
	}
	return errs
}

// CreateReference: POST /references.
func (a *App) CreateReference(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "references", "create") {
		return
	}
	claims := common.FromCtx(r)
	var in referenceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	if errs := validateReference(&in, true); len(errs) > 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Periksa kembali isian referensi", errs)
		return
	}
	var school any
	switch {
	case claims == nil:
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return
	case claims.ActiveRole == common.RoleSuperadmin:
		school = nullIfEmpty(in.SchoolID) // empty = global
	default:
		school = claims.SchoolID
	}
	sort := 0
	if in.SortOrder != nil {
		sort = *in.SortOrder
	}
	v, err := scanRef(a.DB.QueryRowContext(r.Context(), `INSERT INTO reference_values (school_id, category, code, label, sort_order)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+refCols, school, in.Category, in.Code, in.Label, sort))
	if isUniqueViolation(err) {
		common.WriteValidationError(w, http.StatusConflict, "Kode atau label sudah ada di kategori ini", map[string]string{"code": "Sudah dipakai", "label": "Sudah dipakai"})
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan referensi")
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, v, nil, "Referensi ditambahkan")
}

// UpdateReference: PUT /references/{id} — label, order and active only (the
// code is what other records may hold, so it never changes).
func (a *App) UpdateReference(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "references", "edit") {
		return
	}
	claims := common.FromCtx(r)
	var in referenceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	if errs := validateReference(&in, false); len(errs) > 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Periksa kembali isian referensi", errs)
		return
	}
	cur, err := scanRef(a.DB.QueryRowContext(r.Context(), `SELECT `+refCols+` FROM reference_values WHERE id::text=$1`, chi.URLParam(r, "id")))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "Referensi tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat referensi")
		return
	}
	if claims != nil && claims.ActiveRole != common.RoleSuperadmin {
		if cur.SchoolID == nil {
			common.WriteError(w, http.StatusForbidden, "Referensi nasional hanya dapat diubah Super Admin")
			return
		}
		if *cur.SchoolID != claims.SchoolID {
			common.WriteError(w, http.StatusNotFound, "Referensi tidak ditemukan")
			return
		}
	}
	sort, active := cur.SortOrder, cur.Active
	if in.SortOrder != nil {
		sort = *in.SortOrder
	}
	if in.Active != nil {
		active = *in.Active
	}
	v, err := scanRef(a.DB.QueryRowContext(r.Context(), `UPDATE reference_values SET label=$2, sort_order=$3, active=$4, updated_at=now()
		WHERE id=$1 RETURNING `+refCols, cur.ID, in.Label, sort, active))
	if isUniqueViolation(err) {
		common.WriteValidationError(w, http.StatusConflict, "Label sudah ada di kategori ini", map[string]string{"label": "Sudah dipakai"})
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan referensi")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, v, nil, "Referensi disimpan")
}
