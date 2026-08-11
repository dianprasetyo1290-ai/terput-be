package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/report/internal/common"
)

type descriptionRow struct {
	StudentID     string   `json:"student_id"`
	StudentName   string   `json:"student_name"`
	ReportCardID  string   `json:"report_card_id"`
	ComputedScore *float64 `json:"computed_score"`
	FormatifScore *float64 `json:"formatif_score"`
	OverrideScore *float64 `json:"override_score"`
	FinalScore    *float64 `json:"final_score"` // "Nilai Berjalan" — override wins over computed
	// StsScore/SasScore: the actual computed Sumatif Tengah/Akhir Semester
	// achievement scores (see classifyMapping), shown next to the free-text
	// Capaian STS/SAS notes so the narrative can be written against real
	// data instead of just the blended running score.
	StsScore  *float64 `json:"sts_score"`
	SasScore  *float64 `json:"sas_score"`
	MidNote   *string  `json:"mid_note"`
	FinalNote *string  `json:"final_note"`
}

// GetSubjectDescriptionGrid: GET /classrooms/{id}/subjects/{subject_id}/description?term_id=
// Deskripsi Rapor's per-classroom, per-subject grid (old app's
// ReportDescription.vue) — one row per student with their running score
// and editable Capaian STS/SAS notes. Only surfaces rows that already have
// a report_card_subjects entry (i.e. Generate Rapor has run for this
// classroom/term at least once), same precondition as Leger.
func (h *Handler) GetSubjectDescriptionGrid(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	classroomID := chi.URLParam(r, "id")
	subjectID := chi.URLParam(r, "subject_id")
	termID := r.URL.Query().Get("term_id")
	if termID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id query param is required")
		return
	}

	ctx := r.Context()
	cards, err := h.d.Store.ListReportCardsByClassroomTerm(ctx, classroomID, termID, "semester", "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report cards")
		return
	}
	if len(cards) > 0 && claims.ActiveRole != "superadmin" && cards[0].SchoolID != claims.SchoolID {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}

	rows := make([]descriptionRow, 0, len(cards))
	for _, rc := range cards {
		subjects, err := h.d.Store.ListSubjects(ctx, rc.ID)
		if err != nil {
			continue
		}

		var allowedSubjects map[string]bool
		allowedFetched := false

		for _, sub := range subjects {
			if sub.SubjectID != subjectID {
				continue
			}

			if !allowedFetched {
				allowedSubjects = h.d.Academic.ActiveSubjectIDs(ctx, rc.StudentID, claims.SchoolID, rc.AcademicYearID)
				allowedFetched = true
			}
			if allowedSubjects != nil && !allowedSubjects[sub.SubjectID] {
				continue // Skip if this student is not enrolled in this specific religion's variant of Agama
			}

			name := ""
			if rc.StudentName != nil {
				name = *rc.StudentName
			}
			rows = append(rows, descriptionRow{
				StudentID: rc.StudentID, StudentName: name, ReportCardID: rc.ID,
				ComputedScore: sub.ComputedScore, FormatifScore: sub.FormatifScore, OverrideScore: sub.OverrideScore, FinalScore: sub.Final(),
				StsScore: sub.StsScore, SasScore: sub.SasScore,
				MidNote: sub.MidNote, FinalNote: sub.FinalNote,
			})
		}
	}
	common.WriteJSON(w, http.StatusOK, rows, nil)
}
