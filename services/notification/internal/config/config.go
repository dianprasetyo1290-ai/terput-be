// Package config loads notification-service configuration from the
// environment. In APP_ENV=production it fails fast when a secret is missing or
// still carries a development default, so a misconfigured deploy refuses to
// start instead of silently running on shared dev keys.
package config

import (
	"errors"
	"os"
	"strings"
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

	SentryDSN         string
	SentryEnvironment string
}

// Load reads and validates the configuration.
func Load() (Config, error) {
	c := Config{
		AppEnv:            env("APP_ENV", "development"),
		Port:              env("PORT", "8089"),
		DatabaseURL:       env("DATABASE_URL", "postgres://teratai:change-me@localhost:5489/teratai_notification?sslmode=disable"),
		JWTSecret:         os.Getenv("SEKOCI_JWT_SECRET"),
		InternalKey:       os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:       split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178")),
		DeliveryProvider:  os.Getenv("NOTIFICATION_PROVIDER"),
		IdentityURL:       os.Getenv("IDENTITY_URL"),
		SentryDSN:         os.Getenv("SENTRY_DSN"),
		SentryEnvironment: env("SENTRY_ENVIRONMENT", "development"),
	}

	production := c.Production()

	if c.JWTSecret == "" && !production {
		c.JWTSecret = "change-me"
	}
	if c.InternalKey == "" && !production {
		c.InternalKey = "change-me"
	}
	if c.DeliveryProvider == "" {
		if production {
			c.DeliveryProvider = "unconfigured"
		} else {
			c.DeliveryProvider = "dev"
		}
	}

	if production {
		switch {
		case c.JWTSecret == "" || containsDev(c.JWTSecret):
			return Config{}, errors.New("SEKOCI_JWT_SECRET must be explicitly configured in production")
		case c.InternalKey == "" || containsDev(c.InternalKey):
			return Config{}, errors.New("SEKOCI_INTERNAL_KEY must be explicitly configured in production")
		case strings.EqualFold(c.DeliveryProvider, "dev"):
			return Config{}, errors.New("NOTIFICATION_PROVIDER=dev is forbidden in production")
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
