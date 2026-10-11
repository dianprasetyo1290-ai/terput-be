package handler

import (
	"net/http"
	"strings"

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
		// /settings/* opts out: it is a handful of rows that guru edit
		// live from Penilaian Formatif's "+ Tambah Tipe Nilai", and the
		// cache has no invalidation on write -- a 15s stale list there
		// means a guru adds a tipe, doesn't see it, adds it again and
		// gets "sudah ada" against a list that doesn't show it (prod,
		// 2026-09-10). Nothing to gain from caching it anyway.
		// The .docx/.zip mail-merge downloads also opt out: they are binary
		// bodies sized in megabytes, and a rapor must never be served from
		// a cache that could hand one student's document to another.
		// /parent/* opts out for the same reason: the guardian link check
		// must run on every request, and a cache hit would answer before it
		// (revoked link or swapped studentUserId served from a stale entry).
		pr.Use(respCache.Middleware(func(r *http.Request) bool {
			return strings.HasPrefix(r.URL.Path, "/settings/") ||
				strings.HasPrefix(r.URL.Path, "/parent/") ||
				strings.HasSuffix(r.URL.Path, "/docx")
		}))

		pr.Post("/report-cards/generate", h.GenerateReportCards)
		pr.Get("/report-cards/{id}", h.GetReportCard)
		pr.Patch("/report-cards/{id}/subjects/{subject_id}", h.PatchSubjectOverride)
		pr.Patch("/report-cards/{id}/subjects/{subject_id}/notes", h.PatchSubjectNotes)
		// SEKOCI V10 item B: Kokurikuler / Ekstrakurikuler / Tanggapan Wali
		// Kelas -- the hand-entered rapor sections below C. Muatan Lokal.
		pr.Patch("/report-cards/{id}/extras", h.PatchRaporExtras)
		pr.Put("/report-cards/{id}/attendance", h.PatchAttendance)
		// Mail merge: render this rapor through the school's own uploaded
		// .docx template (handler/report_docx.go). Runs alongside the
		// built-in print view, which stays the default.
		pr.Get("/report-cards/{id}/docx", h.DownloadReportCardDocx)
		pr.Get("/classrooms/{id}/report-cards/docx", h.DownloadClassroomDocxZip)
		pr.Get("/settings/report-template-parameters", h.ListMergeParameters)

		pr.Post("/report-cards/{id}/finalize", h.FinalizeReportCard)
		pr.Put("/report-cards/{id}/unlock", h.UnlockReportCard)

		pr.Get("/students/{id}/report-cards", h.ListStudentReportCards)

		// Portal Orang Tua: read-only rapor reads for a wali (guardian),
		// keyed by the student the SPA is viewing. The nested group applies
		// RequireGuardianStudent to both routes, so every request re-checks
		// the identity-service guardian<->student link before any rapor data
		// is loaded. The /parent/... prefix is static, so it cannot collide
		// with the wildcards above (/report-cards/{id}, /students/{id}/...).
		pr.Group(func(pg chi.Router) {
			pg.Use(h.RequireGuardianStudent)
			pg.Get("/parent/children/{studentUserId}/report-cards", h.ListParentReportCards)
			pg.Get("/parent/children/{studentUserId}/report-cards/{reportCardId}", h.GetParentReportCard)
		})

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
		pr.Get("/settings/report-templates/{type}/content", h.GetReportTemplateContent)
	})

	return r
}
