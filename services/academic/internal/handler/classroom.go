package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
	"skci/academic/internal/model"
)

type createClassroomRequest struct {
	Name                  string  `json:"name"`
	GradeLevel            *string `json:"grade_level"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
}

// CreateClassroom: POST /classrooms?academic_year_id=
func (h *Handler) CreateClassroom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	var req createClassroomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	c, err := h.Store.CreateClassroom(r.Context(), model.Classroom{
		SchoolID:              schoolID,
		AcademicYearID:        academicYearID,
		Name:                  req.Name,
		GradeLevel:            req.GradeLevel,
		HomeroomTeacherUserID: req.HomeroomTeacherUserID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, c, nil)
}

// ListClassrooms: GET /classrooms?academic_year_id=&page=&per_page=
func (h *Handler) ListClassrooms(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	page, perPage := common.ParsePage(r)
	items, total, err := h.Store.ListClassrooms(r.Context(), schoolID, academicYearID, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// GetClassroom: GET /classrooms/{id}
func (h *Handler) GetClassroom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	c, err := h.Store.GetClassroom(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, c, nil)
}

type updateClassroomRequest struct {
	Name                  *string `json:"name"`
	GradeLevel            *string `json:"grade_level"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
}

// UpdateClassroom: PUT /classrooms/{id} — rename, change wali kelas, etc.
func (h *Handler) UpdateClassroom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	var req updateClassroomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	c, err := h.Store.UpdateClassroom(r.Context(), schoolID, chi.URLParam(r, "id"), req.Name, req.GradeLevel, req.HomeroomTeacherUserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		common.WriteError(w, http.StatusNotFound, "classroom not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, c, nil)
}

// DeleteClassroom: DELETE /classrooms/{id}
func (h *Handler) DeleteClassroom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteClassroom(r.Context(), schoolID, chi.URLParam(r, "id")); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// RemoveStudentEnrollments: DELETE /students/{student_user_id}/enrollments —
// internal-only (X-Internal-Key + explicit ?school_id=, no unscoped
// fallback since this deletes across every classroom/year for the student).
// Called by identity-service's DeleteUser when the deleted account holds a
// siswa role, so a deleted student's enrollment row stops inflating
// DashboardCountsFor's siswa count.
func (h *Handler) RemoveStudentEnrollments(w http.ResponseWriter, r *http.Request) {
	if !common.IsInternal(r) {
		common.WriteError(w, http.StatusForbidden, "internal only")
		return
	}
	schoolID := r.URL.Query().Get("school_id")
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	studentUserID := chi.URLParam(r, "student_user_id")
	if err := h.Store.RemoveStudentEnrollments(r.Context(), schoolID, studentUserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"student_user_id": studentUserID}, nil)
}

// ListWaliKelas: GET /wali-kelas?academic_year_id=
// Auto-derived from classrooms.homeroom_teacher_user_id — not separately
// stored, so it always reflects the latest Data Kelas edit immediately.
func (h *Handler) ListWaliKelas(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	items, err := h.Store.ListWaliKelas(r.Context(), schoolID, academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

type addStudentRequest struct {
	StudentUserID string `json:"student_user_id"`
}

// AddStudentToClassroom: POST /classrooms/{id}/students
func (h *Handler) AddStudentToClassroom(w http.ResponseWriter, r *http.Request) {
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
	var req addStudentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.StudentUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "student_user_id is required")
		return
	}
	e, err := h.Store.AddClassEnrollment(r.Context(), model.ClassEnrollment{
		SchoolID:       schoolID,
		AcademicYearID: classroom.AcademicYearID,
		ClassroomID:    classroomID,
		StudentUserID:  req.StudentUserID,
		Status:         "active",
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, e, nil)
}

// ListClassroomStudents: GET /classrooms/{id}/students
func (h *Handler) ListClassroomStudents(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	items, err := h.Store.ListClassEnrollments(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// ListEnrollmentsByYear: GET /classrooms/enrollments?academic_year_id=
// Year-wide roster across all classrooms (not scoped to one classroom id) —
// lets the frontend know which students already have ANY classroom
// assignment this year (siswa-picker dedup in Data Kelas) and drives the
// "semua siswa + kelasnya" roster view.
func (h *Handler) ListEnrollmentsByYear(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	academicYearID, ok := h.scopeAcademicYearID(w, r, schoolID)
	if !ok {
		return
	}
	items, err := h.Store.ListClassEnrollmentsByYear(r.Context(), schoolID, academicYearID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// RemoveStudentFromClassroom: DELETE /classrooms/{id}/students/{student_user_id}
func (h *Handler) RemoveStudentFromClassroom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	if err := h.Store.RemoveClassEnrollment(r.Context(), schoolID, chi.URLParam(r, "id"), chi.URLParam(r, "student_user_id")); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "removed"}, nil)
}
