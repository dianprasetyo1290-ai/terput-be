package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
	"skci/exam/internal/store"
)

type createAdmissionExamReq struct {
	SchoolID          string                 `json:"school_id"`
	ExternalReference string                 `json:"external_reference"`
	Title             string                 `json:"title"`
	Description       *string                `json:"description"`
	StartAt           string                 `json:"start_at"`
	EndAt             string                 `json:"end_at"`
	IsShuffled        bool                   `json:"is_shuffled"`
	AdmissionContext  map[string]interface{} `json:"admission_context"`
}

func (h *Handler) InternalUpsertAdmissionExam(w http.ResponseWriter, r *http.Request) {
	var req createAdmissionExamReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	start, err := time.Parse(time.RFC3339, req.StartAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "start_at must be RFC3339")
		return
	}
	end, err := time.Parse(time.RFC3339, req.EndAt)
	if err != nil || !end.After(start) {
		common.WriteError(w, http.StatusBadRequest, "end_at must be RFC3339 and after start_at")
		return
	}
	if req.SchoolID == "" || req.ExternalReference == "" || req.Title == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id, external_reference and title are required")
		return
	}
	// external_reference is mirrored into admission_context by the store, which
	// owns the uniqueness index (idx_exams_admission_external_ref); doing it
	// here too only risked the two drifting apart.
	e, err := h.Store.UpsertAdmissionExam(r.Context(), store.AdmissionExamInput{
		SchoolID: req.SchoolID, ExternalRef: req.ExternalReference, Title: req.Title, Description: req.Description,
		StartAt: start, EndAt: end, IsShuffled: req.IsShuffled, CreatedBy: req.SchoolID, AdmissionContext: req.AdmissionContext,
	})
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, e, nil)
}

type provisionApplicantReq struct {
	SchoolID          string  `json:"school_id"`
	ApplicantID       string  `json:"applicant_id"`
	ExternalReference string  `json:"external_reference"`
	DisplayName       *string `json:"display_name"`
}

func (h *Handler) InternalProvisionApplicant(w http.ResponseWriter, r *http.Request) {
	var req provisionApplicantReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.SchoolID == "" || req.ApplicantID == "" {
		common.WriteError(w, 400, "school_id and applicant_id are required")
		return
	}
	p, err := h.Store.ProvisionApplicant(r.Context(), req.SchoolID, chi.URLParam(r, "id"), req.ApplicantID, req.ExternalReference, req.DisplayName)
	if errors.Is(err, store.ErrAdmissionExamNotFound) {
		common.WriteError(w, 404, "admission exam not found")
		return
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

type issueParticipantAccessReq struct {
	SchoolID    string `json:"school_id"`
	ApplicantID string `json:"applicant_id"`
	TTLSeconds  int    `json:"ttl_seconds"`
}

func (h *Handler) InternalIssueParticipantAccess(w http.ResponseWriter, r *http.Request) {
	var req issueParticipantAccessReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.SchoolID == "" || req.ApplicantID == "" {
		common.WriteError(w, 400, "school_id and applicant_id are required")
		return
	}
	p, err := h.Store.ProvisionApplicant(r.Context(), req.SchoolID, chi.URLParam(r, "id"), req.ApplicantID, "", nil)
	if errors.Is(err, store.ErrAdmissionExamNotFound) {
		common.WriteError(w, 404, "admission exam not found")
		return
	}
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if p == nil {
		common.WriteError(w, 404, "participant not found")
		return
	}
	ttl := req.TTLSeconds
	// Default comes from config (PARTICIPANT_TOKEN_TTL_SECONDS, plumbed through
	// handler.New); a request may shorten it but the 3600s ceiling always holds.
	if ttl <= 0 {
		ttl = int(h.ParticipantTokenTTL / time.Second)
	}
	if ttl <= 0 {
		ttl = 900
	}
	if ttl > 3600 {
		common.WriteError(w, 400, "ttl_seconds cannot exceed 3600")
		return
	}
	now, exp := time.Now(), time.Now().Add(time.Duration(ttl)*time.Second)
	claims := common.ParticipantClaims{SchoolID: req.SchoolID, ExamID: p.ExamID, ParticipantType: model.ParticipantApplicant, Scope: common.ParticipantScopeExamAttempt,
		RegisteredClaims: jwt.RegisteredClaims{Subject: req.ApplicantID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp), ID: store.GenerateCode(24)}}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.ParticipantSecret))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if err := h.Store.SaveParticipantToken(r.Context(), common.TokenHash(raw), req.SchoolID, p.ExamID, req.ApplicantID, exp); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]interface{}{"access_token": raw, "token_type": "Bearer", "scope": common.ParticipantScopeExamAttempt, "expires_at": exp}, nil)
}

