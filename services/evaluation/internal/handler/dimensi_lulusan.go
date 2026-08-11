package handler

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/evaluation/internal/common"
	"skci/evaluation/internal/model"
)

// RPM "8 Dimensi Lulusan" -- an Excel-template-driven import, not one-by-one
// form fields (client's explicit request). Convention follows
// services/identity's internal/handler/import.go (same excelize library,
// already a dependency elsewhere in this codebase -- see go.mod): one sheet,
// a header row naming the 8 fixed dimensions (model.DimensiLulusanFields),
// one data row with the guru's text per dimension.

const dimensiLulusanSheet = "Dimensi Lulusan"

// DownloadDimensiLulusanTemplate serves a blank .xlsx with the 8 dimension
// columns as headers (+ one example row), for the "Unduh Template" button.
func (h *Handler) DownloadDimensiLulusanTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can download this template")
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", dimensiLulusanSheet)
	for i, field := range model.DimensiLulusanFields {
		cellRef, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(dimensiLulusanSheet, cellRef, field.Label)
		exampleRef, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(dimensiLulusanSheet, exampleRef, fmt.Sprintf("Contoh isian untuk %s...", field.Label))
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=rpm_dimensi_lulusan_template.xlsx")
	if err := f.Write(w); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to generate template")
	}
}

// ImportDimensiLulusan (POST /me/lesson-plans/{id}/dimensi-lulusan-import)
// parses the uploaded .xlsx's first data row (row 2) against the 8 fixed
// dimension headers and overwrites this RPM's dimensi_lulusan. Only the RPM's
// own author (guru) may import into it.
func (h *Handler) ImportDimensiLulusan(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can import dimensi lulusan")
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

	rows, err := f.GetRows(dimensiLulusanSheet)
	if err != nil || len(rows) < 2 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("sheet %q must have a header row and at least one data row", dimensiLulusanSheet))
		return
	}
	hdr := map[string]int{}
	for i, h := range rows[0] {
		hdr[h] = i
	}
	dataRow := rows[1]
	dimensi := map[string]interface{}{}
	var missing []string
	for _, field := range model.DimensiLulusanFields {
		col, ok := hdr[field.Label]
		if !ok {
			missing = append(missing, field.Label)
			continue
		}
		val := ""
		if col < len(dataRow) {
			val = dataRow[col]
		}
		dimensi[field.Code] = val
	}
	if len(missing) > 0 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("template columns missing: %v (use the downloaded template's header row)", missing))
		return
	}

	ok, err := h.LessonPlans.SetDimensiLulusan(r.Context(), id, claims.UserID, dimensi)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save dimensi lulusan: "+err.Error())
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
