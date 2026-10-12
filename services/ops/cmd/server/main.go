package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/config"
	"skci/ops/internal/facility"
	"skci/ops/internal/handler"
	"skci/ops/internal/jobs"
	"skci/ops/internal/storage"
	"skci/ops/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	if cfg.SentryDSN != "" {
		if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "ops-service"); err != nil {
			log.Printf("sentry init failed (continuing without error tracking): %v", err)
		}
		defer common.FlushSentry()
	}

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	loc, _ := time.LoadLocation(cfg.SchoolTimezone) // validated in config.Load
	apiClient := client.New(cfg.IdentityURL, cfg.CoreURL, cfg.NotificationURL, cfg.InternalKey, cfg.FeatureCacheTTL)
	fac := &facility.Handler{
		Store:        facility.NewStore(db, loc),
		Client:       apiClient,
		Storage:      storage.NewLocal(cfg.StorageRoot, cfg.MaxUploadBytes, facility.PhotoMIMEs...),
		MaxUpload:    cfg.MaxUploadBytes,
		CallbackBase: cfg.OpsPublicInternalURL,
		// Purchase (T15) and Assets (T16) hooks are registered here by
		// their tasks; nil keeps those features off.
	}
	app := &handler.App{DB: db, Config: &cfg, Client: apiClient, Facility: fac}

	runner := jobs.NewRunner(db, cfg.JobInterval)
	runner.Register("facility_preventive", func(ctx context.Context, _ *sql.DB) error { return fac.RunPreventive(ctx) })
	runner.Register("forward_audit", app.ForwardAudit)
	runner.Start()
	defer runner.Stop()

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		log.Printf("ops-service listening on port %s (%s)", cfg.Port, cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()
	<-stop
	log.Println("shutting down ops-service...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
}