func (h *Handler) InternalAdmissionResult(w http.ResponseWriter, r *http.Request) {
	schoolID, applicantID := r.URL.Query().Get("school_id"), chi.URLParam(r, "applicantId")
	if schoolID == "" {
		common.WriteError(w, 400, "school_id is required")
		return
	}
	result, err := h.Store.AdmissionResult(r.Context(), schoolID, chi.URLParam(r, "id"), applicantID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if result == nil {
		common.WriteError(w, 404, "result not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, result, nil)
}

// participant resolves and re-verifies the applicant context for a
// /participant/** request. The middleware has already proven the token's
// signature, participant_type and scope; this re-checks them against the exam
// in the URL and against the DB record of the exact token, which is what makes
// a token that was never issued here (or has since expired/been revoked) fail
// even with a valid signature.
//
// The participant type is matched through model.ValidParticipantType — the
// closed 'student'|'applicant' union, never a prefix/length-trimmed guess — so
// only an applicant token can ever drive an applicant attempt lookup.
func (h *Handler) participant(w http.ResponseWriter, r *http.Request) (*common.ParticipantClaims, bool) {
	c := common.ParticipantFromCtx(r)
	if c == nil || !model.ValidParticipantType(c.ParticipantType) ||
		c.ParticipantType != model.ParticipantApplicant || c.Scope != common.ParticipantScopeExamAttempt {
		common.WriteError(w, http.StatusForbidden, "participant token is not scoped to this exam")
		return nil, false
	}
	if c.ExamID != chi.URLParam(r, "id") {
		common.WriteError(w, http.StatusForbidden, "participant token is not scoped to this exam")
		return nil, false
	}
	raw := bearerToken(r.Header.Get("Authorization"))
	if raw == "" {
		common.WriteError(w, http.StatusUnauthorized, "participant token is expired or revoked")
		return nil, false
	}
	ok, err := h.Store.ValidateParticipantToken(r.Context(), common.TokenHash(raw), c.SchoolID, c.ExamID, c.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return nil, false
	}
	if !ok {
		common.WriteError(w, http.StatusUnauthorized, "participant token is expired or revoked")
		return nil, false
	}
	return c, true
}

// bearerToken mirrors internal/common's parser (kept local so this file needs
// no export from the middleware) and returns "" for a missing/foreign scheme
// rather than a half-stripped value.
func bearerToken(header string) string {
	v := strings.TrimSpace(header)
	if len(v) < 7 || !strings.EqualFold(v[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(v[7:])
}

func (h *Handler) ApplicantEnterExam(w http.ResponseWriter, r *http.Request) {
	c, ok := h.participant(w, r)
	if !ok {
		return
	}
	a, err := h.Store.EnterApplicantExam(r.Context(), c.SchoolID, c.ExamID, c.Subject)
	if err != nil {
		if errors.Is(err, store.ErrInvalidTransition) {
			common.WriteError(w, 409, "exam cannot be entered now")
		} else {
			common.WriteError(w, 400, err.Error())
		}
		return
	}
	common.WriteJSON(w, http.StatusOK, a, nil)
}

func (h *Handler) applicantAttempt(w http.ResponseWriter, r *http.Request) (*model.ExamAttempt, bool) {
	c, ok := h.participant(w, r)
	if !ok {
		return nil, false
	}
	a, err := h.Store.GetParticipantAttempt(r.Context(), c.ExamID, c.ParticipantType, c.Subject)
	if err != nil {
		writeStoreErr(w, err)
		return nil, false
	}
	if a == nil {
		common.WriteError(w, 404, "no attempt found")
		return nil, false
	}
	return a, true
}

func (h *Handler) ApplicantAttempt(w http.ResponseWriter, r *http.Request) {
	a, ok := h.applicantAttempt(w, r)
	if !ok {
		return
	}
	questions, err := h.Store.ListExamQuestions(r.Context(), a.ExamID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	answers, err := h.Store.ListAnswers(r.Context(), a.ID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	answered := map[string]bool{}
	for _, x := range answers {
		answered[x.ExamQuestionID] = true
	}
	type summary struct {
		No             int     `json:"no"`
		ExamQuestionID string  `json:"exam_question_id"`
		Points         float64 `json:"points"`
		Answered       bool    `json:"answered"`
	}
	byID := map[string]model.ExamQuestion{}
	for _, q := range questions {
		byID[q.ID] = q
	}
	out := []summary{}
	for i, id := range a.QuestionOrder {
		if q, found := byID[id]; found {
			out = append(out, summary{i + 1, id, q.Points, answered[id]})
		}
	}
	common.WriteJSON(w, 200, map[string]interface{}{"attempt": a, "total_questions": len(a.QuestionOrder), "questions": out}, nil)
}

func (h *Handler) ApplicantQuestion(w http.ResponseWriter, r *http.Request) {
	a, ok := h.applicantAttempt(w, r)
	if !ok {
		return
	}
	if a.Status != model.StatusAktif {
		common.WriteError(w, 409, "attempt is not active")
		return
	}
	pos, err := strconv.Atoi(chi.URLParam(r, "position"))
	if err != nil || pos < 1 || pos > len(a.QuestionOrder) {
		common.WriteError(w, 400, "invalid position")
		return
	}
	eq, err := h.Store.GetExamQuestion(r.Context(), a.QuestionOrder[pos-1])
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if eq == nil || eq.Question == nil {
		common.WriteError(w, 404, "question not found")
		return
	}
	view := studentQuestionView{ExamQuestionID: eq.ID, No: pos, QuestionType: eq.Question.QuestionType, Content: eq.Question.Content, ImageURL: eq.Question.ImageURL, Points: eq.Points}
	for _, o := range eq.Question.AnswerOptions {
		view.AnswerOptions = append(view.AnswerOptions, studentAnswerOption{ID: o.ID, Content: o.Content, ImageURL: o.ImageURL})
	}
	answers, _ := h.Store.ListAnswers(r.Context(), a.ID)
	for i := range answers {
		if answers[i].ExamQuestionID == eq.ID {
			view.YourAnswer = &answers[i]
			break
		}
	}
	common.WriteJSON(w, 200, view, nil)
}

func (h *Handler) ApplicantSubmitAnswer(w http.ResponseWriter, r *http.Request) {
	a, ok := h.applicantAttempt(w, r)
	if !ok {
		return
	}
	if a.Status != model.StatusAktif {
		common.WriteError(w, 409, "attempt is not active")
		return
	}
	var req submitAnswerReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	ans, err := h.Store.SubmitAnswer(r.Context(), a.ID, req.ExamQuestionID, req.SelectedOptionID, req.EssayAnswer)
	if err != nil {
		common.WriteError(w, 400, err.Error())
		return
	}
	common.WriteJSON(w, 200, map[string]interface{}{"exam_question_id": ans.ExamQuestionID, "answered_at": ans.AnsweredAt}, nil)
}

func (h *Handler) ApplicantSubmitExam(w http.ResponseWriter, r *http.Request) {
	a, ok := h.applicantAttempt(w, r)
	if !ok {
		return
	}
	updated, err := h.Store.SubmitExam(r.Context(), a.ID)
	if err != nil {
		common.WriteError(w, 409, err.Error())
		return
	}
	common.WriteJSON(w, 200, updated, nil)
}

func (h *Handler) ApplicantFocusLost(w http.ResponseWriter, r *http.Request) {
	c, ok := h.participant(w, r)
	if !ok {
		return
	}
	a, err := h.Store.FocusLostParticipant(r.Context(), c.ExamID, c.ParticipantType, c.Subject)
	if err != nil {
		common.WriteError(w, 409, err.Error())
		return
	}
	common.WriteJSON(w, 200, a, nil)
}
