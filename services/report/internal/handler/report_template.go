package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"skci/report/internal/common"
	"skci/report/internal/docx"
	"skci/report/internal/model"
)

// "Import Template" (Generate Rapor admin screen): lets an admin upload a
// replacement .xlsx for one of the 3 reference rapor layouts (Rapor Tengah
// Semester / Rapor Akhir Semester / Rapor Akhir Semester Halaman 2), stored
// per school per rapor-type.
//
// GetReportTemplateContent (Bug Report SEKOCI GPM V10, ROLE ADMIN "Generate
// Rapor": "Sistem tidak boleh menggunakan layout default jika sudah ada
// template kustom", "hasil rapor ... tidak menampilkan halaman 2 sama
// sekali") reads the uploaded sheet's cell values back out as a row/column
// grid; ReportCardPrintView.vue renders that grid as an actual appended
// second printed page for the akhir_semester_hal2 slot, which is what was
// missing -- the file existed in storage but nothing ever read it back.
//
// ponytail: only the sheet's cell TEXT is read (first sheet, GetRows) --
// no merged-cell spans, styling, or per-student placeholder substitution.
// The tengah_semester/akhir_semester slots (page 1) are intentionally left
// storage-only, unwired from print: page 1 is already a working
// data-driven layout (subject tables, live scores) built to match the
// reference template's structure, and there's no placeholder/mail-merge
// convention specified anywhere for how a raw uploaded grid should
// interleave with per-student data -- replacing it with a raw cell dump
// would regress working output. Upgrade path if a school needs page-1
// layout truly customizable: define a placeholder syntax (e.g. {{nama}})
// in the uploaded sheet and substitute against ReportCard fields the same
// way services/evaluation's dimensi_lulusan import reads a fixed header
// row.

const maxTemplateUploadBytes = 10 << 20 // 10MB

// ListReportTemplates: GET /settings/report-templates
func (h *Handler) ListReportTemplates(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	templates, err := h.d.Store.ListReportTemplates(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report templates")
		return
	}
	common.WriteJSON(w, http.StatusOK, templates, nil)
}

// UploadReportTemplate: POST /settings/report-templates (multipart form:
// "template_type" field + "file" field).
func (h *Handler) UploadReportTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "only admin/superadmin can upload report templates")
		return
	}
	if err := r.ParseMultipartForm(maxTemplateUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "failed to parse multipart form (max 10MB): "+err.Error())
		return
	}
	templateType := strings.TrimSpace(r.FormValue("template_type"))
	if !model.ValidReportTemplateType(templateType) {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("template_type must be one of: %v", model.ReportTemplateTypes))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required (multipart field 'file')")
		return
	}
	defer file.Close()

	// .docx is the mail-merge path (see handler/report_docx.go): a Word
	// file with {{placeholder}} markers that becomes the printed rapor.
	// .xlsx stays accepted for the reference-layout slots that predate it.
	name := strings.ToLower(header.Filename)
	isDocx := strings.HasSuffix(name, ".docx")
	if !isDocx && !strings.HasSuffix(name, ".xlsx") {
		common.WriteError(w, http.StatusBadRequest, "hanya file .docx (mail merge) atau .xlsx yang diterima")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTemplateUploadBytes+1))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to read uploaded file")
		return
	}
	if len(data) > maxTemplateUploadBytes {
		common.WriteError(w, http.StatusBadRequest, "file exceeds 10MB limit")
		return
	}
	if isDocx {
		// Reject a broken upload here rather than at print time, when a
		// wali kelas is trying to print a whole class.
		if _, err := docx.Render(data, docx.Data{}); err != nil {
			common.WriteError(w, http.StatusBadRequest, "file .docx tidak bisa dibaca: "+err.Error())
			return
		}
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		if isDocx {
			contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		} else {
			contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		}
	}

	schoolID := resolveSchoolID(r, claims)
	t, err := h.d.Store.UpsertReportTemplate(r.Context(), schoolID, templateType, header.Filename, contentType, data, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save report template")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// GetReportTemplateContent: GET /settings/report-templates/{type}/content --
// the uploaded sheet's cell text as a row/column grid, consumed by
// ReportCardPrintView.vue to render the akhir_semester_hal2 slot as an
// actual second printed page. 404 (not an empty grid) when nothing's been
// uploaded for this slot yet, so the frontend can tell "no override" apart
// from "template has an empty first sheet".
func (h *Handler) GetReportTemplateContent(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	templateType := chi.URLParam(r, "type")
	if !model.ValidReportTemplateType(templateType) {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("template_type must be one of: %v", model.ReportTemplateTypes))
		return
	}
	schoolID := resolveSchoolID(r, claims)
	t, data, err := h.d.Store.GetReportTemplateFile(r.Context(), schoolID, templateType)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report template")
		return
	}
	if t == nil {
		common.WriteError(w, http.StatusNotFound, "no template uploaded for this rapor-type yet")
		return
	}
	if strings.HasSuffix(strings.ToLower(t.FileName), ".docx") {
		// A .docx slot is rendered server-side by the mail-merge endpoint,
		// not read back as a cell grid. 404 keeps the frontend's existing
		// "no override, use the built-in layout" branch working unchanged.
		common.WriteError(w, http.StatusNotFound, "template slot ini berisi .docx (mail merge), bukan grid .xlsx")
		return
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "uploaded template file is not a readable .xlsx")
		return
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		common.WriteError(w, http.StatusInternalServerError, "uploaded template has no sheets")
		return
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to read template sheet")
		return
	}
	common.WriteJSON(w, http.StatusOK, model.ReportTemplateContent{Type: templateType, Rows: rows}, nil)
}

// DownloadReportTemplate: GET /settings/report-templates/{type}/download --
// lets an admin retrieve/verify what was uploaded (record-keeping only;
// GetReportTemplateContent above is what the print pipeline actually uses).
func (h *Handler) DownloadReportTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	templateType := chi.URLParam(r, "type")
	if !model.ValidReportTemplateType(templateType) {
		common.WriteError(w, http.StatusBadRequest, fmt.Sprintf("template_type must be one of: %v", model.ReportTemplateTypes))
		return
	}
	schoolID := resolveSchoolID(r, claims)
	t, data, err := h.d.Store.GetReportTemplateFile(r.Context(), schoolID, templateType)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report template")
		return
	}
	if t == nil {
		common.WriteError(w, http.StatusNotFound, "no template uploaded for this rapor-type yet")
		return
	}
	w.Header().Set("Content-Type", t.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", t.FileName))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
