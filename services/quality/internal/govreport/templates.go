package govreport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"skci/quality/internal/common"
)

var (
	errTemplateNotFound = errors.New("template tidak ditemukan")
	errTemplateInUse    = errors.New("template masih dipakai laporan yang belum selesai")
)

// validateTemplate checks the fields the DB constraints would otherwise
// reject with a 500.
func validateTemplate(t *RegulatoryTemplate) string {
	t.Code = strings.TrimSpace(t.Code)
	t.Name = strings.TrimSpace(t.Name)
	if t.Code == "" || t.Name == "" || strings.TrimSpace(t.Recipient) == "" {
		return "kode, nama, dan penerima laporan wajib diisi"
	}
	switch t.Frequency {
	case "monthly", "semester", "annual", "adhoc":
	default:
		return "frekuensi harus monthly, semester, annual, atau adhoc"
	}
	if t.ExportFormat == "" {
		t.ExportFormat = "xlsx"
	}
	if t.ExportFormat != "xlsx" && t.ExportFormat != "csv" {
		return "format ekspor harus xlsx atau csv"
	}
	if t.Status == "" {
		t.Status = "active"
	}
	if t.Status != "active" && t.Status != "inactive" {
		return "status harus active atau inactive"
	}
	for _, c := range t.Columns {
		if strings.TrimSpace(c.Label) == "" || strings.TrimSpace(c.MetricKey) == "" {
			return "setiap kolom wajib memiliki label dan metric_key"
		}
	}
	return ""
}

func (s *Store) UpdateTemplate(ctx context.Context, tpl RegulatoryTemplate) error {
	colBytes, _ := json.Marshal(tpl.Columns)
	tag, err := s.pool.Exec(ctx, `
		UPDATE regulatory_templates
		SET code = $3, name = $4, recipient = $5, frequency = $6, columns = $7, export_format = $8, status = $9, updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`,
		tpl.ID, tpl.SchoolID, tpl.Code, tpl.Name, tpl.Recipient, tpl.Frequency, colBytes, tpl.ExportFormat, tpl.Status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errTemplateNotFound
	}
	return nil
}

// DeleteTemplate soft-deletes a template. Reports keep their frozen
// template_snapshot; a template with an open report (draft/pending/approved)
// cannot be removed.
func (s *Store) DeleteTemplate(ctx context.Context, schoolID, id string) error {
	var open int
	if err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM regulatory_reports
		WHERE template_id = $1 AND school_id = $2 AND status IN ('draft', 'pending_approval', 'approved')`,
		id, schoolID).Scan(&open); err != nil {
		return err
	}
	if open > 0 {
		return errTemplateInUse
	}
	tag, err := s.pool.Exec(ctx, `UPDATE regulatory_templates SET deleted_at = now(), updated_at = now() WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errTemplateNotFound
	}
	return nil
}

// ApplyApprovalDecision records the Approval Center decision on a
// regulatory report (callback request_type regulatory_report).
func (s *Store) ApplyApprovalDecision(ctx context.Context, approvalID, decision string) (string, string, error) {
	status := "approved"
	if decision != "approved" {
		status = "rejected"
	}
	var id, schoolID string
	err := s.pool.QueryRow(ctx, `
		UPDATE regulatory_reports SET status = $2, updated_at = now()
		WHERE approval_id = $1::uuid AND status = 'pending_approval'
		RETURNING id, school_id`, approvalID, status).Scan(&id, &schoolID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	return id, schoolID, nil
}

// GetTemplate: GET /gov/report-templates/{id}.
func (h *Handler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	tpl, err := h.store.GetTemplate(r.Context(), h.effectiveSchoolID(r), chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat template")
		return
	}
	if tpl == nil {
		common.WriteError(w, http.StatusNotFound, errTemplateNotFound.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, tpl)
}

// UpdateTemplate: PUT /gov/report-templates/{id} (admin).
func (h *Handler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	var req RegulatoryTemplate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if msg := validateTemplate(&req); msg != "" {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	req.ID = chi.URLParam(r, "id")
	req.SchoolID = schoolID
	before, _ := h.store.GetTemplate(r.Context(), schoolID, req.ID)
	err := h.store.UpdateTemplate(r.Context(), req)
	switch {
	case errors.Is(err, errTemplateNotFound):
		common.WriteError(w, http.StatusNotFound, err.Error())
		return
	case err != nil && strings.Contains(err.Error(), "uq_regulatory_template_code"):
		common.WriteError(w, http.StatusConflict, "kode template sudah dipakai")
		return
	case err != nil:
		common.WriteError(w, http.StatusInternalServerError, "gagal memperbarui template")
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "regulatory_template", req.ID, "update", claims.UserID, claims.ActiveRole, before, req, "Template laporan diperbarui")
	tpl, _ := h.store.GetTemplate(r.Context(), schoolID, req.ID)
	common.WriteJSON(w, http.StatusOK, tpl)
}

// DeleteTemplate: DELETE /gov/report-templates/{id} (admin, soft delete).
func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	schoolID := h.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	err := h.store.DeleteTemplate(r.Context(), schoolID, id)
	switch {
	case errors.Is(err, errTemplateNotFound):
		common.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, errTemplateInUse):
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		common.WriteError(w, http.StatusInternalServerError, "gagal menghapus template")
		return
	}
	_ = h.store.RecordAudit(r.Context(), schoolID, "regulatory_template", id, "delete", claims.UserID, claims.ActiveRole, nil, nil, "Template laporan dihapus")
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Template dihapus"})
}
