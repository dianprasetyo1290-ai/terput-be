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
	StsScore *float64 `json:"sts_score"`
	SasScore *float64 `json:"sas_score"`
	// PraktikScore: Report V5 finding -- Nilai Praktik column, alongside
	// Formatif/PTS(sts)/PAS(sas)/Akhir, on Deskripsi Rapor's grid too.
	PraktikScore *float64 `json:"praktik_score"`
	MidNote      *string  `json:"mid_note"`
	FinalNote    *string  `json:"final_note"`
	// Capaian kolom 2 (Report Sekoci V15 item 1).
	MidNote2   *string `json:"mid_note2"`
	FinalNote2 *string `json:"final_note2"`
}

// GetSubjectDescriptionGrid: GET /classrooms/{id}/subjects/{subject_id}/description?term_id=
// Deskripsi Rapor's per-classroom, per-subject grid (old app's
// ReportDescription.vue) — one row per student with their running score
// and editable Capaian STS/SAS notes. Only surfaces rows that already have
// a report_card_subjects entry (i.e. Generate Rapor has run for this
// classroom/term at least once), same precondition as Leger.
func (h *Handler) GetSubjectDescriptionGrid(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	classroomID := chi.URLParam(r, "id")
	subjectID := chi.URLParam(r, "subject_id")
	termID := r.URL.Query().Get("term_id")
	if termID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id query param is required")
		return
	}
	// Wali kelas's "Deskripsi Rapor" menu reuses this endpoint scoped to
	// their own classroom (see isWaliKelasOwner) — admin/superadmin/guru
	// behavior is unchanged.
	if !isStaff(claims) && !h.isWaliKelasOwner(r.Context(), claims, classroomID) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only, or the wali kelas of this classroom")
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

	// Yadika13 items 4 & 5: the stored Nilai STS/SAS are a snapshot taken at
	// Generate Rapor time, so a koreksi made afterwards never showed up
	// here. Re-read them from exam-service first and write back whatever
	// moved; the loop below then serves the fresh numbers. Best-effort --
	// a failure just leaves the snapshot as it was.
	if len(cards) > 0 {
		h.refreshExamScores(ctx, cards[0].SchoolID, classroomID, termID, subjectID, cards)
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
				StsScore: sub.StsScore, SasScore: sub.SasScore, PraktikScore: sub.PraktikScore,
				MidNote: sub.MidNote, FinalNote: sub.FinalNote,
				MidNote2: sub.MidNote2, FinalNote2: sub.FinalNote2,
			})
		}
	}
	common.WriteJSON(w, http.StatusOK, rows, nil)
}
