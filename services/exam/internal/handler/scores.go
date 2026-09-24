package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
)

// ExamScores: GET /exams/{id}/scores — report-service pulls final scores
// from here for e-raport. Includes the exam_type's mapping_rapor so
// report-service knows which score bucket each row feeds.
//
// Yadika13 item 6: the score reported here is COALESCE(final_score, score) —
// once a guru has ACC'd a sitting, that is the score the rapor sees, and
// later ujian susulan do not move it until the guru ACCs again.
func (h *Handler) ExamScores(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	examType, err := h.Store.GetExamType(r.Context(), exam.ExamTypeID, exam.SchoolID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	rows, err := h.Store.ExamScores(r.Context(), examID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	mappingRapor := ""
	if examType != nil {
		mappingRapor = examType.MappingRapor
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"exam_id":       examID,
		"exam_type_id":  exam.ExamTypeID,
		"mapping_rapor": mappingRapor,
		"scores":        rows,
	}, nil)
}
