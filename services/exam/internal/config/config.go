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
	IdentityURL       string
	AcademicURL       string
	CORSOrigins       []string
	SentryDSN         string
	SentryEnvironment string
}

func Load() Config {
	return Config{
		Port:              getEnv("PORT", "8084"),
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://sekoci:change-me@localhost:5484/skci_exam?sslmode=disable"),
		JWTSecret:         getEnv("SEKOCI_JWT_SECRET", "change-me"),
		InternalKey:       getEnv("SEKOCI_INTERNAL_KEY", "change-me"),
		IdentityURL:       getEnv("IDENTITY_URL", "http://identity:8081"),
		AcademicURL:       getEnv("ACADEMIC_URL", "http://academic:8082"),
		CORSOrigins:       corsOrigins(),
		SentryDSN:         getEnv("SENTRY_DSN", ""),
		SentryEnvironment: getEnv("SENTRY_ENVIRONMENT", "development"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// corsOrigins reads a comma-separated CORS_ALLOWED_ORIGINS so the allowed
// dev-server ports (one per frontend domain agent, see
// docs/FRONTEND_CONVENTIONS.md) are configured once via docker-compose
// rather than hardcoded per service — scales to more origins without a
// code change or rebuild. Falls back to the legacy singular CORS_ORIGIN if
// set, then to the known dev ports.
func corsOrigins() []string {
	raw := getEnv("CORS_ALLOWED_ORIGINS", getEnv("CORS_ORIGIN", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178"))
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
