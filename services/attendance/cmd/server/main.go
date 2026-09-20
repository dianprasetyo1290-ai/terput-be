package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"skci/attendance/internal/cache"
	"skci/attendance/internal/client"
	"skci/attendance/internal/common"
	"skci/attendance/internal/handler"
	"skci/attendance/internal/store"
)

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// corsOrigins reads a comma-separated CORS_ALLOWED_ORIGINS so the allowed
// dev-server ports (one per frontend domain agent, see
// docs/FRONTEND_CONVENTIONS.md) are configured once via docker-compose
// rather than hardcoded per service — scales to more origins without a
// code change or rebuild.
func corsOrigins() []string {
	raw := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178")
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

	port := getEnv("PORT", "8083")
	dbURL := getEnv("DATABASE_URL", "postgres://sekoci:change-me@localhost:5483/skci_attendance?sslmode=disable")
	jwtSecret := getEnv("SEKOCI_JWT_SECRET", "change-me")
	internalKey := getEnv("SEKOCI_INTERNAL_KEY", "change-me")

	if err := common.InitSentry(getEnv("SENTRY_DSN", ""), getEnv("SENTRY_ENVIRONMENT", "development"), "attendance-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	// Cap the pool: database/sql defaults to unlimited, so under load this
	// service alone could exhaust Postgres max_connections and break every
	// other service with it. Matches identity/exam/report/academic.
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	s := store.New(db)
	c := client.New()
	h := handler.New(s, c)

	// Redis response cache (client request 2026-08-29): fails open if
	// REDIS_URL is unset/unreachable, see internal/cache's doc comment.
	respCache := cache.New(getEnv("REDIS_URL", ""), 15*time.Second)

	router := handler.NewRouter(h, handler.RouterConfig{
		JWTSecret:   jwtSecret,
		InternalKey: internalKey,
		CORSOrigins: corsOrigins(),
		Cache:       respCache,
	})

	log.Printf("attendance-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
