package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
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

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, common.SentryMiddleware(), middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "Idempotency-Key"},
		AllowCredentials: true,
	}))

	// Health check (no auth, no envelope) — see docs/CONVENTIONS.md.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Payment Gateway Webhook (no JWT auth, verified via SNAP signature)
	r.Post("/webhooks/nicepay", app.NicepayWebhook)
	// T14: webhook per provider + mandate notifications.
	r.Post("/webhooks/{provider}", app.ProviderWebhook)
	r.Post("/webhooks/{provider}/mandates", app.MandateWebhook)

	// ---------------------------------------------------------------------
	// Portal Orang Tua (identity JWT, active_role=orang_tua). Child
	// ownership is verified against identity on every request.
	// ---------------------------------------------------------------------
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(cfg.JWTSecret))

		pr.Get("/payment-methods", app.ListPaymentMethods)
		pr.Post("/parent/checkout/preview", app.ParentCheckoutPreview)
		pr.Post("/parent/checkout", app.ParentCheckout)
		pr.Get("/parent/transactions/{id}", app.ParentGetTransaction)
		pr.Post("/parent/transactions/{id}/renew", app.ParentRenewTransaction)
		// T14: autodebit (orang_tua, payment_autodebit).
		pr.Get("/parent/autodebit/mandates", app.ParentListMandates)
		pr.Post("/parent/autodebit/mandates", app.ParentCreateMandate)
		pr.Post("/parent/autodebit/mandates/{id}/cancel", app.ParentCancelMandate)
		pr.Get("/parent/autodebit/attempts", app.ParentListAttempts)
		pr.Get("/payment-providers/options", app.ListPaymentProviderOptions)

		pr.Get("/parent/children/{studentUserId}/bills", app.ListChildBills)
		pr.Get("/parent/children/{studentUserId}/bills/{billId}", app.GetChildBill)
		pr.Get("/parent/children/{studentUserId}/bills/{billId}/invoice.pdf", app.ChildBillInvoice)
		pr.Post("/parent/children/{studentUserId}/bills/{billId}/payments", app.SubmitPayment)
		pr.Get("/parent/children/{studentUserId}/payments", app.ListChildPayments)
		pr.Get("/parent/children/{studentUserId}/payments/{paymentId}/proof", app.GetChildPaymentProof)
	})

	// ---------------------------------------------------------------------
	// Portal Siswa (identity JWT, active_role=siswa) — own records only.
	// ---------------------------------------------------------------------
	r.Group(func(st chi.Router) {
		st.Use(common.JWTMiddleware(cfg.JWTSecret))

		st.Get("/me/bills", app.ListMyBills)
		st.Get("/me/bills/{id}/invoice.pdf", app.MyBillInvoice)
		st.Get("/me/bills/visibility", app.GetMyBillVisibility)
		st.Get("/me/payments", app.ListMyPayments)
	})

	// ---------------------------------------------------------------------
	// Admin (identity JWT, admin or superadmin, school-scoped).
	// ---------------------------------------------------------------------
	r.Group(func(ad chi.Router) {
		ad.Use(common.JWTMiddleware(cfg.JWTSecret))

		ad.Get("/admin/schemes", app.ListSchemes)
		ad.Post("/admin/schemes", app.CreateScheme)
		ad.Get("/admin/schemes/{id}", app.GetScheme)
		ad.Put("/admin/schemes/{id}", app.UpdateScheme)
		ad.Delete("/admin/schemes/{id}", app.ArchiveScheme)
		ad.Get("/admin/schemes/{id}/changes", app.ListSchemeChanges)

		ad.Post("/admin/bills/generate", app.GenerateBills)
		ad.Post("/admin/bills/reminders", app.AdminBillReminders)
		ad.Get("/admin/bills/reminder-log", app.ListReminderLog)
		ad.Get("/admin/billing/reminder-settings", app.GetReminderSettings)
		ad.Put("/admin/billing/reminder-settings", app.PutReminderSettings)
		ad.Get("/admin/billing/followups", app.ListFollowups)
		ad.Post("/admin/billing/followups", app.CreateFollowup)
		ad.Get("/admin/bills", app.ListBills)
		ad.Post("/admin/bills/generate-annual", app.GenerateAnnualBills)
		ad.Get("/admin/bills/{id}", app.GetBill)
		ad.Get("/admin/bills/{id}/invoice.pdf", app.AdminBillInvoice)
		ad.Put("/admin/bills/{id}/installments", app.SetBillInstallments)
		ad.Post("/admin/bills/{id}/change-requests", app.CreateBillChangeRequest)
		ad.Get("/admin/bill-change-requests", app.ListBillChangeRequests)
		ad.Get("/admin/arrears", app.ListArrears)

		ad.Get("/admin/payments", app.PaymentQueue)
		ad.Post("/admin/payments/{id}/review", app.ReviewPayment)
		ad.Post("/admin/payments/{id}/reversal", app.ReversePayment)
		ad.Get("/admin/payments/{id}/proof", app.AdminPaymentProof)

		ad.Get("/admin/reconciliation.csv", app.ExportReconciliation)

		ad.Get("/admin/discounts", app.ListDiscounts)
		ad.Get("/admin/discounts/recipients", app.ListDiscountRecipients)
		ad.Post("/admin/discounts", app.CreateDiscount)
		ad.Post("/admin/discounts/{id}/deactivate", app.DeactivateDiscount)

		ad.Get("/admin/refunds", app.ListRefunds)
		ad.Post("/admin/refunds", app.CreateRefund)
		ad.Post("/admin/refunds/{id}/decision", app.DecideRefund)
		ad.Post("/admin/refunds/{id}/process", app.ProcessRefund)

		ad.Get("/admin/finance/summary", app.FinanceSummary)
		ad.Get("/admin/finance/aging", app.FinanceAging)
		ad.Get("/admin/finance/receipts", app.FinanceReceipts)
		ad.Get("/admin/finance/unallocated", app.UnallocatedFunds)
		ad.Get("/admin/students/{id}/ledger", app.StudentLedger)
		// T6 Payment Gateway & Cashier routes
		ad.Get("/admin/payment-methods", app.AdminListPaymentMethods)
		ad.Post("/admin/payment-methods", app.AdminCreatePaymentMethod)
		ad.Put("/admin/payment-methods/{id}", app.AdminUpdatePaymentMethod)
		ad.Delete("/admin/payment-methods/{id}", app.AdminDeletePaymentMethod)

		ad.Get("/admin/transactions", app.AdminListTransactions)
		ad.Get("/admin/transactions/{id}", app.AdminGetTransaction)
		ad.Post("/admin/receipts/{id}/resend", app.AdminResendReceipt)

		ad.Post("/cashier/payments", app.CashierCreatePayment)
		ad.Get("/cashier/payments", app.CashierListPayments)
		ad.Get("/cashier/payments/{id}/receipt.pdf", app.CashierReceiptPDF)

		ad.Post("/admin/payment-corrections", app.AdminCreateCorrection)

		ad.Post("/admin/settlements/import", app.AdminImportSettlement)
		ad.Get("/admin/settlements", app.AdminListSettlements)

		ad.Post("/admin/reconciliation/run", app.AdminRunReconciliation)
		ad.Get("/admin/reconciliation/exceptions", app.AdminListExceptions)
		ad.Post("/admin/reconciliation/exceptions/{id}/resolve", app.AdminResolveException)

		ad.Post("/admin/bank-statements/import", app.AdminImportBankStatement)
		ad.Post("/admin/bank-reconciliation/run", app.AdminRunBankReconciliation)

		ad.Get("/admin/payment-dashboard", app.AdminPaymentDashboard)

		// T14: providers (superadmin), routing, autodebit administration.
		ad.Get("/platform/payment-providers", app.PlatformListProviders)
		ad.Post("/platform/payment-providers", app.PlatformCreateProvider)
		ad.Put("/platform/payment-providers/{code}", app.PlatformUpdateProvider)
		ad.Get("/admin/payment-routing-rules", app.AdminListRoutingRules)
		ad.Post("/admin/payment-routing-rules", app.AdminCreateRoutingRule)
		ad.Post("/admin/payment-routing-rules/preview", app.AdminPreviewRouting)
		ad.Put("/admin/payment-routing-rules/{id}", app.AdminUpdateRoutingRule)
		ad.Delete("/admin/payment-routing-rules/{id}", app.AdminDeleteRoutingRule)
		ad.Get("/admin/autodebit/mandates", app.AdminListMandates)
		ad.Get("/admin/autodebit/attempts", app.AdminListAttempts)
		ad.Get("/admin/autodebit/settings", app.AdminGetAutodebitSettings)
		ad.Put("/admin/autodebit/settings", app.AdminPutAutodebitSettings)

		// P3A: account mapping (BR-ACC-05) and journal posting monitor (IF-ACC-02).
		ad.Get("/admin/journal-accounts", app.GetJournalAccounts)
		ad.Put("/admin/journal-accounts/settings", app.PutJournalSettings)
		ad.Put("/admin/journal-accounts/gateway", app.PutGatewayAccounts)
		ad.Put("/admin/journal-accounts/components/{code}", app.PutComponentAccount)
		ad.Delete("/admin/journal-accounts/components/{code}", app.DeleteComponentAccount)
		ad.Get("/admin/journal-postings", app.ListJournalPostings)
		ad.Get("/admin/journal-postings/{id}", app.GetJournalPosting)
		ad.Post("/admin/journal-postings/{id}/retry", app.AdminRetryJournalPosting)

		ad.Get("/admin/student-bill-visibility", app.GetStudentBillVisibility)
		ad.Put("/admin/student-bill-visibility", app.UpdateStudentBillVisibility)
	})

	// ---------------------------------------------------------------------
	// Internal (service-to-service, X-Internal-Key).
	// ---------------------------------------------------------------------
	r.Group(func(in chi.Router) {
		in.Use(common.InternalAuth(cfg.InternalKey))

		in.Post("/internal/bills/generate", app.InternalGenerateBills)
		in.Post("/internal/bills/generate-annual", app.InternalGenerateAnnualBills)
		in.Post("/internal/bills/change-requests/{id}/callback", app.BillChangeCallback)
		in.Post("/internal/discounts/{id}/callback", app.DiscountCallback)
		in.Get("/internal/students/{studentUserId}/outstanding", app.InternalStudentOutstanding)
		in.Post("/internal/corrections/{id}/callback", app.CorrectionCallback)
		in.Post("/internal/schemes/changes/{id}/callback", app.SchemeChangeCallback)
		in.Post("/internal/refunds/{id}/callback", app.RefundCallback)
		in.Post("/internal/reconciliation/exceptions/{id}/callback", app.ReconExceptionCallback)
		in.Get("/internal/students/{studentUserId}/financial-summary", app.InternalStudentFinancialSummary)
		in.Post("/internal/students/{studentUserId}/exit", app.InternalStudentExit)
		in.Post("/internal/students/{studentUserId}/refund", app.InternalStudentRefund)
		// P3A: exception-queue retry_url targets (IF-ACC-03).
		in.Post("/internal/journal-postings/{id}/retry", app.InternalRetryJournalPosting)
		in.Post("/internal/journal-queue/retry", app.InternalRetryJournalQueue)

		// T24: Insight Read Model & Metric Facts
		in.Get("/internal/metric-facts", app.InternalMetricFacts)
		in.Get("/internal/risk-facts/arrears", app.InternalRiskFactsArrears)

		// T25: 3D Campus live canteen reads
		in.Get("/internal/canteen/directory", app.InternalCanteenDirectory)
		in.Get("/internal/canteen/tenants", app.InternalCanteenTenants)
		in.Get("/internal/canteen/tenants/{id}/menu", app.InternalCanteenTenantMenu)
	})

	log.Printf("billing-service listening on :%s (%s)", cfg.Port, cfg.AppEnv)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
