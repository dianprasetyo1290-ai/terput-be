package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"skci/hr/internal/model"
)

// --- Document Types ---

func ListDocumentTypes(ctx context.Context, q Q, schoolID string) ([]model.DocumentType, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, school_id, code, name, has_expiry, created_at, updated_at
		FROM document_types WHERE school_id = $1 AND deleted_at IS NULL ORDER BY code`, schoolID)
	return collect(rows, err, func(r *sqlRows) (model.DocumentType, error) {
		var dt model.DocumentType
		err := r.Scan(&dt.ID, &dt.SchoolID, &dt.Code, &dt.Name, &dt.HasExpiry, &dt.CreatedAt, &dt.UpdatedAt)
		return dt, err
	})
}

func CreateDocumentType(ctx context.Context, q Q, dt *model.DocumentType) error {
	return q.QueryRowContext(ctx, `INSERT INTO document_types (school_id, code, name, has_expiry)
		VALUES ($1,$2,$3,$4) RETURNING id, created_at, updated_at`,
		dt.SchoolID, dt.Code, dt.Name, dt.HasExpiry).Scan(&dt.ID, &dt.CreatedAt, &dt.UpdatedAt)
}

func UpdateDocumentType(ctx context.Context, q Q, dt *model.DocumentType) error {
	return affected(q.ExecContext(ctx, `UPDATE document_types SET code=$3, name=$4, has_expiry=$5, updated_at=now()
		WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`,
		dt.SchoolID, dt.ID, dt.Code, dt.Name, dt.HasExpiry))
}

func DeleteDocumentType(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `UPDATE document_types SET deleted_at=now() WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`, schoolID, id))
}

// --- Required Documents ---

func ListRequiredDocuments(ctx context.Context, q Q, schoolID, empType string) ([]model.RequiredDocument, error) {
	where := " WHERE rd.school_id = $1"
	args := []any{schoolID}
	if empType != "" {
		args = append(args, empType)
		where += fmt.Sprintf(" AND rd.employment_type = $%d", len(args))
	}
	rows, err := q.QueryContext(ctx, `SELECT rd.id, rd.school_id, rd.employment_type, rd.document_type_id,
		dt.code, dt.name, rd.created_at
		FROM required_documents rd
		JOIN document_types dt ON dt.id = rd.document_type_id`+where+` ORDER BY rd.employment_type, dt.name`, args...)
	return collect(rows, err, func(r *sqlRows) (model.RequiredDocument, error) {
		var rd model.RequiredDocument
		err := r.Scan(&rd.ID, &rd.SchoolID, &rd.EmploymentType, &rd.DocumentTypeID,
			&rd.DocumentTypeCode, &rd.DocumentTypeName, &rd.CreatedAt)
		return rd, err
	})
}

func CreateRequiredDocument(ctx context.Context, q Q, rd *model.RequiredDocument) error {
	return q.QueryRowContext(ctx, `INSERT INTO required_documents (school_id, employment_type, document_type_id)
		VALUES ($1,$2,$3) RETURNING id, created_at`,
		rd.SchoolID, rd.EmploymentType, rd.DocumentTypeID).Scan(&rd.ID, &rd.CreatedAt)
}

func DeleteRequiredDocument(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `DELETE FROM required_documents WHERE school_id=$1 AND id=$2`, schoolID, id))
}

func GetRequiredDocumentIDs(ctx context.Context, q Q, schoolID, empType string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT document_type_id::text FROM required_documents WHERE school_id = $1 AND employment_type = $2`, schoolID, empType)
	return collect(rows, err, func(r *sqlRows) (string, error) {
		var id string
		return id, r.Scan(&id)
	})
}

// --- Employee Documents ---

