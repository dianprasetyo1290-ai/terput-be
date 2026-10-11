package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultDevParticipantSecret is the well-known, non-secret HMAC key used to
// sign applicant CBT participant tokens when EXAM_PARTICIPANT_SECRET is not
// configured. It exists only so a developer can `go run ./cmd/server` and hit
// the /participant/** routes without extra setup — it is committed to the
// repository (see the root .env.example, which ships the same value), so
// Config.Validate refuses to start with it whenever the process is running in
// production. An applicant token is a full exam attempt: whoever can mint one
// for an applicant_id can sit (and submit) that applicant's exam.
const DefaultDevParticipantSecret = "change-me"

// DevSecretMarker is the substring every committed dev secret in this repo
// carries. Validate refuses it in production, matching services/admission's
// existing rule for its own secrets, so copying .env.example into a production
// .env is caught even if the value is edited slightly.
const DevSecretMarker = "dev-"

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
	RedisURL          string
	AppEnv            string
	ParticipantSecret string
	// ParticipantTokenTTL is the default lifetime of an applicant participant
	// token: PARTICIPANT_TOKEN_TTL_SECONDS, clamped to the same 60..3600s
	// window internal/handler/admission.go enforces per request.
	ParticipantTokenTTL time.Duration
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
		RedisURL:          getEnv("REDIS_URL", ""),
		AppEnv:            getEnv("APP_ENV", "development"),
		// ParticipantSecret signs the short-lived, exam-scoped JWT an admission
		// applicant uses on /participant/** (see internal/common/participant_auth.go).
		// Deliberately NOT the user JWT secret: an applicant token must never be
		// accepted by the user-JWT routes (and vice versa), which is only
		// guaranteed when the two are different keys. See Validate.
		ParticipantSecret:   participantSecret(),
		ParticipantTokenTTL: participantTTL(),
	}
}

// participantTTLSeconds bounds mirror internal/handler/admission.go's
// per-request guard (default 900, never above 3600) so a misconfigured
// environment cannot widen the window.
func participantTTL() time.Duration {
	secs, err := strconv.Atoi(getEnv("PARTICIPANT_TOKEN_TTL_SECONDS", "900"))
	if err != nil || secs <= 0 {
		secs = 900
	}
	if secs > 3600 {
		secs = 3600
	}
	return time.Duration(secs) * time.Second
}

// participantSecret resolves the applicant-token signing key, in order:
// EXAM_PARTICIPANT_SECRET (current name), PARTICIPANT_TOKEN_SECRET (the name
// this service shipped first in internal/handler/admission.go), then
// DefaultDevParticipantSecret. The legacy SEKOCI_JWT_SECRET fallback was
// removed on purpose: silently reusing the user-JWT key for participant
// tokens would make an applicant token indistinguishable from a user token.
func participantSecret() string {
	if v := os.Getenv("EXAM_PARTICIPANT_SECRET"); v != "" {
		return v
	}
	if v := os.Getenv("PARTICIPANT_TOKEN_SECRET"); v != "" {
		return v
	}
	return DefaultDevParticipantSecret
}

// IsProduction reports whether this process runs in production. APP_ENV is the
// primary signal (the convention services/admission uses); SENTRY_ENVIRONMENT
// is accepted as a second signal because the shared root docker-compose.yml
// passes SENTRY_ENVIRONMENT to this service but not APP_ENV, and a production
// guard that the real deployment cannot trigger is no guard at all.
func (c Config) IsProduction() bool {
	if strings.EqualFold(strings.TrimSpace(c.AppEnv), "production") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(c.SentryEnvironment)) {
	case "production", "prod":
		return true
	}
	return false
}

// Validate enforces the two invariants main.go depends on:
//
//  1. ParticipantSecret is never empty. An empty HMAC key would let anyone
//     mint an applicant token that every /participant/** route accepts
//     (HS256 with a zero-length key), so an unset secret is a hard startup
//     failure in every environment, not just production.
//  2. In production the committed dev default — or any other value still
//     carrying the "dev-" marker — is refused, so a deployment that forgot
//     EXAM_PARTICIPANT_SECRET cannot quietly run on a secret that is in the
//     repository.
func (c Config) Validate() error {
	if strings.TrimSpace(c.ParticipantSecret) == "" {
		return errors.New("participant token secret must not be empty (set EXAM_PARTICIPANT_SECRET)")
	}
	if c.IsProduction() && (c.ParticipantSecret == DefaultDevParticipantSecret || strings.Contains(c.ParticipantSecret, DevSecretMarker)) {
		return fmt.Errorf("EXAM_PARTICIPANT_SECRET must be explicitly set when running in production (refusing the committed dev default %q)", DefaultDevParticipantSecret)
	}
	return nil
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
