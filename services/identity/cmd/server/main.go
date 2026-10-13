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

	"skci/identity/internal/cache"
	"skci/identity/internal/common"
	"skci/identity/internal/handler"
	"skci/identity/internal/store"
	"skci/identity/internal/whitelabel"
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
	app.CoreURL = getenv("CORE_URL", "http://localhost:8091")
	wlStore := whitelabel.NewStore(db)
	wlHandler := whitelabel.NewHandler(wlStore, app.CoreURL, app.InternalKey)

	// Redis response cache (client request 2026-08-29): fails open if
	// REDIS_URL is unset/unreachable, see internal/cache's doc comment.
	respCache := cache.New(getenv("REDIS_URL", ""), 15*time.Second)
	skipCache := func(r *http.Request) bool {
		// /auth/me directly reflects the caller's just-switched active_role
		// (POST /auth/switch-role) -- caching it risks a stale role
		// showing right when the user is watching for the switch to land.
		if r.URL.Path == "/auth/me" {
			return true
		}
		// Saklar fitur per sekolah: PUT tidak punya cara membatalkan entri
		// cache GET (cache ini tanpa invalidasi, lihat internal/cache), jadi
		// endpoint fiturnya dikecualikan -- superadmin yang baru menyalakan
		// Akuntansi harus langsung melihat statusnya benar, bukan 15 detik
		// yang basi, dan Laravel /sso tidak boleh menerima jawaban "menyala"
		// untuk sekolah yang baru dimatikan. Volumenya kecil (sekali per
		// login untuk SPA), jadi tak ada yang hilang.
		return strings.Contains(r.URL.Path, "/features")
	}

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

	// White-label public branding & manifest (T26, menu 159)
	r.Get("/public/branding", wlHandler.PublicBranding)
	r.Get("/public/manifest.webmanifest", wlHandler.PublicManifest)
	r.Get("/public/branding/{schoolId}/{kind}.png", wlHandler.PublicIcon)
	r.Get("/public/assetlinks.json", wlHandler.PublicAssetLinks)

	// Everything else requires a valid JWT.
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(jwtSecret))
		pr.Use(respCache.Middleware(skipCache))

		pr.Post("/auth/switch-role", app.SwitchRole)
		pr.Get("/auth/me", app.Me)
		pr.Post("/auth/change-password", app.ChangePassword) // Admin's self-service "Ubah Password"
		pr.Get("/me/children", app.MyChildren)
		pr.Post("/me/children/claim", app.ClaimChild)

		// Superadmin dashboard.
		pr.Get("/dashboard", app.Dashboard)

		// Saklar fitur per sekolah (school_features). Ditulis langsung di
		// level root, BUKAN di dalam pr.Route("/schools") di bawah: GET
		// /schools/{id}/features hidup di grup internal-or-jwt (Laravel /sso
		// membacanya dengan X-Internal-Key), dan sebuah pola di dalam
		// sub-router yang di-Mount tidak ikut diperhitungkan terhadap node
		// /schools/{id}/features milik grup itu -- PUT-nya akan jadi 405.
		// Didaftarkan di trie yang sama supaya GET dan PUT berdampingan.
		pr.Put("/schools/{id}/features/{key}", app.SetSchoolFeature)
		pr.Get("/features", app.ListAllFeatures) // semua sekolah sekaligus (superadmin)
		pr.Get("/me/features", app.MyFeatures)   // dipakai SPA setelah login

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
			ur.Post("/import", app.ImportUsers)
			// GET /users/import-template is NOT registered here — see the
			// /users/export precedence comment in the ir group below; it hits
			// the exact same chi Mount-boundary bug (was 500ing with GetUser's
			// "failed to get user" because ir's directly-registered
			// /users/{id} wildcard was matching "import-template" as {id}
			// before this mounted sub-router ever got a chance).
			ur.Put("/{id}", app.UpdateUser)
			ur.Delete("/{id}", app.DeleteUser)
			ur.Post("/{id}/reset-password", app.ResetPassword)
			ur.Post("/{id}/roles", app.AssignUserRole)
			ur.Delete("/{id}/roles/{role}", app.RemoveUserRole)
		})

		// Data Siswa / Data Guru — account-management slice, reusing the
		// Data Pengguna handlers filtered by role (see internal/handler/users.go).
		// Read (list) side is mounted on the internal-or-jwt group below.
		pr.Post("/teachers/{id}/activate", app.ActivateTeacher)
		pr.Post("/teachers/{id}/deactivate", app.DeactivateTeacher)

		pr.Route("/guardian-links", func(gr chi.Router) {
			gr.Get("/", app.ListGuardianLinks)
			gr.Post("/", app.CreateGuardianLink)
			gr.Put("/{id}", app.UpdateGuardianLink)
			gr.Delete("/{id}", app.DeleteGuardianLink)
		})
		pr.Post("/guardian-claim-codes", app.IssueGuardianClaim)

		// Data Jabatan / Peran (same concept) + Organisasi Sekolah.
		pr.Route("/positions", func(pos chi.Router) {
			pos.Get("/", app.ListPositions)
			pos.Post("/", app.CreatePosition)
			pos.Put("/{id}", app.UpdatePosition)
			pos.Delete("/{id}", app.DeletePosition)
			pos.Post("/{id}/assign", app.AssignPosition)
		})
		pr.Delete("/position-assignments/{assignmentId}", app.UnassignPosition)
		// GET /organization itself is NOT registered here -- report-service
		// needs to read it (Kepala Sekolah lookup for E-Raport auto-fill)
		// over X-Internal-Key, which this JWT-only group rejects. It's
		// registered on the internal-or-jwt group below instead, same as
		// /schools/{id} and /academic-years.
		pr.Get("/organization/export", app.ExportOrganizationExcel)
		pr.Get("/organization/import/template", app.DownloadOrganizationTemplate)
		pr.Post("/organization/import", app.ImportOrganization)

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
			an.Delete("/all", app.DeleteAllAnnouncements)
		})

		// White-label management (T26, menu 159)
		pr.Get("/white-label/brandings", wlHandler.GetDraft)
		pr.Put("/white-label/brandings/{schoolId}", wlHandler.SaveDraft)
		pr.Post("/white-label/brandings/{schoolId}/publish", wlHandler.PublishVersion)
		pr.Get("/white-label/brandings/{schoolId}/versions", wlHandler.ListVersions)
		pr.Put("/white-label/brandings/{schoolId}/domain", wlHandler.UpdateDomain)
		pr.Get("/white-label/store-listings/{schoolId}/{store}", wlHandler.GetStoreListing)
		pr.Put("/white-label/store-listings/{schoolId}/{store}", wlHandler.SaveStoreListing)
	})

	// Read-only endpoints other services (report, academic, evaluation,
	// exam) fetch over internal HTTP using X-Internal-Key — see
	// docs/CONVENTIONS.md and internal/common/internal_auth.go. Still
	// reachable with a normal user JWT too (InternalOrJWTMiddleware falls
	// through), so browser traffic is unaffected.
	r.Group(func(in chi.Router) {
		in.Use(common.InternalOnlyMiddleware(internalKey))
		in.Post("/internal/guardians/upsert", app.InternalUpsertGuardian)
		// Guardian relation reads for sibling services (report/academic):
		// which students a guardian is linked to, and a per-student yes/no
		// access check. Both are tenant-scoped by a mandatory ?school_id=
		// because an X-Internal-Key caller carries no JWT claims to derive
		// the school from (400 when it is missing).
		in.Get("/internal/guardians/{guardianUserId}/children", app.InternalGuardianChildren)
		in.Get("/internal/guardians/{guardianUserId}/students/{studentUserId}/access", app.InternalGuardianStudentAccess)
		in.Post("/internal/students/create-or-link", app.InternalCreateOrLinkStudent)
		in.Get("/internal/students/{studentUserId}/guardians", app.InternalStudentGuardians)
		in.Post("/internal/guardian-links", app.InternalCreateGuardianLink)
		in.Get("/internal/conversions/{conversionRef}", app.InternalConversionMapping)
		in.Post("/internal/employees/create-or-link", app.InternalCreateOrLinkEmployee)
		in.Post("/internal/role-accounts/create-or-link", app.InternalCreateOrLinkRoleAccount)
		in.Post("/internal/users/{id}/deactivate", app.InternalDeactivateUser)
		in.Get("/internal/dapodik/students", app.InternalDapodikStudents)

		// T24: Insight Read Model & Metric Facts
		in.Get("/internal/metric-facts", app.InternalMetricFacts)
		in.Get("/internal/schools/{id}/foundation-schools", app.InternalFoundationSchools)
	})

	r.Group(func(ir chi.Router) {
		ir.Use(common.InternalOrJWTMiddleware(jwtSecret, internalKey))
		// Safe for X-Internal-Key callers too: cacheKey falls back to a
		// shared "anon" scope when there are no JWT claims, but every
		// tenant-scoping param these internal reads rely on (school_id
		// etc.) travels as a query string param either way, and the full
		// query string is already part of the key.
		ir.Use(respCache.Middleware(nil))

		// GET /users/export and /users/import-template are registered here
		// (static siblings of /users/{id}), not in the JWT-only /users route
		// block above: chi's route groups are independent sub-routers, so a
		// static route registered in one Group and a wildcard registered in
		// ANOTHER Group covering the same path space do NOT get correctly
		// precedence-resolved against each other (only routes within the
		// SAME router/group get correct static-over-wildcard priority) —
		// found via a real 500 ("failed to get user") when /users/export
		// (and later /users/import-template) was being matched by GetUser's
		// {id} wildcard from this group. Both handlers still do their own
		// requireRole(Admin) check (bypassed only for a trusted
		// X-Internal-Key caller, same as every other route in this group).
		ir.Get("/users/export", app.ExportUsers)
		ir.Get("/users/import-template", app.DownloadUsersTemplate)
		ir.Get("/users/{id}", app.GetUser)
		ir.Get("/users", app.ListUsers)
		// Same handler as GET /users, reached when ids/exclude_ids is too large for
		// a GET query string (see the "Belum ada kelas" / nginx 414 comment in
		// listUsersByRole). A separate path, not POST /users, since that method+path
		// is already CreateUser on the pr group above -- same underlying chi tree.
		ir.Post("/users/search", app.ListUsers)
		ir.Get("/students", app.ListStudents)
		ir.Get("/teachers", app.ListTeachers)
		// Data Sekolah's Riwayat Penghapusan: same static-before-wildcard
		// precedence rule as /users/export above — registered here, not in
		// the JWT-only /schools route block, because GetSchool's "/{id}"
		// wildcard lives in this group. ListDeletedSchools does its own
		// requireRole(superadmin) check same as ExportUsers.
		ir.Get("/schools/deleted", app.ListDeletedSchools)
		ir.Get("/schools/{id}", app.GetSchool)
		// Dibaca Laravel accounting /sso (X-Internal-Key) untuk menolak
		// sekolah yang fitur accounting-nya mati, dan oleh SPA superadmin /
		// admin sekolah itu sendiri (read-only).
		ir.Get("/schools/{id}/features", app.ListSchoolFeatures)
		ir.Get("/academic-years", app.ListAcademicYears)
		// Organisasi Sekolah read: report-service scans this for the
		// current "Kepala Sekolah" jabatan holder (see internal_auth.go).
		// Still reachable with a normal user JWT too (falls through).
		ir.Get("/organization", app.Organization)
	})

	log.Printf("identity-service listening on :%s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}
