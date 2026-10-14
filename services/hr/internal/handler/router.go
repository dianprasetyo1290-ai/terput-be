package handler

import (
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"skci/hr/internal/ats"
	"skci/hr/internal/common"
	"skci/hr/internal/payroll"
	"skci/hr/internal/training"
)

// Route role sets, taken from the endpoint tables of the T2–T5 task specs
// (docs/prd-sekoci/tasks). Superadmin is in every set (cross-school support).
var (
	rolesHR      = []string{common.RoleSuperadmin, common.RoleHRAdmin, common.RoleHRManager}
	rolesOrgRead = append(slices.Clone(rolesHR), common.RoleKepalaSekolah)

	rolesTrainingManage = []string{common.RoleSuperadmin, common.RoleTrainingAdmin, common.RoleHRAdmin}
	rolesTrainingRead   = append(slices.Clone(rolesTrainingManage), common.RoleHRManager, common.RoleKepalaSekolah)

	rolesRequisitionWrite = append(slices.Clone(rolesHR), common.RoleKepalaSekolah)
	rolesRequisitionRead  = append(slices.Clone(rolesRequisitionWrite), common.RoleRecruiter)
	rolesRecruiting       = append(slices.Clone(rolesHR), common.RoleRecruiter)
	rolesCandidateRead    = append(slices.Clone(rolesRecruiting), common.RoleKepalaSekolah)

	rolesPayrollOperate = append(slices.Clone(rolesHR), common.RolePayroll)
	rolesPayrollRead    = append(slices.Clone(rolesPayrollOperate), common.RoleFinanceManager)
)

