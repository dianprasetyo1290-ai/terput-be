package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"skci/attendance/internal/cache"
	"skci/attendance/internal/common"
)

type RouterConfig struct {
	JWTSecret   string
	InternalKey string
	CORSOrigins []string
	Cache       *cache.Cache
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
		// Client request 2026-08-29: cache GET responses in Redis. "Now"
		// dashboards excluded -- caching them defeats their purpose (they
		// exist to show live state, e.g. who's teaching this exact moment).
		pr.Use(cfg.Cache.Middleware(func(r *http.Request) bool {
			// Leave requests change on every submit/review; a 15s stale
			// list would hide the action the user just took.
			if strings.Contains(r.URL.Path, "leave-requests") || strings.Contains(r.URL.Path, "attendance-corrections") ||
				strings.Contains(r.URL.Path, "attendance-lock-settings") || strings.HasSuffix(r.URL.Path, "/student-attendance/daily") {
				return true
			}
			switch r.URL.Path {
			case "/me/sessions/today", "/dashboard/active-teachers":
				return true
			default:
				return false
			}
		}))

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

		// Attendance Report (PRD menu 43) — see report_attendance.go.
		pr.Get("/admin/attendance-report/recap", h.AttendanceReportRecap)
		pr.Get("/admin/attendance-report/trend", h.AttendanceReportTrend)
		pr.Get("/admin/attendance-report/class-comparison", h.AttendanceReportClassComparison)
		pr.Get("/admin/attendance-report/subjects", h.AttendanceReportSubjects)
		pr.Get("/admin/attendance-report/export", h.AttendanceReportExport)

		// dashboard
		pr.Get("/me/sessions/today", h.MySessionsToday)
		pr.Get("/dashboard/active-teachers", h.ActiveTeachers)
		pr.Get("/dashboard/teaching-stats", h.TeachingStats)
		pr.Get("/dashboard/student-attendance-summary", h.StudentAttendanceSummaryDashboard)
		pr.Get("/dashboard/teacher-attendance-summary", h.TeacherAttendanceSummaryDashboard)

		// Portal Orang Tua (wali) — read-only presensi for one linked child.
		// A distinct /parent/children/{studentUserId} subtree (no shared
		// wildcard segment with anything above), so chi's static-vs-wildcard
		// precedence is never in play. The only wali write is leave-requests below.
		pr.Get("/parent/children/{studentUserId}/attendance", h.ParentChildAttendance)

		// Izin / sakit (PRD menu 42 + 48): orang tua submits, wali kelas /
		// admin reviews; approval applies BR-ATT-02.
		pr.Post("/parent/children/{studentUserId}/leave-requests", h.CreateLeaveRequest)
		pr.Get("/parent/children/{studentUserId}/leave-requests", h.ListChildLeaveRequests)
		pr.Get("/leave-requests", h.ListLeaveRequests)
		pr.Post("/leave-requests/{id}/approve", h.ApproveLeaveRequest)
		pr.Post("/leave-requests/{id}/reject", h.RejectLeaveRequest)
		pr.Get("/leave-requests/{id}/attachment", h.LeaveAttachment)

		// P4A (menu 41/69): kunci presensi, input harian, koreksi setelah kunci.
		pr.Get("/admin/attendance-lock-settings", h.GetLockSettingsHandler)
		pr.Put("/admin/attendance-lock-settings", h.PutLockSettingsHandler)
		pr.Get("/admin/student-attendance/daily", h.GetDailyAttendance)
		pr.Post("/admin/student-attendance/daily", h.SaveDailyAttendance)
		pr.Get("/attendance-corrections", h.ListCorrections)
		pr.Post("/attendance-corrections", h.CreateCorrection)
	})

	// report-service integration: reachable either via internal service key
	// or a normal user JWT (see common.InternalOrJWTMiddleware).
	r.Group(func(ir chi.Router) {
		ir.Use(common.InternalOrJWTMiddleware(cfg.JWTSecret, cfg.InternalKey))
		ir.Use(cfg.Cache.Middleware(nil))
		ir.Get("/students/{id}/attendance-summary", h.StudentAttendanceSummary)
	})

	// Service-to-service only (X-Internal-Key, never a browser JWT).
	r.Group(func(in chi.Router) {
		in.Use(common.InternalOnlyMiddleware(cfg.InternalKey))
		in.Post("/internal/student-absences/health", h.InternalHealthAbsence)
		in.Post("/internal/approval-callbacks", h.InternalApprovalCallback) // core Approval Center (P4A)

		// T24: Insight Read Model & Metric Facts
		in.Get("/internal/metric-facts", h.InternalMetricFacts)
		in.Get("/internal/risk-facts/absence", h.InternalRiskFactsAbsence)
	})

	return r
}
