package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
	"skci/billing/internal/handler"
	"skci/billing/internal/jobs"
	"skci/billing/internal/nicepay"
	"skci/billing/internal/storage"
	"skci/billing/internal/store"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := common.InitSentry(cfg.SentryDSN, cfg.SentryEnvironment, "billing-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	app := handler.New(db, cfg, client.New(cfg.IdentityURL, cfg.InternalKey), storage.NewLocal(cfg.StorageRoot, cfg.MaxUploadBytes))

	app.Roster = client.New(cfg.AcademicURL, cfg.InternalKey)
	if cfg.Gateway.NicepayConfigured() {
		np, err := nicepay.NewClient(nicepay.Config{
			BaseURL:      cfg.Gateway.NicepayBaseURL,
			ClientID:     cfg.Gateway.NicepayClientID,
			ChannelID:    cfg.Gateway.NicepayChannelID,
			ClientSecret: cfg.Gateway.NicepayClientSecret,
			PrivateKey:   cfg.Gateway.NicepayPrivateKey,
			NotifPubKey:  cfg.Gateway.NicepayPublicKey,
			StoreID:      cfg.Gateway.NicepayStoreID,
		})
		if err != nil {
			log.Printf("nicepay client init error: %v", err)
		} else {
			app.Nicepay = np
		}
	}
	if cfg.Gateway.CoreURL != "" {
		app.Core = client.New(cfg.Gateway.CoreURL, cfg.InternalKey)
	}
	if cfg.Gateway.NotificationURL != "" {
		app.Notification = client.New(cfg.Gateway.NotificationURL, cfg.InternalKey)
	}
	if cfg.Gateway.AccountingURL != "" {
		app.Accounting = client.New(cfg.Gateway.AccountingURL, cfg.InternalKey)
	}

	// T12 & T14: kantin, dompet, dan payment-extension jobs.
	app.IdentitySvc = client.New(cfg.IdentityURL, cfg.InternalKey)
	app.PaymentFeatures = client.New(cfg.IdentityURL, cfg.InternalKey)
	app.PurposeCharger = app
	if app.PurposeHandlers == nil {
		app.PurposeHandlers = make(map[string]handler.PurposeHandler)
	}
	app.PurposeHandlers["wallet_topup"] = app.WalletTopupHook()

	gwRunner := jobs.NewRunner(db, cfg.Gateway.JobInterval)
	gwRunner.Register("autodebit_schedule", app.JobAutodebitSchedule)
	gwRunner.Register("retail_expiry", app.JobRetailExpiry)
	gwRunner.Register("gateway_expiry", app.JobGatewayExpiry)
	gwRunner.Register("settlement_reconciliation", app.JobSettlementReconciliation)
	gwRunner.Register("provider_health", app.JobProviderHealth)
	gwRunner.Register("gateway_journal", app.JobGatewayJournal)
	// P3A: billing/wallet/canteen auto-journals, then the accounting sender.
	gwRunner.Register("billing_journals", app.JobBillingJournals)
	gwRunner.Register("journal_outbox", app.JobJournalOutbox)
	gwRunner.Register("bill_reminders", app.JobBillReminders)
	gwRunner.Start()
	defer gwRunner.Stop()

	r := newRouter(cfg, app)

	log.Printf("billing-service listening on :%s (%s)", cfg.Port, cfg.AppEnv)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
