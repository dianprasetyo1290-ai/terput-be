package assistant

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Settings
func (s *Store) GetSettings(ctx context.Context, schoolID string) (*AssistantSettings, error) {
	query := `SELECT school_id, log_retention_days, created_at, updated_at
	          FROM assistant_settings WHERE school_id = $1`
	var st AssistantSettings
	err := s.pool.QueryRow(ctx, query, schoolID).Scan(&st.SchoolID, &st.LogRetentionDays, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return &AssistantSettings{SchoolID: schoolID}, nil
	}
	return &st, nil
}

func (s *Store) UpdateSettings(ctx context.Context, schoolID string, logRetentionDays *int) error {
	query := `INSERT INTO assistant_settings (school_id, log_retention_days, updated_at)
	          VALUES ($1, $2, NOW())
	          ON CONFLICT (school_id)
	          DO UPDATE SET log_retention_days = EXCLUDED.log_retention_days, updated_at = NOW()`
	_, err := s.pool.Exec(ctx, query, schoolID, logRetentionDays)
	return err
}

// Help Articles
func (s *Store) ListHelpArticles(ctx context.Context, schoolID *string, status string) ([]HelpArticle, error) {
	query := `SELECT id, school_id, title, body_md, tags, route_name, status, updated_by, created_at, updated_at
	          FROM help_articles WHERE (school_id IS NULL`
	args := []interface{}{}
	if schoolID != nil {
		args = append(args, *schoolID)
		query += fmt.Sprintf(" OR school_id = $%d)", len(args))
	} else {
		query += ")"
	}

	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []HelpArticle
	for rows.Next() {
		var a HelpArticle
		if err := rows.Scan(&a.ID, &a.SchoolID, &a.Title, &a.BodyMd, &a.Tags, &a.RouteName, &a.Status, &a.UpdatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, nil
}

func (s *Store) CreateHelpArticle(ctx context.Context, a HelpArticle) (*HelpArticle, error) {
	query := `INSERT INTO help_articles (school_id, title, body_md, tags, route_name, status, updated_by)
	          VALUES ($1, $2, $3, $4, $5, $6, $7)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, a.SchoolID, a.Title, a.BodyMd, a.Tags, a.RouteName, a.Status, a.UpdatedBy).
		Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) UpdateHelpArticle(ctx context.Context, a HelpArticle) error {
	query := `UPDATE help_articles SET title = $2, body_md = $3, tags = $4, route_name = $5, status = $6, updated_by = $7, updated_at = NOW()
	          WHERE id = $1`
	cmd, err := s.pool.Exec(ctx, query, a.ID, a.Title, a.BodyMd, a.Tags, a.RouteName, a.Status, a.UpdatedBy)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("article not found")
	}
	return nil
}

func (s *Store) DeleteHelpArticle(ctx context.Context, id string) error {
	query := `DELETE FROM help_articles WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id)
	return err
}

// Assistant Intents
func (s *Store) ListIntents(ctx context.Context, schoolID *string) ([]AssistantIntent, error) {
	query := `SELECT id, school_id, key, phrases, tool_key, roles, created_at, updated_at
	          FROM assistant_intents WHERE school_id IS NULL`
	args := []interface{}{}
	if schoolID != nil {
		args = append(args, *schoolID)
		query += fmt.Sprintf(" OR school_id = $%d", len(args))
	}
	query += " ORDER BY created_at ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []AssistantIntent
	for rows.Next() {
		var i AssistantIntent
		if err := rows.Scan(&i.ID, &i.SchoolID, &i.Key, &i.Phrases, &i.ToolKey, &i.Roles, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, i)
	}
	return res, nil
}

func (s *Store) CreateIntent(ctx context.Context, i AssistantIntent) (*AssistantIntent, error) {
	query := `INSERT INTO assistant_intents (school_id, key, phrases, tool_key, roles)
	          VALUES ($1, $2, $3, $4, $5)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, i.SchoolID, i.Key, i.Phrases, i.ToolKey, i.Roles).
		Scan(&i.ID, &i.CreatedAt, &i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (s *Store) UpdateIntent(ctx context.Context, i AssistantIntent) error {
	query := `UPDATE assistant_intents SET phrases = $2, tool_key = $3, roles = $4, updated_at = NOW()
	          WHERE id = $1`
	cmd, err := s.pool.Exec(ctx, query, i.ID, i.Phrases, i.ToolKey, i.Roles)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("intent not found")
	}
	return nil
}

func (s *Store) DeleteIntent(ctx context.Context, id string) error {
	query := `DELETE FROM assistant_intents WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id)
	return err
}

