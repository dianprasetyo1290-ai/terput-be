package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
)

type answerOptionReq struct {
	Content   *string `json:"content"`
	ImageURL  *string `json:"image_url"`
	IsCorrect bool    `json:"is_correct"`
}

type questionReq struct {
	AcademicYearID      string            `json:"academic_year_id"`
	QuestionType        string            `json:"question_type"` // pilihan_ganda | benar_salah | essay | tebak_gambar
	Content             string            `json:"content"`
	ImageURL            *string           `json:"image_url"`
	NoKD                *string           `json:"no_kd"`
	KontenKD            *string           `json:"konten_kd"`
	IndikatorPencapaian *string           `json:"indikator_pencapaian"`
	LevelKognitif       *string           `json:"level_kognitif"`
	Poin                float64           `json:"poin"`
	AnswerOptions       []answerOptionReq `json:"answer_options,omitempty"` // pilihan_ganda (5) / benar_salah (2)
	EssayOptions        []string          `json:"essay_options,omitempty"`  // essay / tebak_gambar acceptable answers ("Opsi")
}

// validateQuestionShape enforces the 4 question-type field rules from the spec:
// pilihan_ganda = exactly 5 options/1 correct, benar_salah = exactly 2/1
// correct, essay/tebak_gambar = at least 1 acceptable "Opsi" answer.
func validateQuestionShape(req questionReq) (string, bool) {
	if req.Content == "" {
		return "content is required", false
	}
	if req.AcademicYearID == "" {
		return "academic_year_id is required", false
	}
	switch req.QuestionType {
	case "pilihan_ganda":
		if len(req.AnswerOptions) != 5 {
			return "pilihan_ganda requires exactly 5 answer_options (a-e)", false
		}
		if countCorrect(req.AnswerOptions) != 1 {
			return "pilihan_ganda requires exactly 1 option flagged is_correct", false
		}
	case "benar_salah":
		if len(req.AnswerOptions) != 2 {
			return "benar_salah requires exactly 2 answer_options (Benar, Salah)", false
		}
		if countCorrect(req.AnswerOptions) != 1 {
			return "benar_salah requires exactly 1 option flagged is_correct", false
		}
	case "essay":
		if len(req.EssayOptions) < 1 {
			return "essay requires at least 1 essay_options entry (Opsi jawaban)", false
		}
	case "tebak_gambar":
		if len(req.EssayOptions) < 1 {
			return "tebak_gambar requires at least 1 essay_options entry (Opsi jawaban)", false
		}
		if req.ImageURL == nil || *req.ImageURL == "" {
			return "tebak_gambar requires image_url on the question stem", false
		}
	default:
		return "question_type must be one of pilihan_ganda|benar_salah|essay|tebak_gambar", false
	}
	return "", true
}

func countCorrect(opts []answerOptionReq) int {
	n := 0
	for _, o := range opts {
		if o.IsCorrect {
			n++
		}
	}
	return n
}

func toModelQuestion(req questionReq, schoolID, folderID, createdBy string) *model.Question {
	q := &model.Question{
		SchoolID:            schoolID,
		AcademicYearID:      req.AcademicYearID,
		FolderID:            folderID,
		CreatedBy:           createdBy,
		QuestionType:        req.QuestionType,
		Content:             req.Content,
		ImageURL:            req.ImageURL,
		NoKD:                req.NoKD,
		KontenKD:            req.KontenKD,
		IndikatorPencapaian: req.IndikatorPencapaian,
		LevelKognitif:       req.LevelKognitif,
		Poin:                req.Poin,
	}
	if q.Poin <= 0 {
		q.Poin = 1
	}
	for _, o := range req.AnswerOptions {
		q.AnswerOptions = append(q.AnswerOptions, model.AnswerOption{Content: o.Content, ImageURL: o.ImageURL, IsCorrect: o.IsCorrect})
	}
	for _, e := range req.EssayOptions {
		q.EssayOptions = append(q.EssayOptions, model.EssayAnswerOption{AnswerText: e})
	}
	return q
}

func (h *Handler) CreateQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	folderID := chi.URLParam(r, "folderId")
	var req questionReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if msg, ok := validateQuestionShape(req); !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	q := toModelQuestion(req, common.EffectiveSchoolID(r, c), folderID, c.UserID)
	if err := h.Store.CreateQuestion(r.Context(), q); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, q, nil)
}

// ListQuestions enforces the guru-only-sees-own-soal rule: admin sees every
// guru's soal in the folder; guru sees only soal where created_by == self.
func (h *Handler) ListQuestions(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	folderID := chi.URLParam(r, "folderId")
	owner := ""
	if c.ActiveRole == "guru" {
		owner = c.UserID
	}
	page, perPage := common.Pagination(r)
	list, total, err := h.Store.ListQuestions(r.Context(), common.EffectiveSchoolID(r, c), folderID, owner, page, perPage)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.Meta{Page: page, PerPage: perPage, Total: total})
}

func (h *Handler) GetQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	q, err := h.Store.GetQuestion(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if q == nil {
		common.WriteError(w, http.StatusNotFound, "question not found")
		return
	}
	if c.ActiveRole == "guru" && q.CreatedBy != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only view soal you created")
		return
	}
	common.WriteJSON(w, http.StatusOK, q, nil)
}

func (h *Handler) UpdateQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	existing, err := h.Store.GetQuestion(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if existing == nil {
		common.WriteError(w, http.StatusNotFound, "question not found")
		return
	}
	// admin can edit any guru's soal (per spec); guru may only edit their own.
	if c.ActiveRole == "guru" && existing.CreatedBy != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only edit soal you created")
		return
	}
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	var req questionReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	req.QuestionType = existing.QuestionType // question_type is immutable after creation
	if msg, ok := validateQuestionShape(req); !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	q := toModelQuestion(req, existing.SchoolID, existing.FolderID, existing.CreatedBy)
	q.ID = id
	replaceOptions := len(req.AnswerOptions) > 0 || len(req.EssayOptions) > 0
	if err := h.Store.UpdateQuestion(r.Context(), q, replaceOptions); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	existing, err := h.Store.GetQuestion(r.Context(), id, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if existing == nil {
		common.WriteError(w, http.StatusNotFound, "question not found")
		return
	}
	if c.ActiveRole == "guru" && existing.CreatedBy != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only delete soal you created")
		return
	}
	if err := h.Store.DeleteQuestion(r.Context(), id, common.EffectiveSchoolID(r, c)); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, nil, nil)
}

// PublishQuestion: admin reviews & publishes a guru's soal.
func (h *Handler) PublishQuestion(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Store.PublishQuestion(r.Context(), id, common.EffectiveSchoolID(r, c)); err != nil {
		writeStoreErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}
