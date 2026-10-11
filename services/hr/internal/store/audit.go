package store

import (
	"context"
	"encoding/json"
)

// AuditEntry is one outbox row. Before/After must already be free of
// sensitive plaintext: use {"changed": [...]} for NIK/NPWP/rekening/gaji.
type AuditEntry struct {
	ID            int64
	SchoolID      string
	Module        string
	EntityType    string
	EntityID      string
	Action        string
	ActorUserID   *string
	ActorRole     *string
	Before        map[string]any
	After         map[string]any
	Reason        *string
	CorrelationID *string
	BeforeRaw     json.RawMessage
	AfterRaw      json.RawMessage
}

func jsonOrNil(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Audit writes an outbox row; pass the transaction that made the change.
func Audit(ctx context.Context, q Q, e AuditEntry) error {
	before, err := jsonOrNil(e.Before)
	if err != nil {
		return err
	}
	after, err := jsonOrNil(e.After)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO audit_events
		(school_id, module, entity_type, entity_id, action, actor_user_id, actor_role, before, after, reason, correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.SchoolID, e.Module, e.EntityType, e.EntityID, e.Action, e.ActorUserID, e.ActorRole, before, after, e.Reason, e.CorrelationID)
	return err
}

// PendingAudit returns the oldest rows not yet forwarded to core.
func PendingAudit(ctx context.Context, q Q, limit int) ([]AuditEntry, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, school_id, module, entity_type, entity_id, action,
		actor_user_id::text, actor_role, before, after, reason, correlation_id
		FROM audit_events WHERE forwarded_at IS NULL ORDER BY id LIMIT $1`, limit)
	return collect(rows, err, func(r *sqlRows) (AuditEntry, error) {
		var e AuditEntry
		err := r.Scan(&e.ID, &e.SchoolID, &e.Module, &e.EntityType, &e.EntityID, &e.Action,
			&e.ActorUserID, &e.ActorRole, &e.BeforeRaw, &e.AfterRaw, &e.Reason, &e.CorrelationID)
		return e, err
	})
}

// MarkAuditForwarded stamps a row once core accepted it.
func MarkAuditForwarded(ctx context.Context, q Q, id int64) error {
	_, err := q.ExecContext(ctx, `UPDATE audit_events SET forwarded_at = now() WHERE id = $1`, id)
	return err
}

// WriteAuditEvent writes an audit event row to the outbox table.
func WriteAuditEvent(ctx context.Context, q Q, schoolID string, actorUserID *string, entityType, entityID, action string, before, after []byte, reason string, correlationID *string) error {
	var rPtr *string
	if reason != "" {
		rPtr = &reason
	}
	_, err := q.ExecContext(ctx, `INSERT INTO audit_events
		(school_id, module, entity_type, entity_id, action, actor_user_id, before, after, reason, correlation_id)
		VALUES ($1, 'hr', $2, $3, $4, $5, $6, $7, $8, $9)`,
		schoolID, entityType, entityID, action, actorUserID, before, after, rPtr, correlationID)
	return err
}
