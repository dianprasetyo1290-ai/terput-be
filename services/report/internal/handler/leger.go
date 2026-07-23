package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/report/internal/common"
	"skci/report/internal/model"
)

// GetLeger builds the classroom x term rekap grid. It is a pure read-model
// over report_cards/report_card_subjects — nothing is separately entered
// here, matching the spec ("essentially a read-model over the same
// ReportCard data").
func (h *Handler) GetLeger(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "leger is only available to guru/admin/superadmin")
		return
	}
	classroomID := chi.URLParam(r, "id")
	termID := r.URL.Query().Get("term_id")
	if termID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id query param is required")
		return
	}

	ctx := r.Context()
	cards, err := h.d.Store.ListReportCardsByClassroomTerm(ctx, classroomID, termID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch leger")
		return
	}
	if len(cards) > 0 && claims.ActiveRole != "superadmin" && cards[0].SchoolID != claims.SchoolID {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}

	leger := model.Leger{
		ClassroomID: classroomID,
		TermID:      termID,
	}
	if len(cards) > 0 {
		leger.ClassroomName = cards[0].ClassroomName
		leger.TermName = cards[0].TermName
	}

	seenSubjects := map[string]model.SubjectRef{}
	rows := make([]model.LegerRow, 0, len(cards))

	for _, rc := range cards {
		subjects, err := h.d.Store.ListSubjects(ctx, rc.ID)
		if err != nil {
			continue
		}
		row := model.LegerRow{
			StudentID:    rc.StudentID,
			StudentName:  rc.StudentName,
			StudentNISN:  rc.StudentNISN,
			ReportCardID: rc.ID,
			Status:       rc.Status,
			Subjects:     map[string]float64{},
			Attendance:   rc.Attendance,
		}
		for _, sub := range subjects {
			if sub.SubjectName != nil {
				ref := model.SubjectRef{ID: sub.SubjectID, Name: *sub.SubjectName}
				if sub.SubjectCode != nil {
					ref.Code = *sub.SubjectCode
				}
				seenSubjects[sub.SubjectID] = ref
			} else if _, ok := seenSubjects[sub.SubjectID]; !ok {
				seenSubjects[sub.SubjectID] = model.SubjectRef{ID: sub.SubjectID}
			}
			if f := sub.Final(); f != nil {
				row.Subjects[sub.SubjectID] = *f
			}
		}
		rows = append(rows, row)
	}

	subjectRefs := make([]model.SubjectRef, 0, len(seenSubjects))
	for _, ref := range seenSubjects {
		subjectRefs = append(subjectRefs, ref)
	}
	leger.Subjects = subjectRefs
	leger.Rows = rows

	common.WriteJSON(w, http.StatusOK, leger, nil)
}
