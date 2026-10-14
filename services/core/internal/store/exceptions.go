package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"skci/core/internal/model"
)

func (s *PGStore) UpsertException(ctx context.Context, exc model.IntegrationException) (*model.IntegrationException, bool, error) {
	payloadJSON, err := json.Marshal(exc.PayloadSummary)
	if err != nil {
		return nil, false, fmt.Errorf("marshal payload summary: %w", err)
	}

	query := `
		INSERT INTO integration_exceptions (
			school_id, source_service, kind, severity, correlation_id, error_message,
			attempts, max_attempts, payload_summary, retry_url, status, idempotency_key, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, 1, $7, $8, $9, 'open', $10, now(), now()
		)
		ON CONFLICT (school_id, idempotency_key) DO UPDATE SET
			attempts = integration_exceptions.attempts + 1,
			error_message = EXCLUDED.error_message,
			payload_summary = EXCLUDED.payload_summary,
			-- the same failure reported again after a manual resolve is a new
			-- occurrence: reopen it instead of hiding it under 'resolved'.
			status = 'open', resolved_by = NULL, resolution_reason = NULL, resolved_at = NULL,
			updated_at = now()
		RETURNING id, (xmax = 0)`

	var id string
	var inserted bool
	err = s.DB.QueryRowContext(ctx, query,
		exc.SchoolID, exc.SourceService, exc.Kind, exc.Severity, exc.CorrelationID, exc.ErrorMessage,
		exc.MaxAttempts, payloadJSON, exc.RetryURL, exc.IdempotencyKey,
	).Scan(&id, &inserted)
	if err != nil {
		return nil, false, fmt.Errorf("upsert exception: %w", err)
	}
	saved, err := s.GetExceptionByID(ctx, exc.SchoolID, id)
	return saved, inserted, err
}

func (s *PGStore) GetExceptionByID(ctx context.Context, schoolID, id string) (*model.IntegrationException, error) {
	query := `
		SELECT id, school_id, source_service, kind, severity, correlation_id, error_message,
		       attempts, max_attempts, payload_summary, retry_url, status, assignee_user_id,
		       resolved_by, resolution_reason, resolved_at, idempotency_key, created_at, updated_at
		FROM integration_exceptions
		WHERE id = $1 AND school_id = $2`
	var e model.IntegrationException
	var payloadJSON []byte
	err := s.DB.QueryRowContext(ctx, query, id, schoolID).Scan(
		&e.ID, &e.SchoolID, &e.SourceService, &e.Kind, &e.Severity, &e.CorrelationID, &e.ErrorMessage,
		&e.Attempts, &e.MaxAttempts, &payloadJSON, &e.RetryURL, &e.Status, &e.AssigneeUserID,
		&e.ResolvedBy, &e.ResolutionReason, &e.ResolvedAt, &e.IdempotencyKey, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get exception: %w", err)
	}

	if len(payloadJSON) > 0 {
		_ = json.Unmarshal(payloadJSON, &e.PayloadSummary)
	}
	return &e, nil
}

// ExceptionFilter scopes the queue. Kinds is the caller's allowed set (never
// empty); SourceService, when set, pins the source (payroll_officer -> hr).
type ExceptionFilter struct {
	SchoolID      string
	Kinds         []string
	SourceService string
	Kind          string
	Status        string
	Severity      string
}

