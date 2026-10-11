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
// notification_preferences.
const (
	ChannelInApp    = "in_app"
	ChannelEmail    = "email"
	ChannelWhatsApp = "whatsapp"
)

// ValidChannels is the closed set stored in every channel column.
var ValidChannels = map[string]bool{
	ChannelInApp:    true,
	ChannelEmail:    true,
	ChannelWhatsApp: true,
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
}

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
}
