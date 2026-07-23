package handler

import (
	"encoding/json"
	"net/http"

	"skci/academic/internal/common"
	"skci/academic/internal/store"
)

type gradePromotionRequest struct {
	SourceAcademicYearID string                   `json:"source_academic_year_id"`
	TargetAcademicYearID string                   `json:"target_academic_year_id"`
	Promotions           []store.StudentPromotion `json:"promotions"`
}

// GradePromotion: POST /grade-promotion — "kenaikan kelas" at per-student
// granularity: the frontend loads each source classroom's roster and sends
// one decision per student (promote to a target classroom, or hold back —
// see KenaikanKelasView.vue and store.PromoteStudents for the "why"). Never
// touches or deletes the source year's rows.
func (h *Handler) GradePromotion(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req gradePromotionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.SourceAcademicYearID == "" || req.TargetAcademicYearID == "" || len(req.Promotions) == 0 {
		common.WriteError(w, http.StatusBadRequest, "source_academic_year_id, target_academic_year_id, and a non-empty promotions array are required")
		return
	}
	results, err := h.Store.PromoteStudents(r.Context(), schoolID, req.SourceAcademicYearID, req.TargetAcademicYearID, req.Promotions)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, results, nil)
}
