package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
	"skci/academic/internal/model"
)

type assignSubjectRequest struct {
	SubjectID     string  `json:"subject_id"`
	TeacherUserID *string `json:"teacher_user_id"`
	IsActive      *bool   `json:"is_active"`
}

// AssignSubjectToClassroom: POST /classrooms/{id}/subjects — assigns a mata
// pelajaran to a classroom AND which guru teaches it there. Editable
// per-pairing (call again / PUT the returned id to change the teacher).
func (h *Handler) AssignSubjectToClassroom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	classroomID := chi.URLParam(r, "id")
	classroom, err := h.Store.GetClassroom(r.Context(), schoolID, classroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if classroom == nil {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}
	var req assignSubjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SubjectID == "" {
		common.WriteError(w, http.StatusBadRequest, "subject_id is required")
		return
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	cs, err := h.Store.AddClassroomSubject(r.Context(), model.ClassroomSubject{
		SchoolID:       schoolID,
		AcademicYearID: classroom.AcademicYearID,
		ClassroomID:    classroomID,
		SubjectID:      req.SubjectID,
		TeacherUserID:  req.TeacherUserID,
		IsActive:       isActive,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, cs, nil)
}

// ListClassroomSubjects: GET /classrooms/{id}/subjects
func (h *Handler) ListClassroomSubjects(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	items, err := h.Store.ListClassroomSubjects(r.Context(), schoolID, chi.URLParam(r, "id"), false)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// GetClassroomSubjectMatrix: GET /classrooms/{id}/subject-matrix — same data
// as ListClassroomSubjects but named per the spec's "Matriks Pelajaran"
// endpoint; exam-service (built separately) calls this to know which
// subject variants are enabled for a classroom.
func (h *Handler) GetClassroomSubjectMatrix(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	items, err := h.Store.ListClassroomSubjects(r.Context(), schoolID, chi.URLParam(r, "id"), false)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

type studentSubjectMatrixResponse struct {
	ClassroomSubjects []model.ClassroomSubject         `json:"classroom_subjects"`
	Enrollments       []model.StudentSubjectEnrollment `json:"enrollments"`
}

// GetStudentSubjectMatrix: GET /classrooms/{id}/student-subject-matrix —
// the per-student checkbox grid for Matriks Pelajaran. Roster (names) comes
// from GET /classrooms/{id}/students (already exists) joined client-side
// with identity's student directory; this endpoint only returns the
// academic-domain half: which subjects are on the grid and which cells are
// currently checked.
func (h *Handler) GetStudentSubjectMatrix(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	classroomID := chi.URLParam(r, "id")
	classroom, err := h.Store.GetClassroom(r.Context(), schoolID, classroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if classroom == nil {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}
	if !h.requireOwnClassroomIfWaliKelas(w, r, schoolID, classroomID) {
		return
	}
	subjects, err := h.Store.ListClassroomSubjects(r.Context(), schoolID, classroomID, true)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	enrollments, err := h.Store.ListEnrollmentsForClassroom(r.Context(), schoolID, classroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, studentSubjectMatrixResponse{ClassroomSubjects: subjects, Enrollments: enrollments}, nil)
}

// normalizeGroup collapses any subject_group that looks like a religion
// variant (e.g. "Agama Islam", "Agama Buddha" — real Data Mata Pelajaran
// entries turned out to be per-religion strings, not one shared "Agama"
// value) into a single "agama" exclusivity pool, so mutual exclusion works
// across religions as the spec requires ("jika sebelumnya sudah terdaftar
// ke agama lain, akan otomatis dipindahkan") regardless of how an admin
// worded each subject's group. Any other group value passes through
// unchanged (exact-match exclusivity, e.g. a hypothetical "Bahasa" elective
// pool — out of scope here, spec only asks for Agama).
func normalizeGroup(g *string) *string {
	if g == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(*g), "agama") {
		agama := "agama"
		return &agama
	}
	return g
}

type matrixCell struct {
	StudentUserID      string `json:"student_user_id"`
	ClassroomSubjectID string `json:"classroom_subject_id"`
	IsEnrolled         bool   `json:"is_enrolled"`
}

type bulkSetMatrixRequest struct {
	Items []matrixCell `json:"items"`
}

// BulkSetStudentSubjectMatrix: POST /classrooms/{id}/subject-matrix/bulk-set
// — the single write path behind Ceklis Semua / Kosongkan Semua / Sync
// Matrix / Auto-Enroll Agama. The frontend computes which cells should end
// up checked/unchecked for each of those (the "algorithm" per button lives
// in MatriksPelajaranView.vue since it already has both the subject-matrix
// and student-religion data loaded); this endpoint just validates every
// item names a classroom_subject that actually belongs to this classroom
// (so one classroom's admin can't write into another's or another
// school's rows) and persists them.
func (h *Handler) BulkSetStudentSubjectMatrix(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	classroomID := chi.URLParam(r, "id")
	classroom, err := h.Store.GetClassroom(r.Context(), schoolID, classroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if classroom == nil {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}
	// Wali kelas's Matriks Pelajaran menu writes into this same endpoint,
	// scoped to their own classroom only (see requireOwnClassroomIfWaliKelas
	// doc comment) — every other role's behavior is unchanged.
	if !h.requireOwnClassroomIfWaliKelas(w, r, schoolID, classroomID) {
		return
	}
	var req bulkSetMatrixRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	subjects, err := h.Store.ListClassroomSubjects(r.Context(), schoolID, classroomID, false)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	belongsToClassroom := make(map[string]bool, len(subjects))
	byGroup := make(map[string][]string, len(subjects)) // normalized group -> classroom_subject_ids in it
	for _, cs := range subjects {
		belongsToClassroom[cs.ID] = true
		if group := normalizeGroup(cs.SubjectGroup); group != nil {
			byGroup[*group] = append(byGroup[*group], cs.ID)
		}
	}
	siblingsOf := make(map[string][]string, len(subjects))
	for _, cs := range subjects {
		group := normalizeGroup(cs.SubjectGroup)
		if group == nil {
			continue
		}
		for _, sibling := range byGroup[*group] {
			if sibling != cs.ID {
				siblingsOf[cs.ID] = append(siblingsOf[cs.ID], sibling)
			}
		}
	}
	items := make([]model.StudentSubjectEnrollment, 0, len(req.Items))
	for _, item := range req.Items {
		if !belongsToClassroom[item.ClassroomSubjectID] {
			common.WriteError(w, http.StatusBadRequest, "classroom_subject_id "+item.ClassroomSubjectID+" is not part of this classroom")
			return
		}
		if item.StudentUserID == "" {
			common.WriteError(w, http.StatusBadRequest, "student_user_id is required")
			return
		}
		items = append(items, model.StudentSubjectEnrollment{
			SchoolID:           schoolID,
			AcademicYearID:     classroom.AcademicYearID,
			StudentUserID:      item.StudentUserID,
			ClassroomSubjectID: item.ClassroomSubjectID,
			IsEnrolled:         item.IsEnrolled,
		})
	}
	if err := h.Store.BulkSetStudentSubjectEnrollments(r.Context(), schoolID, classroom.AcademicYearID, classroomID, items, siblingsOf); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]int{"updated": len(items)}, nil)
}

type updateClassroomSubjectRequest struct {
	TeacherUserID *string `json:"teacher_user_id"`
	IsActive      *bool   `json:"is_active"`
}

// UpdateClassroomSubject: PUT /classroom-subjects/{id} — edit which guru
// teaches, or toggle the matrix is_active flag.
func (h *Handler) UpdateClassroomSubject(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req updateClassroomSubjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cs, err := h.Store.UpdateClassroomSubject(r.Context(), schoolID, chi.URLParam(r, "id"), req.TeacherUserID, req.IsActive)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cs == nil {
		common.WriteError(w, http.StatusNotFound, "classroom-subject mapping not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, cs, nil)
}

// DeleteClassroomSubject: DELETE /classroom-subjects/{id}
func (h *Handler) DeleteClassroomSubject(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteClassroomSubject(r.Context(), schoolID, chi.URLParam(r, "id")); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

type setStudentSubjectEnrollmentRequest struct {
	ClassroomSubjectID string `json:"classroom_subject_id"`
	IsEnrolled         *bool  `json:"is_enrolled"`
}

// SetStudentSubjectEnrollment: POST /students/{id}/subject-enrollments —
// resolves which variant of an elective subject_group (e.g. which Agama
// class) a specific student takes.
func (h *Handler) SetStudentSubjectEnrollment(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	studentUserID := chi.URLParam(r, "id")
	var req setStudentSubjectEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ClassroomSubjectID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_subject_id is required")
		return
	}
	isEnrolled := true
	if req.IsEnrolled != nil {
		isEnrolled = *req.IsEnrolled
	}
	e, err := h.Store.SetStudentSubjectEnrollment(r.Context(), model.StudentSubjectEnrollment{
		SchoolID:           schoolID,
		AcademicYearID:     academicYearID,
		StudentUserID:      studentUserID,
		ClassroomSubjectID: req.ClassroomSubjectID,
		IsEnrolled:         isEnrolled,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, e, nil)
}

// GetStudentActiveSubjects: GET /students/{id}/active-subjects?academic_year_id=
// exam-service calls this (over HTTP, per CONVENTIONS.md's cross-service
// rule) to filter which exam a given student sees.
func (h *Handler) GetStudentActiveSubjects(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	items, err := h.Store.ActiveSubjectsForStudent(r.Context(), schoolID, chi.URLParam(r, "id"), academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// GetStudentClassroom: GET /students/{id}/classroom?academic_year_id= —
// resolves the one classroom a student is actively enrolled in. Consumed by
// exam-service's siswa dashboard (item 20) to scope "Ujian Hari Ini /
// Mendatang / Histori" to exams assigned to the student's own class, and by
// its live-monitor (item 10). A student can have an active enrollment row
// in more than one academic year (e.g. seeded/created ahead of a promotion),
// so when the caller omits academic_year_id we must resolve the school's
// CURRENT year via identity-service (scopeAcademicYearID) rather than
// picking "most recently created" — that picked whichever enrollment was
// inserted last in wall-clock time, not the one for the running year, and
// silently returned the wrong classroom (found live: a student's exam went
// missing from their dashboard because a later admin action had created a
// next-year enrollment row after the exam's own classroom enrollment).
func (h *Handler) GetStudentClassroom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	classroomID, err := h.Store.StudentClassroomID(r.Context(), schoolID, chi.URLParam(r, "id"), academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"classroom_id": classroomID}, nil)
}

// GetTeachingAssignments: GET /internal/teaching-assignments?guru_id=
// evaluation-service calls this to populate "Kelas yang Diampu"/"Mata
// Pelajaran" on the printed Rapor Kinerja Guru. Previously unimplemented —
// evaluation-service's client always got an empty list back, so those
// fields were silently blank on every printed report.
func (h *Handler) GetTeachingAssignments(w http.ResponseWriter, r *http.Request) {
	guruID := r.URL.Query().Get("guru_id")
	if guruID == "" {
		common.WriteError(w, http.StatusBadRequest, "guru_id query param is required")
		return
	}
	items, err := h.Store.ListTeachingAssignments(r.Context(), guruID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}
