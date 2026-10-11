// Package config loads admission-service configuration from the environment.
// It fails fast at startup when a production deployment is missing the secrets
// that must never fall back to a development default.
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

	// Admin JWT verification (shared identity secret) and the admission-only
	// guardian session secret. They must never be the same value.
	JWTSecret         string
	GuardianJWTSecret string
	InternalKey       string

	CORSOrigins []string
	StorageRoot string
	OTPProvider string

	OTPTTL         time.Duration
	OTPCooldown    time.Duration
	GuardianTTL    time.Duration
	OTPMaxAttempts int
	MaxUploadBytes int64

	IdentityURL string
	ExamURL     string
	AcademicURL string

	// ExamPublicURL is the browser-facing base URL used to build the CBT entry
	// link. When empty, /cbt/enter still provisions the participant but
	// returns no redirect URL.
	ExamPublicURL string

	// DefaultSchoolID is the tenant used by the public (unauthenticated) PPDB
	// endpoints. The fork serves one school, so the public SPA never sends a
	// school_id; this keeps discovery/OTP working while still scoping every
	// row to a real tenant.
	DefaultSchoolID string

	// Public branding surfaced by GET /public/discovery.
	SchoolName        string
	SchoolHeadline    string
	SchoolDescription string
	SchoolContact     string

	SentryDSN         string
	SentryEnvironment string
}

// Load reads and validates the configuration.
func Load() (Config, error) {
	c := Config{
		AppEnv:            env("APP_ENV", "development"),
		Port:              env("PORT", "8088"),
		DatabaseURL:       env("DATABASE_URL", "postgres://teratai:change-me@localhost:5487/teratai_admission?sslmode=disable"),
		JWTSecret:         os.Getenv("SEKOCI_JWT_SECRET"),
		GuardianJWTSecret: os.Getenv("ADMISSION_GUARDIAN_JWT_SECRET"),
		InternalKey:       os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:       split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		StorageRoot:       env("STORAGE_ROOT", "./data/private"),
		OTPProvider:       env("OTP_PROVIDER", "dev"),
		OTPTTL:            duration("OTP_TTL", 5*time.Minute),
		OTPCooldown:       duration("OTP_COOLDOWN", time.Minute),
		GuardianTTL:       duration("GUARDIAN_SESSION_TTL", 12*time.Hour),
		OTPMaxAttempts:    integer("OTP_MAX_ATTEMPTS", 5),
		MaxUploadBytes:    int64(integer("MAX_UPLOAD_BYTES", 10<<20)),
		IdentityURL:       os.Getenv("IDENTITY_URL"),
		ExamURL:           os.Getenv("EXAM_URL"),
		AcademicURL:       os.Getenv("ACADEMIC_URL"),
		ExamPublicURL:     os.Getenv("EXAM_PUBLIC_URL"),
		DefaultSchoolID:   os.Getenv("ADMISSION_SCHOOL_ID"),
		SchoolName:        env("SCHOOL_NAME", "Sekolah Teratai"),
		SchoolHeadline:    env("SCHOOL_HEADLINE", "Penerimaan Peserta Didik Baru"),
		SchoolDescription: env("SCHOOL_DESCRIPTION", "Temukan jalur pendaftaran, siapkan dokumen, dan pantau seluruh proses PPDB dalam satu portal."),
		SchoolContact:     env("SCHOOL_CONTACT", ""),
		SentryDSN:         os.Getenv("SENTRY_DSN"),
		SentryEnvironment: env("SENTRY_ENVIRONMENT", "development"),
	}

	production := strings.EqualFold(c.AppEnv, "production")

	if c.JWTSecret == "" && !production {
		c.JWTSecret = "change-me"
	}
	if c.GuardianJWTSecret == "" && !production {
		c.GuardianJWTSecret = "change-me"
	}
	if c.InternalKey == "" && !production {
		c.InternalKey = "change-me"
	}

	if production {
		switch {
		case c.JWTSecret == "" || containsDev(c.JWTSecret):
			return Config{}, errors.New("SEKOCI_JWT_SECRET must be explicitly configured in production")
		case c.GuardianJWTSecret == "" || containsDev(c.GuardianJWTSecret):
			return Config{}, errors.New("ADMISSION_GUARDIAN_JWT_SECRET must be explicitly configured in production")
		case c.InternalKey == "" || containsDev(c.InternalKey):
			return Config{}, errors.New("SEKOCI_INTERNAL_KEY must be explicitly configured in production")
		case c.OTPProvider == "dev":
			return Config{}, errors.New("OTP_PROVIDER=dev is forbidden in production")
		}
		if c.JWTSecret == c.GuardianJWTSecret {
			return Config{}, errors.New("guardian session secret must differ from the admin JWT secret")
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

func duration(key string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func integer(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
