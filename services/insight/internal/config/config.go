package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port                       string
	DatabaseURL                string
	SekociJWTSecret            string
	SekociInternalKey          string
	IdentityURL                string
	BillingURL                 string
	AttendanceURL              string
	AdmissionURL               string
	HrURL                      string
	StudentlifeURL             string
	NotificationURL            string
	CoreURL                    string
	AppPublicURL               string
	MetricRefreshDefaultMin    int
	JobIntervalSeconds         int
	AIProvider                 string
	AITimeoutSeconds           int
	AIMaxContextChars          int
	SentryDSN                  string
	Env                        string
}

func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8096"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/skci_insight?sslmode=disable"
	}
	jwtSecret := os.Getenv("SEKOCI_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "sekoci-local-jwt-secret-min-32-chars-test"
	}
	internalKey := os.Getenv("SEKOCI_INTERNAL_KEY")
	if internalKey == "" {
		internalKey = "internal-key-sekoci-teratai"
	}

	identityURL := os.Getenv("IDENTITY_URL")
	if identityURL == "" {
		identityURL = "http://localhost:8081"
	}
	billingURL := os.Getenv("BILLING_URL")
	if billingURL == "" {
		billingURL = "http://localhost:8082"
	}
	attendanceURL := os.Getenv("ATTENDANCE_URL")
	if attendanceURL == "" {
		attendanceURL = "http://localhost:8083"
	}
	admissionURL := os.Getenv("ADMISSION_URL")
	if admissionURL == "" {
		admissionURL = "http://localhost:8085"
	}
	hrURL := os.Getenv("HR_URL")
	if hrURL == "" {
		hrURL = "http://localhost:8092"
	}
	studentlifeURL := os.Getenv("STUDENTLIFE_URL")
	if studentlifeURL == "" {
		studentlifeURL = "http://localhost:8094"
	}
	notificationURL := os.Getenv("NOTIFICATION_URL")
	if notificationURL == "" {
		notificationURL = "http://localhost:8088"
	}
	coreURL := os.Getenv("CORE_URL")
	if coreURL == "" {
		coreURL = "http://localhost:8080"
	}
	appPublicURL := os.Getenv("APP_PUBLIC_URL")
	if appPublicURL == "" {
		appPublicURL = "http://localhost:3000"
	}

	refreshMin := getEnvInt("METRIC_REFRESH_DEFAULT_MINUTES", 60)
	jobInterval := getEnvInt("JOB_INTERVAL_SECONDS", 60)
	aiProvider := os.Getenv("AI_PROVIDER")
	if aiProvider == "" {
		aiProvider = "unconfigured"
	}
	aiTimeout := getEnvInt("AI_TIMEOUT_SECONDS", 10)
	aiMaxChars := getEnvInt("AI_MAX_CONTEXT_CHARS", 4000)

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	return &Config{
		Port:                    port,
		DatabaseURL:             dbURL,
		SekociJWTSecret:         jwtSecret,
		SekociInternalKey:       internalKey,
		IdentityURL:             identityURL,
		BillingURL:              billingURL,
		AttendanceURL:           attendanceURL,
		AdmissionURL:            admissionURL,
		HrURL:                   hrURL,
		StudentlifeURL:          studentlifeURL,
		NotificationURL:         notificationURL,
		CoreURL:                 coreURL,
		AppPublicURL:            appPublicURL,
		MetricRefreshDefaultMin: refreshMin,
		JobIntervalSeconds:      jobInterval,
		AIProvider:              aiProvider,
		AITimeoutSeconds:        aiTimeout,
		AIMaxContextChars:       aiMaxChars,
		SentryDSN:               os.Getenv("SENTRY_DSN"),
		Env:                     env,
	}
}

func getEnvInt(key string, fallback int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return fallback
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return fallback
	}
	return val
}
