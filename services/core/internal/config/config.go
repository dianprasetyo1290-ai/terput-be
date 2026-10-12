package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration for the core service.
type Config struct {
	AppEnv             string
	Port               string
	DatabaseURL        string
	JWTSecret          string
	InternalKey        string
	CORSOrigins        []string
	NotificationURL    string
	IdentityURL        string
	HRURL              string
	JobIntervalSeconds int
	// CallbackMaxAttempts: deliveries to a source callback_url before the
	// request lands in the exception queue (kind=callback).
	CallbackMaxAttempts int
	// AuditExportMaxRows caps one CSV export of the audit trail.
	AuditExportMaxRows int
	SentryDSN          string
	SentryEnvironment  string
	// Service Desk (T11): ops work orders, private attachments, SLA clock zone.
	OpsURL         string
	StorageRoot    string
	MaxUploadBytes int64
	SchoolTimezone string
	// SelfInternalURL is core's own base URL, used for exception retry_url.
	SelfInternalURL string
}

// Load reads and validates configuration from environment variables.
func Load() (Config, error) {
	c := Config{
		AppEnv:              env("APP_ENV", "development"),
		Port:                env("PORT", "8091"),
		DatabaseURL:         env("DATABASE_URL", "postgres://teratai:change-me@localhost:5432/teratai_core?sslmode=disable"),
		JWTSecret:           os.Getenv("SEKOCI_JWT_SECRET"),
		InternalKey:         os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:         split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176")),
		NotificationURL:     os.Getenv("NOTIFICATION_URL"),
		IdentityURL:         os.Getenv("IDENTITY_URL"),
		HRURL:               os.Getenv("HR_URL"),
		JobIntervalSeconds:  integer("JOB_INTERVAL_SECONDS", 300),
		CallbackMaxAttempts: integer("CALLBACK_MAX_ATTEMPTS", 5),
		AuditExportMaxRows:  integer("AUDIT_EXPORT_MAX_ROWS", 10000),
		SentryDSN:           os.Getenv("SENTRY_DSN"),
		SentryEnvironment:   env("SENTRY_ENVIRONMENT", "development"),
		OpsURL:              os.Getenv("OPS_URL"),
		StorageRoot:         env("STORAGE_ROOT", "./data/private"),
		MaxUploadBytes:      int64(integer("MAX_UPLOAD_BYTES", 10<<20)),
		SchoolTimezone:      env("SCHOOL_TIMEZONE", "Asia/Jakarta"),
	}
	c.SelfInternalURL = env("CORE_INTERNAL_URL", "http://localhost:"+c.Port)

	if c.JobIntervalSeconds <= 0 || c.CallbackMaxAttempts <= 0 || c.AuditExportMaxRows <= 0 {
		return Config{}, errors.New("JOB_INTERVAL_SECONDS, CALLBACK_MAX_ATTEMPTS and AUDIT_EXPORT_MAX_ROWS must be positive")
	}

	production := strings.EqualFold(c.AppEnv, "production")

	if c.JWTSecret == "" && !production {
		c.JWTSecret = "change-me"
	}
	if c.InternalKey == "" && !production {
		c.InternalKey = "change-me"
	}

	if production {
		switch {
		case c.JWTSecret == "" || containsDev(c.JWTSecret):
			return Config{}, errors.New("SEKOCI_JWT_SECRET must be explicitly configured in production")
		case c.InternalKey == "" || containsDev(c.InternalKey):
			return Config{}, errors.New("SEKOCI_INTERNAL_KEY must be explicitly configured in production")
		case c.IdentityURL == "":
			return Config{}, errors.New("IDENTITY_URL must be explicitly configured in production")
		}
	}
	return c, nil
}

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

func integer(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
