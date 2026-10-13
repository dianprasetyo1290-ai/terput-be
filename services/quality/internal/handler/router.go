package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"skci/quality/internal/common"
)

func (a *App) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Liveness check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "quality"})
	})

	// Internal routes protected by X-Internal-Key
	r.Group(func(internal chi.Router) {
		internal.Use(common.InternalKeyMiddleware(a.Config.InternalKey))
		internal.Post("/internal/approval-callbacks", a.InternalApprovalCallback)
	})

	// Protected routes (JWT Auth)
	r.Group(func(api chi.Router) {
		api.Use(common.AuthMiddleware(a.Config.JWTSecret))

		// ------------------- QA & Akreditasi (/qa) -------------------
		api.Route("/qa", func(qa chi.Router) {
			// Instruments & standards
			qa.Get("/instruments", a.QAHandler.ListInstruments)
			qa.Post("/instruments", a.QAHandler.CreateInstrument)
			qa.Get("/instruments/{id}", a.QAHandler.GetInstrument)
			qa.Put("/instruments/{id}", a.QAHandler.UpdateInstrument)
			qa.Delete("/instruments/{id}", a.QAHandler.DeleteInstrument)
			qa.Post("/instruments/{id}/standards", a.QAHandler.CreateStandard)
			qa.Post("/standards/{id}/items", a.QAHandler.CreateItem)
			qa.Post("/items/{id}/indicators", a.QAHandler.CreateIndicator)

			// Import template & execution
			qa.Get("/instruments/import/template", a.QAHandler.GetImportTemplate)
			qa.Post("/instruments/import", a.QAHandler.ImportInstrument)

			// Metrics dictionary proxy & indicator values
			qa.Get("/metrics", a.QAHandler.GetMetrics)
			qa.Get("/indicators/{id}/value", a.QAHandler.GetIndicatorValue)

			// Targets
			qa.Get("/targets", a.QAHandler.ListTargets)
			qa.Post("/targets", a.QAHandler.UpsertTarget)

			// Evidence repository & completeness
			qa.Get("/evidence", a.QAHandler.ListEvidence)
			qa.Post("/evidence", a.QAHandler.CreateEvidence)
			qa.Post("/evidence/{id}/verify", a.QAHandler.VerifyEvidence)
			qa.Get("/completeness", a.QAHandler.GetCompleteness)

			// Self evaluations (EDS)
			qa.Get("/self-evaluations", a.QAHandler.ListSelfEvaluations)
			qa.Post("/self-evaluations", a.QAHandler.CreateSelfEvaluation)
			qa.Get("/self-evaluations/{id}", a.QAHandler.GetSelfEvaluation)
			qa.Put("/self-evaluations/{id}/items/{indicatorId}", a.QAHandler.UpdateSelfEvalItem)
			qa.Post("/self-evaluations/{id}/finalize", a.QAHandler.FinalizeSelfEvaluation)
			qa.Post("/self-evaluations/{id}/reopen", a.QAHandler.ReopenSelfEvaluation)

			// Action plans (RTL)
			qa.Get("/action-plans", a.QAHandler.ListActionPlans)
			qa.Post("/action-plans", a.QAHandler.CreateActionPlan)
			qa.Post("/action-plans/{id}/status", a.QAHandler.UpdateActionPlanStatus)

			// Reports
			qa.Get("/reports/accreditation", a.QAHandler.GetAccreditationReport)
			qa.Get("/reports/comparison", a.QAHandler.GetComparisonReport)
		})

		// ------------------- Gov Reporting & Dapodik (/gov) -------------------
		api.Route("/gov", func(gov chi.Router) {
			// Dapodik mappings
			gov.Get("/dapodik/mappings", a.GovHandler.ListMappings)
			gov.Post("/dapodik/mappings", a.GovHandler.UpsertMapping)
			gov.Delete("/dapodik/mappings/{id}", a.GovHandler.DeleteMapping)

			// Dapodik file import & diffs
			gov.Get("/dapodik/imports", a.GovHandler.ListImports)
			gov.Post("/dapodik/imports", a.GovHandler.ImportDapodikFile)
			gov.Get("/dapodik/imports/{id}/diffs", a.GovHandler.ListDiffs)
			gov.Post("/dapodik/diffs/{id}/decide", a.GovHandler.DecideDiff)
			gov.Post("/dapodik/imports/{id}/export", a.GovHandler.ExportDapodikUpdates)
			gov.Get("/dapodik/logs", a.GovHandler.ListLogs)

			// Regulatory report templates
			gov.Get("/report-templates", a.GovHandler.ListTemplates)
			gov.Post("/report-templates", a.GovHandler.CreateTemplate)

			// Regulatory reports
			gov.Get("/reports", a.GovHandler.ListReports)
			gov.Post("/reports", a.GovHandler.CreateReport)
			gov.Get("/reports/{id}", a.GovHandler.GetReport)
			gov.Get("/reports/{id}/preview", a.GovHandler.PreviewReport)
			gov.Post("/reports/{id}/submit-for-approval", a.GovHandler.SubmitReportForApproval)
			gov.Post("/reports/{id}/mark-submitted", a.GovHandler.MarkReportSubmitted)
			gov.Get("/reports/{id}/export", a.GovHandler.ExportRegulatoryReport)
		})
	})

	return r
}
