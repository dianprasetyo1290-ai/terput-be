package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/report/internal/common"
)

func NewRouter(d *Deps, jwtSecret string, corsOrigins []string) http.Handler {
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
	})

	return r
}
