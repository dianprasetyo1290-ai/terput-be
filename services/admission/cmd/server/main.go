package main

import (
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"skci/admission/internal/common"
	"skci/admission/internal/config"
	"skci/admission/internal/handler"
	"skci/admission/internal/otp"
	"skci/admission/internal/storage"
	"skci/admission/internal/store"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var inner otp.Provider = otp.DevProvider{Writer: os.Stdout, Enabled: cfg.OTPProvider == "dev"}
	if cfg.OTPProvider == "smtp" {
		inner = otp.SMTPProvider{
			Host: os.Getenv("SMTP_HOST"), Port: envOr("SMTP_PORT", "587"),
			User: os.Getenv("SMTP_USER"), Password: os.Getenv("SMTP_PASSWORD"),
			FromName: os.Getenv("SMTP_FROM_NAME"), FromAddr: os.Getenv("SMTP_FROM_ADDRESS"),
			SchoolName: cfg.SchoolName,
		}
	}
	provider := otp.SafeProvider{Inner: inner, Production: cfg.Production()}
	app := handler.New(db, cfg, provider, storage.NewLocal(cfg.StorageRoot, cfg.MaxUploadBytes))

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "X-Guardian-Token", "Idempotency-Key"},
		AllowCredentials: true,
	}))

	// Health check (no auth, no envelope) — see docs/CONVENTIONS.md.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// ---------------------------------------------------------------------
	// Public PPDB catalogue + guardian OTP login.
	// ---------------------------------------------------------------------
	r.Get("/public/discovery", app.Discovery)
	r.Post("/public/otp/request", app.RequestOTP)
	r.Post("/public/otp/verify", app.VerifyOTP)
	r.Post("/public/otp/resend", app.ResendOTP)

	// ---------------------------------------------------------------------
	// Guardian portal (admission guardian session token).
	// ---------------------------------------------------------------------
	r.Group(func(g chi.Router) {
		g.Use(common.GuardianAuth(cfg.GuardianJWTSecret))

		g.Get("/guardian/dashboard", app.GuardianDashboard)

		g.Get("/guardian/applicants", app.ListApplicants)
		g.Post("/guardian/applicants", app.CreateApplicant)
		g.Get("/guardian/applicants/{id}", app.GetApplicant)
		g.Put("/guardian/applicants/{id}", app.UpdateApplicant)
		g.Delete("/guardian/applicants/{id}", app.DeleteApplicant)

		g.Post("/guardian/applications", app.CreateApplication)
		g.Get("/guardian/applications/{id}", app.ApplicationDetail)
		g.Post("/guardian/applications/{id}/submit", app.SubmitApplication)

		g.Get("/guardian/applications/{id}/documents", app.ListDocuments)
		g.Post("/guardian/applications/{id}/documents", app.UploadDocument)
		g.Delete("/guardian/applications/{id}/documents/{docId}", app.DeleteDocument)
		g.Get("/guardian/applications/{id}/documents/{docId}/download", app.DownloadDocument)

		g.Get("/guardian/applications/{id}/charge", app.GetCharge)
		g.Post("/guardian/applications/{id}/charge/voucher", app.ApplyVoucher)

		g.Get("/guardian/applications/{id}/payments", app.ListPayments)
		g.Post("/guardian/applications/{id}/payments", app.SubmitPayment)

		g.Get("/guardian/applications/{id}/cbt", app.GetCBT)
		g.Post("/guardian/applications/{id}/cbt/enter", app.EnterCBT)

		g.Get("/guardian/applications/{id}/decision", app.GetDecision)
		g.Post("/guardian/applications/{id}/decision/respond", app.RespondDecision)

		g.Get("/guardian/applications/{id}/reregistration", app.GetReregistration)
	})

	// ---------------------------------------------------------------------
	// Admin PPDB (identity JWT, admin or superadmin).
	// ---------------------------------------------------------------------
	r.Group(func(ad chi.Router) {
		ad.Use(common.AdminAuth(cfg.JWTSecret))

		ad.Get("/admin/dashboard", app.AdminDashboard)

		ad.Get("/admin/applications", app.ListApplications)
		ad.Get("/admin/applications/{id}", app.AdminApplicationDetail)
		ad.Post("/admin/applications/{id}/request_correction", app.RequestCorrection)
		ad.Post("/admin/applications/{id}/mark_eligible", app.MarkEligible)
		ad.Post("/admin/applications/{id}/reject", app.RejectApplication)

		ad.Get("/admin/config/periods", app.ListPeriods)
		ad.Post("/admin/config/periods", app.CreatePeriod)
		ad.Put("/admin/config/periods/{id}", app.UpdatePeriod)

		ad.Get("/admin/config/waves", app.ListWaves)
		ad.Post("/admin/config/waves", app.CreateWave)
		ad.Put("/admin/config/waves/{id}", app.UpdateWave)

		ad.Get("/admin/config/tracks", app.ListTracks)
		ad.Post("/admin/config/tracks", app.CreateTrack)
		ad.Put("/admin/config/tracks/{id}", app.UpdateTrack)

		ad.Get("/admin/config/document-requirements", app.ListRequirements)
		ad.Post("/admin/config/document-requirements", app.CreateRequirement)
		ad.Put("/admin/config/document-requirements/{id}", app.UpdateRequirement)

		ad.Get("/admin/documents", app.DocumentQueue)
		ad.Post("/admin/documents/{id}/review", app.ReviewDocument)
		ad.Get("/admin/documents/{id}/download", app.AdminDownloadDocument)

		ad.Get("/admin/finance/fee-schemes", app.ListFeeSchemes)
		ad.Post("/admin/finance/fee-schemes", app.CreateFeeScheme)
		ad.Put("/admin/finance/fee-schemes/{id}", app.UpdateFeeScheme)

		ad.Get("/admin/finance/discount-rules", app.ListDiscountRules)
		ad.Post("/admin/finance/discount-rules", app.CreateDiscountRule)
		ad.Put("/admin/finance/discount-rules/{id}", app.UpdateDiscountRule)

		ad.Get("/admin/finance/vouchers", app.ListVouchers)
		ad.Post("/admin/finance/vouchers", app.CreateVoucher)
		ad.Put("/admin/finance/vouchers/{id}", app.UpdateVoucher)

		ad.Get("/admin/payments", app.PaymentQueue)
		ad.Post("/admin/payments/{id}/review", app.ReviewPayment)
		ad.Post("/admin/payments/{id}/reversal", app.ReversePayment)

		ad.Get("/admin/selection/formulas", app.ListFormulas)
		ad.Post("/admin/selection/formulas", app.CreateFormula)
		ad.Put("/admin/selection/formulas/{id}", app.UpdateFormula)
		ad.Get("/admin/selection/scores", app.ListScores)
		ad.Put("/admin/selection/scores/{applicationId}", app.UpsertScore)
		ad.Post("/admin/selection/rankings/preview", app.RankingPreview)
		ad.Post("/admin/selection/rankings", app.RunRanking)
		ad.Post("/admin/selection/decisions/{applicationId}", app.FinalizeDecision)

		ad.Get("/admin/conversions", app.ListConversions)
		ad.Post("/admin/conversions", app.StartConversion)
		ad.Post("/admin/conversions/{id}/retry", app.RetryConversion)

		ad.Get("/admin/audit-events", app.ListAuditEvents)
	})

	log.Printf("admission-service listening on :%s (%s)", cfg.Port, cfg.AppEnv)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
