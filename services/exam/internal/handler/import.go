package handler

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/exam/internal/common"
	"skci/exam/internal/model"
)

// csvColumns per question_type, matching both the import parser and the
// downloadable template below. See README for a worked example of each.
var csvColumns = map[string][]string{
	"pilihan_ganda": {"no_kd", "konten_kd", "indikator_pencapaian", "level_kognitif", "poin", "content", "option_a", "option_b", "option_c", "option_d", "option_e", "correct_option"},
	"benar_salah":   {"no_kd", "konten_kd", "indikator_pencapaian", "level_kognitif", "poin", "content", "correct_answer"},
	"essay":         {"no_kd", "konten_kd", "indikator_pencapaian", "level_kognitif", "poin", "content", "opsi_jawaban"},
	"tebak_gambar":  {"no_kd", "konten_kd", "indikator_pencapaian", "level_kognitif", "poin", "content", "image_url", "opsi_jawaban"},
}

var csvExampleRow = map[string][]string{
	"pilihan_ganda": {"3.1", "Memahami pecahan", "Siswa dapat menjumlahkan pecahan", "C2", "10", "Berapakah 1/2 + 1/4?", "1/2", "3/4", "1", "5/4", "2", "b"},
	"benar_salah":   {"3.2", "Memahami bilangan bulat", "Siswa dapat menentukan tanda bilangan", "C1", "5", "-5 lebih besar dari 3", "salah"},
	"essay":         {"3.3", "Memahami ibukota", "Siswa dapat menyebutkan ibukota negara", "C1", "10", "Apa ibukota Indonesia?", "Jakarta|DKI Jakarta"},
	"tebak_gambar":  {"3.4", "Mengenal bendera", "Siswa dapat mengenali bendera negara", "C1", "10", "Bendera negara apakah ini?", "https://example.com/flag.png", "Indonesia|Republik Indonesia"},
}

// QuestionImportTemplate: GET /question-import-template?type=
func (h *Handler) QuestionImportTemplate(w http.ResponseWriter, r *http.Request) {
	qType := r.URL.Query().Get("type")
	cols, ok := csvColumns[qType]
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "type must be one of pilihan_ganda|benar_salah|essay|tebak_gambar")
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_template.csv", qType))
	cw := csv.NewWriter(w)
	_ = cw.Write(cols)
	if ex, ok := csvExampleRow[qType]; ok {
		_ = cw.Write(ex)
	}
	cw.Flush()
}