func ListEmployeeDocuments(ctx context.Context, q Q, schoolID, empID string) ([]model.EmployeeDocument, error) {
	rows, err := q.QueryContext(ctx, `SELECT ed.id, ed.school_id, ed.employee_id, ed.document_type_id,
		dt.code, dt.name, ed.version, ed.object_key, ed.file_name, ed.mime, ed.size_bytes,
		ed.expires_on::text, ed.uploaded_by::text, ed.requested_by_hr, ed.superseded_at, ed.created_at
		FROM employee_documents ed
		JOIN document_types dt ON dt.id = ed.document_type_id
		WHERE ed.school_id = $1 AND ed.employee_id = $2
		ORDER BY ed.created_at DESC`, schoolID, empID)
	return collect(rows, err, func(r *sqlRows) (model.EmployeeDocument, error) {
		var ed model.EmployeeDocument
		err := r.Scan(&ed.ID, &ed.SchoolID, &ed.EmployeeID, &ed.DocumentTypeID,
			&ed.DocumentTypeCode, &ed.DocumentTypeName, &ed.Version, &ed.ObjectKey, &ed.FileName,
			&ed.MIME, &ed.SizeBytes, &ed.ExpiresOn, &ed.UploadedBy, &ed.RequestedByHR,
			&ed.SupersededAt, &ed.CreatedAt)
		return ed, err
	})
}

func GetEmployeeDocument(ctx context.Context, q Q, schoolID, docID string) (*model.EmployeeDocument, error) {
	var ed model.EmployeeDocument
	err := q.QueryRowContext(ctx, `SELECT ed.id, ed.school_id, ed.employee_id, ed.document_type_id,
		dt.code, dt.name, ed.version, ed.object_key, ed.file_name, ed.mime, ed.size_bytes,
		ed.expires_on::text, ed.uploaded_by::text, ed.requested_by_hr, ed.superseded_at, ed.created_at
		FROM employee_documents ed
		JOIN document_types dt ON dt.id = ed.document_type_id
		WHERE ed.school_id = $1 AND ed.id = $2`, schoolID, docID).
		Scan(&ed.ID, &ed.SchoolID, &ed.EmployeeID, &ed.DocumentTypeID,
			&ed.DocumentTypeCode, &ed.DocumentTypeName, &ed.Version, &ed.ObjectKey, &ed.FileName,
			&ed.MIME, &ed.SizeBytes, &ed.ExpiresOn, &ed.UploadedBy, &ed.RequestedByHR,
			&ed.SupersededAt, &ed.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ed, nil
}

func CreateEmployeeDocument(ctx context.Context, q Q, doc *model.EmployeeDocument) error {
	// Mark older versions as superseded
	_, _ = q.ExecContext(ctx, `UPDATE employee_documents SET superseded_at = now()
		WHERE school_id = $1 AND employee_id = $2 AND document_type_id = $3 AND superseded_at IS NULL`,
		doc.SchoolID, doc.EmployeeID, doc.DocumentTypeID)

	// Get next version
	var maxVer int
	_ = q.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM employee_documents
		WHERE employee_id = $1 AND document_type_id = $2`, doc.EmployeeID, doc.DocumentTypeID).Scan(&maxVer)
	doc.Version = maxVer + 1

	return q.QueryRowContext(ctx, `INSERT INTO employee_documents (school_id, employee_id, document_type_id,
		version, object_key, file_name, mime, size_bytes, expires_on, uploaded_by, requested_by_hr)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::date,NULLIF($10,'')::uuid,$11)
		RETURNING id, created_at`,
		doc.SchoolID, doc.EmployeeID, doc.DocumentTypeID, doc.Version, doc.ObjectKey, doc.FileName,
		doc.MIME, doc.SizeBytes, doc.ExpiresOn, doc.UploadedBy, doc.RequestedByHR).Scan(&doc.ID, &doc.CreatedAt)
}

func DeleteEmployeeDocument(ctx context.Context, q Q, schoolID, docID string) error {
	return affected(q.ExecContext(ctx, `DELETE FROM employee_documents WHERE school_id = $1 AND id = $2`, schoolID, docID))
}

func GetEmployeeUploadedDocumentIDs(ctx context.Context, q Q, schoolID, empID string) ([]string, error) {
	today := time.Now().Format("2006-01-02")
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT document_type_id::text FROM employee_documents
		WHERE school_id = $1 AND employee_id = $2 AND superseded_at IS NULL
		  AND (expires_on IS NULL OR expires_on >= $3::date)`, schoolID, empID, today)
	return collect(rows, err, func(r *sqlRows) (string, error) {
		var id string
		return id, r.Scan(&id)
	})
}

