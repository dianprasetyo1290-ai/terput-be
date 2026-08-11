package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"skci/evaluation/internal/client"
	"skci/evaluation/internal/common"
	"skci/evaluation/internal/config"
	"skci/evaluation/internal/handler"
	"skci/evaluation/internal/store"
)

func main() {
	cfg := config.Load()

	if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "evaluation-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	migrationsPath := "migrations"
	if v := os.Getenv("MIGRATIONS_PATH"); v != "" {
		migrationsPath = v
	}
	if err := store.RunMigrations(migrationsPath, cfg.DatabaseURL); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	log.Println("migrations applied")

	ctx := context.Background()
	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect failed: %v", err)
	}
	defer pool.Close()

	lessonPlanStore := store.NewLessonPlanStore(pool)
	periodStore := store.NewPeriodStore(pool)
	formStore := store.NewFormStore(pool)
	taskStore := store.NewTaskStore(pool)
	protaStore := store.NewProtaStore(pool)
	prosemStore := store.NewProsemStore(pool)
	clients := client.New(cfg.IdentityBaseURL, cfg.AcademicBaseURL, cfg.InternalKey)

	h := handler.New(lessonPlanStore, periodStore, formStore, taskStore, protaStore, prosemStore, clients)
	router := handler.NewRouter(h, cfg.JWTSecret, cfg.CORSOrigins)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("evaluation-service listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
