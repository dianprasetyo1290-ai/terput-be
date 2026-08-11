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

// Program Semester (Prosem) -- NEW FEATURE, Bug Report SEKOCI 5 Agustus
// 2026. Same parent+items import pattern as Prota (prota.go) -- see
// Template_Prosem_LMS_Ready.xlsx sheet TEMPLATE_PROSEM.

const prosemSheet = "TEMPLATE_PROSEM"

var prosemHeaders = []string{
	"No", "Materi Pokok / Aktivitas", "Alokasi (JP)", "Bulan Target", "Minggu Ke-",
	"Estimasi Tanggal (YYYY-MM-DD)", "Tipe Aktivitas LMS", "Status Input (Guru)",
}

func (h *Handler) DownloadProsemTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can download this template")
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", prosemSheet)
	for i, hdr := range prosemHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(prosemSheet, cell, hdr)
	}
	example := []interface{}{1, "Bab 1: Pembahasan Teks Deskripsi Objek", 4, "Juli", 3, "2026-07-20", "Materi Pendukung / Konten", "Belum"}
	for i, v := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		f.SetCellValue(prosemSheet, cell, v)
	}
	f.SetCellValue(prosemSheet, "A3", "TOTAL ALOKASI JP")
	f.SetCellValue(prosemSheet, "C3", 4)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=prosem_template.xlsx")
	if err := f.Write(w); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to generate template")
	}
}

// ImportProsem (POST /me/prosem/import) -- same convention as ImportProta.
func (h *Handler) ImportProsem(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "guru") {
		common.WriteError(w, http.StatusForbidden, "only guru can import Prosem")
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
	rows, err := f.GetRows(prosemSheet)
	if err != nil || len(rows) < 2 {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("sheet %q must have a header row and at least one data row", prosemSheet))
		return
	}

	p := &model.Prosem{
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
		it := model.ProsemItem{No: no}
		if len(row) > 1 {
			it.MateriPokokAktivitas = row[1]
		}
		if len(row) > 2 {
			it.AlokasiJP, _ = strconv.Atoi(row[2])
		}
		if len(row) > 3 {
			it.BulanTarget = row[3]
		}
		if len(row) > 4 {
			it.MingguKe, _ = strconv.Atoi(row[4])
		}
		if len(row) > 5 {
			it.EstimasiTanggal = row[5]
		}
		if len(row) > 6 {
			it.TipeAktivitasLMS = row[6]
		}
		if len(row) > 7 {
			it.StatusInputGuru = row[7]
		}
		p.Items = append(p.Items, it)
	}
	if len(p.Items) == 0 {
		common.WriteError(w, http.StatusBadRequest, "no data rows found -- ensure the No column has numeric values")
		return
	}

	if err := h.Prosem.Import(r.Context(), p); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to import Prosem: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, p, nil)
}

// GET /me/prosem
func (h *Handler) ListMyProsem(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	list, err := h.Prosem.ListByGuru(r.Context(), claims.UserID, claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list Prosem: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// GET /me/prosem/{id}
func (h *Handler) GetMyProsem(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	p, err := h.Prosem.GetByID(r.Context(), id, claims.SchoolID)
	if err != nil || p.GuruUserID != claims.UserID {
		common.WriteError(w, http.StatusNotFound, "Prosem not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

// DELETE /me/prosem/{id}
func (h *Handler) DeleteMyProsem(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	ok, err := h.Prosem.Delete(r.Context(), id, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete Prosem: "+err.Error())
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "Prosem not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// GET /prosem?guru_id= - admin read-only list-all.
func (h *Handler) AdminListProsem(w http.ResponseWriter, r *http.Request) {
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
	list, err := h.Prosem.ListAll(r.Context(), schoolID, guruID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list Prosem: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// GET /prosem/guru/{guru_id} - admin read-detail-by-guru.
func (h *Handler) AdminGetProsemByGuru(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !hasRole(claims, "admin") && !hasRole(claims, "superadmin") {
		common.WriteError(w, http.StatusForbidden, "admin only")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	guruID := chi.URLParam(r, "guru_id")
	list, err := h.Prosem.ListByGuru(r.Context(), guruID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch Prosem: "+err.Error())
		return
	}
	full := make([]*model.Prosem, 0, len(list))
	for _, p := range list {
		detailed, err := h.Prosem.GetByID(r.Context(), p.ID, schoolID)
		if err != nil {
			continue
		}
		full = append(full, detailed)
	}
	common.WriteJSON(w, http.StatusOK, full, nil)
}
