package config

import (
	"os"
	"strings"
)

type Config struct {
	Port              string
	DatabaseURL       string
	JWTSecret         string
	InternalKey       string
	IdentityBaseURL   string
	AcademicBaseURL   string
	CORSOrigins       []string
	SentryDSN         string
	SentryEnvironment string
	RedisURL          string
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// corsOrigins reads a comma-separated CORS_ALLOWED_ORIGINS so the allowed
// dev-server ports (one per frontend domain agent, see
// docs/FRONTEND_CONVENTIONS.md) are configured once via docker-compose
// rather than hardcoded per service — scales to more origins without a
// code change or rebuild. Falls back to the legacy singular
// CORS_ALLOWED_ORIGIN if set, then to the known dev ports.
func corsOrigins() []string {
	raw := getenv("CORS_ALLOWED_ORIGINS", getenv("CORS_ALLOWED_ORIGIN", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178"))
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func Load() Config {
	return Config{
		Port:              getenv("PORT", "8085"),
		DatabaseURL:       getenv("DATABASE_URL", "postgres://sekoci:change-me@localhost:5485/skci_evaluation?sslmode=disable"),
		JWTSecret:         getenv("SEKOCI_JWT_SECRET", "change-me"),
		InternalKey:       getenv("SEKOCI_INTERNAL_KEY", "change-me"),
		IdentityBaseURL:   getenv("IDENTITY_BASE_URL", "http://localhost:8081"),
		AcademicBaseURL:   getenv("ACADEMIC_BASE_URL", "http://localhost:8082"),
		CORSOrigins:       corsOrigins(),
		SentryDSN:         getenv("SENTRY_DSN", ""),
		SentryEnvironment: getenv("SENTRY_ENVIRONMENT", "development"),
		RedisURL:          getenv("REDIS_URL", ""),
	}
}
