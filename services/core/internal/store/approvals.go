package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/core/internal/model"
)

const requestCols = `r.id, r.school_id, r.request_type, r.source_service, r.source_id, r.title, r.summary,
	r.amount_idr, r.flags, r.maker_user_id, r.subject_user_id, r.status, r.current_seq,
	r.callback_url, r.correlation_id, r.idempotency_key, r.flow_snapshot, r.decided_at,
	r.callback_status, r.callback_attempts, r.last_callback_at, r.created_at, r.updated_at`

const stepCols = `id, school_id, request_id, seq, approver_kind, approver_role, approver_user_id,
	status, sla_hours, escalate_to_role, due_at, decided_by, decided_as_delegate_of, comment, decided_at,
	created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

type stringArrayScanner []string

func (s *stringArrayScanner) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*s = []string{}
		return nil
	case []string:
		*s = v
		return nil
	case []any:
		res := make([]string, len(v))
		for i, x := range v {
			res[i] = fmt.Sprint(x)
		}
		*s = res
		return nil
	case string:
		*s = parsePGTextArray(v)
		return nil
	case []byte:
		*s = parsePGTextArray(string(v))
		return nil
	default:
		return fmt.Errorf("cannot scan %T into []string", src)
	}
}

func parsePGTextArray(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" {
		return []string{}
	}
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	parts := strings.Split(s, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), `"`)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func scanRequest(sc rowScanner) (model.ApprovalRequest, error) {
	var req model.ApprovalRequest
	var summaryJSON, snapshotJSON []byte
	var flagsScan stringArrayScanner
	err := sc.Scan(
		&req.ID, &req.SchoolID, &req.RequestType, &req.SourceService, &req.SourceID, &req.Title, &summaryJSON,
		&req.AmountIDR, &flagsScan, &req.MakerUserID, &req.SubjectUserID, &req.Status, &req.CurrentSeq,
		&req.CallbackURL, &req.CorrelationID, &req.IdempotencyKey, &snapshotJSON, &req.DecidedAt,
		&req.CallbackStatus, &req.CallbackAttempts, &req.LastCallbackAt, &req.CreatedAt, &req.UpdatedAt,
	)
	if err != nil {
		return req, err
	}
	req.Flags = []string(flagsScan)
	if req.Flags == nil {
		req.Flags = []string{}
	}
	if len(summaryJSON) > 0 {
		_ = json.Unmarshal(summaryJSON, &req.Summary)
	}
	if len(snapshotJSON) > 0 {
		_ = json.Unmarshal(snapshotJSON, &req.FlowSnapshot)
	}
	return req, nil
}

func scanStep(sc rowScanner) (model.ApprovalStep, error) {
	var st model.ApprovalStep
	err := sc.Scan(
		&st.ID, &st.SchoolID, &st.RequestID, &st.Seq, &st.ApproverKind, &st.ApproverRole, &st.ApproverUserID,
		&st.Status, &st.SLAHours, &st.EscalateToRole, &st.DueAt, &st.DecidedBy, &st.DecidedAsDelegateOf,
		&st.Comment, &st.DecidedAt, &st.CreatedAt, &st.UpdatedAt,
	)
	return st, err
}

// queryRequests runs a request list query and attaches each request's steps.
func (s *PGStore) queryRequests(ctx context.Context, query string, args ...any) ([]model.ApprovalRequest, error) {
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
	}
	var reqs []model.ApprovalRequest
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan request: %w", err)
		}
		reqs = append(reqs, req)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// ponytail: one steps query per row; pages are capped at 200 by common.Page.
	for i := range reqs {
		if reqs[i].Steps, err = s.getRequestSteps(ctx, reqs[i].ID); err != nil {
			return nil, err
		}
	}
	return reqs, nil
}

