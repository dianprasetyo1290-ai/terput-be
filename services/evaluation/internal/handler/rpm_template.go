package handler

import (
	"fmt"
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
// belongs to.
type rpmSection int

const (
	sectionA rpmSection = iota
	sectionB
	sectionC
	sectionD
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
	var rows []rpmTemplateRow
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
			f.SetCellValue(rpmTemplateSheet, fmt.Sprintf("A%d", row), sectionTitles[tr.Section])
			row++
			lastSection = tr.Section
		}
		f.SetCellValue(rpmTemplateSheet, fmt.Sprintf("A%d", row), tr.Label)
		f.SetCellValue(rpmTemplateSheet, fmt.Sprintf("B%d", row), fmt.Sprintf("Isi dari %s", tr.Label))
		row++
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=rpm_template.xlsx")
	if err := f.Write(w); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to generate template")
	}
}

// ImportRpmTemplate (POST /me/lesson-plans/{id}/rpm-import) parses the
// uploaded .xlsx and overwrites this RPM's section_a/b/c and
// dimensi_lulusan in one shot. Matched by the "Field" column's label text
// (case/whitespace-insensitive) against the fixed field list -- rows that
// don't match a known field (e.g. the section-title rows the template
// itself writes) are skipped, not errors, so a guru who edits/reorders
// their copy doesn't get a hard failure over a stray blank/title row. Only
// the RPM's own author (guru) may import into it.
func (h *Handler) ImportRpmTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can import RPM")
		return
	}
	id := chi.URLParam(r, "id")

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required (multipart field 'file')")
		return
	}
	defer file.Close()
	f, err := excelize.OpenReader(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid .xlsx file: "+err.Error())
		return
	}
	defer f.Close()

	rows, err := f.GetRows(rpmTemplateSheet)
	if err != nil || len(rows) < 2 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("sheet %q must have a header row and at least one data row", rpmTemplateSheet))
		return
	}

	byLabel := map[string]rpmTemplateRow{}
	for _, tr := range rpmTemplateRows() {
		byLabel[strings.ToLower(strings.TrimSpace(tr.Label))] = tr
	}

	sectionAData := map[string]interface{}{}
	sectionBData := map[string]interface{}{}
	sectionCData := map[string]interface{}{}
	dimensiData := map[string]interface{}{}
	matched := 0
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
		matched++
		switch tr.Section {
		case sectionA:
			sectionAData[tr.Code] = val
		case sectionB:
			sectionBData[tr.Code] = val
		case sectionC:
			sectionCData[tr.Code] = val
		case sectionD:
			dimensiData[tr.Code] = val
		}
	}
	if matched == 0 {
		common.WriteError(w, http.StatusBadRequest, "no known fields matched -- use the downloaded template's Field column as-is")
		return
	}

	ok, err := h.LessonPlans.ImportSections(r.Context(), id, claims.UserID, sectionAData, sectionBData, sectionCData, dimensiData)
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
