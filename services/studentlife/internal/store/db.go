// Package store holds studentlife's shared SQL plumbing (connection, the Q
// seam, audit outbox, per-school settings). Domain SQL lives in each domain
// package (health, consent, sensitive).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// ErrNotFound is returned when a school-scoped row does not exist.
var ErrNotFound = errors.New("not found")

// Q is satisfied by *sql.DB and *sql.Tx.
type Q interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// One maps sql.ErrNoRows to ErrNotFound.
func One(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Collect scans every row with scan.
func Collect[T any](rows *sql.Rows, err error, scan func(*sql.Rows) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AuditEntry is one outbox row forwarded to core POST /internal/audit.
// Before/After must never carry health plaintext: use {"changed": [...]}.
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
	return Collect(rows, err, func(r *sql.Rows) (AuditEntry, error) {
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

// SchoolLocation returns the school's configured zone (studentlife_settings),
// falling back to the service default when the school has no row.
func SchoolLocation(ctx context.Context, q Q, schoolID, fallback string) *time.Location {
	tz := fallback
	var v string
	if err := q.QueryRowContext(ctx, `SELECT timezone FROM studentlife_settings WHERE school_id = $1`, schoolID).Scan(&v); err == nil && v != "" {
		tz = v
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc, _ = time.LoadLocation(fallback)
	}
	return loc
}
