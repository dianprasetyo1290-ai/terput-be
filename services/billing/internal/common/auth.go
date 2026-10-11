package common

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Roles issued by identity-service (see docs/CONVENTIONS.md).
const (
	RoleSuperadmin = "superadmin"
	RoleAdmin      = "admin"
	RoleParent     = "orang_tua"
	RoleStudent    = "siswa"
	RoleGuru       = "guru"
	RoleWalikelas  = "walikelas"
)

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

// RequireBillReader admits admin/superadmin plus guru/walikelas for read-only
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
	switch c.ActiveRole {
	case RoleAdmin, RoleSuperadmin:
		return c, EffectiveSchoolID(r)
	case RoleGuru, RoleWalikelas:
		return c, c.SchoolID
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
