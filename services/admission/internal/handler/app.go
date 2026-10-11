package handler

import (
	"database/sql"

	"skci/admission/internal/client"
	"skci/admission/internal/config"
	"skci/admission/internal/otp"
	"skci/admission/internal/storage"
)

// App carries every dependency the HTTP handlers need. Handlers talk to the
// database directly through App.DB (the service is small enough that a query
// layer would only add indirection); pure domain rules live in internal/model.
type App struct {
	DB       *sql.DB
	Config   config.Config
	OTP      *otp.Service
	Provider otp.Provider
	Storage  storage.Storage

	Identity *client.Client
	Exam     *client.Client
	Academic *client.Client
}

// New wires the application graph.
func New(db *sql.DB, cfg config.Config, provider otp.Provider, store storage.Storage) *App {
	return &App{
		DB:       db,
		Config:   cfg,
		OTP:      otp.New(cfg.GuardianJWTSecret, cfg.OTPTTL, cfg.OTPCooldown, cfg.OTPMaxAttempts),
		Provider: provider,
		Storage:  store,
		Identity: client.New(cfg.IdentityURL, cfg.InternalKey),
		Exam:     client.New(cfg.ExamURL, cfg.InternalKey),
		Academic: client.New(cfg.AcademicURL, cfg.InternalKey),
	}
}
