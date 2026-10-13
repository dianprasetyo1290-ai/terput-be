package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"skci/academic/internal/cache"
	"skci/academic/internal/client"
	"skci/academic/internal/common"
	"skci/academic/internal/handler"
	"skci/academic/internal/store"
)

// corsOrigins reads a comma-separated CORS_ALLOWED_ORIGINS so the allowed
// dev-server ports (one per frontend domain agent, see
// docs/FRONTEND_CONVENTIONS.md) are configured once via docker-compose
// rather than hardcoded per service — scales to more origins without a
// code change or rebuild.
func corsOrigins() []string {
	raw := os.Getenv("CORS_ALLOWED_ORIGINS")
	if raw == "" {
		raw = "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178"
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func main() {
	_ = godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://sekoci:change-me@localhost:5482/skci_academic?sslmode=disable"
	}
	jwtSecret := os.Getenv("SEKOCI_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "change-me"
	}
	internalKey := os.Getenv("SEKOCI_INTERNAL_KEY")
	if internalKey == "" {
		internalKey = "change-me"
	}

	sentryEnv := os.Getenv("SENTRY_ENVIRONMENT")
	if sentryEnv == "" {
		sentryEnv = "development"
	}
	if err := common.InitSentry(os.Getenv("SENTRY_DSN"), sentryEnv, "academic-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping db: %v", err)
	}

	s := store.New(db)
	ic := client.NewIdentityClient()
	h := handler.New(s, ic)

	// Redis response cache (client request 2026-08-29): fails open if
	// REDIS_URL is unset/unreachable, see internal/cache's doc comment.
	respCache := cache.New(os.Getenv("REDIS_URL"), 15*time.Second)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(common.SentryMiddleware())
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Internal-Key"},
		AllowCredentials: true,
	}))

	r.Get("/healthz", handler.Healthz)

	r.Group(func(r chi.Router) {
		r.Use(common.JWTMiddleware(jwtSecret))
		// Client request 2026-08-29: cache GET responses in Redis.
		r.Use(respCache.Middleware(nil))

		// Tahun Akademik / Term
		r.Post("/academic-years/{academic_year_id}/terms/bootstrap", h.BootstrapTerms)
		r.Get("/terms", h.ListTerms)
		r.Put("/terms/{id}", h.UpdateTerm)

		// Data Kelas (classrooms)
		r.Post("/classrooms", h.CreateClassroom)
		r.Get("/classrooms", h.ListClassrooms)
		r.Put("/classrooms/{id}", h.UpdateClassroom)
		r.Delete("/classrooms/{id}", h.DeleteClassroom)

		r.Post("/classrooms/{id}/students", h.AddStudentToClassroom)
		r.Delete("/classrooms/{id}/students/{student_user_id}", h.RemoveStudentFromClassroom)

		r.Post("/classrooms/{id}/subjects", h.AssignSubjectToClassroom)
		r.Put("/classroom-subjects/{id}", h.UpdateClassroomSubject)
		r.Delete("/classroom-subjects/{id}", h.DeleteClassroomSubject)

		// Matriks Pelajaran per-student grid: write side (bulk-set backs
		// Ceklis Semua / Kosongkan Semua / Sync Matrix / Auto-Enroll Agama).
		r.Post("/classrooms/{id}/subject-matrix/bulk-set", h.BulkSetStudentSubjectMatrix)

		// Wali Kelas (derived)
		r.Get("/wali-kelas", h.ListWaliKelas)

		// Year-wide roster (all classrooms) — siswa-picker dedup + roster view
		r.Get("/classrooms/enrollments", h.ListEnrollmentsByYear)

		// Riwayat perpindahan kelas (FR-39-003)
		r.Get("/classrooms/{id}/enrollment-history", h.ListClassroomEnrollmentHistory)
		r.Get("/students/{id}/enrollment-history", h.ListStudentEnrollmentHistory)

		// Beban mengajar guru + kelas/mapel tanpa guru (FR-40-002, AC-40-01)
		r.Get("/teaching-workload", h.TeachingWorkload)

		// Data Mata Pelajaran: subjects catalog
		r.Post("/subjects", h.CreateSubject)
		r.Get("/subjects", h.ListSubjects)
		r.Put("/subjects/{id}", h.UpdateSubject)
		r.Delete("/subjects/{id}", h.DeleteSubject)

		// Jadwal Pelajaran: jam pelajaran + schedule slots
		r.Post("/classrooms/{id}/schedule-periods", h.CreateSchedulePeriod)
		r.Get("/classrooms/{id}/schedule-periods", h.ListSchedulePeriods)
		r.Put("/schedule-periods/{id}", h.UpdateSchedulePeriod)
		r.Delete("/schedule-periods/{id}", h.DeleteSchedulePeriod)

		r.Post("/classrooms/{id}/schedules", h.CreateSchedule)
		r.Get("/classrooms/{id}/schedules", h.ListClassroomSchedules)
		r.Put("/schedules/{id}", h.UpdateSchedule)
		r.Delete("/schedules/{id}", h.DeleteSchedule)

		r.Get("/me/schedule", h.MySchedule)

		// Student subject enrollments (write side; read side is internal-or-jwt below)
		r.Post("/students/{id}/subject-enrollments", h.SetStudentSubjectEnrollment)

		// Kenaikan Kelas
		r.Post("/grade-promotion", h.GradePromotion)

		// Transfer & Withdrawal Siswa (Menu 78)
		r.Post("/student-status-requests", h.CreateStudentStatusRequest)
		r.Get("/student-status-requests", h.ListStudentStatusRequests)
		r.Get("/student-status-requests/{id}", h.GetStudentStatusRequest)
		r.Post("/student-status-requests/{id}/apply", h.ApplyStudentStatusRequest)
		r.Get("/student-status-requests/financial-summary", h.GetStudentFinancialSummaryProxy)
		r.Get("/academic-settings", h.GetAcademicSettings)
		r.Put("/academic-settings", h.UpdateAcademicSettings)

		// Dashboard
		r.Get("/dashboard/counts", h.DashboardCounts)
	})

	// Materi & Tugas (PRD menus 57/58/70/71, internal/handler/learning*.go).
	// Deliberately NOT behind respCache: lists must reflect a submit/publish
	// immediately and file downloads must never be copied into Redis.
	r.Group(func(r chi.Router) {
		r.Use(common.JWTMiddleware(jwtSecret))
		r.Get("/learning/materials", h.ListLearningMaterials)
		r.Post("/learning/materials", h.CreateLearningMaterial)
		r.Put("/learning/materials/{id}", h.UpdateLearningMaterial)
		r.Delete("/learning/materials/{id}", h.DeleteLearningMaterial)
		r.Get("/learning/materials/{id}/file", h.DownloadLearningMaterialFile)

		r.Get("/learning/assignments", h.ListLearningAssignments)
		r.Post("/learning/assignments", h.CreateLearningAssignment)
		r.Put("/learning/assignments/{id}", h.UpdateLearningAssignment)
		r.Delete("/learning/assignments/{id}", h.DeleteLearningAssignment)
		r.Get("/learning/assignments/{id}/file", h.DownloadLearningAssignmentFile)
		r.Get("/learning/assignments/{id}/submissions", h.ListAssignmentSubmissions)
		r.Post("/learning/assignments/{id}/submission", h.SubmitAssignment)
		r.Get("/learning/submissions/{id}/file", h.DownloadSubmissionFile)
		r.Post("/learning/submissions/{id}/grade", h.GradeSubmission)
	})

	// Read-only endpoints other services (report, exam, evaluation) fetch
	// over internal HTTP using X-Internal-Key — see docs/CONVENTIONS.md and
	// internal/common/internal_auth.go. Still reachable with a normal user
	// JWT too (InternalOrJWTMiddleware falls through), so browser traffic is
	// unaffected. Every handler here goes through scopeSchoolID, which
	// degrades to an unscoped-by-ID lookup for internal callers that don't
	// pass ?school_id= — safe because the row is already uniquely identified
	// by its UUID path param.
	r.Group(func(r chi.Router) {
		r.Use(common.InternalOrJWTMiddleware(jwtSecret, internalKey))
		// active-now is deliberately excluded -- it's "who's teaching this
		// exact moment", the one route in this service where caching would
		// defeat the point (matches attendance-service's identical
		// exclusion of its downstream /dashboard/active-teachers).
		r.Use(respCache.Middleware(func(req *http.Request) bool {
			return req.URL.Path == "/schedules/active-now"
		}))

		r.Get("/terms/{id}", h.GetTerm)
		r.Get("/classrooms/{id}", h.GetClassroom)
		r.Get("/classrooms/{id}/students", h.ListClassroomStudents)
		r.Get("/classrooms/{id}/subjects", h.ListClassroomSubjects)
		r.Get("/classrooms/{id}/subject-matrix", h.GetClassroomSubjectMatrix)
		r.Get("/classrooms/{id}/student-subject-matrix", h.GetStudentSubjectMatrix)
		r.Get("/students/{id}/active-subjects", h.GetStudentActiveSubjects)
		r.Get("/students/{id}/classroom", h.GetStudentClassroom)
		r.Get("/teaching-assignments", h.GetTeachingAssignments)
		r.Get("/subjects/{id}", h.GetSubject)
		r.Get("/schedules/active-now", h.ActiveSchedulesNow)
		r.Get("/schedules/{id}", h.GetScheduleByID)
	})

	// Write access for identity-service's Import Sekoci "existing school"
	// reconciliation (see services/identity/internal/handler/import.go
	// reconcileClassrooms): create/find classrooms and enroll imported
	// students via X-Internal-Key, since that's a cross-service write with
	// no user JWT available. Deliberately a separate group from the
	// read-only internal group above rather than folding these in, so that
	// group's "read-only" contract stays true for report/exam/evaluation.
	r.Group(func(r chi.Router) {
		r.Use(common.InternalOrJWTMiddleware(jwtSecret, internalKey))
		r.Post("/classrooms", h.CreateClassroom)
		r.Get("/classrooms", h.ListClassrooms)
		r.Post("/classrooms/{id}/students", h.AddStudentToClassroom)
		// identity-service's DeleteUser cascade — see RemoveStudentEnrollments.
		r.Delete("/students/{student_user_id}/enrollments", h.RemoveStudentEnrollments)
		// admission-service's applicant->student conversion saga enrollment
		// step (see services/admission/internal/handler/conversion.go
		// academicAdapter.Enroll). Idempotent; see
		// internal/handler/admission_enrollment.go.
		r.Post("/internal/admission/enrollments/upsert", h.UpsertAdmissionEnrollment)

		// Status Request Approval Callback & Apply Job
		r.Post("/internal/student-status-requests/approval-callback", h.ApprovalCallback)
		r.Post("/internal/student-status-requests/apply-pending", h.ApplyPendingRequestsJob)
		r.Get("/internal/enrollments", h.InternalListEnrollments) // T13: read-only, internal key only
	})

	log.Printf("academic-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
