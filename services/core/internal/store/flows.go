package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"skci/core/internal/model"
)

func (s *PGStore) ListFlows(ctx context.Context, schoolID string) ([]model.ApprovalFlow, error) {
	query := `
		SELECT id, school_id, request_type, name, active, created_by, created_at, updated_at
		FROM approval_flows
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`
	rows, err := s.DB.QueryContext(ctx, query, schoolID)
	if err != nil {
		return nil, fmt.Errorf("list flows: %w", err)
	}
	defer rows.Close()

	var flows []model.ApprovalFlow
	for rows.Next() {
		var f model.ApprovalFlow
		if err := rows.Scan(&f.ID, &f.SchoolID, &f.RequestType, &f.Name, &f.Active, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan flow: %w", err)
		}
		flows = append(flows, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range flows {
		steps, err := s.getFlowSteps(ctx, flows[i].ID)
		if err != nil {
			return nil, err
		}
		flows[i].Steps = steps
	}

	return flows, nil
}

func (s *PGStore) GetFlow(ctx context.Context, schoolID, id string) (*model.ApprovalFlow, error) {
	query := `
		SELECT id, school_id, request_type, name, active, created_by, created_at, updated_at
		FROM approval_flows
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`
	var f model.ApprovalFlow
	err := s.DB.QueryRowContext(ctx, query, id, schoolID).Scan(
		&f.ID, &f.SchoolID, &f.RequestType, &f.Name, &f.Active, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get flow: %w", err)
	}

	steps, err := s.getFlowSteps(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	f.Steps = steps
	return &f, nil
}

func (s *PGStore) GetFlowByRequestType(ctx context.Context, schoolID, requestType string) (*model.ApprovalFlow, error) {
	query := `
		SELECT id, school_id, request_type, name, active, created_by, created_at, updated_at
		FROM approval_flows
		WHERE school_id = $1 AND request_type = $2 AND active = true AND deleted_at IS NULL`
	var f model.ApprovalFlow
	err := s.DB.QueryRowContext(ctx, query, schoolID, requestType).Scan(
		&f.ID, &f.SchoolID, &f.RequestType, &f.Name, &f.Active, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get flow by type: %w", err)
	}

	steps, err := s.getFlowSteps(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	f.Steps = steps
	return &f, nil
}

func (s *PGStore) getFlowSteps(ctx context.Context, flowID string) ([]model.FlowStep, error) {
	query := `
		SELECT id, flow_id, seq, approver_kind, approver_role, approver_user_id, min_amount_idr, condition_flag, sla_hours, escalate_to_role
		FROM approval_flow_steps
		WHERE flow_id = $1
		ORDER BY seq ASC`
	rows, err := s.DB.QueryContext(ctx, query, flowID)
	if err != nil {
		return nil, fmt.Errorf("get flow steps: %w", err)
	}
	defer rows.Close()

	var steps []model.FlowStep
	for rows.Next() {
		var st model.FlowStep
		if err := rows.Scan(
			&st.ID, &st.FlowID, &st.Seq, &st.ApproverKind, &st.ApproverRole, &st.ApproverUserID,
			&st.MinAmountIDR, &st.ConditionFlag, &st.SLAHours, &st.EscalateToRole,
		); err != nil {
			return nil, fmt.Errorf("scan flow step: %w", err)
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (s *PGStore) CreateFlow(ctx context.Context, schoolID, createdBy string, flow model.ApprovalFlow) (*model.ApprovalFlow, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO approval_flows (school_id, request_type, name, active, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), now())
		RETURNING id, created_at, updated_at`
	var id string
	var createdAt, updatedAt time.Time
	err = tx.QueryRowContext(ctx, query, schoolID, flow.RequestType, flow.Name, flow.Active, createdBy).Scan(&id, &createdAt, &updatedAt)
	if isUniqueViolation(err) {
		return nil, fmt.Errorf("%w: alur untuk jenis permintaan ini sudah ada", ErrConflict)
	} else if err != nil {
		return nil, fmt.Errorf("insert flow: %w", err)
	}

	for _, step := range flow.Steps {
		stepQuery := `
			INSERT INTO approval_flow_steps (
				school_id, flow_id, seq, approver_kind, approver_role, approver_user_id,
				min_amount_idr, condition_flag, sla_hours, escalate_to_role, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())`
		_, err := tx.ExecContext(ctx, stepQuery,
			schoolID, id, step.Seq, step.ApproverKind, step.ApproverRole, step.ApproverUserID,
			step.MinAmountIDR, step.ConditionFlag, step.SLAHours, step.EscalateToRole,
		)
		if err != nil {
			return nil, fmt.Errorf("insert flow step: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetFlow(ctx, schoolID, id)
}

func (s *PGStore) UpdateFlow(ctx context.Context, schoolID, id string, flow model.ApprovalFlow) (*model.ApprovalFlow, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	query := `
		UPDATE approval_flows
		SET name = $1, active = $2, updated_at = now()
		WHERE id = $3 AND school_id = $4 AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, query, flow.Name, flow.Active, id, schoolID)
	if err != nil {
		return nil, fmt.Errorf("update flow: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}

	// Replace steps
	if _, err := tx.ExecContext(ctx, `DELETE FROM approval_flow_steps WHERE flow_id = $1`, id); err != nil {
		return nil, fmt.Errorf("delete flow steps: %w", err)
	}

	for _, step := range flow.Steps {
		stepQuery := `
			INSERT INTO approval_flow_steps (
				school_id, flow_id, seq, approver_kind, approver_role, approver_user_id,
				min_amount_idr, condition_flag, sla_hours, escalate_to_role, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())`
		_, err := tx.ExecContext(ctx, stepQuery,
			schoolID, id, step.Seq, step.ApproverKind, step.ApproverRole, step.ApproverUserID,
			step.MinAmountIDR, step.ConditionFlag, step.SLAHours, step.EscalateToRole,
		)
		if err != nil {
			return nil, fmt.Errorf("insert updated flow step: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.GetFlow(ctx, schoolID, id)
}

func (s *PGStore) DeleteFlow(ctx context.Context, schoolID, id string) error {
	query := `
		UPDATE approval_flows
		SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`
	res, err := s.DB.ExecContext(ctx, query, id, schoolID)
	if err != nil {
		return fmt.Errorf("delete flow: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