// CreateRequest inserts a request with its resolved steps. A repeated
// (school_id, idempotency_key) returns the existing request with ErrIdempotentHit;
// ON CONFLICT makes that safe under concurrent retries too.
func (s *PGStore) CreateRequest(ctx context.Context, req model.ApprovalRequest, steps []model.ApprovalStep) (*model.ApprovalRequest, error) {
	if existing, err := s.GetRequestByIdempotency(ctx, req.SchoolID, req.IdempotencyKey); err == nil {
		return existing, ErrIdempotentHit
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("%w: request needs at least one step", ErrInvalidRequest)
	}
	summaryJSON, err := json.Marshal(req.Summary)
	if err != nil {
		return nil, fmt.Errorf("marshal summary: %w", err)
	}
	snapshotJSON, err := json.Marshal(req.FlowSnapshot)
	if err != nil {
		return nil, fmt.Errorf("marshal flow snapshot: %w", err)
	}
	flags := req.Flags
	if flags == nil {
		flags = []string{}
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO approval_requests (
			school_id, request_type, source_service, source_id, title, summary,
			amount_idr, flags, maker_user_id, subject_user_id, status, current_seq,
			callback_url, correlation_id, idempotency_key, flow_snapshot
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::text[], $9, $10, 'pending', $11, $12, $13, $14, $15)
		ON CONFLICT (school_id, idempotency_key) DO NOTHING
		RETURNING id`,
		req.SchoolID, req.RequestType, req.SourceService, req.SourceID, req.Title, summaryJSON,
		req.AmountIDR, flags, req.MakerUserID, req.SubjectUserID, steps[0].Seq,
		req.CallbackURL, req.CorrelationID, req.IdempotencyKey, snapshotJSON,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		existing, gerr := s.GetRequestByIdempotency(ctx, req.SchoolID, req.IdempotencyKey)
		if gerr != nil {
			return nil, gerr
		}
		return existing, ErrIdempotentHit
	}
	if err != nil {
		return nil, fmt.Errorf("insert request: %w", err)
	}

	for _, st := range steps {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO approval_steps (
				school_id, request_id, seq, approver_kind, approver_role, approver_user_id,
				status, sla_hours, escalate_to_role, due_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			req.SchoolID, id, st.Seq, st.ApproverKind, st.ApproverRole, st.ApproverUserID,
			st.Status, st.SLAHours, st.EscalateToRole, st.DueAt,
		); err != nil {
			return nil, fmt.Errorf("insert step %d: %w", st.Seq, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetRequestByID(ctx, req.SchoolID, id)
}

func (s *PGStore) GetRequestByID(ctx context.Context, schoolID, id string) (*model.ApprovalRequest, error) {
	req, err := scanRequest(s.DB.QueryRowContext(ctx,
		`SELECT `+requestCols+` FROM approval_requests r WHERE r.id = $1 AND r.school_id = $2`, id, schoolID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	if req.Steps, err = s.getRequestSteps(ctx, req.ID); err != nil {
		return nil, err
	}
	return &req, nil
}

func (s *PGStore) GetRequestByIdempotency(ctx context.Context, schoolID, key string) (*model.ApprovalRequest, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM approval_requests WHERE school_id = $1 AND idempotency_key = $2`, schoolID, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return s.GetRequestByID(ctx, schoolID, id)
}

// GetRequestBySource returns the newest request for a source transaction.
// schoolID "" searches every tenant (internal callers that only know the source).
func (s *PGStore) GetRequestBySource(ctx context.Context, schoolID, sourceService, sourceID string) (*model.ApprovalRequest, error) {
	var id, sid string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id FROM approval_requests
		WHERE ($1 = '' OR school_id::text = $1) AND source_service = $2 AND source_id = $3
		ORDER BY created_at DESC LIMIT 1`, schoolID, sourceService, sourceID).Scan(&id, &sid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	return s.GetRequestByID(ctx, sid, id)
}

func (s *PGStore) getRequestSteps(ctx context.Context, requestID string) ([]model.ApprovalStep, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+stepCols+` FROM approval_steps WHERE request_id = $1 ORDER BY seq`, requestID)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}
	defer rows.Close()
	var steps []model.ApprovalStep
	for rows.Next() {
		st, err := scanStep(rows)
		if err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

// InboxFilter selects what an approver sees. ActorIDs/ActorRoles are the
// user's own id/roles plus those of everyone currently delegating to them.
type InboxFilter struct {
	SchoolID   string
	UserID     string
	ActorIDs   []string
	ActorRoles []string
	Status     string // "pending" (default) or "decided"
}

// ListInbox: "pending" = requests whose current step names one of the actors
// (by user id or role); "decided" = requests where the user decided a step.
func (s *PGStore) ListInbox(ctx context.Context, f InboxFilter, page, perPage int) ([]model.ApprovalRequest, int, error) {
	var where string
	args := []any{f.SchoolID}
	if f.Status == "decided" {
		args = append(args, f.UserID)
		where = `r.school_id = $1 AND EXISTS (
			SELECT 1 FROM approval_steps s WHERE s.request_id = r.id AND s.decided_by = $2)`
	} else {
		ids, roles := f.ActorIDs, f.ActorRoles
		if ids == nil {
			ids = []string{}
		}
		if roles == nil {
			roles = []string{}
		}
		args = append(args, ids, roles)
		where = `r.school_id = $1 AND r.status = 'pending' AND EXISTS (
			SELECT 1 FROM approval_steps s
			WHERE s.request_id = r.id AND s.seq = r.current_seq AND s.status = 'pending'
			  AND (s.approver_user_id = ANY($2::uuid[]) OR s.approver_role = ANY($3::text[])))`
	}

	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_requests r WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count inbox: %w", err)
	}
	n := len(args)
	args = append(args, perPage, (page-1)*perPage)
	reqs, err := s.queryRequests(ctx, fmt.Sprintf(`SELECT %s FROM approval_requests r WHERE %s
		ORDER BY r.created_at DESC LIMIT $%d OFFSET $%d`, requestCols, where, n+1, n+2), args...)
	return reqs, total, err
}

func (s *PGStore) ListMine(ctx context.Context, schoolID, userID string, page, perPage int) ([]model.ApprovalRequest, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM approval_requests WHERE school_id = $1 AND maker_user_id = $2`,
		schoolID, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mine: %w", err)
	}
	reqs, err := s.queryRequests(ctx, `SELECT `+requestCols+` FROM approval_requests r
		WHERE r.school_id = $1 AND r.maker_user_id = $2
		ORDER BY r.created_at DESC LIMIT $3 OFFSET $4`, schoolID, userID, perPage, (page-1)*perPage)
	return reqs, total, err
}

