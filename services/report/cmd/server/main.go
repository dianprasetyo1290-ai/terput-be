package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"skci/report/internal/cache"
	"skci/report/internal/common"
	"skci/report/internal/handler"
	"skci/report/internal/store"
	"skci/report/internal/upstream"
)

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
// code change or rebuild.
func corsOrigins() []string {
	raw := getenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func main() {
	_ = godotenv.Load()

	port := getenv("PORT", "8086")
	dsn := getenv("DATABASE_URL", "postgres://sekoci:change-me@localhost:5486/skci_report?sslmode=disable")
	jwtSecret := getenv("SEKOCI_JWT_SECRET", "change-me")
	internalKey := getenv("SEKOCI_INTERNAL_KEY", "change-me")

	if err := common.InitSentry(getenv("SENTRY_DSN", ""), getenv("SENTRY_ENVIRONMENT", "development"), "report-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	identityURL := getenv("IDENTITY_SERVICE_URL", "http://localhost:8081")
	academicURL := getenv("ACADEMIC_SERVICE_URL", "http://localhost:8082")
	attendanceURL := getenv("ATTENDANCE_SERVICE_URL", "http://localhost:8083")
	examURL := getenv("EXAM_SERVICE_URL", "http://localhost:8084")

	db, err := store.Open(dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	timeout := 3 * time.Second
	identity := upstream.NewIdentityClient(identityURL, internalKey, timeout)
	deps := &handler.Deps{
		Store:      store.New(db),
		Identity:   identity,
		Academic:   upstream.NewAcademicClient(academicURL, internalKey, timeout),
		Attendance: upstream.NewAttendanceClient(attendanceURL, internalKey, timeout),
		Exam:       upstream.NewExamClient(examURL, internalKey, timeout),
	}
	// Portal Orang Tua: the guardian<->student link check is a hard gate on
	// every /parent/* read, so it must fail closed. Only wire the checker
	// when IDENTITY_SERVICE_URL is actually configured -- unlike the other
	// upstreams (which degrade to partial data), an unset identity URL here
	// would otherwise leave the gate unable to authorize anyone, and the
	// handler must answer 503 rather than open up. See
	// handler.RequireGuardianStudent.
	if os.Getenv("IDENTITY_SERVICE_URL") != "" {
		deps.GuardianAuth = identity
	} else {
		log.Printf("IDENTITY_SERVICE_URL not set: /parent/* guardian endpoints will fail closed with 503")
	}

	// Redis response cache (client request 2026-08-29): fails open if
	// REDIS_URL is unset/unreachable, see internal/cache's doc comment.
	respCache := cache.New(getenv("REDIS_URL", ""), 15*time.Second)

	router := handler.NewRouter(deps, jwtSecret, corsOrigins(), respCache)

	log.Printf("report-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