// Conversations & Messages
func (s *Store) GetOrCreateConversation(ctx context.Context, schoolID, userID, activeRole string, convID *string) (*AssistantConversation, error) {
	if convID != nil && *convID != "" {
		var c AssistantConversation
		query := `SELECT id, school_id, user_id, active_role, started_at, last_message_at
		          FROM assistant_conversations WHERE id = $1 AND user_id = $2`
		err := s.pool.QueryRow(ctx, query, *convID, userID).
			Scan(&c.ID, &c.SchoolID, &c.UserID, &c.ActiveRole, &c.StartedAt, &c.LastMessageAt)
		if err == nil {
			return &c, nil
		}
	}

	var c AssistantConversation
	query := `INSERT INTO assistant_conversations (school_id, user_id, active_role)
	          VALUES ($1, $2, $3)
	          RETURNING id, school_id, user_id, active_role, started_at, last_message_at`
	err := s.pool.QueryRow(ctx, query, schoolID, userID, activeRole).
		Scan(&c.ID, &c.SchoolID, &c.UserID, &c.ActiveRole, &c.StartedAt, &c.LastMessageAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListConversations(ctx context.Context, schoolID, userID string) ([]AssistantConversation, error) {
	query := `SELECT id, school_id, user_id, active_role, started_at, last_message_at
	          FROM assistant_conversations WHERE school_id = $1 AND user_id = $2 ORDER BY last_message_at DESC LIMIT 30`
	rows, err := s.pool.Query(ctx, query, schoolID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []AssistantConversation
	for rows.Next() {
		var c AssistantConversation
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.UserID, &c.ActiveRole, &c.StartedAt, &c.LastMessageAt); err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, nil
}

func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]AssistantMessage, error) {
	query := `SELECT id, school_id, conversation_id, sender, content, ai_generated, sources, tool_calls, provider, latency_ms, created_at
	          FROM assistant_messages WHERE conversation_id = $1 ORDER BY created_at ASC`
	rows, err := s.pool.Query(ctx, query, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []AssistantMessage
	for rows.Next() {
		var m AssistantMessage
		var sBytes, tcBytes []byte
		if err := rows.Scan(&m.ID, &m.SchoolID, &m.ConversationID, &m.Sender, &m.Content, &m.AIGenerated, &sBytes, &tcBytes, &m.Provider, &m.LatencyMs, &m.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(sBytes, &m.Sources)
		_ = json.Unmarshal(tcBytes, &m.ToolCalls)
		res = append(res, m)
	}
	return res, nil
}

func (s *Store) CreateMessage(ctx context.Context, m AssistantMessage) (*AssistantMessage, error) {
	sBytes, _ := json.Marshal(m.Sources)
	tcBytes, _ := json.Marshal(m.ToolCalls)
	query := `INSERT INTO assistant_messages (school_id, conversation_id, sender, content, ai_generated, sources, tool_calls, provider, latency_ms)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	          RETURNING id, created_at`
	err := s.pool.QueryRow(ctx, query, m.SchoolID, m.ConversationID, m.Sender, m.Content, m.AIGenerated, sBytes, tcBytes, m.Provider, m.LatencyMs).
		Scan(&m.ID, &m.CreatedAt)
	if err != nil {
		return nil, err
	}

	// update conversation last_message_at
	_, _ = s.pool.Exec(ctx, "UPDATE assistant_conversations SET last_message_at = NOW() WHERE id = $1", m.ConversationID)
	return &m, nil
}

// Actions
func (s *Store) CreateAction(ctx context.Context, a AssistantAction) (*AssistantAction, error) {
	pBytes, _ := json.Marshal(a.Payload)
	prBytes, _ := json.Marshal(a.Preview)
	query := `INSERT INTO assistant_actions (school_id, conversation_id, message_id, action_key, payload, preview, status, proposed_by, expires_at, idempotency_key)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, a.SchoolID, a.ConversationID, a.MessageID, a.ActionKey, pBytes, prBytes, a.Status, a.ProposedBy, a.ExpiresAt, a.IdempotencyKey).
		Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) GetAction(ctx context.Context, id, schoolID string) (*AssistantAction, error) {
	query := `SELECT id, school_id, conversation_id, message_id, action_key, payload, preview, status, proposed_by,
	                 confirmed_at, executed_at, result, expires_at, idempotency_key, created_at, updated_at
	          FROM assistant_actions WHERE id = $1 AND school_id = $2`
	var a AssistantAction
	var pBytes, prBytes, rBytes []byte
	err := s.pool.QueryRow(ctx, query, id, schoolID).
		Scan(&a.ID, &a.SchoolID, &a.ConversationID, &a.MessageID, &a.ActionKey, &pBytes, &prBytes, &a.Status, &a.ProposedBy,
			&a.ConfirmedAt, &a.ExecutedAt, &rBytes, &a.ExpiresAt, &a.IdempotencyKey, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(pBytes, &a.Payload)
	_ = json.Unmarshal(prBytes, &a.Preview)
	if len(rBytes) > 0 {
		_ = json.Unmarshal(rBytes, &a.Result)
	}
	return &a, nil
}

func (s *Store) UpdateActionStatus(ctx context.Context, id, status string, result map[string]interface{}) error {
	rBytes, _ := json.Marshal(result)
	query := `UPDATE assistant_actions SET status = $2, result = $3, executed_at = NOW(), updated_at = NOW() WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, status, rBytes)
	return err
}
