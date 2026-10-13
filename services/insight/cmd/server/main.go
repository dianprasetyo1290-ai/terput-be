package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"skci/insight/internal/assistant"
	assistantProvider "skci/insight/internal/assistant/provider"
	"skci/insight/internal/bi"
	"skci/insight/internal/client"
	"skci/insight/internal/common"
	"skci/insight/internal/config"
	"skci/insight/internal/jobs"
	"skci/insight/internal/metrics"
	"skci/insight/internal/predict"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup Database Connection Pool
	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		log.Printf("Warning: Database ping failed: %v", err)
	}

	// Clients & Providers
	extClient := client.New(cfg)
	isProd := cfg.Env == "production"
	aiProv, err := assistantProvider.New(cfg.AIProvider, isProd)
	if err != nil {
		log.Fatalf("Failed to initialize AI provider: %v", err)
	}

	// Stores
	metricStore := metrics.NewStore(dbPool)
	biStore := bi.NewStore(dbPool)
	asstStore := assistant.NewStore(dbPool)
	predStore := predict.NewStore(dbPool)

	// Handlers
	metricHandler := metrics.NewHandler(cfg, metricStore, extClient)
	biHandler := bi.NewHandler(cfg, biStore, metricStore, extClient)
	asstHandler := assistant.NewHandler(cfg, asstStore, metricStore, extClient, aiProv)
	predHandler := predict.NewHandler(cfg, predStore, extClient)

	// Background Jobs
	runner := jobs.NewRunner(cfg, dbPool, biStore, metricStore, predStore, extClient)
	runner.Start(ctx)

	// Chi Router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "Idempotency-Key"},
		ExposedHeaders:   []string{"Link", "Content-Disposition"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health Check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "insight",
		})
	})

	// Public Token Feed CSV
	r.Get("/bi/feed/{metric_key}.csv", biHandler.ExportFeed)

	// Internal Routes (X-Internal-Key)
	r.Group(func(r chi.Router) {
		r.Use(common.InternalKeyMiddleware(cfg.SekociInternalKey))
		r.Get("/internal/metrics", metricHandler.InternalListMetrics)
		r.Get("/internal/metrics/{key}/value", metricHandler.InternalGetMetricValue)
	})

	// Authenticated Routes (JWT)
	r.Group(func(r chi.Router) {
		r.Use(common.AuthMiddleware(cfg.SekociJWTSecret))

		// Metrics
		r.Get("/metrics", metricHandler.ListMetrics)
		r.Get("/metrics/{key}", metricHandler.GetMetric)
		r.With(common.RequireRoles(common.RoleSuperadmin)).Put("/metrics/{key}/description", metricHandler.UpdateDescription)
		r.With(common.RequireRoles(common.RoleAdmin, common.RoleSuperadmin)).Put("/metrics/{key}/school-params", metricHandler.UpdateSchoolParams)

		// BI & Dashboards
		r.Post("/bi/query", biHandler.Query)

		r.Get("/bi/dashboards", biHandler.ListDashboards)
		r.Post("/bi/dashboards", biHandler.CreateDashboard)
		r.Get("/bi/dashboards/{id}", biHandler.GetDashboard)
		r.Put("/bi/dashboards/{id}", biHandler.UpdateDashboard)
		r.Delete("/bi/dashboards/{id}", biHandler.DeleteDashboard)
		r.Get("/bi/dashboards/{id}/data", biHandler.GetDashboardData)

		r.Get("/bi/reports", biHandler.ListReports)
		r.Post("/bi/reports", biHandler.CreateReport)
		r.Get("/bi/reports/{id}", biHandler.GetReport)
		r.Put("/bi/reports/{id}", biHandler.UpdateReport)
		r.Delete("/bi/reports/{id}", biHandler.DeleteReport)
		r.Get("/bi/reports/{id}/run", biHandler.RunReport)
		r.Post("/bi/reports/{id}/export", biHandler.ExportReport)

		r.Get("/bi/schedules", biHandler.ListSchedules)
		r.Post("/bi/schedules", biHandler.CreateSchedule)
		r.Get("/bi/schedules/{id}/runs", biHandler.ListScheduleRuns)

		r.Get("/bi/export-tokens", biHandler.ListExportTokens)
		r.Post("/bi/export-tokens", biHandler.CreateExportToken)
		r.Delete("/bi/export-tokens/{id}", biHandler.RevokeExportToken)

		r.Get("/bi/scope-grants", biHandler.ListScopeGrants)
		r.Post("/bi/scope-grants", biHandler.CreateScopeGrant)
		r.Delete("/bi/scope-grants/{id}", biHandler.RevokeScopeGrant)

		// Assistant
		r.Post("/assistant/messages", asstHandler.PostMessage)
		r.Get("/assistant/conversations", asstHandler.ListConversations)
		r.Get("/assistant/conversations/{id}", asstHandler.GetConversation)
		r.Post("/assistant/actions/{id}/confirm", asstHandler.ConfirmAction)
		r.Post("/assistant/actions/{id}/cancel", asstHandler.CancelAction)

		r.Get("/assistant/help-articles", asstHandler.ListHelpArticles)
		r.Post("/assistant/help-articles", asstHandler.CreateHelpArticle)
		r.Put("/assistant/help-articles/{id}", asstHandler.UpdateHelpArticle)
		r.Delete("/assistant/help-articles/{id}", asstHandler.DeleteHelpArticle)

		r.Get("/assistant/intents", asstHandler.ListIntents)
		r.Post("/assistant/intents", asstHandler.CreateIntent)
		r.Put("/assistant/intents/{id}", asstHandler.UpdateIntent)
		r.Delete("/assistant/intents/{id}", asstHandler.DeleteIntent)

		r.Get("/assistant/settings", asstHandler.GetSettings)
		r.Put("/assistant/settings", asstHandler.UpdateSettings)

		// Predictive Analytics
		r.Get("/predict/settings", predHandler.GetSettings)
		r.Put("/predict/settings", predHandler.UpdateSettings)

		r.Get("/predict/rules", predHandler.ListRules)
		r.Post("/predict/rules", predHandler.CreateRule)

		r.Get("/predict/arrears", predHandler.GetArrearsRisk)
		r.Get("/predict/absence", predHandler.GetAbsenceRisk)
		r.Get("/predict/ppdb", predHandler.GetPPDBForecast)
		r.Get("/predict/cashflow", predHandler.GetCashflowForecast)
		r.Get("/predict/accuracy", predHandler.GetAccuracy)
		r.Post("/predict/{id}/reviews", predHandler.AddReview)
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("insight service listening on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down insight service...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	fmt.Println("Server gracefully stopped")
}
