package config

import (
	"encoding/base64"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv              string
	Port                string
	DatabaseURL         string
	JWTSecret           string
	InternalKey         string
	CORSOrigins         []string
	HREncKey            []byte
	HRBlindKey          string
	CoreURL             string
	AccountingURL       string // empty: payroll journals stay pending ("belum dikonfigurasi")
	IdentityURL         string
	NotificationURL     string
	HRPublicInternalURL string
	StorageRoot         string
	MaxUploadBytes      int64
	JobInterval         time.Duration
	SentryDSN           string
	SentryEnvironment   string
}

func Load() (Config, error) {
	encKeyB64 := os.Getenv("HR_ENC_KEY")
	if strings.TrimSpace(encKeyB64) == "" {
		encKeyB64 = "change-me"
	}
	encKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encKeyB64))
	if err != nil || len(encKey) != 32 {
		return Config{}, errors.New("HR_ENC_KEY must be valid 32 bytes base64 encoded")
	}

	blindKey := strings.TrimSpace(os.Getenv("HR_BLIND_KEY"))
	if blindKey == "" {
		blindKey = "change-me"
	}

	jobSec := integer("JOB_INTERVAL_SECONDS", 300)
	if jobSec <= 0 {
		jobSec = 300
	}

	c := Config{
		AppEnv:              env("APP_ENV", "development"),
		Port:                env("HR_PORT", env("PORT", "8092")),
		DatabaseURL:         env("DATABASE_URL", "postgres://teratai:change-me@localhost:5492/teratai_hr?sslmode=disable"),
		JWTSecret:           os.Getenv("SEKOCI_JWT_SECRET"),
		InternalKey:         os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:         split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		HREncKey:            encKey,
		HRBlindKey:          blindKey,
		CoreURL:             strings.TrimRight(env("CORE_URL", "http://localhost:8091"), "/"),
		AccountingURL:       strings.TrimRight(os.Getenv("ACCOUNTING_URL"), "/"),
		IdentityURL:         strings.TrimRight(env("IDENTITY_URL", "http://localhost:8081"), "/"),
		NotificationURL:     strings.TrimRight(env("NOTIFICATION_URL", "http://localhost:8089"), "/"),
		HRPublicInternalURL: strings.TrimRight(env("HR_PUBLIC_INTERNAL_URL", "http://localhost:8092"), "/"),
		StorageRoot:         env("STORAGE_ROOT", "./data/private/hr"),
		MaxUploadBytes:      int64(integer("MAX_UPLOAD_BYTES", 10<<20)),
		JobInterval:         time.Duration(jobSec) * time.Second,
		SentryDSN:           os.Getenv("SENTRY_DSN"),
		SentryEnvironment:   env("SENTRY_ENVIRONMENT", "development"),
	}

	production := strings.EqualFold(c.AppEnv, "production")
	if c.JWTSecret == "" && !production {
		c.JWTSecret = "change-me"
	}
	if c.InternalKey == "" && !production {
		c.InternalKey = "change-me"
	}

	if production {
		if c.JWTSecret == "" || strings.Contains(c.JWTSecret, "dev-") || strings.Contains(c.JWTSecret, "change-me") {
			return Config{}, errors.New("SEKOCI_JWT_SECRET must be explicitly configured in production")
		}
		if c.InternalKey == "" || strings.Contains(c.InternalKey, "dev-") {
			return Config{}, errors.New("SEKOCI_INTERNAL_KEY must be explicitly configured in production")
		}
	}

	return c, nil
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
