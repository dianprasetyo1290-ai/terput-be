package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/attendance/internal/common"
)

type RouterConfig struct {
	JWTSecret   string
	InternalKey string
	CORSOrigins []string
}

func NewRouter(h *Handler, cfg RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	// Sentry's Repanic:true reports a panic to error tracking and then
	// re-panics; middleware.Recoverer (mounted right after) is what
	// actually stops it from crashing the process — this service had no
	// panic recovery at all before, found while wiring up error tracking.
	r.Use(common.SentryMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key", "X-Academic-Year-Id"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", Healthz)

	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(cfg.JWTSecret))

		// guru daily flow
		pr.Post("/sessions/checkin", h.CheckIn)
		pr.Get("/sessions/{id}", h.GetSession)
		pr.Put("/sessions/{id}/agenda", h.UpdateAgenda)
		pr.Put("/sessions/{id}/checkout", h.CheckOut) // jam keluar
		pr.Get("/sessions/{id}/roster", h.Roster)
		pr.Get("/sessions/{id}/students", h.ListSessionStudents)
		pr.Post("/sessions/{id}/students/bulk", h.BulkMarkStudents)
		pr.Put("/sessions/{id}/students/{student_id}", h.MarkOneStudent)

		// pergantian guru (substitute teacher) request/response
		pr.Post("/substitute-requests", h.CreateSubstituteRequest)
		pr.Get("/substitute-requests/incoming", h.ListIncomingSubstituteRequests)
		pr.Get("/substitute-requests/outgoing", h.ListOutgoingSubstituteRequests)
		pr.Post("/substitute-requests/{id}/accept", h.AcceptSubstituteRequestHandler)
		pr.Post("/substitute-requests/{id}/reject", h.RejectSubstituteRequestHandler)

		// admin
		pr.Get("/sessions", h.ListSessions)
		pr.Get("/admin/student-attendance", h.StudentAttendanceAdmin)
		pr.Get("/admin/student-attendance/summary", h.StudentAttendanceSummaryAdmin)
		pr.Get("/admin/student-attendance/export", h.StudentAttendanceExport)
		pr.Get("/admin/student-attendance/monthly-grid", h.StudentAttendanceMonthlyGridAdmin)
		pr.Get("/admin/student-attendance/import-template", h.AdminAttendanceImportTemplate)
		pr.Post("/admin/student-attendance/import", h.AdminAttendanceImport)
		pr.Get("/admin/teacher-attendance", h.TeacherAttendanceAdmin)
		pr.Get("/admin/teacher-attendance/export", h.TeacherAttendanceExport)

		// dashboard
		pr.Get("/me/sessions/today", h.MySessionsToday)
		pr.Get("/dashboard/active-teachers", h.ActiveTeachers)
		pr.Get("/dashboard/teaching-stats", h.TeachingStats)
		pr.Get("/dashboard/student-attendance-summary", h.StudentAttendanceSummaryDashboard)
		pr.Get("/dashboard/teacher-attendance-summary", h.TeacherAttendanceSummaryDashboard)
	})

	// report-service integration: reachable either via internal service key
	// or a normal user JWT (see common.InternalOrJWTMiddleware).
	r.Group(func(ir chi.Router) {
		ir.Use(common.InternalOrJWTMiddleware(cfg.JWTSecret, cfg.InternalKey))
		ir.Get("/students/{id}/attendance-summary", h.StudentAttendanceSummary)
	})

	return r
}
