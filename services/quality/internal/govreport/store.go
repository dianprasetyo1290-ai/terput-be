package govreport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) RecordAudit(ctx context.Context, schoolID, entityType, entityID, action, actorID, actorRole string, before, after interface{}, reason string) error {
	var bJSON, aJSON []byte
	if before != nil {
		bJSON, _ = json.Marshal(before)
	}
	if after != nil {
		aJSON, _ = json.Marshal(after)
	}

	query := `
		INSERT INTO audit_events (school_id, module, entity_type, entity_id, action, actor_user_id, actor_role, before, after, reason)
		VALUES ($1, 'gov_reporting', $2, $3, $4, NULLIF($5, '')::uuid, $6, $7, $8, $9)
	`
	_, err := s.pool.Exec(ctx, query, schoolID, entityType, entityID, action, actorID, actorRole, bJSON, aJSON, reason)
	return err
}

// ------------------- Dapodik Mappings -------------------

func (s *Store) ListMappings(ctx context.Context, schoolID, entity string) ([]FieldMapping, error) {
	query := `
		SELECT id, school_id, entity, dapodik_column, sekoci_field, is_match_key, match_priority, normalize, created_at, updated_at
		FROM dapodik_field_mappings
		WHERE school_id = $1 AND ($2 = '' OR entity = $2)
		ORDER BY entity, match_priority ASC NULLS LAST, dapodik_column ASC
	`
	rows, err := s.pool.Query(ctx, query, schoolID, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []FieldMapping
	for rows.Next() {
		var m FieldMapping
		var priority sql.NullInt32
		var cAt, uAt time.Time
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.Entity, &m.DapodikColumn, &m.SekociField, &m.IsMatchKey, &priority, &m.Normalize, &cAt, &uAt); err == nil {
			if priority.Valid {
				m.MatchPriority = int(priority.Int32)
			}
			list = append(list, m)
		}
	}
	if list == nil {
		list = []FieldMapping{}
	}
	return list, nil
}

func (s *Store) UpsertMapping(ctx context.Context, m FieldMapping) (*FieldMapping, error) {
	query := `
		INSERT INTO dapodik_field_mappings (school_id, entity, dapodik_column, sekoci_field, is_match_key, match_priority, normalize)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (school_id, entity, dapodik_column)
		DO UPDATE SET sekoci_field = EXCLUDED.sekoci_field, is_match_key = EXCLUDED.is_match_key,
		              match_priority = EXCLUDED.match_priority, normalize = EXCLUDED.normalize, updated_at = now()
		RETURNING id
	`
	err := s.pool.QueryRow(ctx, query, m.SchoolID, m.Entity, m.DapodikColumn, m.SekociField, m.IsMatchKey, m.MatchPriority, m.Normalize).Scan(&m.ID)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) DeleteMapping(ctx context.Context, schoolID, id string) error {
	query := `DELETE FROM dapodik_field_mappings WHERE id = $1 AND school_id = $2`
	_, err := s.pool.Exec(ctx, query, id, schoolID)
	return err
}

// ------------------- Dapodik Imports & Diffs -------------------

