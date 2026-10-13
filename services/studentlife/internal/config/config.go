// Package config loads studentlife-service settings from the environment.
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
	AppEnv            string
	Port              string
	DatabaseURL       string
	JWTSecret         string
	InternalKey       string
	CORSOrigins       []string
	IdentityURL       string
	AcademicURL       string
	AttendanceURL     string
	NotificationURL   string
	CoreURL           string
	OpsURL            string // opsional: kosong = pengeluaran stok obat tetap pending (T15)
	PublicInternalURL string // dipakai sebagai callback_url approval core
	PortalURL         string // tautan portal di notifikasi wali
	StoragePath       string
	EncKey            []byte
	BlindKey          string
	DefaultTimezone   string
	JobInterval       time.Duration
	JobMaxAttempts    int
	SentryDSN         string
	SentryEnvironment string
}

// Load reads the environment. STUDENTLIFE_ENC_KEY (32 byte base64) and
// STUDENTLIFE_BLIND_KEY have no default: data kesehatan tidak boleh
// terenkripsi dengan kunci bawaan, jadi service gagal start bila kosong.
func Load() (Config, error) {
	encKeyB64 := strings.TrimSpace(os.Getenv("STUDENTLIFE_ENC_KEY"))
	if encKeyB64 == "" {
		return Config{}, errors.New("STUDENTLIFE_ENC_KEY wajib diisi (32 byte, base64)")
	}
	encKey, err := base64.StdEncoding.DecodeString(encKeyB64)
	if err != nil || len(encKey) != 32 {
		return Config{}, errors.New("STUDENTLIFE_ENC_KEY must be valid 32 bytes base64 encoded")
	}
	blindKey := strings.TrimSpace(os.Getenv("STUDENTLIFE_BLIND_KEY"))
	if len(blindKey) < 16 {
		return Config{}, errors.New("STUDENTLIFE_BLIND_KEY wajib diisi (minimal 16 karakter)")
	}

	c := Config{
		AppEnv:            env("APP_ENV", "development"),
		Port:              env("STUDENTLIFE_PORT", env("PORT", "8094")),
		DatabaseURL:       env("DATABASE_URL", "postgres://teratai:change-me@localhost:5432/teratai_studentlife?sslmode=disable"),
		JWTSecret:         os.Getenv("SEKOCI_JWT_SECRET"),
		InternalKey:       os.Getenv("SEKOCI_INTERNAL_KEY"),
		CORSOrigins:       split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		IdentityURL:       trim(env("IDENTITY_URL", "http://localhost:8081")),
		AcademicURL:       trim(env("ACADEMIC_URL", "http://localhost:8082")),
		AttendanceURL:     trim(env("ATTENDANCE_URL", "http://localhost:8083")),
		NotificationURL:   trim(env("NOTIFICATION_URL", "http://localhost:8089")),
		CoreURL:           trim(env("CORE_URL", "http://localhost:8091")),
		OpsURL:            trim(os.Getenv("OPS_URL")),
		PublicInternalURL: trim(env("STUDENTLIFE_PUBLIC_INTERNAL_URL", "http://localhost:8094")),
		PortalURL:         trim(env("PORTAL_URL", "http://localhost:5173")),
		StoragePath:       env("STORAGE_PATH", "./data/storage"),
		EncKey:            encKey,
		BlindKey:          blindKey,
		DefaultTimezone:   env("DEFAULT_TIMEZONE", "Asia/Jakarta"),
		JobInterval:       time.Duration(integer("JOB_INTERVAL_SECONDS", 300)) * time.Second,
		JobMaxAttempts:    integer("JOB_MAX_ATTEMPTS", 5),
		SentryDSN:         os.Getenv("SENTRY_DSN"),
		SentryEnvironment: env("SENTRY_ENVIRONMENT", "development"),
	}
	if _, err := time.LoadLocation(c.DefaultTimezone); err != nil {
		return Config{}, errors.New("DEFAULT_TIMEZONE tidak dikenal")
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

func trim(v string) string { return strings.TrimRight(strings.TrimSpace(v), "/") }

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
