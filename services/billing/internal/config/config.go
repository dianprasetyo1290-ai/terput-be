// Package config loads billing-service configuration from the environment.
// It fails fast at startup when a production deployment is missing a secret
// that must never fall back to a development default.
package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config is the resolved runtime configuration.
type Config struct {
	AppEnv      string
	Port        string
	DatabaseURL string

	// JWTSecret verifies the shared identity JWT (HS256). InternalKey
	// authenticates service-to-service calls (X-Internal-Key).
	JWTSecret   string
	InternalKey string

	CORSOrigins []string

	// IdentityURL is the base URL of identity-service, used to verify that a
	// guardian really owns the child whose bills they are reading.
	IdentityURL string

	// AcademicURL is the base URL of academic-service, used to limit a
	// guru/walikelas to the bills of students in their own classes. Empty
	// means teachers are denied (fail closed).
	AcademicURL string

	// StorageRoot is the private directory that holds transfer proofs. Files
	// are never served directly; they stream through authorized endpoints.
	StorageRoot    string
	MaxUploadBytes int64

	SentryDSN         string
	SentryEnvironment string

	Gateway Gateway

	Wallet Wallet // T12
}

// Load reads and validates the configuration.
func Load() (Config, error) {
	c := Config{
		AppEnv:            env("APP_ENV", "development"),
		Port:              env("PORT", "8090"),
		DatabaseURL:       env("DATABASE_URL", "postgres://teratai:change-me@localhost:5490/teratai_billing?sslmode=disable"),
		JWTSecret:         os.Getenv("SEKOCI_JWT_SECRET"),
		InternalKey:       os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:       split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		IdentityURL:       os.Getenv("IDENTITY_URL"),
		AcademicURL:       os.Getenv("ACADEMIC_URL"),
		StorageRoot:       env("STORAGE_ROOT", "./data/private"),
		MaxUploadBytes:    int64(integer("MAX_UPLOAD_BYTES", 10<<20)),
		SentryDSN:         os.Getenv("SENTRY_DSN"),
		SentryEnvironment: env("SENTRY_ENVIRONMENT", "development"),
	}

	production := strings.EqualFold(c.AppEnv, "production")

	gw, err := loadGateway()
	if err != nil {
		return Config{}, err
	}
	c.Gateway = gw
	if c.Gateway.BillingInternalURL == "" {
		// Dev default only; in docker set BILLING_INTERNAL_URL=http://billing:<port>.
		c.Gateway.BillingInternalURL = "http://localhost:" + c.Port
	}

	if c.JWTSecret == "" && !production {
		c.JWTSecret = "change-me"
	}
	if c.InternalKey == "" && !production {
		c.InternalKey = "change-me"
	}

	c.Wallet = loadWallet(c.JWTSecret) // T12

	if production {
		switch {
		case c.JWTSecret == "" || containsDev(c.JWTSecret):
			return Config{}, errors.New("SEKOCI_JWT_SECRET must be explicitly configured in production")
		case c.InternalKey == "" || containsDev(c.InternalKey):
			return Config{}, errors.New("SEKOCI_INTERNAL_KEY must be explicitly configured in production")
		case c.IdentityURL == "":
			return Config{}, errors.New("IDENTITY_URL must be explicitly configured in production")
		case c.Gateway.SimulatorEnabled:
			return Config{}, errors.New("PAYMENT_SIMULATOR_ENABLED is not allowed in production")
		}
	}
	return c, nil
}

// Production reports whether the service runs with production guards.
func (c Config) Production() bool { return strings.EqualFold(c.AppEnv, "production") }

// SimulatorAllowed reports whether the payment simulator adapter may run:
// only when explicitly enabled and never in production (QA-PAY BUG-004).
func (c Config) SimulatorAllowed() bool { return c.Gateway.SimulatorEnabled && !c.Production() }

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

func integer(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
