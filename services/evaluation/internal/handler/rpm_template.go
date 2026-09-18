package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/evaluation/internal/common"
	"skci/evaluation/internal/model"
)

// RPM full A/B/C/D Excel template -- corrected 2026-08-11 (Bug Report SEKOCI,
// 5 Agustus 2026) to replace the earlier "8 Dimensi Lulusan only" import
// (that guessed field names -- see model.DimensiLulusanFields' doc comment
// and migration 000004) with one file covering all 4 RPM sections, matching
// the client's "Judul RPM" example table: a two-column (Field | Isi) sheet,
// one row per field across A/B/C/D, section-title rows for readability.
// Same excelize convention as services/identity's import (already a
// dependency, see go.mod).

const rpmTemplateSheet = "RPM Template"

// rpmSection identifies which of the 4 RPM sections a template row's field
// belongs to. sectionMeta (Bug Report SEKOCI GPM V10, ROLE GURU "Data RPM")
// is the 3 leading rows outside A/B/C/D -- Judul RPM / Tahun Ajaran / Status
// -- that the client's template puts before "A. Identitas & Karakteristik".
type rpmSection int

const (
	sectionMeta rpmSection = iota
	sectionA
	sectionB
	sectionC
	sectionD
)

const (
	metaTitle = "title"
	metaYear  = "academic_year_label"
	metaState = "status"
)

type rpmTemplateRow struct {
	Section rpmSection
	Code    string
	Label   string
}

// rpmTemplateRows is the full ordered field list the template/import walks,
// built once from model's per-section field lists so this file and the
// guru-facing form (RpmMineFormView.vue) never drift apart on field names.
func rpmTemplateRows() []rpmTemplateRow {
	rows := []rpmTemplateRow{
		{sectionMeta, metaTitle, "Judul RPM"},
		{sectionMeta, metaYear, "Tahun Ajaran"},
		{sectionMeta, metaState, "Status"},
	}
	for _, f := range model.SectionAFields {
		rows = append(rows, rpmTemplateRow{sectionA, f.Code, f.Label})
	}
	for _, f := range model.SectionBFields {
		rows = append(rows, rpmTemplateRow{sectionB, f.Code, f.Label})
	}
	for _, f := range model.SectionCFields {
		rows = append(rows, rpmTemplateRow{sectionC, f.Code, f.Label})
	}
	for _, f := range model.DimensiLulusanFields {
		rows = append(rows, rpmTemplateRow{sectionD, f.Code, f.Label})
	}
	return rows
}

var sectionTitles = map[rpmSection]string{
	sectionA: "A. Identitas & Karakteristik",
	sectionB: "B. Analisis Kesiapan & Desain",
	sectionC: "C. Langkah Pembelajaran",
	sectionD: "D. Indikator 8 Dimensi Lulusan",
}

// DownloadRpmTemplate serves a blank .xlsx covering all 4 RPM sections
// (A/B/C/D), one row per field, for the "Unduh Template" button.
func (h *Handler) DownloadRpmTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can download this template")
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", rpmTemplateSheet)
	f.SetCellValue(rpmTemplateSheet, "A1", "Field")
	f.SetCellValue(rpmTemplateSheet, "B1", "Isi")

	row := 2
	lastSection := rpmSection(-1)
	for _, tr := range rpmTemplateRows() {
		if tr.Section != lastSection {
			// sectionMeta (Judul RPM/Tahun Ajaran/Status) has no section-title
			// row of its own -- it's the 3 leading fields before "A. ...".
			if title, ok := sectionTitles[tr.Section]; ok {
				f.SetCellValue(rpmTemplateSheet, fmt.Sprintf("A%d", row), title)
				row++
			}
			lastSection = tr.Section
		}
		hint := fmt.Sprintf("Isi dari %s", tr.Label)
		if tr.Code == metaState {
			hint = "Draft atau Final"
		}
		f.SetCellValue(rpmTemplateSheet, fmt.Sprintf("A%d", row), tr.Label)
		f.SetCellValue(rpmTemplateSheet, fmt.Sprintf("B%d", row), hint)
		row++
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=rpm_template.xlsx")
	if err := f.Write(w); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to generate template")
	}
}

// rpmParsed is one uploaded RPM template's parsed rows, shared by both
// import entry points below.
type rpmParsed struct {
	Title     string
	YearLabel string
	Status    string
	SectionA  map[string]interface{}
	SectionB  map[string]interface{}
	SectionC  map[string]interface{}
	Dimensi   map[string]interface{}
	Matched   int
}

