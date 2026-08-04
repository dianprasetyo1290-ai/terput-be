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
	list, _ := h.d.Academic.ListTeachingAssignments(r.Context(), claims.UserID) // fail soft: empty list if academic-service is unreachable
	common.WriteJSON(w, http.StatusOK, list, nil)
}

type formativeRowOut struct {
	StudentID   string    `json:"student_id"`
	StudentName string    `json:"student_name"`
	Scores      []float64 `json:"scores"`
}

// ListFormativeGrades: GET /formative?term_id=&classroom_subject_id=&classroom_id=
// classroom_id is required too (report-service has no classroom_subject ->
// classroom lookup of its own — the frontend already has it from
// MyTeachingAssignments, so it's passed through rather than adding another
// upstream round-trip).
func (h *Handler) ListFormativeGrades(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	termID := r.URL.Query().Get("term_id")
	scsID := r.URL.Query().Get("classroom_subject_id")
	classroomID := r.URL.Query().Get("classroom_id")
	if termID == "" || scsID == "" || classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id, classroom_subject_id and classroom_id are required")
		return
	}

	roster, err := h.d.Academic.ListClassroomStudents(r.Context(), classroomID, termID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch classroom roster: "+err.Error())
		return
	}
	grades, err := h.d.Store.ListFormativeGrades(r.Context(), termID, scsID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch formative grades")
		return
	}

	out := make([]formativeRowOut, 0, len(roster))
	for _, stu := range roster {
		name := stu.StudentUserID
		if u, uerr := h.d.Identity.GetUser(r.Context(), stu.StudentUserID); uerr == nil && u != nil {
			name = u.Name
		}
		// grades[...] is a nil slice for a student with no formative_grades
		// rows yet (the common case for a class no guru has scored yet) --
		// encoding/json marshals a nil []float64 as JSON null, not [], which
		// crashed the frontend's r.scores.length/push in loadGrid() and
		// surfaced as "Gagal memuat data penilaian formatif." even though
		// this request itself returned 200. Always emit [] so the frontend
		// always gets an array.
		scores := grades[stu.StudentUserID]
		if scores == nil {
			scores = []float64{}
		}
		out = append(out, formativeRowOut{StudentID: stu.StudentUserID, StudentName: name, Scores: scores})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type upsertFormativeBody struct {
	TermID             string    `json:"term_id"`
	ClassroomSubjectID string    `json:"classroom_subject_id"`
	StudentID          string    `json:"student_id"`
	Scores             []float64 `json:"scores"`
}

// UpsertFormativeGrades: PUT /formative — replace-all semantics per
// student, matching the old app's grid-row save (see
// TeacherFormativeService::upsertAssignments).
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
	if body.TermID == "" || body.ClassroomSubjectID == "" || body.StudentID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id, classroom_subject_id and student_id are required")
		return
	}
	if err := h.d.Store.UpsertFormativeGrades(r.Context(), claims.SchoolID, body.TermID, body.ClassroomSubjectID, body.StudentID, body.Scores, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save formative grades")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"}, nil)
}