func (a *App) Router() http.Handler {
	r := chi.NewRouter()

	trHandler := training.NewHandler(training.NewStore(a.DB), a.Client)
	atsHandler := ats.NewHandler(ats.NewStore(a.DB), a.Client)
	payHandler := payroll.NewHandler(payroll.NewStore(a.DB), a.Client)

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(a.Config.CORSOrigins))

	// Public
	r.Get("/healthz", a.Healthz)
	r.Get("/public/careers/{schoolSlug}", atsHandler.PublicListCareers)
	r.Get("/public/careers/{schoolSlug}/{postingSlug}", atsHandler.PublicGetPosting)
	r.Post("/public/careers/{schoolSlug}/{postingSlug}/apply", atsHandler.PublicApply)
	r.Get("/public/applications/status", atsHandler.PublicApplicationStatus)
	r.Post("/public/offers/{offerId}/accept", atsHandler.PublicAcceptOffer)

	// Internal routes protected by X-Internal-Key
	r.Group(func(internal chi.Router) {
		internal.Use(common.InternalKeyMiddleware(a.Config.InternalKey))

		internal.Post("/internal/approval-callbacks", a.InternalApprovalCallback)
		internal.Get("/internal/headcount/{positionId}", a.InternalGetHeadcountByPosition)
		internal.Get("/internal/employees/by-user/{userId}/supervisor", a.InternalGetSupervisorByUserID)
		internal.Get("/internal/dapodik/ptk", a.InternalDapodikPTK)
		internal.Post("/internal/dapodik/ptk/match", a.InternalDapodikPTKMatch)

		// T24: Insight Read Model & Metric Facts
		internal.Get("/internal/metric-facts", a.InternalMetricFacts)
	})

	// Protected routes (JWT Auth + HRIS Feature Toggle)
	r.Group(func(api chi.Router) {
		api.Use(common.AuthMiddleware(a.Config.JWTSecret))
		api.Use(a.requireHrisFeature)

		// 1. ESS routes accessible to all authenticated school staff/employees
		api.Get("/me/employee", a.MeEmployee)
		api.Get("/me/dashboard", a.MeDashboard)
		api.Post("/me/attendance/check-in", a.MeCheckIn)
		api.Post("/me/attendance/check-out", a.MeCheckOut)
		api.Get("/me/leave-requests", a.MeListLeaveRequests)
		api.Post("/me/leave-requests", a.MeCreateLeaveRequest)
		api.Post("/me/leave-requests/{id}/cancel", a.MeCancelLeaveRequest)
		api.Get("/me/leave-balances", a.MeListLeaveBalances)
		api.Get("/me/overtime-requests", a.MeListOvertimeRequests)
		api.Post("/me/overtime-requests", a.MeCreateOvertimeRequest)
		api.Get("/me/profile-change-requests", a.MeListProfileChangeRequests)
		api.Post("/me/profile-change-requests", a.MeCreateProfileChangeRequest)
		api.Get("/me/documents", a.MeListDocuments)
		api.Post("/me/documents", a.MeUploadDocument)
		api.Get("/documents/{id}/download", a.DownloadDocument)

		// ESS Training & Onboarding
		api.Post("/me/training/enroll", trHandler.EnrollCourse)
		api.Get("/me/training/assignments", trHandler.MyAssignments)
		api.Get("/me/training/assignments/{assignmentId}/quiz", trHandler.MyQuiz)
		api.Post("/me/training/assignments/{assignmentId}/quiz", trHandler.SubmitQuiz)
		api.Post("/me/training/assignments/{assignmentId}/materials/{materialId}/done", trHandler.MarkMaterialDone)
		api.Get("/me/onboarding", trHandler.MyOnboarding)
		api.Post("/onboarding/tasks/{taskId}/done", trHandler.MarkOnboardingTaskDone)

		// ATS Panel Interviews
		api.Get("/ats/interviews/mine", atsHandler.MyInterviews)

		// ESS Payslips
		api.Get("/me/payslips", payHandler.MyPayslips)

		// ESS reads that previously went through HR-admin endpoints and so
		// 403'd for pegawai/guru (QA BUG-WEB role sweep).
		api.Get("/me/attendance", a.MeListAttendance)
		api.Get("/me/training/catalog", trHandler.MyCatalog)
		api.Get("/me/training/courses/{courseId}/materials", trHandler.MyCourseMaterials)
		// Non-sensitive master data every staff form needs (leave request,
		// document upload).
		api.Get("/leave-types", a.ListLeaveTypes)
		api.Get("/document-types", a.ListDocumentTypes)

		// Organisation reads: T2 §7 "baca: hr_manager, kepala_sekolah".
		api.Group(func(org chi.Router) {
			org.Use(common.RequireAnyRole(rolesOrgRead...))
			org.Get("/departments", a.ListDepartments)
			org.Get("/positions", a.ListPositions)
			org.Get("/org-chart", a.GetOrgChart)
			org.Get("/headcount-plans", a.ListHeadcountPlans)
			org.Get("/headcount", a.GetHeadcountOverview)
		})

		// 2. HR Management routes: HR Admin, HR Manager, Superadmin
		api.Group(func(hr chi.Router) {
			hr.Use(common.RequireAnyRole(rolesHR...))

			// Employees
			hr.Get("/employees", a.ListEmployees)
			hr.Get("/employees/export", a.ExportEmployees)
			hr.Get("/employees/{id}", a.GetEmployee)
			hr.Get("/employees/{id}/timeline", a.GetEmployeeTimeline)

			// Documents
			hr.Get("/required-documents", a.ListRequiredDocuments)
			hr.Get("/employees/{id}/documents", a.ListEmployeeDocuments)
			hr.Get("/documents/completeness", a.GetDocumentCompleteness)
			hr.Get("/document-requests", a.ListDocumentRequests)

			// Attendance & Holidays
			hr.Get("/work-schedules", a.ListWorkSchedules)
			hr.Get("/holidays", a.ListHolidays)
			hr.Get("/attendance", a.ListAttendance)
			hr.Get("/attendance/recap", a.GetAttendanceRecap)

			// Leave & Overtime
			hr.Get("/leave-balances", a.ListLeaveBalances)
			hr.Get("/leave-requests", a.ListLeaveRequests)
			hr.Get("/overtime-requests", a.ListOvertimeRequests)

			// Settings
			hr.Get("/settings/hr", a.GetHRSettings)

			// Onboarding (T5 §5: hr_admin; hr_manager reads the plans page)
			hr.Get("/onboarding/templates", trHandler.ListOnboardingTemplates)
			hr.Post("/onboarding/templates", trHandler.CreateOnboardingTemplate)

			// Mutations (HR Admin & Superadmin only)
			hr.Group(func(admin chi.Router) {
				admin.Use(common.RequireAnyRole(
					common.RoleSuperadmin,
					common.RoleHRAdmin,
				))

				// Employee mutations
				admin.Post("/employees", a.CreateEmployee)
				admin.Put("/employees/{id}", a.UpdateEmployee)
				admin.Post("/employees/{id}/assignments", a.CreateAssignment)
				admin.Post("/employees/{id}/offboard", a.OffboardEmployee)

				// Org mutations
				admin.Post("/departments", a.CreateDepartment)
				admin.Put("/departments/{id}", a.UpdateDepartment)
				admin.Delete("/departments/{id}", a.DeleteDepartment)

				admin.Post("/positions", a.CreatePosition)
				admin.Put("/positions/{id}", a.UpdatePosition)
				admin.Delete("/positions/{id}", a.DeletePosition)

				admin.Post("/headcount-plans", a.CreateHeadcountPlan)

				// Document mutations
				admin.Post("/document-types", a.CreateDocumentType)
				admin.Put("/document-types/{id}", a.UpdateDocumentType)
				admin.Delete("/document-types/{id}", a.DeleteDocumentType)

				admin.Post("/required-documents", a.CreateRequiredDocument)
				admin.Delete("/required-documents/{id}", a.DeleteRequiredDocument)

				admin.Post("/employees/{id}/documents", a.UploadEmployeeDocument)
				admin.Delete("/documents/{id}", a.DeleteDocument)

				admin.Post("/document-requests", a.CreateDocumentRequest)
				admin.Put("/document-requests/{id}", a.UpdateDocumentRequest)

				// Attendance & schedule mutations
				admin.Post("/work-schedules", a.CreateWorkSchedule)
				admin.Put("/work-schedules/{id}", a.UpdateWorkSchedule)
				admin.Delete("/work-schedules/{id}", a.DeleteWorkSchedule)

				admin.Post("/holidays", a.CreateHoliday)
				admin.Delete("/holidays/{id}", a.DeleteHoliday)

				admin.Put("/attendance/{id}", a.UpdateAttendance)
				admin.Post("/attendance/import", a.ImportAttendance)
				admin.Post("/attendance-periods/{period}/send", a.SendAttendancePeriod)

				// Leave & overtime mutations
				admin.Post("/leave-types", a.CreateLeaveType)
				admin.Put("/leave-types/{id}", a.UpdateLeaveType)
				admin.Delete("/leave-types/{id}", a.DeleteLeaveType)

				admin.Post("/leave-requests/{id}/approve", a.AdminApproveLeaveRequest)
				admin.Post("/leave-requests/{id}/reject", a.AdminRejectLeaveRequest)
				admin.Post("/overtime-requests/{id}/approve", a.AdminApproveOvertimeRequest)
				admin.Post("/overtime-requests/{id}/reject", a.AdminRejectOvertimeRequest)

				// Settings mutations
				admin.Put("/settings/hr", a.UpdateHRSettings)
			})
		})

		// Training (T5 §5): training_admin manages; hr_manager and
		// kepala_sekolah read assignments/compliance.
		api.Group(func(tr chi.Router) {
			tr.Use(common.RequireAnyRole(rolesTrainingRead...))
			tr.Get("/training/courses", trHandler.ListCourses)
			tr.Get("/training/courses/{id}", trHandler.GetCourse)
			tr.Get("/training/mandatory", trHandler.ListMandatory)
			tr.Get("/training/assignments", trHandler.ListAssignments)
		})
		api.Group(func(tr chi.Router) {
			tr.Use(common.RequireAnyRole(rolesTrainingManage...))
			tr.Post("/training/courses", trHandler.CreateCourse)
			tr.Put("/training/courses/{id}", trHandler.UpdateCourse)
			tr.Get("/training/courses/{courseId}/materials", trHandler.ListMaterials)
			tr.Post("/training/courses/{courseId}/materials", trHandler.CreateMaterial)
			tr.Get("/training/courses/{courseId}/sessions", trHandler.ListSessions)
			tr.Post("/training/courses/{courseId}/sessions", trHandler.CreateSession)
			tr.Get("/training/courses/{courseId}/questions", trHandler.ListQuestions)
			tr.Post("/training/courses/{courseId}/questions", trHandler.CreateQuestion)
			tr.Post("/training/mandatory", trHandler.CreateMandatory)
			tr.Post("/training/assignments", trHandler.CreateAssignment)
		})

		// ATS (T4 §4)
		api.Group(func(ats chi.Router) {
			ats.Use(common.RequireAnyRole(rolesRequisitionRead...))
			ats.Get("/ats/requisitions", atsHandler.ListRequisitions)
		})
		api.Group(func(ats chi.Router) {
			ats.Use(common.RequireAnyRole(rolesRequisitionWrite...))
			ats.Post("/ats/requisitions", atsHandler.CreateRequisition)
			ats.Post("/ats/requisitions/{id}/submit", atsHandler.SubmitRequisition)
		})
		api.Group(func(ats chi.Router) {
			ats.Use(common.RequireAnyRole(rolesCandidateRead...))
			ats.Get("/ats/applications", atsHandler.ListApplications)
			ats.Get("/ats/offers", atsHandler.ListOffers)
		})
		api.Group(func(ats chi.Router) {
			ats.Use(common.RequireAnyRole(rolesRecruiting...))
			ats.Get("/ats/postings", atsHandler.ListPostings)
			ats.Get("/ats/postings/{id}", atsHandler.GetPosting)
			ats.Post("/ats/postings", atsHandler.CreatePosting)
			ats.Post("/ats/postings/{id}/publish", atsHandler.PublishPosting)
			ats.Post("/ats/postings/{id}/close", atsHandler.ClosePosting)
			ats.Post("/ats/applications/{id}/move", atsHandler.MoveApplication)
			ats.Get("/ats/stages", atsHandler.ListStages)
			ats.Get("/ats/scorecard-templates", atsHandler.ListScorecardTemplates)
			ats.Post("/ats/interviews", atsHandler.ScheduleInterview)
			ats.Get("/ats/talent-pool", atsHandler.ListTalentPool)
		})
		api.Group(func(ats chi.Router) {
			ats.Use(common.RequireAnyRole(rolesHR...))
			ats.Post("/ats/scorecard-templates", atsHandler.CreateScorecardTemplate)
			ats.Post("/ats/offers", atsHandler.CreateOffer)
			ats.Post("/ats/offers/{id}/submit", atsHandler.SubmitOffer)
			ats.Post("/ats/offers/{id}/send", atsHandler.SendOffer)
			ats.Get("/ats/settings", atsHandler.GetSettings)
		})

		// Payroll (T3 §5): payroll_officer runs it; hr_manager and
		// finance_manager read runs/components/journals.
		api.Group(func(pay chi.Router) {
			pay.Use(common.RequireAnyRole(rolesPayrollRead...))
			pay.Get("/payroll/components", payHandler.ListComponents)
			pay.Get("/payroll/runs", payHandler.ListRuns)
			pay.Get("/payroll/runs/{id}", payHandler.GetRun)
			pay.Get("/payroll/runs/{id}/lines", payHandler.ListLines)
			pay.Get("/payroll/journals", payHandler.ListJournals)
		})
		api.Group(func(pay chi.Router) {
			pay.Use(common.RequireAnyRole(rolesPayrollOperate...))
			pay.Post("/payroll/components", payHandler.CreateComponent)
			pay.Post("/payroll/components/{id}/submit-approval", payHandler.SubmitComponentApproval)
			pay.Get("/payroll/inputs", payHandler.ListInputs)
			pay.Post("/payroll/inputs", payHandler.SaveInput)
			pay.Post("/payroll/inputs/pull-attendance", payHandler.PullAttendance)
			pay.Post("/payroll/runs", payHandler.CreateRun)
			pay.Post("/payroll/runs/{id}/calculate", payHandler.CalculateRun)
			pay.Post("/payroll/runs/{id}/submit", payHandler.SubmitRun)
			pay.Post("/payroll/runs/{id}/finalize", payHandler.FinalizeRun)
			pay.Get("/payroll/payslips", payHandler.ListPayslips)
		})
	})

	return r
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				allowed := false
				for _, o := range allowedOrigins {
					if o == "*" || o == origin {
						allowed = true
						break
					}
				}
				if allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-Internal-Key, Idempotency-Key")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
