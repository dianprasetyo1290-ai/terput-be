package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"skci/evaluation/internal/common"
	"skci/evaluation/internal/model"
)

type lessonPlanRequest struct {
	Title          string                 `json:"title"`
	AcademicYearID string                 `json:"academic_year_id"`
	SectionA       map[string]interface{} `json:"section_a"`
	SectionB       map[string]interface{} `json:"section_b"`
	SectionC       map[string]interface{} `json:"section_c"`
	// DimensiLulusan (Bug Report SEKOCI GPM V10, ROLE GURU "Data RPM"): the
	// manual "+ Buat RPM" form now has its own "D. Indikator 8 Dimensi
	// Lulusan" fields (previously import-only, see model.LessonPlan's
	// DimensiLulusan doc comment).
	DimensiLulusan map[string]interface{} `json:"dimensi_lulusan"`
	Status         string                 `json:"status"`
}

// resolveAcademicYear returns req's academic_year_id, falling back to
// academic-service's "current" year for the school (best-effort; if
// academic-service is unreachable and none was supplied, the request fails
// with a clear 400 rather than silently writing a null/garbage year id).
func (h *Handler) resolveAcademicYear(schoolID, provided string) (string, bool) {
	if provided != "" {
		return provided, true
	}
	for _, y := range h.Clients.GetAcademicYears(schoolID) {
		if y.Current {
			return y.ID, true
		}
	}
	return "", false
}

// POST /me/lesson-plans - guru creates their own RPM entry.
func (h *Handler) CreateMyLessonPlan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can create their own RPM")
		return
	}
	var req lessonPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	ayID, ok := h.resolveAcademicYear(claims.SchoolID, req.AcademicYearID)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id is required (academic-service current-year lookup unavailable)")
		return
	}
	status := req.Status
	if status == "" {
		status = "draft"
	}
	lp := &model.LessonPlan{
		SchoolID: claims.SchoolID, AcademicYearID: ayID, GuruUserID: claims.UserID,
		Title: req.Title, SectionA: req.SectionA, SectionB: req.SectionB, SectionC: req.SectionC,
		DimensiLulusan: req.DimensiLulusan, Status: status,
	}
	if err := h.LessonPlans.Create(r.Context(), lp); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create lesson plan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, lp, nil)
}

// PUT /me/lesson-plans/{id} - guru updates their own RPM entry.
func (h *Handler) UpdateMyLessonPlan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can edit their own RPM")
		return
	}
	id := chi.URLParam(r, "id")
	var req lessonPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	lp := &model.LessonPlan{Title: req.Title, SectionA: req.SectionA, SectionB: req.SectionB, SectionC: req.SectionC, DimensiLulusan: req.DimensiLulusan, Status: req.Status}
	if lp.Status == "" {
		lp.Status = "draft"
	}
	ok, err := h.LessonPlans.Update(r.Context(), id, claims.UserID, lp)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update lesson plan: "+err.Error())
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "lesson plan not found")
		return
	}
	updated, err := h.LessonPlans.GetByID(r.Context(), id, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to reload lesson plan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// GET /me/lesson-plans - guru lists their own RPM entries.
func (h *Handler) ListMyLessonPlans(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	list, err := h.LessonPlans.ListByGuru(r.Context(), claims.UserID, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list lesson plans: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// GET /me/lesson-plans/{id}
func (h *Handler) GetMyLessonPlan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	lp, err := h.LessonPlans.GetByID(r.Context(), id, claims.SchoolID)
	if err != nil || lp.GuruUserID != claims.UserID {
		common.WriteError(w, http.StatusNotFound, "lesson plan not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, lp, nil)
}

// DELETE /me/lesson-plans/{id}
func (h *Handler) DeleteMyLessonPlan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	ok, err := h.LessonPlans.Delete(r.Context(), id, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete lesson plan: "+err.Error())
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "lesson plan not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// GET /lesson-plans?guru_id= - admin read-only list-all (across guru).
func (h *Handler) AdminListLessonPlans(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	page, perPage := pagination(r)
	var guruID *string
	if g := r.URL.Query().Get("guru_id"); g != "" {
		guruID = &g
	}
	list, total, err := h.LessonPlans.ListAll(r.Context(), schoolID, guruID, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list lesson plans: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, map[string]interface{}{"page": page, "per_page": perPage, "total": total})
}

// GET /lesson-plans/guru/{guru_id} - admin read-detail-by-guru (read-only storage/viewing).
func (h *Handler) AdminGetLessonPlansByGuru(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	guruID := chi.URLParam(r, "guru_id")
	list, err := h.LessonPlans.ListByGuru(r.Context(), guruID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch lesson plans: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}
