// Package store is notification-service's persistence layer. Every query is
// scoped by school_id (CONVENTIONS.md's multi-tenancy rule) and, for personal
// data (notifications, preferences, chat), by user_id as well. Pure SQL, no ORM.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/notification/internal/model"
)

// Sentinel errors the handlers translate into HTTP statuses.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Store is the Postgres-backed implementation.
type Store struct {
	DB *sql.DB
}

// New wraps an existing pool.
func New(db *sql.DB) *Store { return &Store{DB: db} }

// Open connects and verifies the database is reachable.
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
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

// ---- notifications ---------------------------------------------------------

const notificationColumns = `id, school_id, user_id, type, title, body, data, idempotency_key, read_at, created_at`

// CreateNotificationInput is one message to enqueue for a single recipient.
type CreateNotificationInput struct {
	SchoolID       string
	UserID         string
	Type           string
	Title          string
	Body           string
	Data           json.RawMessage
	IdempotencyKey *string
}

// NotificationFilter narrows a personal inbox listing.
type NotificationFilter struct {
	SchoolID string
	UserID   string
	Type     string
	Unread   bool
	Page     int
	PerPage  int
}

func scanNotification(row rowScanner) (*model.Notification, error) {
	var (
		n         model.Notification
		data      []byte
		idem      sql.NullString
		readAt    sql.NullTime
		createdAt time.Time
	)
	if err := row.Scan(&n.ID, &n.SchoolID, &n.UserID, &n.Type, &n.Title, &n.Body,
		&data, &idem, &readAt, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	n.Data = json.RawMessage(data)
	if idem.Valid {
		n.IdempotencyKey = &idem.String
	}
	if readAt.Valid {
		t := readAt.Time
		n.ReadAt = &t
	}
	n.CreatedAt = createdAt
	return &n, nil
}

func dataOrEmpty(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

// CreateNotification inserts one notification. When idempotencyKey is set and a
// row with the same (school_id, idempotency_key) already exists, the existing
// row is returned with created=false — so a retried internal send is a no-op
// instead of a duplicate message.
func (s *Store) CreateNotification(ctx context.Context, in CreateNotificationInput) (*model.Notification, bool, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO notifications (school_id, user_id, type, title, body, data, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7)
		ON CONFLICT (school_id, idempotency_key) DO NOTHING
		RETURNING `+notificationColumns,
		in.SchoolID, in.UserID, in.Type, in.Title, in.Body, dataOrEmpty(in.Data), in.IdempotencyKey)

	n, err := scanNotification(row)
	if errors.Is(err, ErrNotFound) {
		// Conflict: another caller (or a retry) already created this message.
		if in.IdempotencyKey == nil {
			return nil, false, err
		}
		existing, gerr := s.GetNotificationByIdempotencyKey(ctx, in.SchoolID, *in.IdempotencyKey)
		if gerr != nil {
			return nil, false, gerr
		}
		return existing, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return n, true, nil
}

// GetNotificationByIdempotencyKey returns the row previously created for an
// idempotency key, or ErrNotFound.
func (s *Store) GetNotificationByIdempotencyKey(ctx context.Context, schoolID, key string) (*model.Notification, error) {
	return scanNotification(s.DB.QueryRowContext(ctx,
		`SELECT `+notificationColumns+` FROM notifications WHERE school_id=$1 AND idempotency_key=$2`,
		schoolID, key))
}

// GetNotification loads one notification owned by (school, user).
func (s *Store) GetNotification(ctx context.Context, id, schoolID, userID string) (*model.Notification, error) {
	return scanNotification(s.DB.QueryRowContext(ctx,
		`SELECT `+notificationColumns+` FROM notifications WHERE id=$1 AND school_id=$2 AND user_id=$3`,
		id, schoolID, userID))
}

// GetNotificationByID loads one notification for a tenant (internal callers
// that do not act on behalf of a specific recipient).
func (s *Store) GetNotificationByID(ctx context.Context, id, schoolID string) (*model.Notification, error) {
	return scanNotification(s.DB.QueryRowContext(ctx,
		`SELECT `+notificationColumns+` FROM notifications WHERE id=$1 AND school_id=$2`,
		id, schoolID))
}

func (f NotificationFilter) whereClause() (string, []interface{}) {
	args := []interface{}{f.SchoolID, f.UserID}
	where := "school_id = $1 AND user_id = $2"
	if f.Type != "" {
		args = append(args, f.Type)
		where += fmt.Sprintf(" AND type = $%d", len(args))
	}
	if f.Unread {
		where += " AND read_at IS NULL"
	}
	return where, args
}

// ListNotifications returns the caller's notifications plus the total count for
// pagination meta. unread-only is the `?unread=true` filter.
func (s *Store) ListNotifications(ctx context.Context, f NotificationFilter) ([]model.Notification, int, error) {
	where, args := f.whereClause()

	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM notifications WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	perPage := clampPerPage(f.PerPage)
	page := f.Page
	if page < 1 {
		page = 1
	}
	args = append(args, perPage, (page-1)*perPage)
	rows, err := s.DB.QueryContext(ctx, fmt.Sprintf(
		`SELECT %s FROM notifications WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		notificationColumns, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]model.Notification, 0, perPage)
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *n)
	}
	return out, total, rows.Err()
}

