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

	"skci/identity/internal/common"
	"skci/identity/internal/handler"
	"skci/identity/internal/store"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// corsOrigins reads a comma-separated CORS_ALLOWED_ORIGINS so the allowed
// dev-server ports (one per frontend domain agent, see
// docs/FRONTEND_CONVENTIONS.md) are configured once via docker-compose
// rather than hardcoded per service — scales to more origins without a
// code change or rebuild.
func corsOrigins() []string {
	raw := getenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:5174,http://localhost:5175,http://localhost:5176,http://localhost:5177,http://localhost:5178")
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

	port := getenv("PORT", "8081")
	dsn := getenv("DATABASE_URL", "postgres://sekoci:change-me@localhost:5481/skci_identity?sslmode=disable")
	jwtSecret := getenv("SEKOCI_JWT_SECRET", "change-me")
	internalKey := getenv("SEKOCI_INTERNAL_KEY", "change-me")

	// Error tracking: no-op until SENTRY_DSN is set (see docs/CONVENTIONS.md).
	if err := common.InitSentry(getenv("SENTRY_DSN", ""), getenv("SENTRY_ENVIRONMENT", "development"), "identity-service"); err != nil {
		log.Printf("sentry init failed (continuing without error tracking): %v", err)
	}
	defer common.FlushSentry()

	db, err := store.Open(dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}

	app := handler.NewApp(db, jwtSecret, internalKey)

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

	// Public routes (no auth).
	r.Get("/healthz", handler.Healthz)
	r.Post("/auth/login", app.Login)

	// Everything else requires a valid JWT.
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(jwtSecret))

		pr.Post("/auth/switch-role", app.SwitchRole)
		pr.Get("/auth/me", app.Me)

		// Superadmin dashboard.
		pr.Get("/dashboard", app.Dashboard)

		// Data Yayasan (Foundation) — superadmin CRUD.
		pr.Route("/foundations", func(fr chi.Router) {
			fr.Get("/", app.ListFoundations)
			fr.Post("/", app.CreateFoundation)
			fr.Get("/{id}", app.GetFoundation)
			fr.Put("/{id}", app.UpdateFoundation)
			fr.Delete("/{id}", app.DeleteFoundation)
		})

		// Data Sekolah (School) — CRUD + lifecycle actions.
		pr.Route("/schools", func(sr chi.Router) {
			sr.Get("/", app.ListSchools)
			sr.Post("/", app.CreateSchool)
			sr.Get("/import/template", app.DownloadTemplate) // Download Template button
			sr.Post("/import/dry-run", app.DryRunImport)     // Dry Run
			sr.Post("/import", app.Import)                   // Import
			// NOT sr.Get("/deleted", ...) here: GetSchool's "/schools/{id}"
			// wildcard lives in the ir (internal-or-jwt) group below, and per
			// the /users/export precedence gotcha documented on that group,
			// a static route registered in THIS group wouldn't reliably
			// out-rank that wildcard. Registered as ir.Get("/schools/deleted", ...) instead.
			sr.Put("/{id}", app.UpdateSchool)
			sr.Delete("/{id}", app.DeleteSchool)
			sr.Post("/{id}/activate", app.ActivateSchool)
			sr.Post("/{id}/deactivate", app.DeactivateSchool)
			sr.Post("/{id}/assign-admin", app.AssignAdmin)
			sr.Post("/{id}/impersonate", app.Impersonate)
		})

		// Data Pengguna — superadmin: school_id filter optional. Admin uses
		// the same handlers; resolveSchoolScope() forces school_id from JWT
		// claims for any non-superadmin caller. Read (list) side is mounted
		// on the internal-or-jwt group below since siblings (evaluation's
		// guru-list for peer-evaluator pools, etc.) need it too.
		pr.Route("/users", func(ur chi.Router) {
			ur.Post("/", app.CreateUser)
			ur.Put("/{id}", app.UpdateUser)
			ur.Delete("/{id}", app.DeleteUser)
			ur.Post("/{id}/reset-password", app.ResetPassword)
			ur.Post("/{id}/roles", app.AssignUserRole)
		})

		// Data Siswa / Data Guru — account-management slice, reusing the
		// Data Pengguna handlers filtered by role (see internal/handler/users.go).
		// Read (list) side is mounted on the internal-or-jwt group below.
		pr.Post("/teachers/{id}/activate", app.ActivateTeacher)
		pr.Post("/teachers/{id}/deactivate", app.DeactivateTeacher)

		// Data Jabatan / Peran (same concept) + Organisasi Sekolah.
		pr.Route("/positions", func(pos chi.Router) {
			pos.Get("/", app.ListPositions)
			pos.Post("/", app.CreatePosition)
			pos.Put("/{id}", app.UpdatePosition)
			pos.Delete("/{id}", app.DeletePosition)
			pos.Post("/{id}/assign", app.AssignPosition)
		})
		pr.Delete("/position-assignments/{assignmentId}", app.UnassignPosition)
		pr.Get("/organization", app.Organization)

		// Tahun Akademik — identity owns this table; academic-service
		// resolves IDs via this same REST API over HTTP (read side is
		// mounted on the internal-or-jwt group below).
		pr.Route("/academic-years", func(ar chi.Router) {
			ar.Post("/", app.CreateAcademicYear)
			ar.Put("/{id}", app.UpdateAcademicYear)
			ar.Delete("/{id}", app.DeleteAcademicYear)
			ar.Post("/{id}/set-current", app.SetCurrentAcademicYear)
		})

		// Informasi (announcements).
		pr.Route("/announcements", func(an chi.Router) {
			an.Get("/", app.ListVisibleAnnouncements) // "visible to me" list
			an.Get("/all", app.ListAllAnnouncements)  // admin management view
			an.Post("/", app.CreateAnnouncement)
			an.Get("/{id}", app.GetAnnouncement)
			an.Put("/{id}", app.UpdateAnnouncement)
			an.Post("/{id}/publish", app.PublishAnnouncement)
			an.Post("/{id}/unpublish", app.UnpublishAnnouncement)
			an.Delete("/{id}", app.DeleteAnnouncement)
		})
	})

	// Read-only endpoints other services (report, academic, evaluation,
	// exam) fetch over internal HTTP using X-Internal-Key — see
	// docs/CONVENTIONS.md and internal/common/internal_auth.go. Still
	// reachable with a normal user JWT too (InternalOrJWTMiddleware falls
	// through), so browser traffic is unaffected.
	r.Group(func(ir chi.Router) {
		ir.Use(common.InternalOrJWTMiddleware(jwtSecret, internalKey))

		// GET /users/export is registered here (a static sibling of
		// /users/{id}), not in the JWT-only /users route block above:
		// chi's route groups are independent sub-routers, so a static
		// route registered in one Group and a wildcard registered in
		// ANOTHER Group covering the same path space do NOT get correctly
		// precedence-resolved against each other (only routes within the
		// SAME router/group get correct static-over-wildcard priority) —
		// found via a real 500 ("failed to get user") when /users/export
		// was being matched by GetUser's {id} wildcard from this group.
		// ExportUsers still does its own requireRole(Admin) check
		// (bypassed only for a trusted X-Internal-Key caller, same as
		// every other route in this group).
		ir.Get("/users/export", app.ExportUsers)
		ir.Get("/users/{id}", app.GetUser)
		ir.Get("/users", app.ListUsers)
		ir.Get("/students", app.ListStudents)
		ir.Get("/teachers", app.ListTeachers)
		// Data Sekolah's Riwayat Penghapusan: same static-before-wildcard
		// precedence rule as /users/export above — registered here, not in
		// the JWT-only /schools route block, because GetSchool's "/{id}"
		// wildcard lives in this group. ListDeletedSchools does its own
		// requireRole(superadmin) check same as ExportUsers.
		ir.Get("/schools/deleted", app.ListDeletedSchools)
		ir.Get("/schools/{id}", app.GetSchool)
		ir.Get("/academic-years", app.ListAcademicYears)
	})

	log.Printf("identity-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
