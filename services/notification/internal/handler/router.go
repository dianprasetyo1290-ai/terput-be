package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/notification/internal/campaign"
	"skci/notification/internal/common"
)

// RouterConfig is the router's environment wiring.
type RouterConfig struct {
	JWTSecret   string
	InternalKey string
	CORSOrigins []string

	// Campaign mounts menu 147 routes (nil in tests that do not need them).
	Campaign *campaign.Handler
}

// NewRouter builds the HTTP handler.
//
//   - /healthz is unauthenticated.
//   - everything else requires either a user JWT (portal inbox, preferences,
//     chat) or the shared X-Internal-Key (internal send, template admin,
//     delivery retry) — the two groups never overlap.
func NewRouter(h *Handler, cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID, common.CorrelationMiddleware, middleware.RealIP, middleware.Logger)
	r.Use(common.SentryMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "Idempotency-Key", common.CorrelationHeader},
		ExposedHeaders:   []string{common.CorrelationHeader},
		AllowCredentials: true,
	}))

	r.Get("/healthz", Healthz)

	// Portal (end-user JWT): inbox, preferences, chat.
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(cfg.JWTSecret))

		pr.Get("/notifications", h.ListNotifications)
		pr.Get("/notifications/unread-count", h.UnreadCount)
		pr.Post("/notifications/read-all", h.MarkAllRead)
		pr.Post("/notifications/{id}/read", h.MarkRead)
		pr.Get("/notifications/{id}/deliveries", h.ListNotificationDeliveries)

		pr.Get("/notification-preferences", h.GetPreferences)
		pr.Put("/notification-preferences", h.PutPreferences)

		// T26 push notifications (FR-159-004)
		pr.Get("/push/config", h.GetPushConfig)
		pr.Post("/push/subscriptions", h.SubscribePush)
		pr.Delete("/push/subscriptions", h.UnsubscribePush)

		pr.Get("/chat/threads", h.ListThreads)
		pr.Post("/chat/threads", h.CreateThread)
		pr.Get("/chat/threads/{id}/messages", h.ListMessages)
		pr.Post("/chat/threads/{id}/messages", h.CreateMessage)
		pr.Post("/chat/threads/{id}/read", h.MarkThreadRead)

		// T13 teacher–parent chat (FR-148-*).
		pr.Post("/chat/teacher-parent/threads", h.CreateTeacherParentThread)
		pr.Get("/chat/teacher-parent/students", h.TeacherParentStudents)
		pr.Get("/chat/teacher-parent/contacts", h.TeacherParentContacts)
		pr.Post("/chat/threads/{id}/attachments", h.UploadAttachment)
		pr.Post("/chat/threads/{id}/archive", h.ArchiveThread)
		pr.Get("/chat/messages/{id}/attachment", h.DownloadAttachment)
		pr.Post("/chat/messages/{id}/report", h.ReportMessage)
		pr.Get("/chat/moderation/reports", h.ListReports)
		pr.Get("/chat/moderation/reports/{id}", h.GetReport)
		pr.Post("/chat/moderation/reports/{id}/review", h.ReviewReport)
		pr.Get("/notification-channels/status", h.ChannelStatus)
		pr.Get("/notification-templates", h.AdminListTemplates)
		pr.Get("/notification-templates/variables", h.AdminTemplateVariables)
		pr.Post("/notification-templates", h.AdminCreateTemplate)
		pr.Delete("/notification-templates/{id}", h.AdminDeleteTemplate)
		pr.Get("/chat-settings", h.GetChatSettings)
		pr.Put("/chat-settings", h.PutChatSettings)

		if cfg.Campaign != nil {
			cfg.Campaign.Mount(pr)
		}
	})

	// T13 public + provider endpoints (no JWT).
	if cfg.Campaign != nil {
		r.Get("/public/unsubscribe", cfg.Campaign.PublicUnsubscribe)
		r.Post("/webhooks/whatsapp", cfg.Campaign.WhatsAppWebhook)
	}

	// Service-to-service (X-Internal-Key): send notifications, manage
	// templates, retry failed deliveries.
	r.Group(func(ir chi.Router) {
		ir.Use(common.InternalOnlyMiddleware(cfg.InternalKey))

		ir.Post("/internal/notifications", h.InternalSendNotification)
		ir.Post("/internal/notifications/{id}/retry", h.InternalRetryDelivery)

		ir.Get("/internal/notification-templates", h.InternalListTemplates)
		ir.Post("/internal/notification-templates", h.InternalCreateTemplate)
		ir.Get("/internal/notification-templates/{id}", h.InternalGetTemplate)
		ir.Put("/internal/notification-templates/{id}", h.InternalUpdateTemplate)
		ir.Delete("/internal/notification-templates/{id}", h.InternalDeleteTemplate)

		if cfg.Campaign != nil {
			ir.Post("/internal/marketing-consents", cfg.Campaign.InternalSetConsent)
		}
	})

	return r
}
