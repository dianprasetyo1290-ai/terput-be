// Package handler implements notification-service's REST API: the portal
// inbox, per-user channel preferences, internal service-to-service sends with
// idempotent delivery bookkeeping, template administration, and tenant-scoped
// chat with strict participant authorization.
package handler

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/notification/internal/client"
	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/provider"
	"skci/notification/internal/storage"
	"skci/notification/internal/store"
)

// DataStore is the persistence surface the handlers use. *store.Store is the
// production implementation; tests provide an in-memory fake so authorization
// and idempotency rules are exercised without a database.
type DataStore interface {
	// notifications
	CreateNotification(ctx context.Context, in store.CreateNotificationInput) (*model.Notification, bool, error)
	GetNotification(ctx context.Context, id, schoolID, userID string) (*model.Notification, error)
	GetNotificationByID(ctx context.Context, id, schoolID string) (*model.Notification, error)
	ListNotifications(ctx context.Context, f store.NotificationFilter) ([]model.Notification, int, error)
	CountUnreadNotifications(ctx context.Context, schoolID, userID string) (int, error)
	MarkNotificationRead(ctx context.Context, id, schoolID, userID string) (*model.Notification, error)
	MarkAllNotificationsRead(ctx context.Context, schoolID, userID string) (int, error)

	// preferences
	ListNotificationPreferences(ctx context.Context, schoolID, userID string) ([]model.NotificationPreference, error)
	UpsertNotificationPreferences(ctx context.Context, schoolID, userID string, prefs []store.PreferenceInput) ([]model.NotificationPreference, error)
	IsChannelEnabled(ctx context.Context, schoolID, userID, channel string) (bool, error)

	// templates
	ListNotificationTemplates(ctx context.Context, schoolID, key string) ([]model.NotificationTemplate, error)
	GetNotificationTemplate(ctx context.Context, id, schoolID string) (*model.NotificationTemplate, error)
	CreateNotificationTemplate(ctx context.Context, in store.TemplateInput) (*model.NotificationTemplate, error)
	UpdateNotificationTemplate(ctx context.Context, id, schoolID string, in store.TemplateInput) (*model.NotificationTemplate, error)
	DeleteNotificationTemplate(ctx context.Context, id, schoolID string) error
	FindNotificationTemplate(ctx context.Context, schoolID, key, channel string) (*model.NotificationTemplate, error)

	// deliveries
	CreateDelivery(ctx context.Context, notificationID, channel string) (*model.NotificationDelivery, error)
	ListDeliveries(ctx context.Context, notificationID string) ([]model.NotificationDelivery, error)
	UpdateDelivery(ctx context.Context, id string, in store.UpdateDeliveryInput) (*model.NotificationDelivery, error)

	// chat
	CreateChatThread(ctx context.Context, in store.CreateThreadInput) (*model.ChatThread, error)
	ListChatThreadsForUser(ctx context.Context, schoolID, userID string, page, perPage int) ([]model.ChatThread, int, error)
	GetChatThread(ctx context.Context, id, schoolID string) (*model.ChatThread, error)
	IsChatParticipant(ctx context.Context, threadID, userID string) (bool, error)
	ListChatMessages(ctx context.Context, threadID string, page, perPage int) ([]model.ChatMessage, int, error)
	CreateChatMessage(ctx context.Context, in store.CreateMessageInput) (*model.ChatMessage, error)
	MarkChatThreadRead(ctx context.Context, threadID, userID string) ([]string, error)

	// teacher–parent chat (T13)
	FindActiveTeacherParentThread(ctx context.Context, schoolID, teacherID, guardianID, studentID string) (*model.ChatThread, error)
	ArchiveChatThread(ctx context.Context, threadID string) error
	GetChatMessage(ctx context.Context, id, schoolID string) (*model.ChatMessage, error)
	GetChatSettings(ctx context.Context, schoolID string) (*model.ChatSettings, error)
	UpsertChatSettings(ctx context.Context, in model.ChatSettings) error
	CreateChatReport(ctx context.Context, schoolID, messageID, reporterID, reason string) (*model.ChatMessageReport, error)
	GetChatReport(ctx context.Context, id, schoolID string) (*model.ChatMessageReport, error)
	ListChatReports(ctx context.Context, schoolID, status string, page, perPage int) ([]model.ChatMessageReport, int, error)
	ReviewChatReport(ctx context.Context, id, schoolID, status, note, reviewerID string) (*model.ChatMessageReport, error)

	// push subscriptions (T26)
	UpsertPushSubscription(ctx context.Context, sub model.PushSubscription) (*model.PushSubscription, error)
	DeletePushSubscription(ctx context.Context, schoolID, userID, endpoint string) error
	ListActivePushSubscriptions(ctx context.Context, schoolID, userID string) ([]model.PushSubscription, error)
	RevokePushSubscription(ctx context.Context, endpoint string) error
	TouchPushSubscription(ctx context.Context, endpoint string) error
	GetUserPushPreference(ctx context.Context, schoolID, userID string) (*model.NotificationPreference, error)
}

