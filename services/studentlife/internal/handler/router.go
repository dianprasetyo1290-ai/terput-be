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

	"skci/studentlife/internal/activity"
	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/consent"
	"skci/studentlife/internal/counseling"
	"skci/studentlife/internal/discipline"
	"skci/studentlife/internal/health"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

// App holds the service's domain handlers.
type App struct {
	DB         *sql.DB
	Cfg        config.Config
	Client     *client.Client
	Health     *health.Handler
	Consent    *consent.Handler
	Sensitive  *sensitive.Handler
	Counseling *counseling.Handler
	Discipline *discipline.Handler
	Activity   *activity.Handler
}

// New builds the App and registers each module's consent subjects and
// sensitive modules (T20 adds referral → counseling, T21 adds event_registration → activity).
func New(db *sql.DB, cfg config.Config, cr *sensitive.Service, cl *client.Client) *App {
	h := &health.Handler{DB: db, Crypto: cr, Client: cl, Cfg: cfg}
	coun := counseling.NewHandler(db, cr, cl, cfg)
	disc := discipline.NewHandler(db, cl, cfg)
	actStore := activity.NewStore(db)
	act := activity.NewHandler(db, actStore, cl, cfg, nil)

	return &App{
		DB: db, Cfg: cfg, Client: cl, Health: h, Counseling: coun, Discipline: disc, Activity: act,
		Consent: &consent.Handler{DB: db, Client: cl, Subjects: map[string]consent.Subject{
			health.ConsentSubjectProgram:            h.ConsentSubject(),
			counseling.ConsentSubjectReferral:       coun.ConsentSubject(),
			activity.ConsentSubjectEventRegistration: act.ConsentSubject(),
		}},
		Sensitive: &sensitive.Handler{DB: db, Client: cl, ModuleFeatures: map[string]string{
			sensitive.ModuleHealth:     common.FeatureHealth,
			sensitive.ModuleCounseling: common.FeatureCounseling,
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
		in.Get("/internal/achievements/summary", a.Activity.InternalAchievementSummary)
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

		// T20: Bimbingan Konseling (Menu 130, 132)
		pr.Route("/counseling", func(cr chi.Router) {
			cr.Use(a.Client.RequireFeature(common.FeatureCounseling))
			c := a.Counseling
			cr.Get("/categories", c.ListCategories)
			cr.Post("/categories", c.CreateCategory)
			cr.Put("/categories/{id}", c.UpdateCategory)
			cr.Delete("/categories/{id}", c.DeleteCategory)

			cr.Get("/cases", c.ListCases)
			cr.Post("/cases", c.CreateCase)
			cr.Get("/cases/{id}", c.GetCase)
			cr.Put("/cases/{id}", c.UpdateCase)
			cr.Post("/cases/{id}/close", c.CloseCase)
			cr.Post("/cases/{id}/members", c.AddCaseMember)
			cr.Delete("/cases/{id}/members/{userId}", c.RemoveCaseMember)

			cr.Get("/cases/{id}/referrals", c.ListReferrals)
			cr.Post("/cases/{id}/referrals", c.CreateReferral)
			cr.Put("/referrals/{id}", c.UpdateReferral)

			cr.Get("/cases/{id}/tasks", c.ListCaseTasks)
			cr.Post("/cases/{id}/tasks", c.CreateCaseTask)
			cr.Post("/tasks/{id}/done", c.MarkTaskDone)
			cr.Get("/my/tasks", c.ListMyTasks)

			cr.Get("/sessions", c.ListSessions)
			cr.Post("/sessions", c.CreateSession)
			cr.Get("/sessions/{id}", c.GetSession)
			cr.Put("/sessions/{id}", c.UpdateSession)

			cr.Get("/students/{id}/status", c.GetStudentHomeroomStatus) // AC-130-01
			cr.Get("/settings", c.GetSettings)
			cr.Put("/settings", c.PutSettings)
		})

		// T20: Kedisiplinan & Poin (Menu 131)
		pr.Route("/discipline", func(dr chi.Router) {
			dr.Use(a.Client.RequireFeature(common.FeatureDiscipline))
			d := a.Discipline
			dr.Get("/types", d.ListTypes)
			dr.Post("/types", d.CreateType)
			dr.Put("/types/{id}", d.UpdateType)
			dr.Delete("/types/{id}", d.DeleteType)

			dr.Get("/thresholds", d.ListThresholds)
			dr.Post("/thresholds", d.CreateThreshold)
			dr.Put("/thresholds/{id}", d.UpdateThreshold)
			dr.Delete("/thresholds/{id}", d.DeleteThreshold)

			dr.Get("/settings", d.GetSettings)
			dr.Put("/settings", d.PutSettings)

			dr.Get("/records", d.ListRecords)
			dr.Post("/records", d.CreateRecord)
			dr.Get("/records/{id}/evidence/{n}", d.GetEvidenceFile)
			dr.Post("/records/{id}/appeals", d.CreateAppeal)

			dr.Get("/students/{id}/summary", d.GetStudentSummary)
			dr.Get("/actions", d.ListActions)
			dr.Put("/actions/{id}", d.UpdateAction)
			dr.Get("/my/appeals", d.ListMyAppeals)
		})

		// T21: Event, Ekstrakurikuler & Prestasi
		pr.Route("/activities", func(ar chi.Router) {
			ar.Get("/calendar", a.Activity.Calendar)

			ar.Group(func(er chi.Router) {
				er.Use(a.Client.RequireFeature(common.FeatureEvent))
				er.Get("/event-kinds", a.Activity.ListEventKinds)
				er.Post("/event-kinds", a.Activity.CreateEventKind)
				er.Put("/event-kinds/{id}", a.Activity.UpdateEventKind)
				er.Delete("/event-kinds/{id}", a.Activity.DeleteEventKind)

				er.Get("/events", a.Activity.ListEvents)
				er.Post("/events", a.Activity.CreateEvent)
				er.Get("/events/{id}", a.Activity.GetEvent)
				er.Put("/events/{id}", a.Activity.UpdateEvent)
				er.Delete("/events/{id}", a.Activity.DeleteEvent)
				er.Post("/events/{id}/submit", a.Activity.SubmitEvent)
				er.Post("/events/{id}/publish", a.Activity.PublishEvent)
				er.Post("/events/{id}/cancel", a.Activity.CancelEvent)
				er.Get("/events/{id}/registrations", a.Activity.ListEventRegistrations)
				er.Post("/events/{id}/registrations", a.Activity.RegisterStudent)
				er.Post("/registrations/{id}/attendance", a.Activity.MarkAttendance)
			})

			ar.Group(func(xr chi.Router) {
				xr.Use(a.Client.RequireFeature(common.FeatureExtracurricular))
				xr.Get("/ecc-periods", a.Activity.ListPeriods)
				xr.Post("/ecc-periods", a.Activity.CreatePeriod)
				xr.Get("/extracurriculars", a.Activity.ListExtracurriculars)
				xr.Post("/extracurriculars", a.Activity.CreateExtracurricular)
				xr.Get("/extracurriculars/{id}", a.Activity.GetExtracurricular)
				xr.Put("/extracurriculars/{id}", a.Activity.UpdateExtracurricular)
				xr.Delete("/extracurriculars/{id}", a.Activity.DeleteExtracurricular)
				xr.Get("/extracurriculars/{id}/members", a.Activity.ListMembers)
				xr.Post("/extracurriculars/{id}/members/{mid}/status", a.Activity.UpdateMemberStatus)
				xr.Get("/extracurriculars/{id}/sessions", a.Activity.ListSessions)
				xr.Post("/extracurriculars/{id}/sessions", a.Activity.CreateSession)
				xr.Get("/extracurriculars/{id}/assessments", a.Activity.ListAssessments)
				xr.Put("/extracurriculars/{id}/assessments", a.Activity.UpsertAssessment)
			})

			ar.Group(func(hr chi.Router) {
				hr.Use(a.Client.RequireFeature(common.FeatureAchievement))
				hr.Get("/achievements", a.Activity.ListAchievements)
				hr.Post("/achievements", a.Activity.CreateAchievement)
				hr.Post("/achievements/{id}/verify", a.Activity.VerifyAchievement)
				hr.Post("/achievements/{id}/reject", a.Activity.RejectAchievement)
				hr.Get("/achievements/summary", a.Activity.AchievementSummary)
			})
		})

		pr.Route("/me/activities", func(mr chi.Router) {
			mr.Get("/events", func(w http.ResponseWriter, r *http.Request) {
				claims := common.GetClaims(r.Context())
				list, err := a.Activity.Store.ListMyRegistrations(r.Context(), claims.SchoolID, claims.UserID)
				if err != nil {
					common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kegiatan saya")
					return
				}
				common.WriteSuccess(w, list, nil)
			})
			mr.Post("/events/{id}/register", a.Activity.RegisterSelf)
			mr.Delete("/registrations/{id}", a.Activity.CancelRegistration)
			mr.Get("/extracurriculars", func(w http.ResponseWriter, r *http.Request) {
				claims := common.GetClaims(r.Context())
				list, err := a.Activity.Store.ListMyMemberships(r.Context(), claims.SchoolID, claims.UserID)
				if err != nil {
					common.WriteError(w, http.StatusInternalServerError, "Gagal memuat ekskul saya")
					return
				}
				common.WriteSuccess(w, list, nil)
			})
			mr.Post("/extracurriculars/{id}/join", a.Activity.JoinEccSelf)
			mr.Post("/extracurriculars/{id}/leave", a.Activity.LeaveEccSelf)
			mr.Get("/achievements", a.Activity.ListAchievements)
			mr.Post("/achievements", a.Activity.CreateAchievement)
		})

		pr.Route("/parent/activities", func(par chi.Router) {
			par.Get("/events", func(w http.ResponseWriter, r *http.Request) {
				claims := common.GetClaims(r.Context())
				studentID := r.URL.Query().Get("student_id")
				list, err := a.Activity.Store.ListMyRegistrations(r.Context(), claims.SchoolID, studentID)
				if err != nil {
					common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kegiatan anak")
					return
				}
				common.WriteSuccess(w, list, nil)
			})
			par.Post("/events/{id}/register", a.Activity.RegisterByParent)
			par.Get("/extracurriculars", func(w http.ResponseWriter, r *http.Request) {
				claims := common.GetClaims(r.Context())
				studentID := r.URL.Query().Get("student_id")
				list, err := a.Activity.Store.ListMyMemberships(r.Context(), claims.SchoolID, studentID)
				if err != nil {
					common.WriteError(w, http.StatusInternalServerError, "Gagal memuat ekskul anak")
					return
				}
				common.WriteSuccess(w, list, nil)
			})
			par.Post("/extracurriculars/{id}/join", a.Activity.JoinEccByParent)
			par.Get("/achievements", a.Activity.ListAchievements)
			par.Post("/achievements", a.Activity.CreateAchievement)
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
	Comment     string `json:"comment"`
}

// ApprovalCallback: POST /internal/approval-callbacks (core, idempotent),
// dispatched per request_type (T20 adds discipline_appeal, T21 adds event).
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
	case "discipline_appeal":
		a.disciplineAppealCallback(w, r, body)
	case "event":
		a.eventApprovalCallback(w, r, body)
	default:
		common.WriteError(w, http.StatusBadRequest, "request_type tidak dikenal")
	}
}

func (a *App) eventApprovalCallback(w http.ResponseWriter, r *http.Request, body approvalCallback) {
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses callback")
		return
	}
	defer tx.Rollback()

	if err := a.Activity.ApplyApprovalDecision(r.Context(), tx, body.SourceID, body.Decision, body.DecidedBy); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses keputusan approval event")
		return
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan keputusan callback")
		return
	}

	common.WriteSuccess(w, map[string]string{"id": body.SourceID, "status": body.Decision}, nil)
}

func (a *App) disciplineAppealCallback(w http.ResponseWriter, r *http.Request, body approvalCallback) {
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses callback")
		return
	}
	defer tx.Rollback()

	if err := a.Discipline.ApplyApprovalDecision(r.Context(), tx, body.SourceID, body.Decision, body.DecidedBy, body.Comment); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "Keberatan tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses keputusan banding")
		return
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan keputusan callback")
		return
	}

	common.WriteSuccess(w, map[string]string{"id": body.SourceID, "status": body.Decision}, nil)
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
