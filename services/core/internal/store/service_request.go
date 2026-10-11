package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"skci/core/internal/model"
)

func (s *PGStore) ListServiceRequestCategories(ctx context.Context, schoolID string, activeOnly bool) ([]model.ServiceRequestCategory, error) {
	query := `
		SELECT id, school_id, name, coalesce(description, ''), handler_role, sla_hours, requires_student, active, deleted_at, created_at, updated_at
		FROM service_request_categories
		WHERE school_id = $1 AND deleted_at IS NULL`
	if activeOnly {
		query += ` AND active = true`
	}
	query += ` ORDER BY name ASC`

	rows, err := s.DB.QueryContext(ctx, query, schoolID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var list []model.ServiceRequestCategory
	for rows.Next() {
		var c model.ServiceRequestCategory
		if err := rows.Scan(
			&c.ID, &c.SchoolID, &c.Name, &c.Description, &c.HandlerRole, &c.SLAHours,
			&c.RequiresStudent, &c.Active, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *PGStore) GetServiceRequestCategory(ctx context.Context, schoolID, id string) (*model.ServiceRequestCategory, error) {
	query := `
		SELECT id, school_id, name, coalesce(description, ''), handler_role, sla_hours, requires_student, active, deleted_at, created_at, updated_at
		FROM service_request_categories
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`

	var c model.ServiceRequestCategory
	err := s.DB.QueryRowContext(ctx, query, schoolID, id).Scan(
		&c.ID, &c.SchoolID, &c.Name, &c.Description, &c.HandlerRole, &c.SLAHours,
		&c.RequiresStudent, &c.Active, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get category: %w", err)
	}
	return &c, nil
}

func (s *PGStore) CreateServiceRequestCategory(ctx context.Context, cat model.ServiceRequestCategory) (*model.ServiceRequestCategory, error) {
	query := `
		INSERT INTO service_request_categories (
			school_id, name, description, handler_role, sla_hours, requires_student, active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now())
		RETURNING id, school_id, name, coalesce(description, ''), handler_role, sla_hours, requires_student, active, deleted_at, created_at, updated_at`

	var c model.ServiceRequestCategory
	err := s.DB.QueryRowContext(
		ctx, query,
		cat.SchoolID, cat.Name, cat.Description, cat.HandlerRole, cat.SLAHours, cat.RequiresStudent, cat.Active,
	).Scan(
		&c.ID, &c.SchoolID, &c.Name, &c.Description, &c.HandlerRole, &c.SLAHours,
		&c.RequiresStudent, &c.Active, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	return &c, nil
}

func (s *PGStore) UpdateServiceRequestCategory(ctx context.Context, schoolID, id string, cat model.ServiceRequestCategory) (*model.ServiceRequestCategory, error) {
	query := `
		UPDATE service_request_categories
		SET name = $1, description = $2, handler_role = $3, sla_hours = $4, requires_student = $5, active = $6, updated_at = now()
		WHERE school_id = $7 AND id = $8 AND deleted_at IS NULL
		RETURNING id, school_id, name, coalesce(description, ''), handler_role, sla_hours, requires_student, active, deleted_at, created_at, updated_at`

	var c model.ServiceRequestCategory
	err := s.DB.QueryRowContext(
		ctx, query,
		cat.Name, cat.Description, cat.HandlerRole, cat.SLAHours, cat.RequiresStudent, cat.Active,
		schoolID, id,
	).Scan(
		&c.ID, &c.SchoolID, &c.Name, &c.Description, &c.HandlerRole, &c.SLAHours,
		&c.RequiresStudent, &c.Active, &c.DeletedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("update category: %w", err)
	}
	return &c, nil
}

func (s *PGStore) DeleteServiceRequestCategory(ctx context.Context, schoolID, id string) error {
	query := `
		UPDATE service_request_categories
		SET deleted_at = now(), updated_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`

	res, err := s.DB.ExecContext(ctx, query, schoolID, id)
	if err != nil {
		return fmt.Errorf("delete category: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) CreateServiceRequest(ctx context.Context, req model.ServiceRequest) (*model.ServiceRequest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if len(req.Attachments) == 0 {
		req.Attachments = json.RawMessage("[]")
	}
	if len(req.ResultAttachments) == 0 {
		req.ResultAttachments = json.RawMessage("[]")
	}

	query := `
		INSERT INTO service_requests (
			school_id, category_id, requester_user_id, student_user_id,
			subject, description, attachments, status, assignee_user_id, result_attachments,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
		RETURNING id, school_id, category_id, requester_user_id, student_user_id,
		          subject, description, attachments, status, assignee_user_id, result_attachments,
		          created_at, updated_at`

	var out model.ServiceRequest
	err = tx.QueryRowContext(
		ctx, query,
		req.SchoolID, req.CategoryID, req.RequesterUserID, req.StudentUserID,
		req.Subject, req.Description, req.Attachments, "submitted", req.AssigneeUserID, req.ResultAttachments,
	).Scan(
		&out.ID, &out.SchoolID, &out.CategoryID, &out.RequesterUserID, &out.StudentUserID,
		&out.Subject, &out.Description, &out.Attachments, &out.Status, &out.AssigneeUserID, &out.ResultAttachments,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Insert initial event
	eventQuery := `
		INSERT INTO service_request_events (
			school_id, request_id, from_status, to_status, note, actor_user_id, created_at
		) VALUES ($1, $2, NULL, 'submitted', 'Permintaan diajukan oleh orang tua', $3, now())`
	if _, err := tx.ExecContext(ctx, eventQuery, out.SchoolID, out.ID, out.RequesterUserID); err != nil {
		return nil, fmt.Errorf("insert initial event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *PGStore) GetServiceRequest(ctx context.Context, schoolID, id string) (*model.ServiceRequest, error) {
	query := `
		SELECT r.id, r.school_id, r.category_id, c.name, c.handler_role,
		       r.requester_user_id, r.student_user_id,
		       r.subject, r.description, r.attachments, r.status, r.assignee_user_id, r.result_attachments,
		       r.created_at, r.updated_at
		FROM service_requests r
		JOIN service_request_categories c ON c.id = r.category_id
		WHERE r.school_id = $1 AND r.id = $2`

	var r model.ServiceRequest
	err := s.DB.QueryRowContext(ctx, query, schoolID, id).Scan(
		&r.ID, &r.SchoolID, &r.CategoryID, &r.CategoryName, &r.HandlerRole,
		&r.RequesterUserID, &r.StudentUserID,
		&r.Subject, &r.Description, &r.Attachments, &r.Status, &r.AssigneeUserID, &r.ResultAttachments,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	return &r, nil
}

func (s *PGStore) ListParentServiceRequests(ctx context.Context, schoolID, requesterUserID string) ([]model.ServiceRequest, error) {
	query := `
		SELECT r.id, r.school_id, r.category_id, c.name, c.handler_role,
		       r.requester_user_id, r.student_user_id,
		       r.subject, r.description, r.attachments, r.status, r.assignee_user_id, r.result_attachments,
		       r.created_at, r.updated_at
		FROM service_requests r
		JOIN service_request_categories c ON c.id = r.category_id
		WHERE r.school_id = $1 AND r.requester_user_id = $2
		ORDER BY r.created_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, schoolID, requesterUserID)
	if err != nil {
		return nil, fmt.Errorf("list parent requests: %w", err)
	}
	defer rows.Close()

	var list []model.ServiceRequest
	for rows.Next() {
		var r model.ServiceRequest
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.CategoryID, &r.CategoryName, &r.HandlerRole,
			&r.RequesterUserID, &r.StudentUserID,
			&r.Subject, &r.Description, &r.Attachments, &r.Status, &r.AssigneeUserID, &r.ResultAttachments,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan parent request: %w", err)
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *PGStore) ListHandlerServiceRequests(ctx context.Context, schoolID string, roles []string, status string) ([]model.ServiceRequest, error) {
	if len(roles) == 0 {
		return []model.ServiceRequest{}, nil
	}

	query := `
		SELECT r.id, r.school_id, r.category_id, c.name, c.handler_role,
		       r.requester_user_id, r.student_user_id,
		       r.subject, r.description, r.attachments, r.status, r.assignee_user_id, r.result_attachments,
		       r.created_at, r.updated_at
		FROM service_requests r
		JOIN service_request_categories c ON c.id = r.category_id
		WHERE r.school_id = $1`

	args := []interface{}{schoolID}
	argIdx := 2

	// Role filter: category handler_role must be in user's roles
	query += fmt.Sprintf(" AND c.handler_role = ANY($%d)", argIdx)
	args = append(args, roles)
	argIdx++

	if status != "" {
		query += fmt.Sprintf(" AND r.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	query += ` ORDER BY r.created_at DESC`

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list handler requests: %w", err)
	}
	defer rows.Close()

	var list []model.ServiceRequest
	for rows.Next() {
		var r model.ServiceRequest
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.CategoryID, &r.CategoryName, &r.HandlerRole,
			&r.RequesterUserID, &r.StudentUserID,
			&r.Subject, &r.Description, &r.Attachments, &r.Status, &r.AssigneeUserID, &r.ResultAttachments,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan handler request: %w", err)
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *PGStore) UpdateServiceRequestStatus(ctx context.Context, schoolID, id, newStatus, note, actorUserID string) (*model.ServiceRequest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Lock request row
	queryGet := `
		SELECT status
		FROM service_requests
		WHERE school_id = $1 AND id = $2
		FOR UPDATE`

	var currentStatus string
	err = tx.QueryRowContext(ctx, queryGet, schoolID, id).Scan(&currentStatus)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}

	if err := model.ValidateStatusTransition(currentStatus, newStatus, note); err != nil {
		return nil, err
	}

	queryUpdate := `
		UPDATE service_requests
		SET status = $1, assignee_user_id = coalesce(assignee_user_id, $2), updated_at = now()
		WHERE school_id = $3 AND id = $4
		RETURNING id, school_id, category_id, requester_user_id, student_user_id,
		          subject, description, attachments, status, assignee_user_id, result_attachments,
		          created_at, updated_at`

	var out model.ServiceRequest
	err = tx.QueryRowContext(ctx, queryUpdate, newStatus, actorUserID, schoolID, id).Scan(
		&out.ID, &out.SchoolID, &out.CategoryID, &out.RequesterUserID, &out.StudentUserID,
		&out.Subject, &out.Description, &out.Attachments, &out.Status, &out.AssigneeUserID, &out.ResultAttachments,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	queryEvent := `
		INSERT INTO service_request_events (
			school_id, request_id, from_status, to_status, note, actor_user_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, now())`

	if _, err := tx.ExecContext(ctx, queryEvent, schoolID, id, currentStatus, newStatus, note, actorUserID); err != nil {
		return nil, fmt.Errorf("insert status event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *PGStore) UpdateServiceRequestResult(ctx context.Context, schoolID, id string, resultAttachments []byte) (*model.ServiceRequest, error) {
	if len(resultAttachments) == 0 {
		resultAttachments = []byte("[]")
	}

	query := `
		UPDATE service_requests
		SET result_attachments = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3
		RETURNING id, school_id, category_id, requester_user_id, student_user_id,
		          subject, description, attachments, status, assignee_user_id, result_attachments,
		          created_at, updated_at`

	var out model.ServiceRequest
	err := s.DB.QueryRowContext(ctx, query, resultAttachments, schoolID, id).Scan(
		&out.ID, &out.SchoolID, &out.CategoryID, &out.RequesterUserID, &out.StudentUserID,
		&out.Subject, &out.Description, &out.Attachments, &out.Status, &out.AssigneeUserID, &out.ResultAttachments,
		&out.CreatedAt, &out.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("update result: %w", err)
	}
	return &out, nil
}

func (s *PGStore) ListServiceRequestEvents(ctx context.Context, schoolID, requestID string) ([]model.ServiceRequestEvent, error) {
	query := `
		SELECT id, school_id, request_id, from_status, to_status, coalesce(note, ''), actor_user_id, created_at
		FROM service_request_events
		WHERE school_id = $1 AND request_id = $2
		ORDER BY created_at ASC`

	rows, err := s.DB.QueryContext(ctx, query, schoolID, requestID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var list []model.ServiceRequestEvent
	for rows.Next() {
		var ev model.ServiceRequestEvent
		if err := rows.Scan(
			&ev.ID, &ev.SchoolID, &ev.RequestID, &ev.FromStatus, &ev.ToStatus,
			&ev.Note, &ev.ActorUserID, &ev.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		list = append(list, ev)
	}
	return list, rows.Err()
}

func (s *PGStore) CanUserHandleServiceRequests(ctx context.Context, schoolID string, roles []string) ([]string, error) {
	if len(roles) == 0 {
		return []string{}, nil
	}

	query := `
		SELECT DISTINCT handler_role
		FROM service_request_categories
		WHERE school_id = $1 AND deleted_at IS NULL AND active = true AND handler_role = ANY($2)`

	rows, err := s.DB.QueryContext(ctx, query, schoolID, roles)
	if err != nil {
		return nil, fmt.Errorf("check handler roles: %w", err)
	}
	defer rows.Close()

	var handledRoles []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		handledRoles = append(handledRoles, r)
	}
	return handledRoles, rows.Err()
}
