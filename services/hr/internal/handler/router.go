package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"skci/hr/internal/common"
)

func (a *App) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(a.Config.CORSOrigins))

	// Public
	r.Get("/healthz", a.Healthz)

	// Internal routes protected by X-Internal-Key
	r.Group(func(internal chi.Router) {
		internal.Use(common.InternalKeyMiddleware(a.Config.InternalKey))

		internal.Post("/internal/approval-callbacks", a.InternalApprovalCallback)
		internal.Get("/internal/headcount/{positionId}", a.InternalGetHeadcountByPosition)
		internal.Get("/internal/employees/by-user/{userId}/supervisor", a.InternalGetSupervisorByUserID)
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

		// 2. HR Management routes: HR Admin, HR Manager, Superadmin
		api.Group(func(hr chi.Router) {
			hr.Use(common.RequireAnyRole(
				common.RoleSuperadmin,
				common.RoleHRAdmin,
				common.RoleHRManager,
			))

			// Employees
			hr.Get("/employees", a.ListEmployees)
			hr.Get("/employees/export", a.ExportEmployees)
			hr.Get("/employees/{id}", a.GetEmployee)
			hr.Get("/employees/{id}/timeline", a.GetEmployeeTimeline)

			// Organization
			hr.Get("/departments", a.ListDepartments)
			hr.Get("/positions", a.ListPositions)
			hr.Get("/org-chart", a.GetOrgChart)
			hr.Get("/headcount-plans", a.ListHeadcountPlans)
			hr.Get("/headcount", a.GetHeadcountOverview)

			// Documents
			hr.Get("/document-types", a.ListDocumentTypes)
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
			hr.Get("/leave-types", a.ListLeaveTypes)
			hr.Get("/leave-balances", a.ListLeaveBalances)
			hr.Get("/leave-requests", a.ListLeaveRequests)
			hr.Get("/overtime-requests", a.ListOvertimeRequests)

			// Settings
			hr.Get("/settings/hr", a.GetHRSettings)

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
