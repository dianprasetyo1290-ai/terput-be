package common

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Roles issued by identity-service (see docs/CONVENTIONS.md).
const (
	RoleSuperadmin     = "superadmin"
	RoleAdmin          = "admin"
	RoleParent         = "orang_tua"
	RoleStudent        = "siswa"
	RoleGuru           = "guru"
	RoleWalikelas      = "walikelas"
	RoleFinanceManager = "finance_manager"
	RoleFinanceAdmin   = "finance_admin"
	RoleAccounting     = "accounting"
	RoleKasir          = "kasir"
	RoleKepalaYayasan  = "kepala_yayasan"
	RoleKepalaSekolah  = "kepala_sekolah"
)

// financeStaff may manage billing (PRD menus 17–25: Finance Admin, Finance
// Manager; admin acts as Finance Admin). financeViewers may additionally
// read finance reports (dashboard, aging) without changing anything.
var (
	financeStaff   = []string{RoleAdmin, RoleSuperadmin, RoleFinanceManager, RoleFinanceAdmin}
	financeViewers = append(append([]string{}, financeStaff...), RoleKepalaYayasan, RoleKepalaSekolah)
)

func hasRole(role string, allowed []string) bool {
	for _, a := range allowed {
		if role == a {
			return true
		}
	}
	return false
}

// TenantFor resolves the school a request acts on: a superadmin may pick a
// tenant with ?school_id=, every other role is pinned to its JWT school_id.
func TenantFor(r *http.Request, c *Claims) string {
	if c.ActiveRole == RoleSuperadmin {
		return EffectiveSchoolID(r)
	}
	return c.SchoolID
}

func requireTenant(w http.ResponseWriter, r *http.Request, allowed []string) (*Claims, string) {
	c := FromCtx(r)
	if c == nil || !hasRole(c.ActiveRole, allowed) {
		WriteError(w, http.StatusForbidden, "akses keuangan ditolak untuk peran ini")
		return nil, ""
	}
	schoolID := TenantFor(r, c)
	if schoolID == "" {
		WriteError(w, http.StatusBadRequest, "school_id is required")
		return nil, ""
	}
	return c, schoolID
}

// RequireFinanceStaff admits the roles that manage billing and returns the
// tenant school.
func RequireFinanceStaff(w http.ResponseWriter, r *http.Request) (*Claims, string) {
	return requireTenant(w, r, financeStaff)
}

// RequireFinanceViewer admits finance staff plus the read-only report roles
// (Kepala Yayasan, Kepala Sekolah) and returns the tenant school.
func RequireFinanceViewer(w http.ResponseWriter, r *http.Request) (*Claims, string) {
	return requireTenant(w, r, financeViewers)
}

// IsFinanceStaff reports whether role may change billing data.
func IsFinanceStaff(role string) bool { return hasRole(role, financeStaff) }

// Claims is the identity JWT payload billing-service verifies. It never
// issues tokens — identity is the only login authority.
type Claims struct {
	UserID     string   `json:"sub"`
	SchoolID   string   `json:"school_id"`
	ActiveRole string   `json:"active_role"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

type ctxKey string

const claimsCtxKey ctxKey = "claims"

// JWTMiddleware verifies the shared identity JWT (HS256) and stashes the
// claims in the request context.
func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			tokenStr := strings.TrimPrefix(authz, "Bearer ")
			if tokenStr == "" {
				WriteError(w, http.StatusUnauthorized, "missing token")
				return
			}
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !token.Valid {
				WriteError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromCtx returns the verified JWT claims, if any.
func FromCtx(r *http.Request) *Claims {
	c, _ := r.Context().Value(claimsCtxKey).(*Claims)
	return c
}

// RequireParent enforces an active_role of orang_tua and a tenant school_id.
// On failure it writes the error envelope and returns nil.
func RequireParent(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || c.ActiveRole != RoleParent {
		WriteError(w, http.StatusForbidden, "parent role required")
		return nil
	}
	if c.SchoolID == "" {
		WriteError(w, http.StatusForbidden, "school scope required")
		return nil
	}
	return c
}

// RequireStudent enforces an active_role of siswa and a tenant school_id.
// Siswa endpoints only ever read the caller's own (claims.UserID) records.
func RequireStudent(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || c.ActiveRole != RoleStudent {
		WriteError(w, http.StatusForbidden, "student role required")
		return nil
	}
	if c.SchoolID == "" || c.UserID == "" {
		WriteError(w, http.StatusForbidden, "school scope required")
		return nil
	}
	return c
}

// RequireAdmin enforces an admin/superadmin active_role. A superadmin may
// carry an empty school_id and select a tenant via ?school_id=.
func RequireAdmin(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || (c.ActiveRole != RoleAdmin && c.ActiveRole != RoleSuperadmin) {
		WriteError(w, http.StatusForbidden, "admin role required")
		return nil
	}
	return c
}

// RequireFinance admits admin, superadmin, finance_manager, or finance_admin.
func RequireFinance(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || (c.ActiveRole != RoleAdmin && c.ActiveRole != RoleSuperadmin && c.ActiveRole != RoleFinanceManager && c.ActiveRole != RoleFinanceAdmin) {
		WriteError(w, http.StatusForbidden, "finance role required")
		return nil
	}
	return c
}

// RequireFinanceOrAccounting admits admin, superadmin, finance_manager, finance_admin, or accounting.
func RequireFinanceOrAccounting(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || (c.ActiveRole != RoleAdmin && c.ActiveRole != RoleSuperadmin && c.ActiveRole != RoleFinanceManager && c.ActiveRole != RoleFinanceAdmin && c.ActiveRole != RoleAccounting) {
		WriteError(w, http.StatusForbidden, "finance or accounting role required")
		return nil
	}
	return c
}

// RequireCashier admits admin, superadmin, kasir, finance_admin, or finance_manager.
func RequireCashier(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || (c.ActiveRole != RoleAdmin && c.ActiveRole != RoleSuperadmin && c.ActiveRole != RoleKasir && c.ActiveRole != RoleFinanceAdmin && c.ActiveRole != RoleFinanceManager) {
		WriteError(w, http.StatusForbidden, "cashier role required")
		return nil
	}
	return c
}

// RequireBillReader admits finance viewers plus guru/walikelas for read-only
// bill listings (guru "Keuangan Kelas"). Returns the claims and the tenant:
// teachers are pinned to their own JWT school_id, never the ?school_id=
// override, so a teacher can't read another school's bills. Handlers must
// additionally limit teachers to their own classes' students (see
// handler.teacherMaySee).
func RequireBillReader(w http.ResponseWriter, r *http.Request) (*Claims, string) {
	c := FromCtx(r)
	if c == nil {
		WriteError(w, http.StatusForbidden, "admin role required")
		return nil, ""
	}
	switch {
	case c.ActiveRole == RoleGuru || c.ActiveRole == RoleWalikelas:
		return c, c.SchoolID
	case hasRole(c.ActiveRole, financeViewers):
		return c, TenantFor(r, c)
	}
	WriteError(w, http.StatusForbidden, "admin role required")
	return nil, ""
}

// EffectiveSchoolID resolves the tenant for an admin request: the JWT's
// school_id when present, otherwise the ?school_id= override a superadmin
// sends. Returns "" when neither is available.
func EffectiveSchoolID(r *http.Request) string {
	if c := FromCtx(r); c != nil && c.SchoolID != "" {
		return c.SchoolID
	}
	return strings.TrimSpace(r.URL.Query().Get("school_id"))
}
