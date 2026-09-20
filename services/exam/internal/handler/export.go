package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
)

// ExportQuestions: GET /exams/{id}/questions/export?type=pilihan_ganda
//
// The inverse of ImportQuestions, writing the SAME columns as
// QuestionImportTemplate so a downloaded file can be re-uploaded as is.
// That round-trip is the whole point of the request ("REPORT FITUR ADMIN",
// 21 Sep 2026): when a guru uploads soal to the wrong exam or folder, the
// admin had no way to get them back out -- only re-typing every question.
//
// One question_type per file, because import parses exactly one column
// layout per file; an exam mixing types exports one file per type.
func (h *Handler) ExportQuestions(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	qType := r.URL.Query().Get("type")
	cols, ok := csvColumns[qType]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "type must be one of pilihan_ganda|benar_salah|essay|tebak_gambar")
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
	rows, err := h.Store.ListExamQuestions(r.Context(), examID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s.csv", safeFilename(exam.Title), qType))
	cw := csv.NewWriter(w)
	_ = cw.Write(cols)
	for _, eq := range rows {
		if eq.Question == nil || eq.Question.QuestionType != qType {
			continue
		}
		_ = cw.Write(questionToRow(qType, eq.Question))
	}
	cw.Flush()
}

// questionToRow mirrors rowToQuestionReq column for column. Keep the two in
// step: a column added to csvColumns must be written here too, or the
// export silently drops it on the way back in.
func questionToRow(qType string, q *model.Question) []string {
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	row := []string{
		deref(q.NoKD), deref(q.KontenKD), deref(q.IndikatorPencapaian), deref(q.LevelKognitif),
		strconv.FormatFloat(q.Poin, 'f', -1, 64), q.Content,
	}
	switch qType {
	case "pilihan_ganda":
		// AnswerOptions come back ordered by order_index, which import wrote
		// as a..e in that order; correct_option is the label of the one
		// flagged is_correct.
		labels := []string{"a", "b", "c", "d", "e"}
		opts := make([]string, len(labels))
		correct := ""
		for i, o := range q.AnswerOptions {
			if i >= len(labels) {
				break
			}
			opts[i] = deref(o.Content)
			if o.IsCorrect {
				correct = labels[i]
			}
		}
		row = append(row, opts...)
		row = append(row, correct)
	case "benar_salah":
		answer := ""
		for _, o := range q.AnswerOptions {
			if o.IsCorrect {
				answer = strings.ToLower(deref(o.Content))
			}
		}
		row = append(row, answer)
	case "essay":
		row = append(row, joinOpsi(q.EssayOptions))
	case "tebak_gambar":
		row = append(row, deref(q.ImageURL), joinOpsi(q.EssayOptions))
	}
	return row
}

func joinOpsi(opts []model.EssayAnswerOption) string {
	parts := make([]string, 0, len(opts))
	for _, o := range opts {
		parts = append(parts, o.AnswerText)
	}
	return strings.Join(parts, "|")
}

// safeFilename keeps the exam title recognisable in the download while
// staying safe inside a Content-Disposition header (no quotes, no CR/LF,
// no path separators).
func safeFilename(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		case r == ' ':
			return '_'
		}
		return -1
	}, s)
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "soal"
	}
	return s
}
