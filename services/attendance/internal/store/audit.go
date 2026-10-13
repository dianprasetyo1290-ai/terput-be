package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

// execer is satisfied by *sql.DB and *sql.Tx, so an audit row can be written
// in the same transaction as the change it records.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// AuditEntry is one audit_outbox row, forwarded to core POST /internal/audit
// by the handler's audit forwarder (BR-ATT-01: corrections after lock are
// recorded in the central audit trail; FR-42-002: leave decisions too).
type AuditEntry struct {
	ID            int64
	SchoolID      string
	Module        string
	EntityType    string
	EntityID      string
	Action        string
	ActorUserID   *string
	ActorRole     *string
	Before        any
	After         any
	Reason        *string
	CorrelationID *string
	BeforeRaw     json.RawMessage
	AfterRaw      json.RawMessage
}

func jsonOrNil(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// nilIfEmpty turns "" into a NULL-able pointer.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// writeAudit inserts one outbox row using q (DB or Tx).
func writeAudit(ctx context.Context, q execer, e AuditEntry) error {
	before, err := jsonOrNil(e.Before)
	if err != nil {
		return err
	}
	after, err := jsonOrNil(e.After)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO audit_outbox
		(school_id, module, entity_type, entity_id, action, actor_user_id, actor_role, before, after, reason, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.SchoolID, e.Module, e.EntityType, e.EntityID, e.Action, e.ActorUserID, e.ActorRole, before, after, e.Reason, e.CorrelationID)
	return err
}

// EnqueueAudit writes an outbox row outside any transaction.
func (s *Store) EnqueueAudit(ctx context.Context, e AuditEntry) error {
	return writeAudit(ctx, s.DB, e)
}

// PendingAudit returns the oldest rows not yet forwarded to core.
func (s *Store) PendingAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, school_id, module, entity_type, entity_id, action,
		actor_user_id::text, actor_role, before, after, reason, correlation_id
		FROM audit_outbox WHERE forwarded_at IS NULL ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var before, after []byte
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.Module, &e.EntityType, &e.EntityID, &e.Action,
			&e.ActorUserID, &e.ActorRole, &before, &after, &e.Reason, &e.CorrelationID); err != nil {
			return nil, err
		}
		e.BeforeRaw, e.AfterRaw = before, after
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkAuditForwarded stamps a row once core accepted it.
func (s *Store) MarkAuditForwarded(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE audit_outbox SET forwarded_at = now() WHERE id = $1`, id)
	return err
}

// MarkAuditAttempt counts a failed delivery (kept for retry).
func (s *Store) MarkAuditAttempt(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE audit_outbox SET attempts = attempts + 1 WHERE id = $1`, id)
	return err
}
