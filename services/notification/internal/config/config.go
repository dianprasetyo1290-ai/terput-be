// Package config loads notification-service configuration from the
// environment. In APP_ENV=production it fails fast when a secret is missing or
// still carries a development default, so a misconfigured deploy refuses to
// start instead of silently running on shared dev keys.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the resolved runtime configuration.
type Config struct {
	AppEnv      string
	Port        string
	DatabaseURL string

	// Identity JWT verification (HS256) and the shared service-to-service key.
	JWTSecret   string
	InternalKey string

	CORSOrigins []string

	// DeliveryProvider selects the outbound channel adapter: "dev" (no-op,
	// development only), "noop" (explicit no-op) or "unconfigured" (production
	// refuses to send on email/whatsapp until a real provider is wired).
	DeliveryProvider string

	IdentityURL string

	// BUG-029: real per-channel adapters. EmailProvider "smtp" uses SMTP_*;
	// WhatsAppNotifyProvider "http" uses WA_API_URL/WA_API_TOKEN. Empty means
	// the channel falls back to DeliveryProvider (unconfigured in production:
	// every send is recorded as failed, never as a fake success).
	EmailProvider          string
	SMTPHost               string
	SMTPPort               int
	SMTPUsername           string
	SMTPPassword           string
	SMTPFrom               string
	SMTPImplicitTLS        bool
	WhatsAppNotifyProvider string
	WAAPIURL               string
	WAAPIToken             string

	// InternalURL is this service's base URL as other services reach it
	// (e.g. http://notification:8089). Used as retry_url of a failed delivery
	// reported to core's exception queue; empty = no retry button.
	InternalURL string

	// T13: sibling services read by the campaign segment resolver, the
	// teacher–parent relation checks and the central audit log.
	AcademicURL  string
	AdmissionURL string
	CoreURL      string

	// WhatsAppProvider selects the WhatsApp template adapter (IF-ADV-02,
	// OQ-38). Empty = disabled (WhatsApp campaigns rejected with 409); "noop"
	// is allowed for development and tests only.
	WhatsAppProvider      string
	WhatsAppWebhookSecret string
	WhatsAppStopKeywords  []string

	// UnsubscribeSecret signs the HMAC token in every campaign email's
	// unsubscribe link; PublicURL is the browser-facing base of this service.
	UnsubscribeSecret string
	PublicURL         string

	CampaignJobInterval time.Duration
	StorageRoot         string
	// Timezone is the zone service hours are evaluated in.
	Timezone string

	// T26: WebPush configuration
	WebPushVAPIDPublicKey  string
	WebPushVAPIDPrivateKey string
	WebPushSubject         string
	WebPushTTLSeconds      int

	SentryDSN         string
	SentryEnvironment string
}

