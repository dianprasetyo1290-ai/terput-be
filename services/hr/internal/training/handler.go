package training

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/common"
)

type Handler struct {
	Store *Store
}

func NewHandler(s *Store) *Handler {
	return &Handler{Store: s}
}

// -----------------------------------------------------------------------------
// Courses Handlers
// -----------------------------------------------------------------------------

func (h *Handler) ListCourses(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")

	list, err := h.Store.ListCourses(r.Context(), schoolID, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat kursus: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetCourse(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	cr, err := h.Store.GetCourse(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "kursus tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, cr, nil)
}

func (h *Handler) CreateCourse(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	var cr Course
	if err := json.NewDecoder(r.Body).Decode(&cr); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	cr.SchoolID = schoolID
	if cr.Status == "" {
		cr.Status = "draft"
	}

	created, err := h.Store.CreateCourse(r.Context(), cr)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat kursus: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateCourse(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var cr Course
	if err := json.NewDecoder(r.Body).Decode(&cr); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	cr.SchoolID = schoolID
	cr.ID = id

	updated, err := h.Store.UpdateCourse(r.Context(), cr)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memperbarui kursus: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// Materials
func (h *Handler) ListMaterials(w http.ResponseWriter, r *http.Request) {
	courseID := chi.URLParam(r, "courseId")
	list, err := h.Store.ListMaterials(r.Context(), courseID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat materi: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateMaterial(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	courseID := chi.URLParam(r, "courseId")

	var m CourseMaterial
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	m.SchoolID = schoolID
	m.CourseID = courseID

	created, err := h.Store.CreateMaterial(r.Context(), m)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menambah materi: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// Sessions
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	courseID := chi.URLParam(r, "courseId")
	list, err := h.Store.ListSessions(r.Context(), courseID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat sesi: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	courseID := chi.URLParam(r, "courseId")

	var ses CourseSession
	if err := json.NewDecoder(r.Body).Decode(&ses); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	ses.SchoolID = schoolID
	ses.CourseID = courseID

	created, err := h.Store.CreateSession(r.Context(), ses)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menambah sesi: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// Questions
func (h *Handler) ListQuestions(w http.ResponseWriter, r *http.Request) {
	courseID := chi.URLParam(r, "courseId")
	list, err := h.Store.ListQuestions(r.Context(), courseID, true)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat kuis: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateQuestion(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	courseID := chi.URLParam(r, "courseId")

	var q QuizQuestion
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	q.SchoolID = schoolID
	q.CourseID = courseID

	created, err := h.Store.CreateQuestion(r.Context(), q)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menambah soal kuis: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// -----------------------------------------------------------------------------
// Mandatory Trainings Handlers
// -----------------------------------------------------------------------------

func (h *Handler) ListMandatory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListMandatory(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat pelatihan wajib: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateMandatory(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var body struct {
		PositionID string `json:"position_id"`
		CourseID   string `json:"course_id"`
		DueDays    int    `json:"due_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if body.DueDays <= 0 {
		body.DueDays = 30
	}

	created, err := h.Store.CreateMandatory(r.Context(), schoolID, body.PositionID, body.CourseID, body.DueDays)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat penugasan wajib: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// -----------------------------------------------------------------------------
// Assignments Handlers (Admin & ESS)
// -----------------------------------------------------------------------------

func (h *Handler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	empID := r.URL.Query().Get("employee_id")

	list, err := h.Store.ListAssignments(r.Context(), schoolID, empID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat penugasan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var a Assignment
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	a.SchoolID = schoolID
	if a.Source == "" {
		a.Source = "manual"
	}

	created, err := h.Store.CreateAssignment(r.Context(), a)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menugaskan pelatihan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// ESS Endpoints
func (h *Handler) MyAssignments(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListAssignments(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat pelatihan saya: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) MyQuiz(w http.ResponseWriter, r *http.Request) {
	courseID := chi.URLParam(r, "courseId")
	// Questions returned with correct answers STRIPPED for employee security
	list, err := h.Store.ListQuestions(r.Context(), courseID, false)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat soal kuis: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) SubmitQuiz(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	assignmentID := chi.URLParam(r, "assignmentId")

	var body struct {
		Answers []int `json:"answers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}

	attempt, err := h.Store.SubmitQuiz(r.Context(), schoolID, assignmentID, body.Answers)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menilai kuis: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, attempt, nil)
}

func (h *Handler) MarkMaterialDone(w http.ResponseWriter, r *http.Request) {
	assignmentID := chi.URLParam(r, "assignmentId")
	materialID := chi.URLParam(r, "materialId")

	if err := h.Store.MarkMaterialDone(r.Context(), assignmentID, materialID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menandai materi: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "materi selesai"}, nil)
}

// -----------------------------------------------------------------------------
// Onboarding Handlers
// -----------------------------------------------------------------------------

func (h *Handler) ListOnboardingTemplates(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListOnboardingTemplates(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat template: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateOnboardingTemplate(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var t OnboardingTemplate
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	t.SchoolID = schoolID

	created, err := h.Store.CreateOnboardingTemplate(r.Context(), t)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat template onboarding: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) MyOnboarding(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	plan, err := h.Store.GetOnboardingPlan(r.Context(), schoolID, claims.UserID)
	if err != nil {
		// No active onboarding plan
		common.WriteJSON(w, http.StatusOK, nil, nil)
		return
	}
	common.WriteJSON(w, http.StatusOK, plan, nil)
}

func (h *Handler) MarkOnboardingTaskDone(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	taskID := chi.URLParam(r, "taskId")

	if err := h.Store.MarkTaskDone(r.Context(), schoolID, taskID, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyelesaikan tugas: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "tugas diselesaikan"}, nil)
}