func (s *PGStore) ListExceptions(ctx context.Context, f ExceptionFilter, page, perPage int) ([]model.IntegrationException, int, error) {
	if len(f.Kinds) == 0 {
		return nil, 0, nil
	}
	kind, status, severity := f.Kind, f.Status, f.Severity
	args := []any{f.SchoolID, f.Kinds}
	where := []string{"school_id = $1", "kind = ANY($2::text[])"}
	if f.SourceService != "" {
		args = append(args, f.SourceService)
		where = append(where, fmt.Sprintf("source_service = $%d", len(args)))
	}

	if kind != "" {
		args = append(args, kind)
		where = append(where, fmt.Sprintf("kind = $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if severity != "" {
		args = append(args, severity)
		where = append(where, fmt.Sprintf("severity = $%d", len(args)))
	}

	whereClause := strings.Join(where, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM integration_exceptions WHERE %s", whereClause)
	var total int
	if err := s.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count exceptions: %w", err)
	}

	offset := (page - 1) * perPage
	args = append(args, perPage, offset)
	listQuery := fmt.Sprintf(`
		SELECT id, school_id, source_service, kind, severity, correlation_id, error_message,
		       attempts, max_attempts, payload_summary, retry_url, status, assignee_user_id,
		       resolved_by, resolution_reason, resolved_at, idempotency_key, created_at, updated_at
		FROM integration_exceptions
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, len(args)-1, len(args))

	rows, err := s.DB.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list exceptions: %w", err)
	}
	defer rows.Close()

	var exceptions []model.IntegrationException
	for rows.Next() {
		var e model.IntegrationException
		var payloadJSON []byte
		if err := rows.Scan(
			&e.ID, &e.SchoolID, &e.SourceService, &e.Kind, &e.Severity, &e.CorrelationID, &e.ErrorMessage,
			&e.Attempts, &e.MaxAttempts, &payloadJSON, &e.RetryURL, &e.Status, &e.AssigneeUserID,
			&e.ResolvedBy, &e.ResolutionReason, &e.ResolvedAt, &e.IdempotencyKey, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan exception: %w", err)
		}
		if len(payloadJSON) > 0 {
			_ = json.Unmarshal(payloadJSON, &e.PayloadSummary)
		}
		exceptions = append(exceptions, e)
	}
	return exceptions, total, rows.Err()
}

// updateOpenException runs an UPDATE guarded by status <> 'resolved'.
// No row: ErrNotFound if the id does not exist, else ErrConflict (already resolved).
func (s *PGStore) updateOpenException(ctx context.Context, schoolID, id, set string, args ...any) error {
	n := len(args)
	q := fmt.Sprintf(`UPDATE integration_exceptions SET %s, updated_at = now()
		WHERE id = $%d AND school_id = $%d AND status <> 'resolved'`, set, n+1, n+2)
	res, err := s.DB.ExecContext(ctx, q, append(args, id, schoolID)...)
	if err != nil {
		return fmt.Errorf("update exception: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows > 0 {
		return nil
	}
	if _, err := s.GetExceptionByID(ctx, schoolID, id); err != nil {
		return err
	}
	return fmt.Errorf("%w: item sudah diselesaikan", ErrConflict)
}

func (s *PGStore) AssignException(ctx context.Context, schoolID, id, assigneeUserID string) error {
	return s.updateOpenException(ctx, schoolID, id, "assignee_user_id = $1", assigneeUserID)
}

func (s *PGStore) ResolveException(ctx context.Context, schoolID, id, resolvedBy, reason string) error {
	return s.updateOpenException(ctx, schoolID, id,
		"status = 'resolved', resolved_by = $1, resolution_reason = $2, resolved_at = now()", resolvedBy, reason)
}

// ErrRetryInProgress: another retry of the same item holds the lock (BUG-167).
var ErrRetryInProgress = fmt.Errorf("%w: retry item ini sedang diproses, tunggu hingga selesai", ErrConflict)

// ClaimExceptionForRetry atomically moves an open item to 'retrying' and
// counts the attempt; the row itself is the lock. A 'retrying' row older than
// staleAfter (worker died mid-retry) may be claimed again. Concurrent callers
// get ErrRetryInProgress, a resolved item ErrConflict.
func (s *PGStore) ClaimExceptionForRetry(ctx context.Context, schoolID, id string, staleAfter time.Duration) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE integration_exceptions
		SET status = 'retrying', attempts = attempts + 1, updated_at = now()
		WHERE id = $1 AND school_id = $2
		  AND (status = 'open' OR (status = 'retrying' AND updated_at < now() - make_interval(secs => $3)))`,
		id, schoolID, staleAfter.Seconds())
	if err != nil {
		return fmt.Errorf("claim exception: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	cur, err := s.GetExceptionByID(ctx, schoolID, id)
	if err != nil {
		return err
	}
	if cur.Status == model.ExceptionStatusRetrying {
		return ErrRetryInProgress
	}
	return fmt.Errorf("%w: item sudah diselesaikan", ErrConflict)
}

// ReopenException puts a failed retry back to 'open'.
func (s *PGStore) ReopenException(ctx context.Context, schoolID, id, lastError string) error {
	return s.updateOpenException(ctx, schoolID, id, "status = 'open', error_message = $1", lastError)
}
