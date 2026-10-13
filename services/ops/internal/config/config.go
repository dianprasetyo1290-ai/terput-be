// Package config reads the ops service's environment (T9 §1).
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
	AppEnv               string
	Port                 string
	DatabaseURL          string
	JWTSecret            string
	InternalKey          string
	CORSOrigins          []string
	CoreURL              string
	IdentityURL          string
	NotificationURL      string
	HRURL                string
	BillingURL           string
	AccountingURL        string
	OpsPublicInternalURL string
	OpsEncKey            []byte
	OpsBlindKey          string
	StorageRoot          string
	MaxUploadBytes       int64
	JobInterval          time.Duration
	FeatureCacheTTL      time.Duration
	SchoolTimezone       string
	SentryDSN            string
	SentryEnvironment    string
}

func Load() (Config, error) {
	encKeyB64 := os.Getenv("OPS_ENC_KEY")
	if strings.TrimSpace(encKeyB64) == "" {
		encKeyB64 = "change-me"
	}
	encKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encKeyB64))
	if err != nil || len(encKey) != 32 {
		return Config{}, errors.New("OPS_ENC_KEY must be valid 32 bytes base64 encoded")
	}

	blindKey := strings.TrimSpace(os.Getenv("OPS_BLIND_KEY"))
	if blindKey == "" {
		blindKey = "change-me"
	}

	c := Config{
		AppEnv:               env("APP_ENV", "development"),
		Port:                 env("OPS_PORT", env("PORT", "8093")),
		DatabaseURL:          env("DATABASE_URL", "postgres://teratai:change-me@localhost:5432/teratai_ops?sslmode=disable"),
		JWTSecret:            os.Getenv("SEKOCI_JWT_SECRET"),
		InternalKey:          os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:          split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		CoreURL:              trim(env("CORE_URL", "http://localhost:8091")),
		IdentityURL:          trim(env("IDENTITY_URL", "http://localhost:8081")),
		NotificationURL:      trim(env("NOTIFICATION_URL", "http://localhost:8089")),
		HRURL:                trim(env("HR_URL", "http://localhost:8092")),
		BillingURL:           trim(env("BILLING_URL", "http://localhost:8090")),
		AccountingURL:        trim(env("ACCOUNTING_URL", "http://localhost:8000")),
		OpsPublicInternalURL: trim(env("OPS_PUBLIC_INTERNAL_URL", "http://localhost:8093")),
		OpsEncKey:            encKey,
		OpsBlindKey:          blindKey,
		StorageRoot:          env("STORAGE_ROOT", "./data/private/ops"),
		MaxUploadBytes:       int64(integer("MAX_UPLOAD_BYTES", 5<<20)),
		JobInterval:          time.Duration(integer("JOB_INTERVAL_SECONDS", 300)) * time.Second,
		FeatureCacheTTL:      time.Duration(integer("FEATURE_CACHE_SECONDS", 60)) * time.Second,
		SchoolTimezone:       env("SCHOOL_TIMEZONE", "Asia/Jakarta"),
		SentryDSN:            os.Getenv("SENTRY_DSN"),
		SentryEnvironment:    env("SENTRY_ENVIRONMENT", "development"),
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
	if _, err := time.LoadLocation(c.SchoolTimezone); err != nil {
		return Config{}, errors.New("SCHOOL_TIMEZONE is not a valid IANA zone")
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func trim(v string) string { return strings.TrimRight(v, "/") }

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
