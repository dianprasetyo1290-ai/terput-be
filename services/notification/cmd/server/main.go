// Command server is notification-service's HTTP entrypoint: the portal inbox,
// per-user notification preferences, internal service-to-service sends,
// template administration, delivery bookkeeping and tenant-scoped chat.
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"github.com/joho/godotenv"

	"skci/notification/internal/campaign"
	"skci/notification/internal/client"
	"skci/notification/internal/common"
	"skci/notification/internal/config"
	"skci/notification/internal/handler"
	"skci/notification/internal/jobs"
	"skci/notification/internal/provider"
	"skci/notification/internal/storage"
	"skci/notification/internal/store"
)

// maxAttachmentCeiling is the storage-level hard cap; the per-school limit
// (chat_settings.max_attachment_bytes) is enforced before it in the handler.
const maxAttachmentCeiling = 50 << 20

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "notification-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Close()

	prod := cfg.Production()
	delivery, err := provider.New(cfg.DeliveryProvider, prod)
	if err != nil {
		log.Fatalf("delivery provider: %v", err)
	}
	if prod && delivery.Name() == "dev-noop" {
		log.Fatal("refusing to start: a no-op delivery provider is not allowed in production")
	}
	router := provider.ChannelRouter{Default: delivery}
	if cfg.EmailProvider == "smtp" {
		router.Email, err = provider.NewSMTP(provider.SMTPConfig{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword, From: cfg.SMTPFrom, ImplicitTLS: cfg.SMTPImplicitTLS})
		if err != nil {
			log.Fatalf("email provider: %v", err)
		}
	}
	if cfg.WhatsAppNotifyProvider == "http" {
		router.WhatsApp, err = provider.NewHTTPWhatsApp(provider.HTTPWhatsAppConfig{URL: cfg.WAAPIURL, Token: cfg.WAAPIToken})
		if err != nil {
			log.Fatalf("whatsapp notification provider: %v", err)
		}
	}
	delivery = router

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("SCHOOL_TIMEZONE: %v", err)
	}
	whatsapp, err := provider.NewWhatsApp(cfg.WhatsAppProvider, cfg.WhatsAppWebhookSecret, cfg.WhatsAppStopKeywords)
	if err != nil {
		log.Fatalf("whatsapp provider: %v", err)
	}

	identity := client.New(cfg.IdentityURL, cfg.InternalKey)
	academic := client.New(cfg.AcademicURL, cfg.InternalKey)
	core := client.New(cfg.CoreURL, cfg.InternalKey)
	audit := func(ctx context.Context, e client.AuditEvent) {
		if err := core.Audit(ctx, e); err != nil {
			log.Printf("core audit (%s %s): %v", e.EntityType, e.Action, err)
		}
	}

	st := store.New(db)
	h := handler.New(st, identity, delivery, prod)
	h.Relations = client.Relations{Identity: identity, Academic: academic}
	h.Attachments = storage.NewLocal(cfg.StorageRoot, maxAttachmentCeiling)
	h.Audit = audit
	exceptions := common.NewExceptionReporter(cfg.CoreURL, cfg.InternalKey, "notification")
	h.Exceptions = exceptions.Report
	h.InternalURL = cfg.InternalURL
	h.Location = loc
	h.PushProvider = provider.NewWebPushProvider(provider.WebPushConfig{
		VAPIDPublicKey:  cfg.WebPushVAPIDPublicKey,
		VAPIDPrivateKey: cfg.WebPushVAPIDPrivateKey,
		Subject:         cfg.WebPushSubject,
		TTLSeconds:      cfg.WebPushTTLSeconds,
	})

	campaignSvc := &campaign.Service{
		Store:  campaign.NewStore(db),
		InApp:  st,
		Source: campaign.ClientSource{Identity: identity, Academic: academic, Admission: client.New(cfg.AdmissionURL, cfg.InternalKey)},
		Email:  delivery, WhatsApp: whatsapp, Audit: audit,
		UnsubscribeSecret: cfg.UnsubscribeSecret, PublicURL: cfg.PublicURL, Loc: loc,
		Exceptions: exceptions.Report,
	}
	runner := jobs.NewRunner(db, cfg.CampaignJobInterval)
	runner.Register("campaign_dispatch", func(ctx context.Context, _ *sql.DB) error { return campaignSvc.RunDue(ctx) })
	runner.Start()
	defer runner.Stop()

	httpRouter := handler.NewRouter(h, handler.RouterConfig{
		JWTSecret:   cfg.JWTSecret,
		InternalKey: cfg.InternalKey,
		CORSOrigins: cfg.CORSOrigins,
		Campaign:    campaign.NewHandler(campaignSvc),
	})

	log.Printf("notification-service listening on :%s (%s, provider=%s, whatsapp=%v)", cfg.Port, cfg.AppEnv, delivery.Name(), whatsapp != nil)
	if err := http.ListenAndServe(":"+cfg.Port, httpRouter); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
