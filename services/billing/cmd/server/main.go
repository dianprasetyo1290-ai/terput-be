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

	// ---------------------------------------------------------------------
	// Portal Orang Tua (identity JWT, active_role=orang_tua). Child
	// ownership is verified against identity on every request.
	// ---------------------------------------------------------------------
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(cfg.JWTSecret))

		pr.Get("/parent/children/{studentUserId}/bills", app.ListChildBills)
		pr.Get("/parent/children/{studentUserId}/bills/{billId}", app.GetChildBill)
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

		ad.Post("/admin/bills/generate", app.GenerateBills)
		ad.Get("/admin/bills", app.ListBills)
		ad.Get("/admin/arrears", app.ListArrears)

		ad.Get("/admin/payments", app.PaymentQueue)
		ad.Post("/admin/payments/{id}/review", app.ReviewPayment)
		ad.Post("/admin/payments/{id}/reversal", app.ReversePayment)
		ad.Get("/admin/payments/{id}/proof", app.AdminPaymentProof)

		ad.Get("/admin/reconciliation.csv", app.ExportReconciliation)

		ad.Get("/admin/discounts", app.ListDiscounts)
		ad.Get("/admin/discounts/recipients", app.ListDiscountRecipients)

		ad.Get("/admin/refunds", app.ListRefunds)
		ad.Post("/admin/refunds", app.CreateRefund)
		ad.Post("/admin/refunds/{id}/decision", app.DecideRefund)
		ad.Post("/admin/refunds/{id}/process", app.ProcessRefund)

		ad.Get("/admin/finance/summary", app.FinanceSummary)
	})

	// ---------------------------------------------------------------------
	// Internal (service-to-service, X-Internal-Key).
	// ---------------------------------------------------------------------
	r.Group(func(in chi.Router) {
		in.Use(common.InternalAuth(cfg.InternalKey))

		in.Post("/internal/bills/generate", app.InternalGenerateBills)
		in.Get("/internal/students/{studentUserId}/outstanding", app.InternalStudentOutstanding)
	})

	log.Printf("billing-service listening on :%s (%s)", cfg.Port, cfg.AppEnv)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}