// CountUnreadNotifications is the inbox badge count.
func (s *Store) CountUnreadNotifications(ctx context.Context, schoolID, userID string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT count(*) FROM notifications WHERE school_id=$1 AND user_id=$2 AND read_at IS NULL`,
		schoolID, userID).Scan(&n)
	return n, err
}

// MarkNotificationRead sets read_at once; a second call leaves it untouched
// (COALESCE) so marking read is idempotent.
func (s *Store) MarkNotificationRead(ctx context.Context, id, schoolID, userID string) (*model.Notification, error) {
	return scanNotification(s.DB.QueryRowContext(ctx, `
		UPDATE notifications SET read_at = COALESCE(read_at, now())
		WHERE id=$1 AND school_id=$2 AND user_id=$3
		RETURNING `+notificationColumns, id, schoolID, userID))
}

// MarkAllNotificationsRead marks every unread notification read and reports how
// many rows changed.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, schoolID, userID string) (int, error) {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE notifications SET read_at = now() WHERE school_id=$1 AND user_id=$2 AND read_at IS NULL`,
		schoolID, userID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---- preferences -----------------------------------------------------------

// PreferenceInput is one channel on/off pair.
type PreferenceInput struct {
	Channel string
	Enabled bool
}

// ListNotificationPreferences returns only the stored overrides; callers merge
// them with model.DefaultPreference for channels with no row.
func (s *Store) ListNotificationPreferences(ctx context.Context, schoolID, userID string) ([]model.NotificationPreference, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT school_id, user_id, channel, enabled, updated_at
		FROM notification_preferences WHERE school_id=$1 AND user_id=$2 ORDER BY channel`,
		schoolID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.NotificationPreference, 0, 3)
	for rows.Next() {
		var p model.NotificationPreference
		if err := rows.Scan(&p.SchoolID, &p.UserID, &p.Channel, &p.Enabled, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertNotificationPreferences writes the given channel switches atomically and
// returns the full merged list.
func (s *Store) UpsertNotificationPreferences(ctx context.Context, schoolID, userID string, prefs []PreferenceInput) ([]model.NotificationPreference, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	for _, p := range prefs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notification_preferences (school_id, user_id, channel, enabled)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (school_id, user_id, channel)
			DO UPDATE SET enabled=EXCLUDED.enabled, updated_at=now()`,
			schoolID, userID, p.Channel, p.Enabled); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListNotificationPreferences(ctx, schoolID, userID)
}

// IsChannelEnabled reports the effective switch for a channel: the stored row,
// or the model default when none exists.
func (s *Store) IsChannelEnabled(ctx context.Context, schoolID, userID, channel string) (bool, error) {
	var enabled bool
	err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT enabled FROM notification_preferences
			WHERE school_id=$1 AND user_id=$2 AND channel=$3), $4)`,
		schoolID, userID, channel, model.DefaultPreference(channel)).Scan(&enabled)
	return enabled, err
}

// ---- templates -------------------------------------------------------------

const templateColumns = `id, school_id, key, version, channel, subject, body, created_at, updated_at`

// TemplateInput is a template create/update payload.
type TemplateInput struct {
	SchoolID string
	Key      string
	Version  int
	Channel  string
	Subject  string
	Body     string
}

func scanTemplate(row rowScanner) (*model.NotificationTemplate, error) {
	var t model.NotificationTemplate
	if err := row.Scan(&t.ID, &t.SchoolID, &t.Key, &t.Version, &t.Channel, &t.Subject, &t.Body, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

// ListNotificationTemplates lists a tenant's templates, optionally narrowed to
// one key.
func (s *Store) ListNotificationTemplates(ctx context.Context, schoolID, key string) ([]model.NotificationTemplate, error) {
	args := []interface{}{schoolID}
	where := "school_id = $1"
	if key != "" {
		args = append(args, key)
		where += fmt.Sprintf(" AND key = $%d", len(args))
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+templateColumns+` FROM notification_templates WHERE `+where+` ORDER BY key, channel, version DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.NotificationTemplate, 0)
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetNotificationTemplate loads one template row by id within a tenant.
func (s *Store) GetNotificationTemplate(ctx context.Context, id, schoolID string) (*model.NotificationTemplate, error) {
	return scanTemplate(s.DB.QueryRowContext(ctx,
		`SELECT `+templateColumns+` FROM notification_templates WHERE id=$1 AND school_id=$2`, id, schoolID))
}

