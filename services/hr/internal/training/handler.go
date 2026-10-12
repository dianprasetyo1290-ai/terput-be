package training

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/client"
	"skci/hr/internal/common"
	"skci/hr/internal/training/calc"
)

type Handler struct {
	Store     *Store
	Approvals *client.Client
}

func NewHandler(s *Store, approvals *client.Client) *Handler {
	return &Handler{Store: s, Approvals: approvals}
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

// EnrollCourse allows an employee to self-enroll in a published course.
// If the course is paid (cost_idr > 0, AC-99-02), it creates an approval request in Core
// with request_type 'training_paid' and places the assignment into 'pending_approval'.
func (h *Handler) EnrollCourse(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	// Resolve employee ID for this user account
	var emp struct {
		ID       string
		FullName string
	}
	err := h.Store.DB.QueryRowContext(r.Context(), `
		SELECT id, full_name FROM employees WHERE school_id = $1 AND (user_id = $2 OR id = $2)
	`, schoolID, claims.UserID).Scan(&emp.ID, &emp.FullName)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "data pegawai tidak ditemukan untuk akun ini")
		return
	}

	var body struct {
		CourseID  string  `json:"course_id"`
		SessionID *string `json:"session_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if body.CourseID == "" {
		common.WriteError(w, http.StatusBadRequest, "course_id wajib diisi")
		return
	}

	course, err := h.Store.GetCourse(r.Context(), schoolID, body.CourseID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "kursus tidak ditemukan")
		return
	}

	needsApproval := calc.NeedsApproval(course.CostIDR)

	tx, err := h.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memulai transaksi: "+err.Error())
		return
	}
	defer tx.Rollback()

	as, err := h.Store.EnrollTx(r.Context(), tx, schoolID, emp.ID, body.CourseID, body.SessionID, needsApproval)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			common.WriteError(w, http.StatusConflict, "Anda sudah terdaftar pada kursus ini")
			return
		}
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "kursus tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if needsApproval {
		if h.Approvals == nil {
			common.WriteError(w, http.StatusServiceUnavailable, "layanan approval tidak terkonfigurasi")
			return
		}
		cost := course.CostIDR
		appID, err := h.Approvals.CreateApproval(r.Context(), client.ApprovalRequest{
			SchoolID:      schoolID,
			RequestType:   client.ReqTypeTrainingPaid,
			SourceID:      as.ID,
			Title:         fmt.Sprintf("Pendaftaran Pelatihan: %s oleh %s", course.Title, emp.FullName),
			Summary:       map[string]any{"course_title": course.Title, "cost_idr": course.CostIDR, "employee_name": emp.FullName},
			AmountIDR:     &cost,
			MakerUserID:   claims.UserID,
			SubjectUserID: claims.UserID,
		})
		if err != nil {
			log.Printf("training assignment %s: create approval: %v", as.ID, err)
			status, msg := client.ApprovalErrorMessage(err)
			common.WriteError(w, status, msg)
			return
		}
		if err := h.Store.UpdateAssignmentApprovalTx(r.Context(), tx, as.ID, appID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan approval id: "+err.Error())
			return
		}
		as.ApprovalID = &appID
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan pendaftaran: "+err.Error())
		return
	}

	as.CourseTitle = course.Title
	as.EmployeeName = emp.FullName
	as.DisplayStatus = calc.DisplayStatus(as.Status, as.DueDate, time.Now())
	common.WriteJSON(w, http.StatusCreated, as, nil)
}

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
