// Command server is notification-service's HTTP entrypoint: the portal inbox,
// per-user notification preferences, internal service-to-service sends,
// template administration, delivery bookkeeping and tenant-scoped chat.
package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"

	"skci/notification/internal/client"
	"skci/notification/internal/common"
	"skci/notification/internal/config"
	"skci/notification/internal/handler"
	"skci/notification/internal/provider"
	"skci/notification/internal/store"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "notification-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Close()

	prod := cfg.Production()
	delivery, err := provider.New(cfg.DeliveryProvider, prod)
	if err != nil {
		log.Fatalf("delivery provider: %v", err)
	}
	if prod && delivery.Name() == "dev-noop" {
		log.Fatal("refusing to start: a no-op delivery provider is not allowed in production")
	}

	h := handler.New(store.New(db), client.New(cfg.IdentityURL, cfg.InternalKey), delivery, prod)
	router := handler.NewRouter(h, handler.RouterConfig{
		JWTSecret:   cfg.JWTSecret,
		InternalKey: cfg.InternalKey,
		CORSOrigins: cfg.CORSOrigins,
	})

	log.Printf("notification-service listening on :%s (%s, provider=%s)", cfg.Port, cfg.AppEnv, delivery.Name())
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