// --- Document Requests ---

func ListDocumentRequests(ctx context.Context, q Q, schoolID, empID, status string, page, perPage int) ([]model.DocumentRequest, int, error) {
	where := " WHERE dr.school_id = $1"
	args := []any{schoolID}
	if empID != "" {
		args = append(args, empID)
		where += fmt.Sprintf(" AND dr.employee_id = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND dr.status = $%d", len(args))
	}

	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(*) FROM document_requests dr"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sqlText := `SELECT dr.id, dr.school_id, dr.employee_id, e.full_name, dr.document_type_id, dt.name,
		dr.note, dr.status, dr.created_at, dr.updated_at
		FROM document_requests dr
		JOIN employees e ON e.id = dr.employee_id
		JOIN document_types dt ON dt.id = dr.document_type_id` + where + ` ORDER BY dr.created_at DESC`

	if perPage > 0 {
		sqlText += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, (page-1)*perPage)
	}

	rows, err := q.QueryContext(ctx, sqlText, args...)
	out, err := collect(rows, err, func(r *sqlRows) (model.DocumentRequest, error) {
		var dr model.DocumentRequest
		err := r.Scan(&dr.ID, &dr.SchoolID, &dr.EmployeeID, &dr.EmployeeName, &dr.DocumentTypeID,
			&dr.DocumentTypeName, &dr.Note, &dr.Status, &dr.CreatedAt, &dr.UpdatedAt)
		return dr, err
	})
	return out, total, err
}

func CreateDocumentRequest(ctx context.Context, q Q, req *model.DocumentRequest) error {
	if req.Status == "" {
		req.Status = "open"
	}
	return q.QueryRowContext(ctx, `INSERT INTO document_requests (school_id, employee_id, document_type_id, note, status)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at, updated_at`,
		req.SchoolID, req.EmployeeID, req.DocumentTypeID, req.Note, req.Status).Scan(&req.ID, &req.CreatedAt, &req.UpdatedAt)
}

func UpdateDocumentRequestStatus(ctx context.Context, q Q, schoolID, id, status string) error {
	return affected(q.ExecContext(ctx, `UPDATE document_requests SET status = $3, updated_at = now() WHERE school_id = $1 AND id = $2`, schoolID, id, status))
}

func GetDocumentsCompletenessSummary(ctx context.Context, q Q, schoolID string) (any, error) {
	today := time.Now().Format("2006-01-02")
	// Count active employees and missing documents
	rows, err := q.QueryContext(ctx, `
		SELECT e.id, e.full_name, cur.employment_type,
			COUNT(DISTINCT rd.document_type_id) AS required_count,
			COUNT(DISTINCT ed.document_type_id) AS uploaded_count
		FROM employees e
		JOIN LATERAL (
			SELECT ea.employment_type FROM employment_assignments ea
			WHERE ea.employee_id = e.id AND `+activeOn("ea", "$2::date")+`
			ORDER BY ea.effective_from DESC LIMIT 1
		) cur ON true
		LEFT JOIN required_documents rd ON rd.school_id = e.school_id AND rd.employment_type = cur.employment_type
		LEFT JOIN employee_documents ed ON ed.school_id = e.school_id AND ed.employee_id = e.id
			AND ed.document_type_id = rd.document_type_id AND ed.superseded_at IS NULL
			AND (ed.expires_on IS NULL OR ed.expires_on >= $2::date)
		WHERE e.school_id = $1 AND e.status = 'active' AND e.deleted_at IS NULL
		GROUP BY e.id, e.full_name, cur.employment_type
	`, schoolID, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	totalActive := 0
	completeCount := 0
	incompleteCount := 0

	for rows.Next() {
		var (
			id, name, empType string
			reqCount, upCount int
		)
		if err := rows.Scan(&id, &name, &empType, &reqCount, &upCount); err != nil {
			return nil, err
		}
		totalActive++
		if reqCount > 0 && upCount >= reqCount {
			completeCount++
		} else {
			incompleteCount++
		}
	}

	return map[string]any{
		"total_employees": totalActive,
		"complete":        completeCount,
		"incomplete":      incompleteCount,
	}, rows.Err()
}
