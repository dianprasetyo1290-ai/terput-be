package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"skci/core/internal/model"
)

const auditGrantCols = `id, school_id, user_id, valid_from, valid_to, reason, granted_by, created_at, revoked_at, revoked_by`

func scanAuditGrant(row rowScanner) (model.AuditAccessGrant, error) {
	var g model.AuditAccessGrant
	err := row.Scan(&g.ID, &g.SchoolID, &g.UserID, &g.ValidFrom, &g.ValidTo, &g.Reason, &g.GrantedBy, &g.CreatedAt, &g.RevokedAt, &g.RevokedBy)
	return g, err
}

func (s *PGStore) CreateAuditGrant(ctx context.Context, g model.AuditAccessGrant) (*model.AuditAccessGrant, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO audit_access_grants (school_id, user_id, valid_from, valid_to, reason, granted_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+auditGrantCols, g.SchoolID, g.UserID, g.ValidFrom, g.ValidTo, g.Reason, g.GrantedBy)
	out, err := scanAuditGrant(row)
	if err != nil {
		return nil, fmt.Errorf("insert audit grant: %w", err)
	}
	return &out, nil
}

func (s *PGStore) ListAuditGrants(ctx context.Context, schoolID string) ([]model.AuditAccessGrant, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+auditGrantCols+`
		FROM audit_access_grants WHERE school_id = $1 ORDER BY created_at DESC LIMIT 500`, schoolID)
	if err != nil {
		return nil, fmt.Errorf("list audit grants: %w", err)
	}
	defer rows.Close()
	var res []model.AuditAccessGrant
	for rows.Next() {
		g, err := scanAuditGrant(rows)
		if err != nil {
			return nil, fmt.Errorf("scan audit grant: %w", err)
		}
		res = append(res, g)
	}
	return res, rows.Err()
}

func (s *PGStore) RevokeAuditGrant(ctx context.Context, schoolID, id, revokedBy string) (*model.AuditAccessGrant, error) {
	row := s.DB.QueryRowContext(ctx, `
		UPDATE audit_access_grants SET revoked_at = now(), revoked_by = $3
		WHERE id = $1 AND school_id = $2 AND revoked_at IS NULL
		RETURNING `+auditGrantCols, id, schoolID, revokedBy)
	g, err := scanAuditGrant(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("revoke audit grant: %w", err)
	}
	return &g, nil
}

func (s *PGStore) HasActiveAuditGrant(ctx context.Context, schoolID, userID string) (bool, error) {
	var ok bool
	err := s.DB.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM audit_access_grants
		WHERE school_id = $1 AND user_id = $2 AND revoked_at IS NULL
		  AND valid_from <= now() AND valid_to > now())`, schoolID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check audit grant: %w", err)
	}
	return ok, nil
}
