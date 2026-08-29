package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/report/internal/cache"
	"skci/report/internal/common"
)

func NewRouter(d *Deps, jwtSecret string, corsOrigins []string, respCache *cache.Cache) http.Handler {
	h := New(d)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(common.SentryMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", Healthz)

	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(jwtSecret))
		// Client request 2026-08-29: cache GET responses in Redis.
		pr.Use(respCache.Middleware(nil))

		pr.Post("/report-cards/generate", h.GenerateReportCards)
		pr.Get("/report-cards/{id}", h.GetReportCard)
		pr.Patch("/report-cards/{id}/subjects/{subject_id}", h.PatchSubjectOverride)
		pr.Patch("/report-cards/{id}/subjects/{subject_id}/notes", h.PatchSubjectNotes)
		pr.Post("/report-cards/{id}/finalize", h.FinalizeReportCard)
		pr.Put("/report-cards/{id}/unlock", h.UnlockReportCard)

		pr.Get("/students/{id}/report-cards", h.ListStudentReportCards)

		pr.Get("/classrooms/{id}/leger", h.GetLeger)
		pr.Get("/classrooms/{id}/subjects/{subject_id}/description", h.GetSubjectDescriptionGrid)

		// Penilaian Formatif (item 5: dropdown-per-tipe-penilaian, see
		// handler/formative.go)
		pr.Get("/me/teaching-assignments", h.MyTeachingAssignments)
		pr.Get("/formative", h.ListFormativeGrades)
		pr.Put("/formative", h.UpsertFormativeGrades)

		// Deskripsi Rapor templates
		pr.Get("/note-templates", h.GetNoteTemplate)
		pr.Put("/note-templates", h.SetNoteTemplate)

		// Item 4: dynamic report settings (assessment types + weight
		// formulas) admin configures, everything else reads.
		pr.Get("/settings/assessment-types", h.ListAssessmentTypes)
		pr.Post("/settings/assessment-types", h.CreateAssessmentType)
		pr.Put("/settings/assessment-types/{id}", h.UpdateAssessmentType)
		pr.Delete("/settings/assessment-types/{id}", h.DeleteAssessmentType)
		pr.Get("/settings/weights", h.GetWeightSettings)
		pr.Put("/settings/weights", h.SetWeightSettings)

		// Import Template (Generate Rapor "Import Template" popup) --
		// upload/list/download reference .xlsx files, per school per
		// rapor-type. See handler/report_template.go's ponytail note:
		// storage/record-keeping only, not wired into PDF generation.
		pr.Get("/settings/report-templates", h.ListReportTemplates)
		pr.Post("/settings/report-templates", h.UploadReportTemplate)
		pr.Get("/settings/report-templates/{type}/download", h.DownloadReportTemplate)
	})

	return r
}
