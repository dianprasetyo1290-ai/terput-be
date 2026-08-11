package store

import (
	"context"
	"database/sql"

	"skci/report/internal/model"
)

// UpsertReportTemplate stores (or replaces) the one uploaded .xlsx for a
// given (school_id, template_type) slot -- "Import Template" on the
// Generate Rapor admin screen. One row per school per rapor-type, latest
// upload wins (UNIQUE (school_id, template_type) + ON CONFLICT DO UPDATE).
func (s *Store) UpsertReportTemplate(ctx context.Context, schoolID, templateType, fileName, contentType string, data []byte, uploadedBy string) (*model.ReportTemplate, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO report_templates (school_id, template_type, file_name, content_type, file_size, file_data, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (school_id, template_type) DO UPDATE SET
			file_name = EXCLUDED.file_name,
			content_type = EXCLUDED.content_type,
			file_size = EXCLUDED.file_size,
			file_data = EXCLUDED.file_data,
			uploaded_by = EXCLUDED.uploaded_by,
			uploaded_at = now()
		RETURNING id, school_id, template_type, file_name, content_type, file_size, uploaded_by, uploaded_at`,
		schoolID, templateType, fileName, contentType, len(data), data, uploadedBy)

	var t model.ReportTemplate
	var uploadedBySql sql.NullString
	if err := row.Scan(&t.ID, &t.SchoolID, &t.Type, &t.FileName, &t.ContentType, &t.FileSize, &uploadedBySql, &t.UploadedAt); err != nil {
		return nil, err
	}
	if uploadedBySql.Valid {
		t.UploadedBy = &uploadedBySql.String
	}
	return &t, nil
}

// ListReportTemplates returns metadata (no file bytes) for every rapor-type
// slot a school has uploaded a replacement template for -- missing slots
// simply aren't in the result, the frontend shows those as "Belum ada file
// diunggah".
func (s *Store) ListReportTemplates(ctx context.Context, schoolID string) ([]model.ReportTemplate, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, template_type, file_name, content_type, file_size, uploaded_by, uploaded_at
		FROM report_templates WHERE school_id = $1 ORDER BY template_type`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ReportTemplate
	for rows.Next() {
		var t model.ReportTemplate
		var uploadedBySql sql.NullString
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Type, &t.FileName, &t.ContentType, &t.FileSize, &uploadedBySql, &t.UploadedAt); err != nil {
			return nil, err
		}
		if uploadedBySql.Valid {
			t.UploadedBy = &uploadedBySql.String
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetReportTemplateFile fetches one template's raw bytes for the download
// endpoint (admin verifying what was uploaded). Returns nil, nil if no
// template has been uploaded for this school/type yet.
func (s *Store) GetReportTemplateFile(ctx context.Context, schoolID, templateType string) (*model.ReportTemplate, []byte, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, template_type, file_name, content_type, file_size, file_data, uploaded_by, uploaded_at
		FROM report_templates WHERE school_id = $1 AND template_type = $2`, schoolID, templateType)

	var t model.ReportTemplate
	var data []byte
	var uploadedBySql sql.NullString
	err := row.Scan(&t.ID, &t.SchoolID, &t.Type, &t.FileName, &t.ContentType, &t.FileSize, &data, &uploadedBySql, &t.UploadedAt)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if uploadedBySql.Valid {
		t.UploadedBy = &uploadedBySql.String
	}
	return &t, data, nil
}
