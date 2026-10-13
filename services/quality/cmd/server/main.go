package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"skci/quality/internal/client"
	"skci/quality/internal/config"
	"skci/quality/internal/govreport"
	"skci/quality/internal/handler"
	"skci/quality/internal/jobs"
	"skci/quality/internal/qa"
	"skci/quality/internal/storage"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to parse database url: %v", err)
	}
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Printf("warning: database ping failed: %v", err)
	}

	st, err := storage.NewLocalStorage(cfg.StorageRoot)
	if err != nil {
		log.Printf("warning: local storage initialization: %v", err)
	}

	clients := client.NewClients(cfg.InternalKey, cfg.InsightURL, cfg.CoreURL, cfg.IdentityURL, cfg.AcademicURL, cfg.HRURL, cfg.NotificationURL)

	qaStore := qa.NewStore(pool)
	govStore := govreport.NewStore(pool)

	qaHandler := qa.NewHandler(qaStore, clients, st)
	govHandler := govreport.NewHandler(govStore, clients, st, cfg.QualityHashKey, cfg.ImportMaxRows)

	app := handler.NewApp(cfg, qaStore, govStore, qaHandler, govHandler, clients, st)

	// Start background jobs runner
	jobRunner := jobs.NewRunner(pool, clients, time.Duration(cfg.JobIntervalSeconds)*time.Second)
	jobRunner.Start()
	defer jobRunner.Stop()

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.Router(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("quality service listening on port %s", cfg.Port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	_ = os.Getenv
}
