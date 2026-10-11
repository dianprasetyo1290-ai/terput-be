package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/joho/godotenv"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/config"
	"skci/core/internal/handler"
	"skci/core/internal/jobs"
	"skci/core/internal/store"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "core-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	st := store.NewPGStore(db)
	cl := client.New(cfg.NotificationURL, cfg.IdentityURL, cfg.HRURL, cfg.InternalKey)

	// Periodic job runner (escalation + callback retries)
	jobRunner := jobs.NewRunner(db, st, cl, time.Duration(cfg.JobIntervalSeconds)*time.Second, cfg.CallbackMaxAttempts)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobRunner.Start(ctx)

	appHandler := handler.New(st, cfg, cl)
	r := handler.NewRouter(appHandler, cfg)

	log.Printf("core-service listening on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
