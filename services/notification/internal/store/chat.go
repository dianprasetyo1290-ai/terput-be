package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"skci/notification/internal/model"
)

// ---- chat ------------------------------------------------------------------

const threadColumns = `id, school_id, subject, created_by, created_at, closed_at`

// ParticipantInput is one member to add to a new thread.
type ParticipantInput struct {
	UserID string
	Role   string
}

// CreateThreadInput is a new thread plus its participants.
type CreateThreadInput struct {
	SchoolID     string
	Subject      string
	CreatedBy    string
	Participants []ParticipantInput
}

// CreateMessageInput is one chat message.
type CreateMessageInput struct {
	ThreadID      string
	SenderUserID  string
	Body          string
	AttachmentKey *string
}

func scanThread(row rowScanner) (*model.ChatThread, error) {
	var (
		t        model.ChatThread
		closedAt sql.NullTime
	)
	if err := row.Scan(&t.ID, &t.SchoolID, &t.Subject, &t.CreatedBy, &t.CreatedAt, &closedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if closedAt.Valid {
		ct := closedAt.Time
		t.ClosedAt = &ct
	}
	return &t, nil
}

// CreateChatThread inserts a thread and its participants in one transaction.
// The creator is always a participant (role owner) even if not named again.
func (s *Store) CreateChatThread(ctx context.Context, in CreateThreadInput) (*model.ChatThread, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	t, err := scanThread(tx.QueryRowContext(ctx, `
		INSERT INTO chat_threads (school_id, subject, created_by)
		VALUES ($1,$2,$3)
		RETURNING `+threadColumns, in.SchoolID, in.Subject, in.CreatedBy))
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	add := func(userID, role string) error {
		if userID == "" || seen[userID] {
			return nil
		}
		seen[userID] = true
		if role == "" {
			role = model.ChatRoleMember
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO chat_thread_participants (thread_id, user_id, role)
			VALUES ($1,$2,$3) ON CONFLICT (thread_id, user_id) DO NOTHING`, t.ID, userID, role)
		return err
	}
	if err := add(in.CreatedBy, model.ChatRoleOwner); err != nil {
		return nil, err
	}
	for _, p := range in.Participants {
		if err := add(p.UserID, p.Role); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	participants, err := s.ListChatParticipants(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.Participants = participants
	return t, nil
}

// ListChatParticipants returns a thread's members.
func (s *Store) ListChatParticipants(ctx context.Context, threadID string) ([]model.ChatParticipant, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT thread_id, user_id, role, joined_at FROM chat_thread_participants
		WHERE thread_id=$1 ORDER BY joined_at, user_id`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.ChatParticipant, 0, 4)
	for rows.Next() {
		var p model.ChatParticipant
		if err := rows.Scan(&p.ThreadID, &p.UserID, &p.Role, &p.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListChatThreadsForUser returns ONLY the threads the caller participates in,
// scoped to the caller's school. A user can never see another user's thread
// through this list.
func (s *Store) ListChatThreadsForUser(ctx context.Context, schoolID, userID string, page, perPage int) ([]model.ChatThread, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM chat_threads t
		JOIN chat_thread_participants p ON p.thread_id = t.id
		WHERE t.school_id=$1 AND p.user_id=$2`, schoolID, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	perPage = clampPerPage(perPage)
	if page < 1 {
		page = 1
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT t.id, t.school_id, t.subject, t.created_by, t.created_at, t.closed_at,
		       (SELECT count(*) FROM chat_messages m WHERE m.thread_id = t.id) AS message_count,
		       (SELECT max(m.created_at) FROM chat_messages m WHERE m.thread_id = t.id) AS last_message_at
		FROM chat_threads t
		JOIN chat_thread_participants p ON p.thread_id = t.id
		WHERE t.school_id=$1 AND p.user_id=$2
		ORDER BY t.created_at DESC
		LIMIT $3 OFFSET $4`, schoolID, userID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]model.ChatThread, 0, perPage)
	for rows.Next() {
		var (
			t             model.ChatThread
			closedAt      sql.NullTime
			lastMessageAt sql.NullTime
		)
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Subject, &t.CreatedBy, &t.CreatedAt,
			&closedAt, &t.MessageCount, &lastMessageAt); err != nil {
			return nil, 0, err
		}
		if closedAt.Valid {
			ct := closedAt.Time
			t.ClosedAt = &ct
		}
		if lastMessageAt.Valid {
			lm := lastMessageAt.Time
			t.LastMessageAt = &lm
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		participants, err := s.ListChatParticipants(ctx, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Participants = participants
	}
	return out, total, nil
}

// GetChatThread loads one thread within a tenant, or ErrNotFound. The tenant
// filter is what makes a cross-school thread id look like it does not exist.
func (s *Store) GetChatThread(ctx context.Context, id, schoolID string) (*model.ChatThread, error) {
	t, err := scanThread(s.DB.QueryRowContext(ctx,
		`SELECT `+threadColumns+` FROM chat_threads WHERE id=$1 AND school_id=$2`, id, schoolID))
	if err != nil {
		return nil, err
	}
	participants, err := s.ListChatParticipants(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	t.Participants = participants
	return t, nil
}

// IsChatParticipant is the authorization check every thread route runs: only a
// named participant may read or post.
func (s *Store) IsChatParticipant(ctx context.Context, threadID, userID string) (bool, error) {
	var ok bool
	err := s.DB.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM chat_thread_participants WHERE thread_id=$1 AND user_id=$2)`,
		threadID, userID).Scan(&ok)
	return ok, err
}

// ListChatMessages returns a thread's messages (oldest first) plus the total.
// read_by is aggregated per message so the client can render receipts.
func (s *Store) ListChatMessages(ctx context.Context, threadID string, page, perPage int) ([]model.ChatMessage, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM chat_messages WHERE thread_id=$1`, threadID).Scan(&total); err != nil {
		return nil, 0, err
	}

	perPage = clampPerPage(perPage)
	if page < 1 {
		page = 1
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT m.id, m.thread_id, m.sender_user_id, m.body, m.attachment_key, m.created_at,
		       COALESCE((SELECT json_agg(r.user_id ORDER BY r.read_at)
		                 FROM chat_message_reads r WHERE r.message_id = m.id), '[]'::json) AS read_by
		FROM chat_messages m
		WHERE m.thread_id=$1
		ORDER BY m.created_at ASC
		LIMIT $2 OFFSET $3`, threadID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]model.ChatMessage, 0, perPage)
	for rows.Next() {
		var (
			m             model.ChatMessage
			attachmentKey sql.NullString
			readBy        []byte
		)
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.SenderUserID, &m.Body, &attachmentKey, &m.CreatedAt, &readBy); err != nil {
			return nil, 0, err
		}
		if attachmentKey.Valid {
			m.AttachmentKey = &attachmentKey.String
		}
		if len(readBy) > 0 {
			_ = json.Unmarshal(readBy, &m.ReadBy)
		}
		m.ReadCount = len(m.ReadBy)
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// CreateChatMessage inserts one message and returns it.
func (s *Store) CreateChatMessage(ctx context.Context, in CreateMessageInput) (*model.ChatMessage, error) {
	var (
		m             model.ChatMessage
		attachmentKey sql.NullString
	)
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO chat_messages (thread_id, sender_user_id, body, attachment_key)
		VALUES ($1,$2,$3,$4)
		RETURNING id, thread_id, sender_user_id, body, attachment_key, created_at`,
		in.ThreadID, in.SenderUserID, in.Body, in.AttachmentKey).Scan(
		&m.ID, &m.ThreadID, &m.SenderUserID, &m.Body, &attachmentKey, &m.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if attachmentKey.Valid {
		m.AttachmentKey = &attachmentKey.String
	}
	m.ReadBy = []string{}
	return &m, nil
}

// MarkChatThreadRead records a read receipt for every message in the thread the
// caller did not send, and returns the message ids newly marked. ON CONFLICT DO
// NOTHING makes a second call a no-op returning an empty slice.
func (s *Store) MarkChatThreadRead(ctx context.Context, threadID, userID string) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		INSERT INTO chat_message_reads (message_id, user_id)
		SELECT m.id, $2 FROM chat_messages m
		WHERE m.thread_id=$1 AND m.sender_user_id <> $2
		ON CONFLICT (message_id, user_id) DO NOTHING
		RETURNING message_id`, threadID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
