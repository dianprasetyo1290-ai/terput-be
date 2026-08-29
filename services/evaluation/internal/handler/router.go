package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/evaluation/internal/cache"
	"skci/evaluation/internal/common"
)

func NewRouter(h *Handler, jwtSecret string, corsOrigins []string, respCache *cache.Cache) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(common.SentryMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", Healthz)

	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(jwtSecret))
		// Client request 2026-08-29: cache GET responses in Redis.
		pr.Use(respCache.Middleware(nil))

		// Feature 1: RPM (lesson plans)
		pr.Route("/me/lesson-plans", func(sr chi.Router) {
			sr.Post("/", h.CreateMyLessonPlan)
			sr.Get("/", h.ListMyLessonPlans)
			sr.Get("/rpm-template", h.DownloadRpmTemplate)
			sr.Get("/{id}", h.GetMyLessonPlan)
			sr.Put("/{id}", h.UpdateMyLessonPlan)
			sr.Delete("/{id}", h.DeleteMyLessonPlan)
			sr.Post("/{id}/rpm-import", h.ImportRpmTemplate)
		})
		pr.Route("/lesson-plans", func(sr chi.Router) {
			sr.Get("/", h.AdminListLessonPlans)
			sr.Get("/guru/{guru_id}", h.AdminGetLessonPlansByGuru)
		})

		// NEW: Program Tahunan (Prota) -- see internal/handler/prota.go
		pr.Route("/me/prota", func(sr chi.Router) {
			sr.Get("/template", h.DownloadProtaTemplate)
			sr.Post("/import", h.ImportProta)
			sr.Get("/", h.ListMyProta)
			sr.Get("/{id}", h.GetMyProta)
			sr.Delete("/{id}", h.DeleteMyProta)
		})
		pr.Route("/prota", func(sr chi.Router) {
			sr.Get("/", h.AdminListProta)
			sr.Get("/guru/{guru_id}", h.AdminGetProtaByGuru)
		})

		// NEW: Program Semester (Prosem) -- see internal/handler/prosem.go
		pr.Route("/me/prosem", func(sr chi.Router) {
			sr.Get("/template", h.DownloadProsemTemplate)
			sr.Post("/import", h.ImportProsem)
			sr.Get("/", h.ListMyProsem)
			sr.Get("/{id}", h.GetMyProsem)
			sr.Delete("/{id}", h.DeleteMyProsem)
		})
		pr.Route("/prosem", func(sr chi.Router) {
			sr.Get("/", h.AdminListProsem)
			sr.Get("/guru/{guru_id}", h.AdminGetProsemByGuru)
		})

		// Feature 2: Penilaian Guru
		pr.Get("/forms", h.ListForms)

		pr.Route("/periods", func(sr chi.Router) {
			sr.Post("/", h.CreatePeriod)
			sr.Get("/", h.ListPeriods)
			sr.Route("/{id}", func(pr2 chi.Router) {
				pr2.Get("/", h.GetPeriod)
				pr2.Put("/", h.UpdatePeriod)
				pr2.Delete("/", h.DeletePeriod)
				pr2.Get("/confirm-delete", h.ConfirmDeletePeriod)
				pr2.Post("/publish", h.PublishPeriod)
				pr2.Post("/unpublish", h.UnpublishPeriod)
				pr2.Get("/available-peer-evaluators", h.AvailablePeerEvaluators)
				pr2.Post("/evaluations", h.SubmitEvaluation)
				pr2.Get("/my-tasks", h.MyTasks)
				pr2.Get("/results", h.PeriodResults)
				pr2.Get("/guru/{guru_id}/report", h.GuruReport)
				pr2.Get("/leadership-results", h.LeadershipResults)
			})
		})
	})

	return r
}
