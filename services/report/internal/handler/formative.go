package handler

import (
	"encoding/json"
	"net/http"

	"skci/report/internal/common"
)

// MyTeachingAssignments: GET /me/teaching-assignments — guru's own
// class+subject pairings, for the Penilaian Formatif picker (mirrors
// evaluation-service's teacher-assignment lookup, but exposed here too
// since report-service is the one that needs classroom_subject_id).
func (h *Handler) MyTeachingAssignments(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != "guru" {
		common.WriteError(w, http.StatusForbidden, "only guru can view their teaching assignments")
		return
	}
	// ?academic_year_id= scopes the picker to Data Kelas' current year --
	// without it last year's classes show up as duplicates (SEKOCI V7).
	list, _ := h.d.Academic.ListTeachingAssignments(r.Context(), claims.UserID, r.URL.Query().Get("academic_year_id")) // fail soft: empty list if academic-service is unreachable
	common.WriteJSON(w, http.StatusOK, list, nil)
}

type formativeRowOut struct {
	StudentID   string   `json:"student_id"`
	StudentName string   `json:"student_name"`
	Score       *float64 `json:"score"`
	// AutoFilled: true when Score came from exam-service (item 5's
	// STS/SAS auto-populate) rather than a previously saved
	// formative_grades row -- the frontend uses this to show a "dari
	// hasil ujian" hint. Once the guru hits Simpan, a real row is
	// written and this flips to false on the next load (the saved value
	// is now guru-owned, per the deliberate "editable/can be raised"
	// business rule).
	AutoFilled bool `json:"auto_filled"`
}

// ListFormativeGrades: GET /formative?term_id=&classroom_subject_id=&classroom_id=&assessment_type_id=&subject_id=&academic_year_id=
// Item 5 UX rework: the guru picks ONE tipe penilaian (assessment_type_id)
// from a dropdown and gets one score column for the whole roster, instead
// of the old free-form add/remove-column spreadsheet grid.
// classroom_id/subject_id/academic_year_id are passed through by the
// frontend (already has them from MyTeachingAssignments/the term picker)
// rather than adding extra upstream round-trips, matching this handler's
// existing convention.
func (h *Handler) ListFormativeGrades(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	termID := r.URL.Query().Get("term_id")
	scsID := r.URL.Query().Get("classroom_subject_id")
	classroomID := r.URL.Query().Get("classroom_id")
	assessmentTypeID := r.URL.Query().Get("assessment_type_id")
	if termID == "" || scsID == "" || classroomID == "" || assessmentTypeID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id, classroom_subject_id, classroom_id and assessment_type_id are required")
		return
	}

	roster, err := h.d.Academic.ListClassroomStudents(r.Context(), classroomID, termID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch classroom roster: "+err.Error())
		return
	}
	scores, err := h.d.Store.ListFormativeScoresForType(r.Context(), termID, scsID, assessmentTypeID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch formative grades")
		return
	}

	// Item 5: if this tipe penilaian is exam-linked (STS/SAS), auto-fill a
	// suggestion for any student with no saved score yet from exam-service
	// -- still fully editable (client's explicit "raise a low score so
	// the student can pass" rule), and only a suggestion: saving writes a
	// real formative_grades row that wins from then on.
	var examBucket string
	var academicYearID, subjectID string
	if at, aerr := h.d.Store.GetAssessmentType(r.Context(), assessmentTypeID); aerr == nil && at != nil && at.ExamLinked {
		examBucket = at.Bucket // "sts" | "sas"
		academicYearID = r.URL.Query().Get("academic_year_id")
		subjectID = r.URL.Query().Get("subject_id")
	}

	// Batched once for the whole class instead of once per student --
	// the old per-student helper re-fetched the exam list, the exam types
	// and every score sheet inside this loop (Yadika13 item 4 made this path
	// hot enough to matter: it is now the screen a guru refreshes to see a
	// koreksi land).
	var freshExamScores map[string]map[string]float64
	if examBucket != "" && academicYearID != "" && subjectID != "" {
		freshExamScores = h.examBucketScores(r.Context(), classroomID, termID, academicYearID, subjectID)
	}

	out := make([]formativeRowOut, 0, len(roster))
	for _, stu := range roster {
		name := stu.StudentUserID
		if u, uerr := h.d.Identity.GetUser(r.Context(), stu.StudentUserID); uerr == nil && u != nil {
			name = u.Name
		}
		row := formativeRowOut{StudentID: stu.StudentUserID, StudentName: name}
		if v, ok := scores[stu.StudentUserID]; ok {
			row.Score = &v
		} else if v, ok := freshExamScores[examBucket][stu.StudentUserID]; ok {
			score := v
			row.Score = &score
			row.AutoFilled = true
		}
		out = append(out, row)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type upsertFormativeBody struct {
	TermID             string  `json:"term_id"`
	ClassroomSubjectID string  `json:"classroom_subject_id"`
	StudentID          string  `json:"student_id"`
	AssessmentTypeID   string  `json:"assessment_type_id"`
	Score              float64 `json:"score"`
}

// UpsertFormativeGrades: PUT /formative — save one student's score for one
// assessment type (item 5's per-cell save, replacing the old whole-row
// replace-all).
func (h *Handler) UpsertFormativeGrades(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != "guru" {
		common.WriteError(w, http.StatusForbidden, "only guru can enter formative grades")
		return
	}
	var body upsertFormativeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.TermID == "" || body.ClassroomSubjectID == "" || body.StudentID == "" || body.AssessmentTypeID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id, classroom_subject_id, student_id and assessment_type_id are required")
		return
	}

	// Verify that the guru is assigned to teach this classroom_subject
	if h.d != nil && h.d.Academic != nil {
		assignments, err := h.d.Academic.ListTeachingAssignments(r.Context(), claims.UserID, "")
		if err == nil && len(assignments) > 0 {
			assigned := false
			for _, a := range assignments {
				if a.ClassroomSubjectID == body.ClassroomSubjectID {
					assigned = true
					break
				}
			}
			if !assigned {
				common.WriteError(w, http.StatusForbidden, "guru is not assigned to this classroom subject")
				return
			}
		}
	}

	if err := h.d.Store.UpsertFormativeScore(r.Context(), claims.SchoolID, body.TermID, body.ClassroomSubjectID, body.StudentID, body.AssessmentTypeID, body.Score, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save formative grade")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"}, nil)
}
