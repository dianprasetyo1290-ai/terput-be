// Package handler wires studentlife's HTTP routes, the generic approval
// callback dispatcher and the background jobs.
package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/consent"
	"skci/studentlife/internal/health"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

// App holds the service's domain handlers.
type App struct {
	DB        *sql.DB
	Cfg       config.Config
	Client    *client.Client
	Health    *health.Handler
	Consent   *consent.Handler
	Sensitive *sensitive.Handler
}

// New builds the App and registers each module's consent subjects and
// sensitive modules (T20 adds referral → counseling here).
func New(db *sql.DB, cfg config.Config, cr *sensitive.Service, cl *client.Client) *App {
	h := &health.Handler{DB: db, Crypto: cr, Client: cl, Cfg: cfg}
	return &App{
		DB: db, Cfg: cfg, Client: cl, Health: h,
		Consent: &consent.Handler{DB: db, Client: cl, Subjects: map[string]consent.Subject{
			health.ConsentSubjectProgram: h.ConsentSubject(),
		}},
		Sensitive: &sensitive.Handler{DB: db, Client: cl, ModuleFeatures: map[string]string{
			sensitive.ModuleHealth: common.FeatureHealth,
		}},
	}
}

// Router builds the chi router.
func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, common.SentryMiddleware(), middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   a.Cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "Idempotency-Key"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Group(func(in chi.Router) {
		in.Use(common.InternalAuth(a.Cfg.InternalKey))
		in.Post("/internal/approval-callbacks", a.ApprovalCallback)
	})

	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(a.Cfg.JWTSecret))

		// Generic: fitur dicek per subject_type / modul di handler.
		pr.Get("/consents/my", a.Consent.ListMine)
		pr.Post("/consents/{id}/decide", a.Consent.Decide)
		pr.Get("/sensitive/access-policies", a.Sensitive.ListPolicies)
		pr.Post("/sensitive/access-policies", a.Sensitive.GrantPolicy)
		pr.Delete("/sensitive/access-policies/{id}", a.Sensitive.RevokePolicy)
		pr.Get("/sensitive/access-logs", a.Sensitive.ListLogs)

		pr.Route("/health", func(hr chi.Router) {
			hr.Use(a.Client.RequireFeature(common.FeatureHealth))
			h := a.Health
			hr.Get("/students", h.ListStudents)
			hr.Get("/classrooms", h.ListClassrooms)
			hr.Get("/students/{id}/profile", h.GetProfile)
			hr.Put("/students/{id}/profile", h.PutProfile)
			hr.Post("/students/{id}/profile-changes", h.CreateChange)
			hr.Get("/students/{id}/visits", h.StudentVisits)
			hr.Get("/students/{id}/programs", h.StudentPrograms)
			hr.Get("/alerts", h.ListAlerts)
			hr.Get("/profile-changes", h.ListChanges)
			hr.Get("/profile-changes/{id}", h.GetChange)
			hr.Get("/visits", h.ListVisits)
			hr.Post("/visits", h.CreateVisit)
			hr.Get("/visits/{id}", h.GetVisit)
			hr.Post("/visits/{id}/addenda", h.AddAddendum)
			hr.Get("/medicines", h.ListMedicines)
			hr.Post("/medicines", h.CreateMedicine)
			hr.Put("/medicines/{id}", h.UpdateMedicine)
			hr.Delete("/medicines/{id}", h.DeleteMedicine)
			hr.Get("/programs", h.ListPrograms)
			hr.Post("/programs", h.CreateProgram)
			hr.Get("/programs/{id}", h.GetProgram)
			hr.Put("/programs/{id}", h.UpdateProgram)
			hr.Delete("/programs/{id}", h.CancelProgram)
			hr.Post("/programs/{id}/publish", h.PublishProgram)
			hr.Post("/programs/{id}/complete", h.CompleteProgram)
			hr.Get("/programs/{id}/participants", h.ListParticipants)
			hr.Put("/programs/{id}/participants/{pid}/result", h.RecordResult)
			hr.Post("/programs/{id}/participants/{pid}/follow-up-done", h.FollowUpDone)
			hr.Get("/programs/{id}/recap", h.Recap)
		})
	})
	return r
}

type approvalCallback struct {
	ApprovalID  string `json:"approval_id"`
	SourceID    string `json:"source_id"`
	RequestType string `json:"request_type"`
	Decision    string `json:"decision"`
	DecidedBy   string `json:"decided_by"`
}

// ApprovalCallback: POST /internal/approval-callbacks (core, idempotent),
// dispatched per request_type (T20 adds its own types).
func (a *App) ApprovalCallback(w http.ResponseWriter, r *http.Request) {
	var body approvalCallback
	if !common.Decode(w, r, &body) {
		return
	}
	if body.SourceID == "" || (body.Decision != "approved" && body.Decision != "rejected") {
		common.WriteError(w, http.StatusBadRequest, "Data callback tidak lengkap atau keputusan tidak valid")
		return
	}
	switch body.RequestType {
	case health.RequestTypeProfileUpdate:
		a.healthProfileCallback(w, r, body)
	default:
		common.WriteError(w, http.StatusBadRequest, "request_type tidak dikenal")
	}
}

func (a *App) healthProfileCallback(w http.ResponseWriter, r *http.Request, body approvalCallback) {
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses callback")
		return
	}
	defer tx.Rollback()
	ch, status, err := a.Health.ApplyApprovalDecision(r, tx, body.SourceID, body.Decision, body.DecidedBy)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Usulan tidak ditemukan")
		return
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses callback")
		return
	}
	if status != "" {
		labels := map[string]string{health.ChangeApproved: "disetujui", health.ChangeRejected: "ditolak",
			health.ChangeSuperseded: "tidak diterapkan karena profil sudah diperbarui"}
		_ = a.Client.Notify(r.Context(), client.Notification{SchoolID: ch.SchoolID, UserID: ch.SubmittedBy, Type: "health_profile_change",
			Title: "Perubahan profil kesehatan " + labels[status], Body: "Usulan perubahan profil kesehatan Ananda " + labels[status] + " oleh petugas UKS.",
			Data: map[string]any{"change_id": ch.ID, "student_id": ch.StudentID, "status": status}},
			"studentlife:change:"+ch.ID+":"+status)
	} else {
		status = ch.Status
	}
	common.WriteSuccess(w, map[string]string{"id": ch.ID, "status": status}, nil)
}

// JobForwardAudit forwards audit outbox rows to core POST /internal/audit.
func (a *App) JobForwardAudit(ctx context.Context, db *sql.DB) error {
	rows, err := store.PendingAudit(ctx, db, 200)
	if err != nil {
		return err
	}
	for _, e := range rows {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := a.Client.PostAudit(cctx, client.AuditEvent{SchoolID: e.SchoolID, Service: "studentlife", Module: e.Module,
			EntityType: e.EntityType, EntityID: e.EntityID, Action: e.Action, ActorUserID: e.ActorUserID, ActorRole: e.ActorRole,
			Before: e.BeforeRaw, After: e.AfterRaw, Reason: e.Reason, CorrelationID: e.CorrelationID})
		cancel()
		if err != nil {
			return err
		}
		if err := store.MarkAuditForwarded(ctx, db, e.ID); err != nil {
			return err
		}
	}
	return nil
}
