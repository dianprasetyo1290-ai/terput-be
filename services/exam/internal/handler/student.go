package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
	"skci/exam/internal/store"
)

// MyExams: GET /me/exams?status=today|upcoming|history
func (h *Handler) MyExams(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "siswa") {
		return
	}
	bucket := r.URL.Query().Get("status")
	if bucket != "today" && bucket != "upcoming" && bucket != "history" {
		common.WriteError(w, http.StatusBadRequest, "status must be one of today|upcoming|history")
		return
	}
	rows, err := h.Store.ListMyExams(r.Context(), c.UserID, bucket)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, rows, nil)
}

type enterExamReq struct {
	Code string `json:"code"`
}

// EnterExam: POST /exams/{id}/enter — validates the voucher, then
// starts (belum_mulai->aktif) or resumes (aktif stays aktif) the attempt.
func (h *Handler) EnterExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "siswa") {
		return
	}
	examID := chi.URLParam(r, "id")
	var req enterExamReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.Code == "" {
		common.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}
	attempt, err := h.Store.EnterExam(r.Context(), examID, c.UserID, req.Code)
	if err != nil {
		switch err {
		case store.ErrTokenInvalid:
			common.WriteError(w, http.StatusUnauthorized, "voucher code is invalid, expired, or was never issued to you")
		case store.ErrInvalidTransition:
			common.WriteError(w, http.StatusConflict, "exam cannot be entered in its current state (dilarang/blokir/selesai)")
		default:
			writeStoreErr(w, err)
		}
		return
	}
	common.WriteJSON(w, http.StatusOK, attempt, nil)
}