// FindNotificationTemplate returns the newest version of a key for a channel.
func (s *Store) FindNotificationTemplate(ctx context.Context, schoolID, key, channel string) (*model.NotificationTemplate, error) {
	return scanTemplate(s.DB.QueryRowContext(ctx, `
		SELECT `+templateColumns+` FROM notification_templates
		WHERE school_id=$1 AND key=$2 AND channel=$3
		ORDER BY version DESC LIMIT 1`, schoolID, key, channel))
}

// CreateNotificationTemplate inserts a new version. A version <= 0 means "next
// version for this key+channel", so callers can add a new revision without
// first reading the current max.
func (s *Store) CreateNotificationTemplate(ctx context.Context, in TemplateInput) (*model.NotificationTemplate, error) {
	version := in.Version
	if version <= 0 {
		if err := s.DB.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version),0)+1 FROM notification_templates
			WHERE school_id=$1 AND key=$2 AND channel=$3`,
			in.SchoolID, in.Key, in.Channel).Scan(&version); err != nil {
			return nil, err
		}
	}
	t, err := scanTemplate(s.DB.QueryRowContext(ctx, `
		INSERT INTO notification_templates (school_id, key, version, channel, subject, body)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING `+templateColumns,
		in.SchoolID, in.Key, version, in.Channel, in.Subject, in.Body))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return t, err
}

// UpdateNotificationTemplate replaces the subject/body of one template row.
func (s *Store) UpdateNotificationTemplate(ctx context.Context, id, schoolID string, in TemplateInput) (*model.NotificationTemplate, error) {
	return scanTemplate(s.DB.QueryRowContext(ctx, `
		UPDATE notification_templates
		SET subject=$3, body=$4, updated_at=now()
		WHERE id=$1 AND school_id=$2
		RETURNING `+templateColumns, id, schoolID, in.Subject, in.Body))
}

// DeleteNotificationTemplate removes a template row.
func (s *Store) DeleteNotificationTemplate(ctx context.Context, id, schoolID string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM notification_templates WHERE id=$1 AND school_id=$2`, id, schoolID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- deliveries ------------------------------------------------------------

const deliveryColumns = `id, notification_id, channel, status, attempts, last_error, provider_message_id, created_at, updated_at`

func scanDelivery(row rowScanner) (*model.NotificationDelivery, error) {
	var (
		d                   model.NotificationDelivery
		lastErr, providerID sql.NullString
	)
	if err := row.Scan(&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.Attempts,
		&lastErr, &providerID, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if lastErr.Valid {
		d.LastError = &lastErr.String
	}
	if providerID.Valid {
		d.ProviderMessageID = &providerID.String
	}
	return &d, nil
}

// CreateDelivery creates (or returns) the ledger row for one channel.
func (s *Store) CreateDelivery(ctx context.Context, notificationID, channel string) (*model.NotificationDelivery, error) {
	return scanDelivery(s.DB.QueryRowContext(ctx, `
		INSERT INTO notification_deliveries (notification_id, channel, status)
		VALUES ($1,$2,'pending')
		ON CONFLICT (notification_id, channel) DO UPDATE SET updated_at=now()
		RETURNING `+deliveryColumns, notificationID, channel))
}

// ListDeliveries returns every channel ledger for a notification.
func (s *Store) ListDeliveries(ctx context.Context, notificationID string) ([]model.NotificationDelivery, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+deliveryColumns+` FROM notification_deliveries WHERE notification_id=$1 ORDER BY channel`, notificationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.NotificationDelivery, 0, 3)
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// UpdateDeliveryInput records the outcome of one delivery attempt.
type UpdateDeliveryInput struct {
	Status            string
	LastError         string
	ProviderMessageID string
	IncrementAttempts bool
}

// UpdateDelivery writes an attempt's outcome. A successful send clears any
// previous last_error so the row does not keep a stale failure.
func (s *Store) UpdateDelivery(ctx context.Context, id string, in UpdateDeliveryInput) (*model.NotificationDelivery, error) {
	var lastErr interface{}
	if in.Status != model.DeliverySent && in.LastError != "" {
		lastErr = in.LastError
	}
	var providerID interface{}
	if in.ProviderMessageID != "" {
		providerID = in.ProviderMessageID
	}
	increment := 0
	if in.IncrementAttempts {
		increment = 1
	}
	return scanDelivery(s.DB.QueryRowContext(ctx, `
		UPDATE notification_deliveries
		SET status=$2, last_error=$3, provider_message_id=COALESCE($4, provider_message_id),
		    attempts = attempts + $5, updated_at=now()
		WHERE id=$1
		RETURNING `+deliveryColumns, id, in.Status, lastErr, providerID, increment))
}

func clampPerPage(n int) int {
	if n < 1 {
		return 20
	}
	if n > 200 {
		return 200
	}
	return n
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "SQLSTATE 23505")
}
