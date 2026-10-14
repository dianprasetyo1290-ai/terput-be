package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"skci/core/internal/model"
)

func (s *PGStore) InsertAudit(ctx context.Context, ev model.AuditEvent) error {
	var beforeJSON, afterJSON []byte
	var err error
	if ev.BeforeData != nil {
		beforeJSON, err = json.Marshal(ev.BeforeData)
		if err != nil {
			return fmt.Errorf("marshal before data: %w", err)
		}
	}
	if ev.AfterData != nil {
		afterJSON, err = json.Marshal(ev.AfterData)
		if err != nil {
			return fmt.Errorf("marshal after data: %w", err)
		}
	}

	query := `
		INSERT INTO audit_events (
			school_id, service, module, entity_type, entity_id, action,
			actor_user_id, actor_role, before_data, after_data, reason, correlation_id, ip_hash, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now()
		)`
	_, err = s.DB.ExecContext(ctx, query,
		ev.SchoolID, ev.Service, ev.Module, ev.EntityType, ev.EntityID, ev.Action,
		ev.ActorUserID, ev.ActorRole, beforeJSON, afterJSON, ev.Reason, ev.CorrelationID, ev.IPHash,
	)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

// AuditFilter: SchoolID nil = every tenant incl. global (superadmin only);
// non-nil = exactly that school (no global rows leak to a school auditor).
// To is exclusive.
type AuditFilter struct {
	SchoolID      *string
	UserID        string
	Module        string
	Service       string
	Actions       []string
	EntityID      string
	CorrelationID string
	From          *time.Time
	To            *time.Time
}

func (s *PGStore) ListAudit(ctx context.Context, f AuditFilter, page, perPage int) ([]model.AuditEvent, int, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.SchoolID != nil {
		add("school_id = $%d", *f.SchoolID)
	}
	if f.UserID != "" {
		add("actor_user_id = $%d", f.UserID)
	}
	if f.Module != "" {
		add("module = $%d", f.Module)
	}
	if f.Service != "" {
		add("service = $%d", f.Service)
	}
	if len(f.Actions) > 0 {
		add("action = ANY($%d::text[])", f.Actions)
	}
	if f.EntityID != "" {
		add("entity_id = $%d", f.EntityID)
	}
	if f.CorrelationID != "" {
		add("correlation_id = $%d", f.CorrelationID)
	}
	if f.From != nil {
		add("created_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("created_at < $%d", *f.To)
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_events %s", whereClause)
	var total int
	if err := s.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit: %w", err)
	}

	offset := (page - 1) * perPage
	args = append(args, perPage, offset)
	listQuery := fmt.Sprintf(`
		SELECT id, school_id, service, module, entity_type, entity_id, action,
		       actor_user_id, actor_role, before_data, after_data, reason, correlation_id, ip_hash, created_at
		FROM audit_events
		%s
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, whereClause, len(args)-1, len(args))

	rows, err := s.DB.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()

	var events []model.AuditEvent
	for rows.Next() {
		var ev model.AuditEvent
		var beforeJSON, afterJSON []byte
		if err := rows.Scan(
			&ev.ID, &ev.SchoolID, &ev.Service, &ev.Module, &ev.EntityType, &ev.EntityID, &ev.Action,
			&ev.ActorUserID, &ev.ActorRole, &beforeJSON, &afterJSON, &ev.Reason, &ev.CorrelationID, &ev.IPHash, &ev.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit: %w", err)
		}
		if len(beforeJSON) > 0 {
			var b any
			_ = json.Unmarshal(beforeJSON, &b)
			ev.BeforeData = b
		}
		if len(afterJSON) > 0 {
			var a any
			_ = json.Unmarshal(afterJSON, &a)
			ev.AfterData = a
		}
		events = append(events, ev)
	}

	return events, total, rows.Err()
}

// AuditFacets lists the distinct services, modules and actions recorded since
// `since` (schoolID nil = every tenant).
func (s *PGStore) AuditFacets(ctx context.Context, schoolID *string, since time.Time) (model.AuditFacets, error) {
	out := model.AuditFacets{Services: []string{}, Modules: []string{}, Actions: []string{}}
	for _, c := range []struct {
		col string
		dst *[]string
	}{{"service", &out.Services}, {"module", &out.Modules}, {"action", &out.Actions}} {
		rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT `+c.col+` FROM audit_events
			WHERE ($1::uuid IS NULL OR school_id = $1) AND created_at >= $2 AND `+c.col+` <> ''
			ORDER BY 1 LIMIT 500`, schoolID, since)
		if err != nil {
			return out, fmt.Errorf("audit facets %s: %w", c.col, err)
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				return out, err
			}
			*c.dst = append(*c.dst, v)
		}
		if err := rows.Close(); err != nil {
			return out, err
		}
	}
	return out, nil
}