// parseRpmTemplateFile reads an uploaded .xlsx against the fixed field list
// (rpmTemplateRows), matched by the "Field" column's label text
// (case/whitespace-insensitive) -- rows that don't match a known field
// (e.g. the section-title rows the template itself writes) are skipped,
// not errors, so a guru who edits/reorders their copy doesn't get a hard
// failure over a stray blank/title row.
func parseRpmTemplateFile(file io.Reader) (rpmParsed, error) {
	var out rpmParsed
	f, err := excelize.OpenReader(file)
	if err != nil {
		return out, fmt.Errorf("invalid .xlsx file: %w", err)
	}
	defer f.Close()

	rows, err := f.GetRows(rpmTemplateSheet)
	if err != nil || len(rows) < 2 {
		return out, fmt.Errorf("sheet %q must have a header row and at least one data row", rpmTemplateSheet)
	}

	byLabel := map[string]rpmTemplateRow{}
	for _, tr := range rpmTemplateRows() {
		byLabel[strings.ToLower(strings.TrimSpace(tr.Label))] = tr
	}

	out.SectionA = map[string]interface{}{}
	out.SectionB = map[string]interface{}{}
	out.SectionC = map[string]interface{}{}
	out.Dimensi = map[string]interface{}{}
	for _, dataRow := range rows[1:] {
		if len(dataRow) == 0 {
			continue
		}
		tr, ok := byLabel[strings.ToLower(strings.TrimSpace(dataRow[0]))]
		if !ok {
			continue
		}
		val := ""
		if len(dataRow) > 1 {
			val = dataRow[1]
		}
		out.Matched++
		switch {
		case tr.Section == sectionMeta && tr.Code == metaTitle:
			out.Title = val
		case tr.Section == sectionMeta && tr.Code == metaYear:
			out.YearLabel = val
		case tr.Section == sectionMeta && tr.Code == metaState:
			out.Status = strings.ToLower(strings.TrimSpace(val))
		case tr.Section == sectionA:
			out.SectionA[tr.Code] = val
		case tr.Section == sectionB:
			out.SectionB[tr.Code] = val
		case tr.Section == sectionC:
			out.SectionC[tr.Code] = val
		case tr.Section == sectionD:
			out.Dimensi[tr.Code] = val
		}
	}
	if out.Matched == 0 {
		return out, fmt.Errorf("no known fields matched -- use the downloaded template's Field column as-is")
	}
	return out, nil
}

func readUploadedFile(r *http.Request) (io.Reader, error) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		return nil, fmt.Errorf("failed to parse multipart form: %w", err)
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("file is required (multipart field 'file')")
	}
	return file, nil
}

// ImportRpmTemplate (POST /me/lesson-plans/{id}/rpm-import) overwrites an
// EXISTING RPM's section_a/b/c/dimensi_lulusan (and title/status, if those
// rows are filled in) in one shot. Only the RPM's own author (guru) may
// import into it. academic_year_id is left untouched on purpose -- moving
// an existing RPM to a different tahun ajaran isn't part of this flow.
func (h *Handler) ImportRpmTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can import RPM")
		return
	}
	id := chi.URLParam(r, "id")

	file, err := readUploadedFile(r)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	parsed, err := parseRpmTemplateFile(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	ok, err := h.LessonPlans.ImportSections(r.Context(), id, claims.UserID, parsed.Title, parsed.Status, parsed.SectionA, parsed.SectionB, parsed.SectionC, parsed.Dimensi)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save RPM import: "+err.Error())
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "lesson plan not found")
		return
	}
	updated, err := h.LessonPlans.GetByID(r.Context(), id, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to reload lesson plan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// ImportRpmCreate (POST /me/lesson-plans/rpm-import, no {id}) is the "Modul
// RPM" list page's "Import RPM" button (Bug Report SEKOCI GPM V10, ROLE
// GURU "Data RPM": Download/Import belong on the list page, adding a NEW
// RPM -- not the manual create form, which is a separate flow). The
// template's "Tahun Ajaran" row is a free-text label (not a UUID), so it's
// resolved against the school's academic years by case-insensitive
// substring match; no match (or a blank row) falls back to the current
// year, same as the manual create form's default.
func (h *Handler) ImportRpmCreate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can import RPM")
		return
	}

	file, err := readUploadedFile(r)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	parsed, err := parseRpmTemplateFile(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	ayID := h.resolveAcademicYearByLabel(claims.SchoolID, parsed.YearLabel)
	if ayID == "" {
		common.WriteError(w, http.StatusBadRequest, "could not resolve tahun ajaran (identity-service unavailable or no current year set)")
		return
	}
	status := parsed.Status
	if status != "final" {
		status = "draft"
	}
	lp := &model.LessonPlan{
		SchoolID: claims.SchoolID, AcademicYearID: ayID, GuruUserID: claims.UserID,
		Title: parsed.Title, SectionA: parsed.SectionA, SectionB: parsed.SectionB, SectionC: parsed.SectionC,
		DimensiLulusan: parsed.Dimensi, Status: status,
	}
	if lp.Title == "" {
		lp.Title = "RPM (Import)"
	}
	if err := h.LessonPlans.Create(r.Context(), lp); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create lesson plan: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, lp, nil)
}

// resolveAcademicYearByLabel matches the template's free-text "Tahun
// Ajaran" cell against the school's academic years (case-insensitive
// substring, e.g. "2026/2027" matching a "2026/2027 (Ganjil)" label);
// falls back to the current year, same default rule as resolveAcademicYear.
func (h *Handler) resolveAcademicYearByLabel(schoolID, label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	years := h.Clients.GetAcademicYears(schoolID)
	if label != "" {
		for _, y := range years {
			if strings.Contains(strings.ToLower(y.Label), label) {
				return y.ID
			}
		}
	}
	for _, y := range years {
		if y.Current {
			return y.ID
		}
	}
	return ""
}