// Relations answers BR-COM-02 relation questions (identity + academic).
// client.Relations is the production implementation; tests use a fake.
// Every method must fail closed: on error the caller denies.
type Relations interface {
	UserRoles(ctx context.Context, schoolID, userID string) ([]string, error)
	UserName(ctx context.Context, userID string) string
	TeacherStudentIDs(ctx context.Context, schoolID, teacherID string) (map[string]bool, error)
	GuardianHasAccess(ctx context.Context, schoolID, guardianID, studentID string) (bool, error)
	GuardianChildren(ctx context.Context, schoolID, guardianID string) ([]string, error)
	StudentGuardians(ctx context.Context, schoolID, studentID string) ([]client.StudentGuardian, error)
	StudentTeachers(ctx context.Context, schoolID, studentID string) ([]client.StudentTeacher, error)
	UsersByRole(ctx context.Context, schoolID, role string) ([]client.UserProfile, error)
}

// Handler carries every dependency the HTTP handlers need.
type Handler struct {
	Store        DataStore
	Client       *client.Client
	Provider     provider.Provider
	PushProvider provider.PushSender
	Production   bool

	// T13 chat dependencies, set by main (and tests) after New.
	Relations   Relations
	Attachments storage.Storage
	Audit       func(ctx context.Context, e client.AuditEvent)
	Location    *time.Location
	Now         func() time.Time

	// BUG-113: failed external deliveries go to core's exception queue.
	// InternalURL (this service as core reaches it) builds the retry_url.
	Exceptions  func(ctx context.Context, e common.ExceptionReport) error
	InternalURL string
	// CampaignWhatsApp is the marketing WA adapter (nil = disabled); only
	// its configured state is reported.
	CampaignWhatsApp provider.TemplateSender
	wg               sync.WaitGroup
}

// New wires the handler.
func New(s DataStore, c *client.Client, p provider.Provider, production bool) *Handler {
	return &Handler{Store: s, Client: c, Provider: p, Production: production}
}

// param reads a chi path parameter.
func param(r *http.Request, key string) string { return chi.URLParam(r, key) }

// effectiveSchoolID applies CONVENTIONS.md's multi-tenancy rule: a normal
// caller is always scoped to their JWT school_id; a superadmin (whose token has
// no school_id) may name one with ?school_id=.
func effectiveSchoolID(r *http.Request, claims *common.Claims) string {
	if claims.ActiveRole == "superadmin" {
		if s := strings.TrimSpace(r.URL.Query().Get("school_id")); s != "" {
			return s
		}
	}
	return claims.SchoolID
}

// requireSchool writes a 400 and reports false when a school scope cannot be
// resolved (e.g. a superadmin calling a tenant route without ?school_id=).
func requireSchool(w http.ResponseWriter, schoolID string) bool {
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school scope is required (superadmin must pass ?school_id=)")
		return false
	}
	return true
}

// strptr returns a pointer to v, or nil for an empty string.
func strptr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