// ImportQuestions: POST /exams/{id}/questions/import — multipart file (CSV;
// .xlsx also accepted via excelize, see README Assumptions) + required
// "type" form field + optional "folder_id".
func (h *Handler) ImportQuestions(w http.ResponseWriter, r *http.Request) {
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
		hasAttempt, err := h.Store.HasAnyAttempt(r.Context(), examID)
		if err != nil {
			writeStoreErr(w, err)
			return
		}
		if hasAttempt {
			common.WriteError(w, http.StatusConflict, "cannot import questions: siswa sudah mulai mengerjakan ujian ini")
			return
		}
	}

	if err := r.ParseMultipartForm(20 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}
	qType := r.FormValue("type")
	if _, ok := csvColumns[qType]; !ok {
		common.WriteError(w, http.StatusBadRequest, "type must be one of pilihan_ganda|benar_salah|essay|tebak_gambar")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required (multipart field 'file')")
		return
	}
	defer file.Close()

	var records [][]string
	if strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
		records, err = parseXLSX(file)
	} else {
		records, err = parseCSV(file)
	}
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "could not parse file: "+err.Error())
		return
	}
	if len(records) < 2 {
		common.WriteError(w, http.StatusBadRequest, "file must contain a header row plus at least one data row")
		return
	}

	// Item 7: importing soal while building an exam must NOT spawn a new Bank
	// Soal folder. Reuse the folder the exam itself was created from
	// (exam.FolderID, set when the guru entered the builder from a Bank
	// Soal folder's "Kelola Ujian" button); if the exam has no folder
	// context at all, require an explicit folder_id rather than silently
	// creating one.
	folderID := r.FormValue("folder_id")
	if folderID == "" && exam.FolderID != nil {
		folderID = *exam.FolderID
	}
	if folderID == "" {
		common.WriteError(w, http.StatusBadRequest, "folder_id is required: this exam has no Bank Soal folder to import into")
		return
	}

	cols := csvColumns[qType]
	header0 := records[0]
	idx := map[string]int{}
	for i, col := range header0 {
		idx[strings.TrimSpace(strings.ToLower(col))] = i
	}
	for _, want := range cols {
		if _, ok := idx[want]; !ok {
			common.WriteError(w, http.StatusBadRequest, "missing required column: "+want)
			return
		}
	}

	var created []*model.Question
	for rowNum, row := range records[1:] {
		req, msg := rowToQuestionReq(qType, row, idx, exam.AcademicYearID)
		if msg != "" {
			common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("row %d: %s", rowNum+2, msg))
			return
		}
		if vmsg, ok := validateQuestionShape(req); !ok {
			common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("row %d: %s", rowNum+2, vmsg))
			return
		}
		q := toModelQuestion(req, exam.SchoolID, folderID, c.UserID)
		if err := h.Store.CreateQuestion(r.Context(), q); err != nil {
			writeStoreErr(w, err)
			return
		}
		if _, err := h.Store.AddExamQuestion(r.Context(), examID, q.ID, q.Poin); err != nil {
			writeStoreErr(w, err)
			return
		}
		created = append(created, q)
	}

	if exam.IsPublished {
		if err := h.Store.RenormalizePublishedPoints(r.Context(), examID); err != nil {
			common.WriteJSON(w, http.StatusCreated, map[string]interface{}{
				"folder_id":      folderID,
				"imported_count": len(created),
				"questions":      created,
				"warning":        "soal diimpor, tapi gagal menyesuaikan ulang poin otomatis: " + err.Error(),
			}, nil)
			return
		}
	}

	common.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"folder_id":      folderID,
		"imported_count": len(created),
		"questions":      created,
	}, nil)
}

func parseCSV(r io.Reader) ([][]string, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	return cr.ReadAll()
}

func parseXLSX(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found")
	}
	return f.GetRows(sheets[0])
}

func rowToQuestionReq(qType string, row []string, idx map[string]int, academicYearID string) (questionReq, string) {
	get := func(col string) string {
		i, ok := idx[col]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	poin, err := strconv.ParseFloat(get("poin"), 64)
	if err != nil || poin <= 0 {
		poin = 1
	}
	req := questionReq{
		AcademicYearID:      academicYearID,
		QuestionType:        qType,
		Content:             get("content"),
		NoKD:                strPtr(get("no_kd")),
		KontenKD:            strPtr(get("konten_kd")),
		IndikatorPencapaian: strPtr(get("indikator_pencapaian")),
		LevelKognitif:       strPtr(get("level_kognitif")),
		Poin:                poin,
	}
	if req.Content == "" {
		return req, "content column is empty"
	}

	switch qType {
	case "pilihan_ganda":
		labels := []string{"a", "b", "c", "d", "e"}
		correct := strings.ToLower(get("correct_option"))
		for i, l := range labels {
			req.AnswerOptions = append(req.AnswerOptions, answerOptionReq{
				Content:   strPtr(get("option_" + l)),
				IsCorrect: l == correct,
			})
			_ = i
		}
	case "benar_salah":
		correct := strings.ToLower(get("correct_answer"))
		req.AnswerOptions = []answerOptionReq{
			{Content: strPtr("Benar"), IsCorrect: correct == "benar"},
			{Content: strPtr("Salah"), IsCorrect: correct == "salah"},
		}
	case "essay":
		req.EssayOptions = splitOpsi(get("opsi_jawaban"))
	case "tebak_gambar":
		req.ImageURL = strPtr(get("image_url"))
		req.EssayOptions = splitOpsi(get("opsi_jawaban"))
	}
	return req, ""
}

func splitOpsi(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "|")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
