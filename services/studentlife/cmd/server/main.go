package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/handler"
	"skci/studentlife/internal/jobs"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "studentlife-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer db.Close()

	cr, err := sensitive.New(cfg.EncKey, cfg.BlindKey)
	if err != nil {
		log.Fatalf("failed to initialize crypto: %v", err)
	}
	cl := client.New(client.URLs{Identity: cfg.IdentityURL, Academic: cfg.AcademicURL, Attendance: cfg.AttendanceURL,
		Notification: cfg.NotificationURL, Core: cfg.CoreURL, Ops: cfg.OpsURL}, cfg.InternalKey)
	app := handler.New(db, cfg, cr, cl)

	runner := jobs.NewRunner(db, cfg.JobInterval)
	runner.Register("studentlife_visit_sync", app.Health.JobSyncVisits)
	runner.Register("studentlife_inventory_issues", app.Health.JobInventoryIssues)
	runner.Register("studentlife_consent_reminders", app.Health.JobConsentReminders)
	runner.Register("studentlife_forward_audit", app.JobForwardAudit)
	runner.Start()
	defer runner.Stop()

	server := &http.Server{Addr: ":" + cfg.Port, Handler: app.Router(), ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		log.Printf("studentlife-service listening on :%s (%s)", cfg.Port, cfg.AppEnv)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
