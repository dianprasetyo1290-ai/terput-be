package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/report/internal/common"
	"skci/report/internal/model"
)

// "Import Template" (Generate Rapor admin screen): lets an admin upload a
// replacement .xlsx for one of the 3 reference rapor layouts (Rapor Tengah
// Semester / Rapor Akhir Semester / Rapor Akhir Semester Halaman 2), stored
// per school per rapor-type.
//
// ponytail: this is upload/storage/replace ONLY -- the stored .xlsx is not
// parsed or rendered from. Building a generic "render PDF from an arbitrary
// uploaded template" engine is out of scope (YAGNI) for this pass; actual
// print output still comes from the hardcoded-but-dynamic-per-school layout
// in ReportCardPrintView.vue. Upgrade path if a school actually needs
// custom layouts: parse the uploaded sheet's cell positions the same way
// services/evaluation's dimensi_lulusan import reads a fixed header row,
// and feed that into the print view instead of the hardcoded template.

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

	name := strings.ToLower(header.Filename)
	if !strings.HasSuffix(name, ".xlsx") {
		common.WriteError(w, http.StatusBadRequest, "only .xlsx files are accepted")
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
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}

	schoolID := resolveSchoolID(r, claims)
	t, err := h.d.Store.UpsertReportTemplate(r.Context(), schoolID, templateType, header.Filename, contentType, data, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save report template")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// DownloadReportTemplate: GET /settings/report-templates/{type}/download --
// lets an admin retrieve/verify what was uploaded (record-keeping; not used
// by the print pipeline, see the scope-cut note above).
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