// DecideStep applies approve/reject to the current step under a row lock on
// the request. A stale seq or a request that is no longer pending is ErrConflict.
func (s *PGStore) DecideStep(ctx context.Context, schoolID, reqID string, seq int, decision, decidedBy string, delegateOf *string, comment string) (*model.ApprovalRequest, error) {
	if decision != model.StepStatusApproved && decision != model.StepStatusRejected {
		return nil, fmt.Errorf("%w: invalid decision %q", ErrInvalidRequest, decision)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status string
	var currentSeq sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT status, current_seq FROM approval_requests
		WHERE id = $1 AND school_id = $2 FOR UPDATE`, reqID, schoolID).Scan(&status, &currentSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock request: %w", err)
	}
	if status != model.RequestStatusPending {
		return nil, fmt.Errorf("%w: permintaan sudah berstatus %s", ErrConflict, status)
	}
	if !currentSeq.Valid || int(currentSeq.Int64) != seq {
		return nil, fmt.Errorf("%w: tahap %d bukan tahap aktif", ErrConflict, seq)
	}

	var c *string
	if comment != "" {
		c = &comment
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_steps
		SET status = $1, decided_by = $2, decided_as_delegate_of = $3, comment = $4, decided_at = now(), updated_at = now()
		WHERE request_id = $5 AND seq = $6`,
		decision, decidedBy, delegateOf, c, reqID, seq); err != nil {
		return nil, fmt.Errorf("decide step: %w", err)
	}

	if decision == model.StepStatusRejected {
		if _, err := tx.ExecContext(ctx, `
			UPDATE approval_steps SET status = 'skipped', updated_at = now()
			WHERE request_id = $1 AND seq > $2`, reqID, seq); err != nil {
			return nil, fmt.Errorf("skip remaining steps: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE approval_requests SET status = 'rejected', current_seq = NULL, decided_at = now(), updated_at = now()
			WHERE id = $1`, reqID); err != nil {
			return nil, fmt.Errorf("reject request: %w", err)
		}
	} else {
		var nextSeq int
		var sla sql.NullInt64
		err = tx.QueryRowContext(ctx, `
			SELECT seq, sla_hours FROM approval_steps
			WHERE request_id = $1 AND seq > $2 ORDER BY seq LIMIT 1`, reqID, seq).Scan(&nextSeq, &sla)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `
				UPDATE approval_requests SET status = 'approved', current_seq = NULL, decided_at = now(), updated_at = now()
				WHERE id = $1`, reqID); err != nil {
				return nil, fmt.Errorf("approve request: %w", err)
			}
		case err != nil:
			return nil, fmt.Errorf("next step: %w", err)
		default:
			if _, err := tx.ExecContext(ctx, `
				UPDATE approval_steps
				SET status = 'pending', due_at = CASE WHEN $1::int IS NULL THEN NULL ELSE now() + make_interval(hours => $1::int) END,
				    updated_at = now()
				WHERE request_id = $2 AND seq = $3`, nullInt(sla), reqID, nextSeq); err != nil {
				return nil, fmt.Errorf("advance step: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE approval_requests SET current_seq = $1, updated_at = now() WHERE id = $2`, nextSeq, reqID); err != nil {
				return nil, fmt.Errorf("advance request: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetRequestByID(ctx, schoolID, reqID)
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// CancelRequest cancels a pending request (source-initiated, e.g. leave withdrawn).
func (s *PGStore) CancelRequest(ctx context.Context, reqID string) (*model.ApprovalRequest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var schoolID, status string
	err = tx.QueryRowContext(ctx, `SELECT school_id, status FROM approval_requests WHERE id = $1 FOR UPDATE`, reqID).Scan(&schoolID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock request: %w", err)
	}
	if status != model.RequestStatusPending {
		return nil, fmt.Errorf("%w: hanya permintaan pending yang dapat dibatalkan (status %s)", ErrConflict, status)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_requests
		SET status = 'cancelled', current_seq = NULL, callback_status = 'done', decided_at = now(), updated_at = now()
		WHERE id = $1`, reqID); err != nil {
		return nil, fmt.Errorf("cancel request: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_steps SET status = 'skipped', updated_at = now()
		WHERE request_id = $1 AND status IN ('waiting', 'pending')`, reqID); err != nil {
		return nil, fmt.Errorf("skip steps: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetRequestByID(ctx, schoolID, reqID)
}

func (s *PGStore) MarkCallbackDone(ctx context.Context, reqID string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE approval_requests
		SET callback_status = 'done', callback_attempts = callback_attempts + 1, last_callback_at = now(), updated_at = now()
		WHERE id = $1`, reqID)
	return err
}

// MarkCallbackFailed counts a failed delivery atomically; once attempts reach
// maxAttempts the status flips to 'failed' and exhausted is true.
func (s *PGStore) MarkCallbackFailed(ctx context.Context, reqID string, maxAttempts int) (attempts int, exhausted bool, err error) {
	var status string
	err = s.DB.QueryRowContext(ctx, `
		UPDATE approval_requests
		SET callback_attempts = callback_attempts + 1,
		    callback_status = CASE WHEN callback_attempts + 1 >= $2 THEN 'failed' ELSE 'pending' END,
		    last_callback_at = now(), updated_at = now()
		WHERE id = $1
		RETURNING callback_attempts, callback_status`, reqID, maxAttempts).Scan(&attempts, &status)
	return attempts, status == "failed", err
}

// ListPendingCallbacks returns decided requests whose callback has not been
// delivered yet and still has attempts left.
func (s *PGStore) ListPendingCallbacks(ctx context.Context, maxAttempts int) ([]model.ApprovalRequest, error) {
	return s.queryRequests(ctx, `SELECT `+requestCols+` FROM approval_requests r
		WHERE r.status IN ('approved', 'rejected') AND r.callback_status = 'pending' AND r.callback_attempts < $1
		ORDER BY r.decided_at LIMIT 50`, maxAttempts)
}

// ListExpiredSteps returns current pending steps past due that have an escalation target.
func (s *PGStore) ListExpiredSteps(ctx context.Context) ([]model.ApprovalStep, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+prefixed("s", stepCols)+`
		FROM approval_steps s JOIN approval_requests r ON r.id = s.request_id
		WHERE s.status = 'pending' AND s.due_at < now() AND s.escalate_to_role IS NOT NULL
		  AND r.status = 'pending' AND r.current_seq = s.seq
		ORDER BY s.due_at LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("list expired steps: %w", err)
	}
	defer rows.Close()
	var steps []model.ApprovalStep
	for rows.Next() {
		st, err := scanStep(rows)
		if err != nil {
			return nil, fmt.Errorf("scan expired step: %w", err)
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

// EscalateStep marks an overdue step 'escalated' and inserts a replacement
// step for its escalate_to_role right after it (later steps shift by one).
// The replacement has no escalate_to_role, so it is never escalated again.
// Returns ErrConflict when the step is no longer the overdue current step.
func (s *PGStore) EscalateStep(ctx context.Context, stepID string) (*model.ApprovalStep, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var reqID string
	var currentSeq sql.NullInt64
	var reqStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT id, status, current_seq FROM approval_requests
		WHERE id = (SELECT request_id FROM approval_steps WHERE id = $1) FOR UPDATE`, stepID).
		Scan(&reqID, &reqStatus, &currentSeq)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("lock request: %w", err)
	}

	st, err := scanStep(tx.QueryRowContext(ctx, `SELECT `+stepCols+` FROM approval_steps WHERE id = $1`, stepID))
	if err != nil {
		return nil, fmt.Errorf("load step: %w", err)
	}
	if reqStatus != model.RequestStatusPending || !currentSeq.Valid || int(currentSeq.Int64) != st.Seq ||
		st.Status != model.StepStatusPending || st.EscalateToRole == nil || st.DueAt == nil || st.DueAt.After(time.Now()) {
		return nil, fmt.Errorf("%w: tahap tidak lagi perlu dieskalasi", ErrConflict)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE approval_steps SET status = 'escalated', updated_at = now() WHERE id = $1`, st.ID); err != nil {
		return nil, fmt.Errorf("mark escalated: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SET CONSTRAINTS approval_steps_request_seq_key DEFERRED`); err != nil {
		return nil, fmt.Errorf("defer constraint: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE approval_steps SET seq = seq + 1 WHERE request_id = $1 AND seq > $2`, reqID, st.Seq); err != nil {
		return nil, fmt.Errorf("shift steps: %w", err)
	}
	newSeq := st.Seq + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approval_steps (school_id, request_id, seq, approver_kind, approver_role, status, sla_hours, due_at)
		VALUES ($1, $2, $3, 'role', $4, 'pending', $5,
		        CASE WHEN $5::int IS NULL THEN NULL ELSE now() + make_interval(hours => $5::int) END)`,
		st.SchoolID, reqID, newSeq, *st.EscalateToRole, st.SLAHours); err != nil {
		return nil, fmt.Errorf("insert replacement step: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE approval_requests SET current_seq = $1, updated_at = now() WHERE id = $2`, newSeq, reqID); err != nil {
		return nil, fmt.Errorf("advance request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	repl, err := scanStep(s.DB.QueryRowContext(ctx, `SELECT `+stepCols+` FROM approval_steps WHERE request_id = $1 AND seq = $2`, reqID, newSeq))
	if err != nil {
		return nil, fmt.Errorf("load replacement step: %w", err)
	}
	return &repl, nil
}

// prefixed qualifies a comma-separated column list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = alias + "." + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}
