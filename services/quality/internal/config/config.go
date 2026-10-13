package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port               string
	DatabaseURL        string
	JWTSecret          string
	InternalKey        string
	QualityInternalURL string
	CoreURL            string
	IdentityURL        string
	AcademicURL        string
	HRURL              string
	InsightURL         string
	NotificationURL    string
	StorageRoot        string
	EvidenceMaxBytes   int64
	ImportMaxRows      int
	QualityHashKey     string
	JobIntervalSeconds int
	SentryDSN          string
}

func Load() *Config {
	port := getEnv("PORT", "8095")
	dbURL := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/skci_quality?sslmode=disable")
	jwtSecret := getEnv("SEKOCI_JWT_SECRET", "change-me")
	internalKey := getEnv("SEKOCI_INTERNAL_KEY", "change-me")
	qualityInternalURL := getEnv("QUALITY_INTERNAL_URL", "http://quality:"+port)
	coreURL := getEnv("CORE_URL", "http://core:8081")
	identityURL := getEnv("IDENTITY_URL", "http://identity:8080")
	academicURL := getEnv("ACADEMIC_URL", "http://academic:8082")
	hrURL := getEnv("HR_URL", "http://hr:8091")
	insightURL := getEnv("INSIGHT_URL", "http://insight:8096")
	notificationURL := getEnv("NOTIFICATION_URL", "http://notification:8085")
	storageRoot := getEnv("STORAGE_ROOT", "/data/quality")

	evidenceMaxBytes := int64(25 * 1024 * 1024) // 25 MB default
	if v := os.Getenv("EVIDENCE_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			evidenceMaxBytes = n
		}
	}

	importMaxRows := 10000
	if v := os.Getenv("IMPORT_MAX_ROWS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			importMaxRows = n
		}
	}

	hashKey := getEnv("QUALITY_HASH_KEY", "change-me")

	jobInterval := 60
	if v := os.Getenv("JOB_INTERVAL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			jobInterval = n
		}
	}

	return &Config{
		Port:               port,
		DatabaseURL:        dbURL,
		JWTSecret:          jwtSecret,
		InternalKey:        internalKey,
		QualityInternalURL: qualityInternalURL,
		CoreURL:            coreURL,
		IdentityURL:        identityURL,
		AcademicURL:        academicURL,
		HRURL:              hrURL,
		InsightURL:         insightURL,
		NotificationURL:    notificationURL,
		StorageRoot:        storageRoot,
		EvidenceMaxBytes:   evidenceMaxBytes,
		ImportMaxRows:      importMaxRows,
		QualityHashKey:     hashKey,
		JobIntervalSeconds: jobInterval,
		SentryDSN:          os.Getenv("SENTRY_DSN"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
