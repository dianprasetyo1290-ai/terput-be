package store

import (
	"context"
	"database/sql"
	"errors"

	"skci/notification/internal/model"
)

// UpsertPushSubscription inserts or updates a push subscription and marks push preference enabled.
func (s *Store) UpsertPushSubscription(ctx context.Context, sub model.PushSubscription) (*model.PushSubscription, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var out model.PushSubscription
	err = tx.QueryRowContext(ctx, `
		INSERT INTO push_subscriptions (
			school_id, user_id, endpoint, p256dh, auth, user_agent, created_at, revoked_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, now(), NULL
		)
		ON CONFLICT (endpoint) DO UPDATE SET
			school_id = EXCLUDED.school_id,
			user_id = EXCLUDED.user_id,
			p256dh = EXCLUDED.p256dh,
			auth = EXCLUDED.auth,
			user_agent = COALESCE(EXCLUDED.user_agent, push_subscriptions.user_agent),
			revoked_at = NULL
		RETURNING id, school_id, user_id, endpoint, p256dh, auth, user_agent, created_at, last_success_at, revoked_at
	`, sub.SchoolID, sub.UserID, sub.Endpoint, sub.P256dh, sub.Auth, sub.UserAgent).Scan(
		&out.ID, &out.SchoolID, &out.UserID, &out.Endpoint, &out.P256dh, &out.Auth,
		&out.UserAgent, &out.CreatedAt, &out.LastSuccessAt, &out.RevokedAt,
	)
	if err != nil {
		return nil, err
	}

	// Set preference push = true as explicit consent (T26 §4)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO notification_preferences (school_id, user_id, channel, enabled)
		VALUES ($1, $2, 'push', true)
		ON CONFLICT (school_id, user_id, channel)
		DO UPDATE SET enabled = true, updated_at = now()
	`, sub.SchoolID, sub.UserID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePushSubscription marks or removes a push subscription for a user.
func (s *Store) DeletePushSubscription(ctx context.Context, schoolID, userID, endpoint string) error {
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM push_subscriptions
		WHERE school_id = $1 AND user_id = $2 AND endpoint = $3
	`, schoolID, userID, endpoint)
	return err
}

// ListActivePushSubscriptions returns all active (non-revoked) push subscriptions for a user.
func (s *Store) ListActivePushSubscriptions(ctx context.Context, schoolID, userID string) ([]model.PushSubscription, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, user_id, endpoint, p256dh, auth, user_agent, created_at, last_success_at, revoked_at
		FROM push_subscriptions
		WHERE school_id = $1 AND user_id = $2 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, schoolID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.PushSubscription
	for rows.Next() {
		var sub model.PushSubscription
		if err := rows.Scan(
			&sub.ID, &sub.SchoolID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth,
			&sub.UserAgent, &sub.CreatedAt, &sub.LastSuccessAt, &sub.RevokedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, sub)
	}
	return list, rows.Err()
}

// RevokePushSubscription sets revoked_at = now() when endpoint returns 404/410.
func (s *Store) RevokePushSubscription(ctx context.Context, endpoint string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE push_subscriptions
		SET revoked_at = now()
		WHERE endpoint = $1 AND revoked_at IS NULL
	`, endpoint)
	return err
}

// TouchPushSubscription sets last_success_at = now() upon successful delivery.
func (s *Store) TouchPushSubscription(ctx context.Context, endpoint string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE push_subscriptions
		SET last_success_at = now()
		WHERE endpoint = $1
	`, endpoint)
	return err
}

// GetUserPushPreference returns the user's push preference record, or nil if none exists.
func (s *Store) GetUserPushPreference(ctx context.Context, schoolID, userID string) (*model.NotificationPreference, error) {
	var p model.NotificationPreference
	err := s.DB.QueryRowContext(ctx, `
		SELECT school_id, user_id, channel, enabled, updated_at
		FROM notification_preferences
		WHERE school_id = $1 AND user_id = $2 AND channel = 'push'
	`, schoolID, userID).Scan(&p.SchoolID, &p.UserID, &p.Channel, &p.Enabled, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
