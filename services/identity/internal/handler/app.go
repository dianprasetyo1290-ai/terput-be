package handler

import (
	"database/sql"
	"net/http"
	"strings"

	"skci/identity/internal/client"
	"skci/identity/internal/common"
)

// App holds shared dependencies for all handlers.
type App struct {
	DB          *sql.DB
	JWTSecret   string
	InternalKey string
	Exam        *client.ExamClient
	Academic    *client.AcademicClient
}

func NewApp(db *sql.DB, jwtSecret, internalKey string) *App {
	return &App{DB: db, JWTSecret: jwtSecret, InternalKey: internalKey, Exam: client.NewExamClient(), Academic: client.NewAcademicClient()}
}

// requireRole rejects the request unless the caller's active_role is one of
// allowed, or the caller is superadmin (superadmin can act as any dashboard
// role's admin-level endpoints unless a handler says otherwise).
func requireRole(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	claims := common.FromCtx(r)
	if claims == nil {
		if common.IsInternal(r) {
			// Trusted sibling-service call authenticated via X-Internal-Key
			// (see internal_auth.go) — only ever true on the specific
			// read-only routes mounted under InternalOrJWTMiddleware in
			// main.go, so bypassing the role check here is safe.
			return true
		}
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return false
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		return true
	}
	for _, a := range allowed {
		if claims.ActiveRole == a {
			return true
		}
	}
	common.WriteError(w, http.StatusForbidden, "forbidden: requires role "+strings.Join(allowed, " or "))
	return false
}

// resolveSchoolScope implements the multi-tenancy rule from CONVENTIONS.md:
// non-superadmin callers are hard-scoped to their own JWT school_id;
// superadmin may pass ?school_id= to filter, or omit it to see all schools.
// Returns (schoolID, isFiltered). isFiltered=false means "no school filter,
// show everything" (only ever true for superadmin).
func resolveSchoolScope(r *http.Request) (schoolID string, isFiltered bool) {
	claims := common.FromCtx(r)
	if claims == nil && common.IsInternal(r) {
		// Trusted sibling-service call: behave like an unfiltered
		// superadmin — an explicit ?school_id= filters, omitting it means
		// "no filter" (only ever reachable on the internal-or-jwt route
		// group in main.go).
		if q := r.URL.Query().Get("school_id"); q != "" {
			return q, true
		}
		return "", false
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		q := r.URL.Query().Get("school_id")
		if q == "" {
			return "", false
		}
		return q, true
	}
	return claims.SchoolID, true
}