// MyAttempt: GET /exams/{id}/attempt — the student's own current attempt,
// with the resolved question at current_position and answered-state per
// question. Never leaks is_correct / acceptable-answer lists to the student.
func (h *Handler) MyAttempt(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "siswa") {
		return
	}
	examID := chi.URLParam(r, "id")
	attempt, err := h.Store.GetAttempt(r.Context(), examID, c.UserID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if attempt == nil {
		common.WriteError(w, http.StatusNotFound, "no attempt found; enter the exam with your voucher first")
		return
	}
	examQuestions, err := h.Store.ListExamQuestions(r.Context(), examID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	byID := map[string]model.ExamQuestion{}
	for _, eq := range examQuestions {
		byID[eq.ID] = eq
	}
	answers, err := h.Store.ListAnswers(r.Context(), attempt.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	answeredSet := map[string]bool{}
	for _, a := range answers {
		answeredSet[a.ExamQuestionID] = true
	}

	type qSummary struct {
		No             int    `json:"no"`
		ExamQuestionID string `json:"exam_question_id"`
		Points         float64 `json:"points"`
		Answered       bool   `json:"answered"`
	}
	var summary []qSummary
	for i, eqID := range attempt.QuestionOrder {
		eq, ok := byID[eqID]
		if !ok {
			continue
		}
		summary = append(summary, qSummary{No: i + 1, ExamQuestionID: eq.ID, Points: eq.Points, Answered: answeredSet[eq.ID]})
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"attempt":         attempt,
		"total_questions": len(attempt.QuestionOrder),
		"questions":       summary,
	}, nil)
}

// studentQuestionView strips every answer-key signal before returning a
// question to a student: is_correct on options, and the essay/tebak_gambar
// acceptable-answer list are never sent to the client.
type studentAnswerOption struct {
	ID       string  `json:"id"`
	Content  *string `json:"content,omitempty"`
	ImageURL *string `json:"image_url,omitempty"`
}

type studentQuestionView struct {
	ExamQuestionID string                 `json:"exam_question_id"`
	No             int                    `json:"no"`
	QuestionType   string                 `json:"question_type"`
	Content        string                 `json:"content"`
	ImageURL       *string                `json:"image_url,omitempty"`
	Points         float64                `json:"points"`
	AnswerOptions  []studentAnswerOption  `json:"answer_options,omitempty"`
	YourAnswer     *model.ExamAttemptAnswer `json:"your_answer,omitempty"`
}

// AttemptQuestion: GET /exams/{id}/attempt/questions/{position} — position is
// the 1-indexed DISPLAY number, always sequential regardless of the
// underlying per-attempt shuffle.
func (h *Handler) AttemptQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "siswa") {
		return
	}
	examID := chi.URLParam(r, "id")
	posStr := chi.URLParam(r, "position")
	pos := parsePositiveInt(posStr)
	if pos < 1 {
		common.WriteError(w, http.StatusBadRequest, "position must be a positive integer")
		return
	}
	attempt, err := h.Store.GetAttempt(r.Context(), examID, c.UserID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if attempt == nil || attempt.Status != model.StatusAktif {
		common.WriteError(w, http.StatusConflict, "no active attempt; enter the exam first")
		return
	}
	if pos > len(attempt.QuestionOrder) {
		common.WriteError(w, http.StatusNotFound, "position out of range")
		return
	}
	eqID := attempt.QuestionOrder[pos-1]
	eq, err := h.Store.GetExamQuestion(r.Context(), eqID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if eq == nil || eq.Question == nil {
		common.WriteError(w, http.StatusNotFound, "question not found")
		return
	}
	view := studentQuestionView{
		ExamQuestionID: eq.ID,
		No:             pos,
		QuestionType:   eq.Question.QuestionType,
		Content:        eq.Question.Content,
		ImageURL:       eq.Question.ImageURL,
		Points:         eq.Points,
	}
	for _, o := range eq.Question.AnswerOptions {
		view.AnswerOptions = append(view.AnswerOptions, studentAnswerOption{ID: o.ID, Content: o.Content, ImageURL: o.ImageURL})
	}
	answers, err := h.Store.ListAnswers(r.Context(), attempt.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	for i := range answers {
		if answers[i].ExamQuestionID == eqID {
			view.YourAnswer = &answers[i]
			break
		}
	}
	common.WriteJSON(w, http.StatusOK, view, nil)
}

type submitAnswerReq struct {
	ExamQuestionID   string  `json:"exam_question_id"`
	SelectedOptionID *string `json:"selected_option_id,omitempty"` // pilihan_ganda / benar_salah
	EssayAnswer      *string `json:"essay_answer,omitempty"`       // essay / tebak_gambar
}

// SubmitAnswer: POST /exams/{id}/attempt/answers
func (h *Handler) SubmitAnswer(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "siswa") {
		return
	}
	examID := chi.URLParam(r, "id")
	var req submitAnswerReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.ExamQuestionID == "" {
		common.WriteError(w, http.StatusBadRequest, "exam_question_id is required")
		return
	}
	attempt, err := h.Store.GetAttempt(r.Context(), examID, c.UserID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if attempt == nil {
		common.WriteError(w, http.StatusNotFound, "no attempt found")
		return
	}
	if attempt.Status != model.StatusAktif {
		common.WriteError(w, http.StatusConflict, "attempt is not aktif (current status: "+attempt.Status+")")
		return
	}
	ans, err := h.Store.SubmitAnswer(r.Context(), attempt.ID, req.ExamQuestionID, req.SelectedOptionID, req.EssayAnswer)
	if err != nil {
		if err == store.ErrInvalidTransition {
			common.WriteError(w, http.StatusConflict, "attempt is no longer aktif")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// don't leak is_correct/points_awarded to the student while the exam is in progress
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"exam_question_id": ans.ExamQuestionID,
		"answered_at":      ans.AnsweredAt,
	}, nil)
}

// SubmitExam: POST /exams/{id}/attempt/submit — finalizes (aktif -> selesai).
func (h *Handler) SubmitExam(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "siswa") {
		return
	}
	examID := chi.URLParam(r, "id")
	attempt, err := h.Store.GetAttempt(r.Context(), examID, c.UserID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if attempt == nil {
		common.WriteError(w, http.StatusNotFound, "no attempt found")
		return
	}
	updated, err := h.Store.SubmitExam(r.Context(), attempt.ID)
	if err != nil {
		if err == store.ErrInvalidTransition {
			common.WriteError(w, http.StatusConflict, "attempt is not aktif")
			return
		}
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// FocusLost: POST /exams/{id}/attempt/focus-lost — anti-cheat event from the
// exam-taking client (tab switch / alt-tab). Auto-transitions aktif -> blokir.
func (h *Handler) FocusLost(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "siswa") {
		return
	}
	examID := chi.URLParam(r, "id")
	attempt, err := h.Store.FocusLost(r.Context(), examID, c.UserID)
	if err != nil {
		if err == store.ErrInvalidTransition {
			common.WriteError(w, http.StatusConflict, "no aktif attempt to block")
			return
		}
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, attempt, nil)
}

func parsePositiveInt(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return -1
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
