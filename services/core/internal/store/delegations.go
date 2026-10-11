package store

import (
	"context"
	"fmt"
	"time"

	"skci/core/internal/model"
)

func (s *PGStore) ListDelegations(ctx context.Context, schoolID, userID string) ([]model.ApprovalDelegation, error) {
	query := `
		SELECT id, school_id, from_user_id, to_user_id, valid_from, valid_to, created_at, revoked_at
		FROM approval_delegations
		WHERE school_id = $1 AND (from_user_id = $2 OR to_user_id = $2)
		ORDER BY created_at DESC`
	rows, err := s.DB.QueryContext(ctx, query, schoolID, userID)
	if err != nil {
		return nil, fmt.Errorf("list delegations: %w", err)
	}
	defer rows.Close()

	var res []model.ApprovalDelegation
	for rows.Next() {
		var d model.ApprovalDelegation
		if err := rows.Scan(&d.ID, &d.SchoolID, &d.FromUserID, &d.ToUserID, &d.ValidFrom, &d.ValidTo, &d.CreatedAt, &d.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan delegation: %w", err)
		}
		res = append(res, d)
	}
	return res, rows.Err()
}

func (s *PGStore) CreateDelegation(ctx context.Context, del model.ApprovalDelegation) (*model.ApprovalDelegation, error) {
	query := `
		INSERT INTO approval_delegations (school_id, from_user_id, to_user_id, valid_from, valid_to, created_at)
		VALUES ($1, $2, $3, $4, $5, now())
		RETURNING id, created_at`
	var id string
	var createdAt time.Time
	err := s.DB.QueryRowContext(ctx, query, del.SchoolID, del.FromUserID, del.ToUserID, del.ValidFrom, del.ValidTo).Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert delegation: %w", err)
	}

	del.ID = id
	del.CreatedAt = createdAt
	return &del, nil
}

func (s *PGStore) RevokeDelegation(ctx context.Context, schoolID, id, userID string) error {
	query := `
		UPDATE approval_delegations
		SET revoked_at = now()
		WHERE id = $1 AND school_id = $2 AND from_user_id = $3 AND revoked_at IS NULL`
	res, err := s.DB.ExecContext(ctx, query, id, schoolID, userID)
	if err != nil {
		return fmt.Errorf("revoke delegation: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) GetActiveDelegationsForUser(ctx context.Context, schoolID, userID string) ([]model.ActiveDelegation, error) {
	query := `
		SELECT from_user_id, to_user_id
		FROM approval_delegations
		WHERE school_id = $1 AND to_user_id = $2 AND revoked_at IS NULL
		  AND valid_from <= now() AND valid_to >= now()`
	rows, err := s.DB.QueryContext(ctx, query, schoolID, userID)
	if err != nil {
		return nil, fmt.Errorf("get active delegations: %w", err)
	}
	defer rows.Close()

	var res []model.ActiveDelegation
	for rows.Next() {
		var ad model.ActiveDelegation
		if err := rows.Scan(&ad.FromUserID, &ad.ToUserID); err != nil {
			return nil, fmt.Errorf("scan active delegation: %w", err)
		}
		res = append(res, ad)
	}
	return res, rows.Err()
}
