package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/evaluation/internal/common"
	"skci/evaluation/internal/model"
)

// Program Tahunan (Prota) -- NEW FEATURE, Bug Report SEKOCI 5 Agustus 2026.
// Excel-import-driven, multi-row parent+items (unlike RPM's single-record
// dimensi import) -- see Template_Prota_LMS_Ready.xlsx sheet TEMPLATE_PROTA.
// Same excelize convention as identity-service's import (go.mod dependency,
// already used by RPM's own template, see rpm_template.go).

const protaSheet = "TEMPLATE_PROTA"

var protaHeaders = []string{
	"No", "Semester", "Elemen/Kode Bab", "Kompetensi Dasar/Tujuan Pembelajaran (TP)",
	"Alokasi Waktu (JP)", "Keterangan/Target Asesmen",
}

// DownloadProtaTemplate serves a blank .xlsx with the TEMPLATE_PROTA header
// row, one example row, and a trailing TOTAL summary row for guidance.
func (h *Handler) DownloadProtaTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can download this template")
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", protaSheet)
	for i, hdr := range protaHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(protaSheet, cell, hdr)
	}
	example := []interface{}{1, 1, "Bab 1: Teks Deskripsi", "3.1 Memahami isi teks deskripsi objek...", 12, "Tugas Mandiri 1 & Kuis 1"}
	for i, v := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(protaSheet, cell, v)
	}
	f.SetCellValue(protaSheet, "A3", "TOTAL ALOKASI WAKTU (TAHUNAN)")
	f.SetCellValue(protaSheet, "E3", 12)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=prota_template.xlsx")
	if err := f.Write(w); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to generate template")
	}
}

// ImportProta (POST /me/prota/import) parses a filled TEMPLATE_PROTA .xlsx
// (multipart field "file") plus the parent's identifying metadata
// (multipart fields "mata_pelajaran", "kelas", "academic_year_id" -- not in
// the row data, so they ride alongside the file) and creates a new Prota +
// its line items. Rows whose "No" column doesn't parse as an integer (the
// template's own TOTAL summary row, blank rows) are skipped, not errors.
func (h *Handler) ImportProta(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can import Prota")
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}
	mataPelajaran := r.FormValue("mata_pelajaran")
	kelas := r.FormValue("kelas")
	if mataPelajaran == "" || kelas == "" {
		common.WriteError(w, http.StatusBadRequest, "mata_pelajaran and kelas are required")
		return
	}
	ayID, ok := h.resolveAcademicYear(claims.SchoolID, r.FormValue("academic_year_id"))
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "academic_year_id is required (academic-service current-year lookup unavailable)")
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
	rows, err := f.GetRows(protaSheet)
	if err != nil || len(rows) < 2 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("sheet %q must have a header row and at least one data row", protaSheet))
		return
	}

	p := &model.Prota{
		SchoolID: claims.SchoolID, AcademicYearID: ayID, GuruUserID: claims.UserID,
		MataPelajaran: mataPelajaran, Kelas: kelas,
	}
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		no, err := strconv.Atoi(row[0])
		if err != nil {
			continue // TOTAL row / blank row / non-data row
		}
		it := model.ProtaItem{No: no}
		if len(row) > 1 {
			it.Semester = row[1]
		}
		if len(row) > 2 {
			it.ElemenKodeBab = row[2]
		}
		if len(row) > 3 {
			it.KompetensiDasarTP = row[3]
		}
		if len(row) > 4 {
			it.AlokasiWaktuJP, _ = strconv.Atoi(row[4])
		}
		if len(row) > 5 {
			it.KeteranganTargetAsesmen = row[5]
		}
		p.Items = append(p.Items, it)
	}
	if len(p.Items) == 0 {
		common.WriteError(w, http.StatusBadRequest, "no data rows found -- ensure the No column has numeric values")
		return
	}

	if err := h.Prota.Import(r.Context(), p); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to import Prota: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, p, nil)
}

// GET /me/prota - guru lists their own Prota records.
func (h *Handler) ListMyProta(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	list, err := h.Prota.ListByGuru(r.Context(), claims.UserID, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list Prota: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// GET /me/prota/{id} - full detail incl. items, for printing.
func (h *Handler) GetMyProta(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	p, err := h.Prota.GetByID(r.Context(), id, claims.SchoolID)
	if err != nil || p.GuruUserID != claims.UserID {
		common.WriteError(w, http.StatusNotFound, "Prota not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

// DELETE /me/prota/{id}
func (h *Handler) DeleteMyProta(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	ok, err := h.Prota.Delete(r.Context(), id, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete Prota: "+err.Error())
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "Prota not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// GET /prota?guru_id= - admin read-only list-all.
func (h *Handler) AdminListProta(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	var guruID *string
	if g := r.URL.Query().Get("guru_id"); g != "" {
		guruID = &g
	}
	list, err := h.Prota.ListAll(r.Context(), schoolID, guruID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list Prota: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// GET /prota/guru/{guru_id} - admin read-detail-by-guru.
func (h *Handler) AdminGetProtaByGuru(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	guruID := chi.URLParam(r, "guru_id")
	list, err := h.Prota.ListByGuru(r.Context(), guruID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch Prota: "+err.Error())
		return
	}
	// admin detail view needs items (for print) -- ListByGuru doesn't load
	// them (see ProtaStore.ListByGuru comment), so fetch each record's full
	// detail. Small N per guru in practice (one Prota per mapel/kelas/tahun).
	full := make([]*model.Prota, 0, len(list))
	for _, p := range list {
		detailed, err := h.Prota.GetByID(r.Context(), p.ID, schoolID)
		if err != nil {
			continue
		}
		full = append(full, detailed)
	}
	common.WriteJSON(w, http.StatusOK, full, nil)
}
