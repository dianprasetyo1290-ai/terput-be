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
	"skci/ops/internal/crypto"
	"skci/ops/internal/facility"
	"skci/ops/internal/handler"
	"skci/ops/internal/inventory"
	"skci/ops/internal/jobs"
	"skci/ops/internal/library"
	"skci/ops/internal/procurement"
	"skci/ops/internal/storage"
	"skci/ops/internal/store"
	"skci/ops/internal/transport"
)

type woPurchaseRequester struct {
	store *procurement.Store
}

func (w *woPurchaseRequester) CreateFromWorkOrder(ctx context.Context, in facility.WOPurchaseInput) (string, error) {
	neededBy := time.Now().AddDate(0, 0, 7)
	reason := "Kebutuhan perbaikan work order: " + in.WorkOrderCode
	if in.WorkOrderCode == "" {
		reason = "Kebutuhan perbaikan work order: " + in.WorkOrderID
	}
	pr := procurement.PurchaseRequest{
		SchoolID:        in.SchoolID,
		RequesterUserID: in.RequesterUserID,
		Reason:          reason,
		NeededBy:        &neededBy,
	}
	var lines []procurement.PurchaseRequestLine
	for _, l := range in.Lines {
		unitPrice := int64(0)
		if l.Qty > 0 {
			unitPrice = l.EstAmountIDR / int64(l.Qty)
		}
		lines = append(lines, procurement.PurchaseRequestLine{
			Description:     l.Description,
			Qty:             int64(l.Qty),
			Unit:            "pcs",
			EstUnitPriceIDR: unitPrice,
		})
	}
	res, err := w.store.CreatePR(ctx, pr, lines, in.WorkOrderID)
	if err != nil {
		return "", err
	}
	return res.Number, nil
}

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

	cryptoSvc, err := crypto.New(cfg.OpsEncKey, cfg.OpsBlindKey)
	if err != nil {
		log.Fatalf("failed to init crypto service: %v", err)
	}

	loc, _ := time.LoadLocation(cfg.SchoolTimezone) // validated in config.Load
	apiClient := client.New(cfg.IdentityURL, cfg.CoreURL, cfg.NotificationURL, cfg.InternalKey, cfg.FeatureCacheTTL)

	invStore := inventory.NewStore(db, loc)
	invStorage := storage.NewLocal(cfg.StorageRoot, cfg.MaxUploadBytes, "application/pdf")
	invHandler := inventory.NewHandler(invStore, apiClient, invStorage, cfg.OpsPublicInternalURL)

	procStore := procurement.NewStore(db, cryptoSvc)
	procStorage := storage.NewLocal(cfg.StorageRoot, cfg.MaxUploadBytes, "application/pdf")
	procHandler := procurement.NewHandler(procStore, apiClient, procStorage, cfg.OpsPublicInternalURL)

	libStore := library.NewStore(db, loc)
	libRunner := library.NewJobRunner(db, apiClient, cfg.AccountingURL, cfg.InternalKey, loc)
	libHandler := library.NewHandler(libStore, apiClient, libRunner)

	trStore := transport.NewStore(db, loc)
	trRunner := transport.NewJobRunner(db, apiClient, cfg.AccountingURL, cfg.InternalKey, loc)
	trHandler := transport.NewHandler(trStore, apiClient, trRunner, cfg.HRURL, cfg.AccountingURL)

	fac := &facility.Handler{
		Store:        facility.NewStore(db, loc),
		Client:       apiClient,
		Storage:      storage.NewLocal(cfg.StorageRoot, cfg.MaxUploadBytes, facility.PhotoMIMEs...),
		MaxUpload:    cfg.MaxUploadBytes,
		CallbackBase: cfg.OpsPublicInternalURL,
		Purchase:     &woPurchaseRequester{store: procStore},
	}
	app := &handler.App{
		DB:          db,
		Config:      &cfg,
		Client:      apiClient,
		Facility:    fac,
		Inventory:   invHandler,
		Procurement: procHandler,
		Library:     libHandler,
		Transport:   trHandler,
	}

	syncJob := procurement.NewSyncJob(db, apiClient, cfg.AccountingURL, cfg.InternalKey)

	runner := jobs.NewRunner(db, cfg.JobInterval)
	runner.Register("facility_preventive", func(ctx context.Context, _ *sql.DB) error { return fac.RunPreventive(ctx) })
	runner.Register("vendor_accounting_sync", func(ctx context.Context, _ *sql.DB) error { return syncJob.RunSyncVendors(ctx) })
	runner.Register("library_daily", func(ctx context.Context, _ *sql.DB) error { return libRunner.RunDailyTasksForAll(ctx) })
	runner.Register("library_sync_members", func(ctx context.Context, _ *sql.DB) error { return libRunner.RunMemberSyncForAll(ctx) })
	runner.Register("transport_daily", func(ctx context.Context, _ *sql.DB) error { return trRunner.RunDailyTasksForAll(ctx) })
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