func (s *Store) CreateImport(ctx context.Context, imp DapodikImport) (*DapodikImport, error) {
	query := `
		INSERT INTO dapodik_imports (school_id, entity, file_name, file_checksum, row_count, imported_by, status)
		VALUES ($1, $2, $3, $4, $5, $6::uuid, $7)
		RETURNING id, created_at, updated_at
	`
	err := s.pool.QueryRow(ctx, query, imp.SchoolID, imp.Entity, imp.FileName, imp.FileChecksum, imp.RowCount, imp.ImportedBy, imp.Status).Scan(
		&imp.ID, &imp.CreatedAt, &imp.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &imp, nil
}

func (s *Store) InsertImportRows(ctx context.Context, importID, schoolID string, rows []Record) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for i, r := range rows {
		dataBytes, _ := json.Marshal(r)
		_, err := tx.Exec(ctx, `
			INSERT INTO dapodik_import_rows (school_id, import_id, row_no, data)
			VALUES ($1, $2, $3, $4)
		`, schoolID, importID, i+1, dataBytes)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) InsertDiffs(ctx context.Context, schoolID, importID, entity string, diffs []DiffItem) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, d := range diffs {
		_, err := tx.Exec(ctx, `
			INSERT INTO dapodik_diffs (school_id, import_id, entity, sekoci_ref, kind, field, sekoci_value, dapodik_value, status)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, 'open')
		`, schoolID, importID, entity, d.SekociRef, d.Kind, d.Field, d.SekociValue, d.DapodikValue)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListImports(ctx context.Context, schoolID, entity string) ([]DapodikImport, error) {
	query := `
		SELECT id, school_id, entity, file_name, file_checksum, row_count, imported_by::text, status, created_at, updated_at
		FROM dapodik_imports
		WHERE school_id = $1 AND ($2 = '' OR entity = $2)
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, schoolID, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []DapodikImport
	for rows.Next() {
		var imp DapodikImport
		if err := rows.Scan(&imp.ID, &imp.SchoolID, &imp.Entity, &imp.FileName, &imp.FileChecksum, &imp.RowCount, &imp.ImportedBy, &imp.Status, &imp.CreatedAt, &imp.UpdatedAt); err == nil {
			list = append(list, imp)
		}
	}
	if list == nil {
		list = []DapodikImport{}
	}
	return list, nil
}

func (s *Store) ListDiffs(ctx context.Context, schoolID, importID, kind, status string) ([]DapodikDiff, error) {
	query := `
		SELECT id, school_id, import_id, entity, sekoci_ref, import_row_id::text, kind, field, sekoci_value, dapodik_value,
		       status, decision, decision_reason, decided_by::text, decided_at, created_at, updated_at
		FROM dapodik_diffs
		WHERE school_id = $1 AND import_id = $2
		  AND ($3 = '' OR kind = $3)
		  AND ($4 = '' OR status = $4)
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, schoolID, importID, kind, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []DapodikDiff
	for rows.Next() {
		var d DapodikDiff
		var rID, dBy sql.NullString
		if err := rows.Scan(&d.ID, &d.SchoolID, &d.ImportID, &d.Entity, &d.SekociRef, &rID, &d.Kind, &d.Field,
			&d.SekociValue, &d.DapodikValue, &d.Status, &d.Decision, &d.DecisionReason, &dBy, &d.DecidedAt, &d.CreatedAt, &d.UpdatedAt); err == nil {
			if rID.Valid {
				d.ImportRowID = &rID.String
			}
			if dBy.Valid {
				d.DecidedBy = &dBy.String
			}
			list = append(list, d)
		}
	}
	if list == nil {
		list = []DapodikDiff{}
	}
	return list, nil
}

func (s *Store) DecideDiff(ctx context.Context, schoolID, id, decision, reason, decidedBy string) error {
	query := `
		UPDATE dapodik_diffs
		SET status = 'decided', decision = $3, decision_reason = $4, decided_by = NULLIF($5, '')::uuid, decided_at = now(), updated_at = now()
		WHERE id = $1 AND school_id = $2
	`
	res, err := s.pool.Exec(ctx, query, id, schoolID, decision, reason, decidedBy)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return errors.New("diff not found")
	}
	return nil
}

func (s *Store) LogDapodikSync(ctx context.Context, schoolID, entity, action, actorID, importID string, summary map[string]interface{}) error {
	sumBytes, _ := json.Marshal(summary)
	query := `
		INSERT INTO dapodik_sync_logs (school_id, entity, action, actor_user_id, import_id, summary)
		VALUES ($1, $2, $3, $4::uuid, NULLIF($5, '')::uuid, $6)
	`
	_, err := s.pool.Exec(ctx, query, schoolID, entity, action, actorID, importID, sumBytes)
	return err
}

func (s *Store) ListSyncLogs(ctx context.Context, schoolID string) ([]map[string]interface{}, error) {
	query := `
		SELECT id, school_id, entity, action, actor_user_id::text, import_id::text, summary, created_at
		FROM dapodik_sync_logs
		WHERE school_id = $1
		ORDER BY created_at DESC LIMIT 100
	`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []map[string]interface{}
	for rows.Next() {
		var id, sID, entity, action, actorID, impID string
		var sumBytes []byte
		var cAt time.Time
		if err := rows.Scan(&id, &sID, &entity, &action, &actorID, &impID, &sumBytes, &cAt); err == nil {
			var summary map[string]interface{}
			_ = json.Unmarshal(sumBytes, &summary)
			list = append(list, map[string]interface{}{
				"id":            id,
				"school_id":     sID,
				"entity":        entity,
				"action":        action,
				"actor_user_id": actorID,
				"import_id":     impID,
				"summary":       summary,
				"created_at":    cAt,
			})
		}
	}
	if list == nil {
		list = []map[string]interface{}{}
	}
	return list, nil
}

// ------------------- Regulatory Templates & Reports -------------------

func (s *Store) ListTemplates(ctx context.Context, schoolID string) ([]RegulatoryTemplate, error) {
	query := `
		SELECT id, school_id, code, name, recipient, frequency, columns, export_format, status, created_at, updated_at
		FROM regulatory_templates
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RegulatoryTemplate
	for rows.Next() {
		var tpl RegulatoryTemplate
		var colBytes []byte
		if err := rows.Scan(&tpl.ID, &tpl.SchoolID, &tpl.Code, &tpl.Name, &tpl.Recipient, &tpl.Frequency, &colBytes, &tpl.ExportFormat, &tpl.Status, &tpl.CreatedAt, &tpl.UpdatedAt); err == nil {
			_ = json.Unmarshal(colBytes, &tpl.Columns)
			list = append(list, tpl)
		}
	}
	if list == nil {
		list = []RegulatoryTemplate{}
	}
	return list, nil
}

func (s *Store) GetTemplate(ctx context.Context, schoolID, id string) (*RegulatoryTemplate, error) {
	query := `
		SELECT id, school_id, code, name, recipient, frequency, columns, export_format, status, created_at, updated_at
		FROM regulatory_templates
		WHERE id = $1 AND school_id::text = $2 AND deleted_at IS NULL
	`
	var tpl RegulatoryTemplate
	var colBytes []byte
	err := s.pool.QueryRow(ctx, query, id, schoolID).Scan(
		&tpl.ID, &tpl.SchoolID, &tpl.Code, &tpl.Name, &tpl.Recipient, &tpl.Frequency, &colBytes, &tpl.ExportFormat, &tpl.Status, &tpl.CreatedAt, &tpl.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(colBytes, &tpl.Columns)
	return &tpl, nil
}

func (s *Store) CreateTemplate(ctx context.Context, tpl RegulatoryTemplate) (*RegulatoryTemplate, error) {
	colBytes, _ := json.Marshal(tpl.Columns)
	query := `
		INSERT INTO regulatory_templates (school_id, code, name, recipient, frequency, columns, export_format, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`
	err := s.pool.QueryRow(ctx, query, tpl.SchoolID, tpl.Code, tpl.Name, tpl.Recipient, tpl.Frequency, colBytes, tpl.ExportFormat, tpl.Status).Scan(
		&tpl.ID, &tpl.CreatedAt, &tpl.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (s *Store) ListReports(ctx context.Context, schoolID, status string) ([]RegulatoryReport, error) {
	query := `
		SELECT id, school_id, template_id, period_from::text, period_to::text, status, approval_id::text,
		       snapshot_at, submitted_at, submitted_by::text, submission_note, export_object_key, idempotency_key, created_at, updated_at
		FROM regulatory_reports
		WHERE school_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
	`
	rows, err := s.pool.Query(ctx, query, schoolID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RegulatoryReport
	for rows.Next() {
		var rep RegulatoryReport
		var appID, subBy, expKey sql.NullString
		if err := rows.Scan(&rep.ID, &rep.SchoolID, &rep.TemplateID, &rep.PeriodFrom, &rep.PeriodTo, &rep.Status, &appID,
			&rep.SnapshotAt, &rep.SubmittedAt, &subBy, &rep.SubmissionNote, &expKey, &rep.IdempotencyKey, &rep.CreatedAt, &rep.UpdatedAt); err == nil {
			if appID.Valid {
				rep.ApprovalID = &appID.String
			}
			if subBy.Valid {
				rep.SubmittedBy = &subBy.String
			}
			if expKey.Valid {
				rep.ExportObjectKey = &expKey.String
			}
			list = append(list, rep)
		}
	}
	if list == nil {
		list = []RegulatoryReport{}
	}
	return list, nil
}

func (s *Store) GetReport(ctx context.Context, schoolID, id string) (*RegulatoryReport, error) {
	query := `
		SELECT id, school_id, template_id, period_from::text, period_to::text, status, approval_id::text,
		       template_snapshot, data_snapshot, snapshot_at, submitted_at, submitted_by::text, submission_note,
		       export_object_key, idempotency_key, created_at, updated_at
		FROM regulatory_reports
		WHERE id = $1 AND school_id::text = $2
	`
	var rep RegulatoryReport
	var appID, subBy, expKey sql.NullString
	var tSnapBytes, dSnapBytes []byte
	err := s.pool.QueryRow(ctx, query, id, schoolID).Scan(
		&rep.ID, &rep.SchoolID, &rep.TemplateID, &rep.PeriodFrom, &rep.PeriodTo, &rep.Status, &appID,
		&tSnapBytes, &dSnapBytes, &rep.SnapshotAt, &rep.SubmittedAt, &subBy, &rep.SubmissionNote,
		&expKey, &rep.IdempotencyKey, &rep.CreatedAt, &rep.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if appID.Valid {
		rep.ApprovalID = &appID.String
	}
	if subBy.Valid {
		rep.SubmittedBy = &subBy.String
	}
	if expKey.Valid {
		rep.ExportObjectKey = &expKey.String
	}
	if len(tSnapBytes) > 0 {
		var tSnap RegulatoryTemplate
		_ = json.Unmarshal(tSnapBytes, &tSnap)
		rep.TemplateSnapshot = &tSnap
	}
	if len(dSnapBytes) > 0 {
		var dSnap RegulatoryDataSnapshot
		_ = json.Unmarshal(dSnapBytes, &dSnap)
		rep.DataSnapshot = &dSnap
	}
	return &rep, nil
}

func (s *Store) CreateReport(ctx context.Context, rep RegulatoryReport) (*RegulatoryReport, error) {
	query := `
		INSERT INTO regulatory_reports (school_id, template_id, period_from, period_to, status, idempotency_key)
		VALUES ($1, $2, $3::date, $4::date, 'draft', $5)
		RETURNING id, created_at, updated_at
	`
	err := s.pool.QueryRow(ctx, query, rep.SchoolID, rep.TemplateID, rep.PeriodFrom, rep.PeriodTo, rep.IdempotencyKey).Scan(
		&rep.ID, &rep.CreatedAt, &rep.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	rep.Status = "draft"
	return &rep, nil
}

func (s *Store) SubmitReportForApproval(ctx context.Context, id, schoolID, approvalID string, tSnap RegulatoryTemplate, dSnap RegulatoryDataSnapshot) error {
	tBytes, _ := json.Marshal(tSnap)
	dBytes, _ := json.Marshal(dSnap)
	query := `
		UPDATE regulatory_reports
		SET status = 'pending_approval', approval_id = $3::uuid, template_snapshot = $4, data_snapshot = $5,
		    snapshot_at = now(), updated_at = now()
		WHERE id = $1 AND school_id = $2
	`
	_, err := s.pool.Exec(ctx, query, id, schoolID, approvalID, tBytes, dBytes)
	return err
}

func (s *Store) MarkReportSubmitted(ctx context.Context, id, schoolID, userID, note, exportObjectKey string) error {
	query := `
		UPDATE regulatory_reports
		SET status = 'submitted', submitted_at = now(), submitted_by = $3::uuid, submission_note = $4,
		    export_object_key = NULLIF($5, ''), updated_at = now()
		WHERE id = $1 AND school_id = $2
	`
	_, err := s.pool.Exec(ctx, query, id, schoolID, userID, note, exportObjectKey)
	return err
}
