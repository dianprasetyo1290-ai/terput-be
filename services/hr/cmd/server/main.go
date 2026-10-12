package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"skci/hr/internal/client"
	"skci/hr/internal/common"
	"skci/hr/internal/config"
	"skci/hr/internal/crypto"
	"skci/hr/internal/handler"
	"skci/hr/internal/jobs"
	"skci/hr/internal/storage"
	"skci/hr/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	if cfg.SentryDSN != "" {
		if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "hr-service"); err != nil {
			log.Printf("sentry init failed (continuing without error tracking): %v", err)
		}
		defer common.FlushSentry()
	}

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	cryptoSvc, err := crypto.New(cfg.HREncKey, cfg.HRBlindKey)
	if err != nil {
		log.Fatalf("failed to initialize crypto: %v", err)
	}

	storageSvc := storage.NewLocal(cfg.StorageRoot, cfg.MaxUploadBytes)
	apiClient := client.New(cfg.IdentityURL, cfg.CoreURL, cfg.NotificationURL, cfg.InternalKey)
	apiClient.CallbackURL = cfg.HRPublicInternalURL + "/internal/approval-callbacks"

	app := handler.NewApp(db, &cfg, cryptoSvc, storageSvc, apiClient)

	// Background jobs runner with postgres advisory locks
	jobHandlers := jobs.NewJobHandlers(apiClient)
	runner := jobs.NewRunner(db, cfg.JobInterval)
	runner.Register("contract_reminders", jobHandlers.ContractReminders)
	runner.Register("document_reminders", jobHandlers.DocumentReminders)
	runner.Register("scheduled_offboarding", jobHandlers.ScheduledOffboarding)
	runner.Register("monthly_accrual", jobHandlers.MonthlyAccrual)
	runner.Start()
	defer runner.Stop()

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("hr-service listening on port %s (%s)", cfg.Port, cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-stop
	log.Println("shutting down hr-service...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("server shutdown error: %v", err)
	}
	log.Println("hr-service stopped cleanly")
}
