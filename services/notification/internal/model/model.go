// Package model carries the notification-service domain types and the small,
// closed sets of values (channels, delivery statuses, chat roles) that both the
// store and the handlers enforce.
package model

import (
	"encoding/json"
	"time"
)

// Delivery channels. in_app is the portal inbox; email and whatsapp are
// delivered through the provider adapter and are opt-in via
// notification_preferences. push delivers via WebPush RFC 8291.
const (
	ChannelInApp    = "in_app"
	ChannelEmail    = "email"
	ChannelWhatsApp = "whatsapp"
	ChannelPush     = "push"
)

// ValidChannels is the closed set stored in every channel column.
var ValidChannels = map[string]bool{
	ChannelInApp:    true,
	ChannelEmail:    true,
	ChannelWhatsApp: true,
	ChannelPush:     true,
}

// PushSubscription records one WebPush endpoint and crypto keys for a user.
type PushSubscription struct {
	ID            string     `json:"id"`
	SchoolID      string     `json:"school_id"`
	UserID        string     `json:"user_id"`
	Endpoint      string     `json:"endpoint"`
	P256dh        string     `json:"p256dh"`
	Auth          string     `json:"auth"`
	UserAgent     *string    `json:"user_agent,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}

// ShouldPush reports whether a push notification should be dispatched (T26, CC-08).
func ShouldPush(prefRow *NotificationPreference, hasActiveSubscription bool) bool {
	if !hasActiveSubscription {
		return false
	}
	if prefRow == nil {
		return true // Default on when active subscription exists
	}
	return prefRow.Enabled
}

// IsGoneStatus reports whether an HTTP status code indicates a dead push endpoint.
func IsGoneStatus(code int) bool {
	return code == 404 || code == 410
}

// Notification is one message addressed to a single recipient.
type Notification struct {
	ID             string                 `json:"id"`
	SchoolID       string                 `json:"school_id"`
	UserID         string                 `json:"user_id"`
	Type           string                 `json:"type"`
	Title          string                 `json:"title"`
	Body           string                 `json:"body"`
	Data           json.RawMessage        `json:"data"`
	IdempotencyKey *string                `json:"idempotency_key,omitempty"`
	ReadAt         *time.Time             `json:"read_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	Deliveries     []NotificationDelivery `json:"deliveries,omitempty"`
}

// Unread reports whether the notification has not been read yet.
func (n Notification) Unread() bool { return n.ReadAt == nil }

// NotificationPreference is a per-user, per-channel on/off switch.
type NotificationPreference struct {
	SchoolID  string    `json:"school_id"`
	UserID    string    `json:"user_id"`
	Channel   string    `json:"channel"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultPreference reports the effective default for a channel when the user
// has stored no row: the in-app inbox is always on, external channels are
// opt-in.
func DefaultPreference(channel string) bool { return channel == ChannelInApp }

// NotificationTemplate is one versioned message body.
type NotificationTemplate struct {
	ID        string    `json:"id"`
	SchoolID  string    `json:"school_id"`
	Key       string    `json:"key"`
	Version   int       `json:"version"`
	Channel   string    `json:"channel"`
	Subject   string    `json:"subject"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Delivery lifecycle states.
const (
	DeliveryPending = "pending"
	DeliverySent    = "sent"
	DeliveryFailed  = "failed"
)

// ValidDeliveryStatuses is the closed set enforced by the DB CHECK and by Go.
var ValidDeliveryStatuses = map[string]bool{
	DeliveryPending: true,
	DeliverySent:    true,
	DeliveryFailed:  true,
}

// NotificationDelivery is the ledger row for one (notification, channel).
type NotificationDelivery struct {
	ID                string    `json:"id"`
	NotificationID    string    `json:"notification_id"`
	Channel           string    `json:"channel"`
	Status            string    `json:"status"`
	Attempts          int       `json:"attempts"`
	LastError         *string   `json:"last_error,omitempty"`
	ProviderMessageID *string   `json:"provider_message_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Chat participant roles.
const (
	ChatRoleOwner  = "owner"
	ChatRoleMember = "member"
)

// ChatThread is a tenant-scoped conversation.
type ChatThread struct {
	ID            string            `json:"id"`
	SchoolID      string            `json:"school_id"`
	Subject       string            `json:"subject"`
	CreatedBy     string            `json:"created_by"`
	CreatedAt     time.Time         `json:"created_at"`
	ClosedAt      *time.Time        `json:"closed_at,omitempty"`
	Participants  []ChatParticipant `json:"participants,omitempty"`
	MessageCount  int               `json:"message_count"`
	LastMessageAt *time.Time        `json:"last_message_at,omitempty"`

	// T13 teacher–parent extension (kind general = the 0002 behaviour).
	Kind           string     `json:"kind"`
	StudentUserID  *string    `json:"student_user_id,omitempty"`
	TeacherUserID  *string    `json:"teacher_user_id,omitempty"`
	GuardianUserID *string    `json:"guardian_user_id,omitempty"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
}

// Chat thread kinds.
const (
	ChatKindGeneral       = "general"
	ChatKindTeacherParent = "teacher_parent"
)

// Closed reports whether the thread was closed.
func (t ChatThread) Closed() bool { return t.ClosedAt != nil }

// ChatParticipant is one member of a thread. Only participants may read or
// post in a thread.
type ChatParticipant struct {
	ThreadID string    `json:"thread_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// ChatMessage is one message in a thread.
type ChatMessage struct {
	ID            string    `json:"id"`
	ThreadID      string    `json:"thread_id"`
	SenderUserID  string    `json:"sender_user_id"`
	Body          string    `json:"body"`
	AttachmentKey *string   `json:"attachment_key,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	ReadBy        []string  `json:"read_by,omitempty"`
	ReadCount     int       `json:"read_count"`

	AttachmentName      *string `json:"attachment_name,omitempty"`
	AttachmentMime      *string `json:"attachment_mime,omitempty"`
	AttachmentSize      *int64  `json:"attachment_size,omitempty"`
	OutsideServiceHours bool    `json:"outside_service_hours"`
}

// ChatSettings is a school's chat configuration (FR-148-003, attachments).
type ChatSettings struct {
	SchoolID           string  `json:"school_id"`
	ServiceStart       *string `json:"service_start"`
	ServiceEnd         *string `json:"service_end"`
	ServiceDays        []int   `json:"service_days"`
	MaxAttachmentBytes int64   `json:"max_attachment_bytes"`
}

// DefaultMaxAttachmentBytes mirrors the DB default (5 MB, FR-16-002) for a
// school without a chat_settings row.
const DefaultMaxAttachmentBytes int64 = 5 << 20

// Chat report statuses.
const (
	ReportOpen      = "open"
	ReportReviewed  = "reviewed"
	ReportDismissed = "dismissed"
)

// ChatMessageReport is a report of an inappropriate message (FR-148-004).
type ChatMessageReport struct {
	ID             string     `json:"id"`
	SchoolID       string     `json:"school_id"`
	MessageID      string     `json:"message_id"`
	ThreadID       string     `json:"thread_id"`
	ReporterUserID string     `json:"reporter_user_id"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	ReviewedBy     *string    `json:"reviewed_by,omitempty"`
	ReviewNote     *string    `json:"review_note,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	MessageBody    string     `json:"message_body"`
	MessageSender  string     `json:"message_sender_user_id"`
}
