package main

import (
	"log"
	"net/http"
	"time"

	"github.com/joho/godotenv"

	"skci/exam/internal/cache"
	"skci/exam/internal/common"
	"skci/exam/internal/config"
	"skci/exam/internal/handler"
	"skci/exam/internal/httpclient"
	"skci/exam/internal/store"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "exam-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	s := store.New(db)
	ic := httpclient.New(cfg.IdentityURL, cfg.AcademicURL, cfg.InternalKey)
	h := handler.New(s, ic)

	// Redis response cache (client request 2026-08-29): fails open if
	// REDIS_URL is unset/unreachable, see internal/cache's doc comment.
	respCache := cache.New(cfg.RedisURL, 15*time.Second)

	router := handler.NewRouter(h, cfg.JWTSecret, cfg.InternalKey, cfg.CORSOrigins, respCache)

	log.Printf("exam-service listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatal(err)
	}
}
