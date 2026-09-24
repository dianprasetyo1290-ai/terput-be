package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"skci/report/internal/common"
	"skci/report/internal/model"
)

// isPgErrorCode reports whether err is a Postgres error with the given
// SQLSTATE code (driver here is pgx/v5/stdlib, so errors surface as
// *pgconn.PgError, not lib/pq's *pq.Error -- that package is only an
// indirect dep used elsewhere for pq.Array literal formatting).
func isPgErrorCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// Item 4: "Rapor dibuat dinamis, diatur di Admin" -- this file is the
// admin config surface (assessment types + weight formulas) the rest of
// report-service's generate/formative logic reads instead of hardcoding.
// GET is staff (admin/superadmin/guru) since the guru dropdown (item 5)
// needs the type list too; mutations are admin-only, except Create which
// guru may also call (Yadika13 item 8).

// ListAssessmentTypes: GET /settings/assessment-types
func (h *Handler) ListAssessmentTypes(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	types, err := h.d.Store.ListAssessmentTypes(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch assessment types")
		return
	}
	common.WriteJSON(w, http.StatusOK, types, nil)
}

type assessmentTypeBody struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Bucket        string  `json:"bucket"`
	WeightPercent float64 `json:"weight_percent"`
	ExamLinked    bool    `json:"exam_linked"`
	SortOrder     int     `json:"sort_order"`
}

func validBucket(b string) bool {
	switch b {
	case "formatif", "praktik", "sts", "sas":
		return true
	}
	return false
}

// CreateAssessmentType: POST /settings/assessment-types
//
// Was admin-owned (SEKOCI V7 removed guru's "+ Tambah Tipe Nilai"); Yadika13
// item 8 brings it back for guru, narrowed to a plain formatif column --
// see the role branch below.
func (h *Handler) CreateAssessmentType(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	// Yadika13 item 8: guru mapel may now create a tipe penilaian too
	// ("Formatif 4", "Tugas", "Kuis"), which used to be admin-only. Update
	// and Delete stay admin-only: adding a column is additive, editing or
	// removing one reaches into every other guru's grid.
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	var body assessmentTypeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !isAdmin(claims) {
		// A guru gets exactly one shape: a plain extra formatif column.
		// bucket sts/sas would divert scores into the rapor's exam-linked
		// buckets, and weight_percent/exam_linked are rapor-formula
		// concerns that stay with admin.
		body.Bucket = "formatif"
		body.WeightPercent = 0
		body.ExamLinked = false
	}
	body.Code = strings.TrimSpace(body.Code)
	body.Name = strings.TrimSpace(body.Name)
	if body.Code == "" || body.Name == "" || !validBucket(body.Bucket) {
		common.WriteError(w, http.StatusBadRequest, "code, name are required and bucket must be one of formatif/praktik/sts/sas")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	t, err := h.d.Store.CreateAssessmentType(r.Context(), schoolID, body.Code, body.Name, body.Bucket, body.WeightPercent, body.ExamLinked, body.SortOrder)
	if err != nil {
		if isPgErrorCode(err, "23505") {
			common.WriteError(w, http.StatusConflict, "a tipe penilaian with this code already exists for this school")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "failed to create assessment type")
		return
	}
	common.WriteJSON(w, http.StatusCreated, t, nil)
}

// UpdateAssessmentType: PUT /settings/assessment-types/{id} -- admin-only,
// see CreateAssessmentType.
func (h *Handler) UpdateAssessmentType(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin only")
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.d.Store.GetAssessmentType(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch assessment type")
		return
	}
	if existing == nil || (claims.ActiveRole != "superadmin" && existing.SchoolID != claims.SchoolID) {
		common.WriteError(w, http.StatusNotFound, "assessment type not found")
		return
	}
	var body assessmentTypeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || !validBucket(body.Bucket) {
		common.WriteError(w, http.StatusBadRequest, "name is required and bucket must be one of formatif/praktik/sts/sas")
		return
	}
	t, err := h.d.Store.UpdateAssessmentType(r.Context(), id, body.Name, body.Bucket, body.WeightPercent, body.ExamLinked, body.SortOrder)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update assessment type")
		return
	}
	if t == nil {
		common.WriteError(w, http.StatusNotFound, "assessment type not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// DeleteAssessmentType: DELETE /settings/assessment-types/{id} -- admin-only,
// see CreateAssessmentType.
func (h *Handler) DeleteAssessmentType(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin only")
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.d.Store.GetAssessmentType(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch assessment type")
		return
	}
	if existing == nil || (claims.ActiveRole != "superadmin" && existing.SchoolID != claims.SchoolID) {
		common.WriteError(w, http.StatusNotFound, "assessment type not found")
		return
	}
	if err := h.d.Store.DeleteAssessmentType(r.Context(), id); err != nil {
		if isPgErrorCode(err, "23503") {
			common.WriteError(w, http.StatusConflict, "tipe penilaian ini masih dipakai di data nilai formatif, tidak bisa dihapus")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "failed to delete assessment type")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// GetWeightSettings: GET /settings/weights
func (h *Handler) GetWeightSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	ws, err := h.d.Store.GetWeightSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch weight settings")
		return
	}
	common.WriteJSON(w, http.StatusOK, ws, nil)
}

// SetWeightSettings: PUT /settings/weights
func (h *Handler) SetWeightSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "only admin/superadmin can edit weight settings")
		return
	}
	var body model.WeightSettings
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.SchoolID = resolveSchoolID(r, claims)
	if err := h.d.Store.SetWeightSettings(r.Context(), body, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save weight settings")
		return
	}
	common.WriteJSON(w, http.StatusOK, body, nil)
}
