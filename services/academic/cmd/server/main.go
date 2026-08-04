package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

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

		// Dashboard
		r.Get("/dashboard/counts", h.DashboardCounts)
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

	log.Printf("academic-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
