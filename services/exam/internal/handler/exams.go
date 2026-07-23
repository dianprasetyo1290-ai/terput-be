package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
	"skci/exam/internal/store"
)

type examReq struct {
	AcademicYearID string  `json:"academic_year_id"`
	TermID         string  `json:"term_id"`       // Semester/Tahun Ajaran
	ExamTypeID     string  `json:"exam_type_id"`  // Tipe Ujian
	Title          string  `json:"title"`         // Judul Ujian
	Description    *string `json:"description"`   // Deskripsi/catatan (optional)
	ClassroomID    string  `json:"classroom_id"`  // Mata Pelajaran: kelas
	SubjectID      string  `json:"subject_id"`    // Mata Pelajaran: mapel
	TeacherID      string  `json:"teacher_id"`    // admin assigns explicitly; guru defaults to self
	StartAt        string  `json:"start_at"`      // RFC3339, Tanggal Mulai
	EndAt          string  `json:"end_at"`        // RFC3339, Tanggal Selesai
	IsShuffled     bool    `json:"is_shuffled"`   // Acak Soal
}

// validateExamReq enforces the spec's "ALL mandatory" header fields, the
// no-past-date rule, and (for guru) that they can only assign themselves as
// teacher. See README Assumptions re: trusting classroom_id/subject_id
// straight from the JWT+payload instead of round-tripping academic-service.
func (h *Handler) validateExamReq(req examReq, c *common.Claims) (string, bool, time.Time, time.Time) {
	if req.AcademicYearID == "" || req.TermID == "" || req.ExamTypeID == "" || req.Title == "" ||
		req.ClassroomID == "" || req.SubjectID == "" {
		return "academic_year_id, term_id, exam_type_id, title, classroom_id and subject_id are all mandatory", false, time.Time{}, time.Time{}
	}
	start, err := time.Parse(time.RFC3339, req.StartAt)
	if err != nil {
		return "start_at must be RFC3339", false, time.Time{}, time.Time{}
	}
	end, err := time.Parse(time.RFC3339, req.EndAt)
	if err != nil {
		return "end_at must be RFC3339", false, time.Time{}, time.Time{}
	}
	now := time.Now()
	if start.Before(now) {
		return "start_at must not be in the past", false, time.Time{}, time.Time{}
	}
	if end.Before(now) {
		return "end_at must not be in the past", false, time.Time{}, time.Time{}
	}
	if end.Before(start) {
		return "end_at must not be before start_at", false, time.Time{}, time.Time{}
	}
	if c.ActiveRole == "guru" && req.TeacherID != "" && req.TeacherID != c.UserID {
		return "guru may only create exams for themselves as teacher_id", false, time.Time{}, time.Time{}
	}
	return "", true, start, end
}

func (h *Handler) CreateExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	var req examReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	teacherID := req.TeacherID
	if c.ActiveRole == "guru" || teacherID == "" {
		teacherID = c.UserID
	}
	req.TeacherID = teacherID
	msg, ok, start, end := h.validateExamReq(req, c)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	e := &model.Exam{
		SchoolID:       common.EffectiveSchoolID(r, c),
		AcademicYearID: req.AcademicYearID,
		TermID:         req.TermID,
		ExamTypeID:     req.ExamTypeID,
		Title:          req.Title,
		Description:    req.Description,
		ClassroomID:    req.ClassroomID,
		SubjectID:      req.SubjectID,
		TeacherID:      teacherID,
		StartAt:        start,
		EndAt:          end,
		IsShuffled:     req.IsShuffled,
		CreatedBy:      c.UserID,
	}
	if err := h.Store.CreateExam(r.Context(), e); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, e, nil)
}

func (h *Handler) ListExams(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	f := store.ExamFilter{
		SchoolID:       common.EffectiveSchoolID(r, c),
		AcademicYearID: r.URL.Query().Get("academic_year_id"),
		TermID:         r.URL.Query().Get("term_id"),
		ClassroomID:    r.URL.Query().Get("classroom_id"),
	}
	if c != nil && (r.URL.Query().Get("mine") == "true" || c.ActiveRole == "guru") {
		f.TeacherID = c.UserID
	}
	if v := r.URL.Query().Get("is_published"); v != "" {
		b := v == "true"
		f.IsPublished = &b
	}
	page, perPage := common.Pagination(r)
	list, total, err := h.Store.ListExams(r.Context(), f, page, perPage)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) GetExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	e, err := h.Store.GetExam(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if e == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	questions, err := h.Store.ListExamQuestions(r.Context(), id)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"exam": e, "questions": questions}, nil)
}

func (h *Handler) UpdateExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.Store.GetExam(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if existing == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && existing.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only edit your own exams")
		return
	}
	var req examReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.TeacherID == "" {
		req.TeacherID = existing.TeacherID
	}
	msg, ok, start, end := h.validateExamReq(req, c)
	if !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	e := &model.Exam{
		ID: id, SchoolID: existing.SchoolID,
		TermID: req.TermID, ExamTypeID: req.ExamTypeID, Title: req.Title, Description: req.Description,
		ClassroomID: req.ClassroomID, SubjectID: req.SubjectID, TeacherID: req.TeacherID,
		StartAt: start, EndAt: end, IsShuffled: req.IsShuffled,
	}
	if err := h.Store.UpdateExam(r.Context(), e); err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusConflict, "exam is published and no longer editable")
			return
		}
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteExam(r.Context(), id, common.EffectiveSchoolID(r, c)); err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusConflict, "exam is published (or not found) and cannot be deleted")
			return
		}
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}

type addExamQuestionReq struct {
	QuestionID string  `json:"question_id"`
	Poin       float64 `json:"poin"` // optional override of the bank-soal question's raw poin for this exam
}

func (h *Handler) AddExamQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
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
	if exam.IsPublished {
		common.WriteError(w, http.StatusConflict, "cannot modify questions on a published exam")
		return
	}
	var req addExamQuestionReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	q, err := h.Store.GetQuestion(r.Context(), req.QuestionID, exam.SchoolID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if q == nil {
		common.WriteError(w, http.StatusNotFound, "question not found")
		return
	}
	poin := req.Poin
	if poin <= 0 {
		poin = q.Poin
	}
	eq, err := h.Store.AddExamQuestion(r.Context(), examID, req.QuestionID, poin)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, eq, nil)
}

func (h *Handler) RemoveExamQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
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
	if exam.IsPublished {
		common.WriteError(w, http.StatusConflict, "cannot modify questions on a published exam")
		return
	}
	eqID := chi.URLParam(r, "eqId")
	if err := h.Store.RemoveExamQuestion(r.Context(), examID, eqID); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}

// PublishExam normalizes points to sum to 100 (see README "Score
// normalization") then flips the exam to published/live.
func (h *Handler) PublishExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
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
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only publish your own exams")
		return
	}
	if err := h.Store.NormalizeAndPublish(r.Context(), examID, exam.SchoolID); err != nil {
		if errors.Is(err, store.ErrNotEditable) {
			common.WriteError(w, http.StatusConflict, "exam not found or already published")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, _ := h.Store.GetExam(r.Context(), examID, exam.SchoolID)
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// DuplicateExam creates a fresh, unpublished copy of an exam (settings +
// question associations) for reuse. Tokens/attempts are never copied — see
// Store.DuplicateExam.
func (h *Handler) DuplicateExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
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
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only duplicate your own exams")
		return
	}
	dup, err := h.Store.DuplicateExam(r.Context(), examID, exam.SchoolID, c.UserID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, dup, nil)
}
