package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"skci/quality/internal/common"
)

// Role groups (T23 §5). Superadmin always passes RequireRoles. admin is the
// school administrator and may do what the QA coordinator does; the web nav
// (router/domains/quality.routes.ts) offers each page to the same roles.
var (
	rolesQAWrite  = []string{common.RoleQACoordinator, common.RoleAdmin}
	rolesQAManage = []string{common.RoleQACoordinator, common.RoleKepalaSekolah, common.RoleAdmin}
	rolesQARead   = []string{common.RoleQACoordinator, common.RoleKepalaSekolah, common.RoleAdmin, common.RoleKepalaYayasan, common.RoleAuditor}
	rolesQAVerify = []string{common.RoleQACoordinator, common.RoleAdmin}
	rolesCompare  = []string{common.RoleKepalaYayasan}
	rolesDapodik  = []string{common.RoleOperatorDapodik, common.RoleAdmin}
	rolesGovAdmin = []string{common.RoleAdmin}
	rolesGovRead  = []string{common.RoleAdmin, common.RoleKepalaSekolah, common.RoleKepalaYayasan}
	rolesTplRead  = []string{common.RoleAdmin, common.RoleKepalaSekolah, common.RoleKepalaYayasan, common.RoleOperatorDapodik}
)

// requireSchool rejects requests without a school scope: a superadmin must
// pass ?school_id=, otherwise store queries would run unscoped.
func requireSchool(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := common.FromContext(r.Context())
		sid := ""
		if c != nil {
			sid = c.SchoolID
			if c.ActiveRole == common.RoleSuperadmin {
				if q := r.URL.Query().Get("school_id"); q != "" {
					sid = q
				}
			}
		}
		if sid == "" {
			common.WriteError(w, http.StatusBadRequest, "school_id wajib diisi")
			return
		}
		next.ServeHTTP(w, r)
	})
}

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
		api.Use(requireSchool)
		with := func(roles []string) chi.Router { return api.With(common.RequireRoles(roles...)) }

		// ------------------- QA & Akreditasi (/qa) -------------------
		read, write, manage := with(rolesQARead), with(rolesQAWrite), with(rolesQAManage)

		// Instruments & standards
		read.Get("/qa/instruments", a.QAHandler.ListInstruments)
		write.Post("/qa/instruments", a.QAHandler.CreateInstrument)
		write.Get("/qa/instruments/import/template", a.QAHandler.GetImportTemplate)
		write.Post("/qa/instruments/import", a.QAHandler.ImportInstrument)
		read.Get("/qa/instruments/{id}", a.QAHandler.GetInstrument)
		write.Put("/qa/instruments/{id}", a.QAHandler.UpdateInstrument)
		write.Delete("/qa/instruments/{id}", a.QAHandler.DeleteInstrument)
		write.Post("/qa/instruments/{id}/new-version", a.QAHandler.NewInstrumentVersion)
		write.Post("/qa/instruments/{id}/standards", a.QAHandler.CreateStandard)
		write.Put("/qa/instruments/{id}/standards/{stdId}", a.QAHandler.UpdateStandard)
		write.Delete("/qa/instruments/{id}/standards/{stdId}", a.QAHandler.DeleteStandard)
		write.Post("/qa/instruments/{id}/items", a.QAHandler.CreateInstrumentItem)
		write.Put("/qa/instruments/{id}/items/{itemId}", a.QAHandler.UpdateItem)
		write.Delete("/qa/instruments/{id}/items/{itemId}", a.QAHandler.DeleteItem)
		write.Post("/qa/instruments/{id}/indicators", a.QAHandler.CreateInstrumentIndicator)
		write.Put("/qa/instruments/{id}/indicators/{indId}", a.QAHandler.UpdateIndicator)
		write.Delete("/qa/instruments/{id}/indicators/{indId}", a.QAHandler.DeleteIndicator)
		// Legacy nested creates kept for API clients.
		write.Post("/qa/standards/{id}/items", a.QAHandler.CreateItem)
		write.Post("/qa/items/{id}/indicators", a.QAHandler.CreateIndicator)

		// Metrics dictionary proxy & indicator values
		read.Get("/qa/metrics", a.QAHandler.GetMetrics)
		read.Get("/qa/indicators/{id}/value", a.QAHandler.GetIndicatorValue)

		// Targets
		read.Get("/qa/targets", a.QAHandler.ListTargets)
		manage.Post("/qa/targets", a.QAHandler.UpsertTarget)

		// Evidence repository & completeness. Upload/new version/delete are
		// open to any school member (indicator owners upload their own);
		// the handler limits changes to the uploader or a coordinator.
		read.Get("/qa/evidence", a.QAHandler.ListEvidence)
		api.Post("/qa/evidence", a.QAHandler.CreateEvidence)
		api.Post("/qa/evidence/{id}/new-version", a.QAHandler.NewEvidenceVersion)
		api.Delete("/qa/evidence/{id}", a.QAHandler.DeleteEvidence)
		read.Get("/qa/evidence/{id}/download", a.QAHandler.DownloadEvidence)
		verify := with(rolesQAVerify)
		verify.Post("/qa/evidence/{id}/verify", a.QAHandler.VerifyEvidence)
		read.Get("/qa/completeness", a.QAHandler.GetCompleteness)

		// Self evaluations (EDS)
		read.Get("/qa/self-evaluations", a.QAHandler.ListSelfEvaluations)
		manage.Post("/qa/self-evaluations", a.QAHandler.CreateSelfEvaluation)
		read.Get("/qa/self-evaluations/{id}", a.QAHandler.GetSelfEvaluation)
		manage.Put("/qa/self-evaluations/{id}/items/{indicatorId}", a.QAHandler.UpdateSelfEvalItem)
		manage.Post("/qa/self-evaluations/{id}/refresh-metrics", a.QAHandler.RefreshMetrics)
		manage.Post("/qa/self-evaluations/{id}/finalize", a.QAHandler.FinalizeSelfEvaluation)
		manage.Post("/qa/self-evaluations/{id}/reopen", a.QAHandler.ReopenSelfEvaluation)

		// Action plans (RTL): coordinators see all, owners their own.
		api.Get("/qa/action-plans", a.QAHandler.ListActionPlans)
		manage.Post("/qa/action-plans", a.QAHandler.CreateActionPlan)
		api.Put("/qa/action-plans/{id}", a.QAHandler.UpdateActionPlan)
		api.Delete("/qa/action-plans/{id}", a.QAHandler.DeleteActionPlan)
		api.Post("/qa/action-plans/{id}/status", a.QAHandler.UpdateActionPlanStatus)

		// Reports
		read.Get("/qa/reports/self-evaluation/{id}", a.QAHandler.SelfEvaluationReport)
		read.Get("/qa/reports/accreditation", a.QAHandler.GetAccreditationReport)
		compare := with(rolesCompare)
		compare.Get("/qa/reports/comparison", a.QAHandler.GetComparisonReport)

		// ------------------- Gov Reporting & Dapodik (/gov) -------------------
		dapodik := with(rolesDapodik)
		dapodik.Get("/gov/dapodik/mappings", a.GovHandler.ListMappings)
		dapodik.Post("/gov/dapodik/mappings", a.GovHandler.UpsertMapping)
		dapodik.Delete("/gov/dapodik/mappings/{id}", a.GovHandler.DeleteMapping)
		dapodik.Get("/gov/dapodik/imports", a.GovHandler.ListImports)
		dapodik.Post("/gov/dapodik/imports", a.GovHandler.ImportDapodikFile)
		dapodik.Get("/gov/dapodik/imports/{id}/diffs", a.GovHandler.ListDiffs)
		dapodik.Post("/gov/dapodik/diffs/{id}/decide", a.GovHandler.DecideDiff)
		dapodik.Post("/gov/dapodik/imports/{id}/export", a.GovHandler.ExportDapodikUpdates)
		dapodik.Get("/gov/dapodik/logs", a.GovHandler.ListLogs)

		// Regulatory report templates
		gadmin, gread := with(rolesGovAdmin), with(rolesGovRead)
		tplRead := with(rolesTplRead)
		tplRead.Get("/gov/report-templates", a.GovHandler.ListTemplates)
		gadmin.Post("/gov/report-templates", a.GovHandler.CreateTemplate)
		tplRead.Get("/gov/report-templates/{id}", a.GovHandler.GetTemplate)
		gadmin.Put("/gov/report-templates/{id}", a.GovHandler.UpdateTemplate)
		gadmin.Delete("/gov/report-templates/{id}", a.GovHandler.DeleteTemplate)

		// Regulatory reports
		gread.Get("/gov/reports", a.GovHandler.ListReports)
		gadmin.Post("/gov/reports", a.GovHandler.CreateReport)
		gread.Get("/gov/reports/{id}", a.GovHandler.GetReport)
		gread.Get("/gov/reports/{id}/preview", a.GovHandler.PreviewReport)
		gadmin.Post("/gov/reports/{id}/submit-for-approval", a.GovHandler.SubmitReportForApproval)
		gadmin.Post("/gov/reports/{id}/mark-submitted", a.GovHandler.MarkReportSubmitted)
		gread.Get("/gov/reports/{id}/export", a.GovHandler.ExportRegulatoryReport)
	})

	return r
}