// Load reads and validates the configuration.
func Load() (Config, error) {
	c := Config{
		AppEnv:                 env("APP_ENV", "development"),
		Port:                   env("PORT", "8089"),
		DatabaseURL:            env("DATABASE_URL", "postgres://teratai:change-me@localhost:5489/teratai_notification?sslmode=disable"),
		JWTSecret:              os.Getenv("SEKOCI_JWT_SECRET"),
		InternalKey:            os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:            split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178")),
		DeliveryProvider:       os.Getenv("NOTIFICATION_PROVIDER"),
		IdentityURL:            os.Getenv("IDENTITY_URL"),
		EmailProvider:          strings.ToLower(strings.TrimSpace(os.Getenv("NOTIFICATION_EMAIL_PROVIDER"))),
		SMTPHost:               os.Getenv("SMTP_HOST"),
		SMTPPort:               positiveInt("SMTP_PORT", 587),
		SMTPUsername:           os.Getenv("SMTP_USERNAME"),
		SMTPPassword:           os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:               os.Getenv("SMTP_FROM"),
		SMTPImplicitTLS:        strings.EqualFold(os.Getenv("SMTP_TLS"), "implicit"),
		WhatsAppNotifyProvider: strings.ToLower(strings.TrimSpace(os.Getenv("NOTIFICATION_WHATSAPP_PROVIDER"))),
		WAAPIURL:               os.Getenv("WA_API_URL"),
		WAAPIToken:             os.Getenv("WA_API_TOKEN"),
		InternalURL:            strings.TrimRight(os.Getenv("NOTIFICATION_INTERNAL_URL"), "/"),
		AcademicURL:            os.Getenv("ACADEMIC_URL"),
		AdmissionURL:           os.Getenv("ADMISSION_URL"),
		CoreURL:                os.Getenv("CORE_URL"),
		WhatsAppProvider:       strings.ToLower(strings.TrimSpace(os.Getenv("WHATSAPP_PROVIDER"))),
		WhatsAppWebhookSecret:  os.Getenv("WHATSAPP_WEBHOOK_SECRET"),
		WhatsAppStopKeywords:   split(env("WHATSAPP_STOP_KEYWORDS", "STOP,BERHENTI")),
		UnsubscribeSecret:      os.Getenv("CAMPAIGN_UNSUBSCRIBE_SECRET"),
		PublicURL:              strings.TrimRight(env("NOTIFICATION_PUBLIC_URL", "http://localhost:8089"), "/"),
		CampaignJobInterval:    time.Duration(positiveInt("CAMPAIGN_JOB_INTERVAL_SECONDS", 60)) * time.Second,
		StorageRoot:            env("STORAGE_ROOT", "./data/private"),
		Timezone:               env("SCHOOL_TIMEZONE", "Asia/Jakarta"),
		WebPushVAPIDPublicKey:  os.Getenv("WEBPUSH_VAPID_PUBLIC_KEY"),
		WebPushVAPIDPrivateKey: os.Getenv("WEBPUSH_VAPID_PRIVATE_KEY"),
		WebPushSubject:         env("WEBPUSH_SUBJECT", "mailto:admin@sekoci.id"),
		WebPushTTLSeconds:      positiveInt("WEBPUSH_TTL_SECONDS", 86400),
		SentryDSN:              os.Getenv("SENTRY_DSN"),
		SentryEnvironment:      env("SENTRY_ENVIRONMENT", "development"),
	}

	production := c.Production()

	if c.JWTSecret == "" && !production {
		c.JWTSecret = "change-me"
	}
	if c.InternalKey == "" && !production {
		c.InternalKey = "change-me"
	}
	if c.UnsubscribeSecret == "" && !production {
		c.UnsubscribeSecret = "sekoci-dev-unsubscribe-secret-change-me"
	}
	if c.WhatsAppWebhookSecret == "" && !production {
		c.WhatsAppWebhookSecret = "sekoci-dev-wa-webhook-change-me"
	}
	if c.DeliveryProvider == "" {
		if production {
			c.DeliveryProvider = "unconfigured"
		} else {
			c.DeliveryProvider = "dev"
		}
	}

	switch c.EmailProvider {
	case "", "smtp":
	default:
		return Config{}, errors.New("NOTIFICATION_EMAIL_PROVIDER must be empty or smtp")
	}
	switch c.WhatsAppNotifyProvider {
	case "", "http":
	default:
		return Config{}, errors.New("NOTIFICATION_WHATSAPP_PROVIDER must be empty or http")
	}

	if production {
		switch {
		case c.JWTSecret == "" || containsDev(c.JWTSecret):
			return Config{}, errors.New("SEKOCI_JWT_SECRET must be explicitly configured in production")
		case c.InternalKey == "" || containsDev(c.InternalKey):
			return Config{}, errors.New("SEKOCI_INTERNAL_KEY must be explicitly configured in production")
		case strings.EqualFold(c.DeliveryProvider, "dev"):
			return Config{}, errors.New("NOTIFICATION_PROVIDER=dev is forbidden in production")
		case c.UnsubscribeSecret == "" || containsDev(c.UnsubscribeSecret):
			return Config{}, errors.New("CAMPAIGN_UNSUBSCRIBE_SECRET must be explicitly configured in production")
		case c.WhatsAppProvider == "noop" || c.WhatsAppProvider == "dev":
			return Config{}, errors.New("WHATSAPP_PROVIDER=noop/dev is forbidden in production")
		case c.WhatsAppProvider != "" && (c.WhatsAppWebhookSecret == "" || containsDev(c.WhatsAppWebhookSecret)):
			return Config{}, errors.New("WHATSAPP_WEBHOOK_SECRET must be explicitly configured when WHATSAPP_PROVIDER is set")
		}
	}
	return c, nil
}

// Production reports whether the service runs with production guards.
func (c Config) Production() bool { return strings.EqualFold(c.AppEnv, "production") }

func containsDev(v string) bool {
	return strings.Contains(v, "dev-") || strings.Contains(v, "change-me")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func split(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func positiveInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
