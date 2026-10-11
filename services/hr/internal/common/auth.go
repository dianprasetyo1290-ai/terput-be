package common

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Roles issued by identity-service that the hr service cares about.
const (
	RoleSuperadmin    = "superadmin"
	RoleAdmin         = "admin"
	RoleHRAdmin       = "hr_admin"
	RoleHRManager     = "hr_manager"
	RolePayroll       = "payroll_officer"
	RoleKepalaSekolah = "kepala_sekolah"
	RolePegawai       = "pegawai"
	RoleGuru          = "guru"
)

// Role groups used by the router (spec §7).
var (
	HRAdmin   = []string{RoleHRAdmin}
	HRReaders = []string{RoleHRAdmin, RoleHRManager}
	// OrgReaders may read organisation, headcount and (masked) employee data.
	OrgReaders = []string{RoleHRAdmin, RoleHRManager, RoleKepalaSekolah, RolePayroll}
)

// Claims is the identity JWT payload. hr never issues tokens.
type Claims struct {
	UserID     string   `json:"sub"`
	Subject    string   `json:"-"`
	SchoolID   string   `json:"school_id"`
	ActiveRole string   `json:"active_role"`
	Role       string   `json:"role,omitempty"`
	Email      string   `json:"email,omitempty"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

type ctxKey string

const claimsCtxKey ctxKey = "claims"

// JWTMiddleware verifies the shared identity JWT (HS256).
func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
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
			if claims.UserID == "" && claims.RegisteredClaims.Subject != "" {
				claims.UserID = claims.RegisteredClaims.Subject
			}
			if claims.Subject == "" {
				claims.Subject = claims.UserID
			}
			if claims.Role == "" && claims.ActiveRole != "" {
				claims.Role = claims.ActiveRole
			}
			if claims.ActiveRole == "" && claims.Role != "" {
				claims.ActiveRole = claims.Role
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsCtxKey, claims)))
		})
	}
}

// AuthMiddleware is an alias for JWTMiddleware.
func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return JWTMiddleware(secret)
}

// FromCtx returns the verified JWT claims, if any.
func FromCtx(r *http.Request) *Claims {
	c, _ := r.Context().Value(claimsCtxKey).(*Claims)
	if c != nil {
		if c.UserID == "" && c.RegisteredClaims.Subject != "" {
			c.UserID = c.RegisteredClaims.Subject
		}
		if c.Subject == "" {
			c.Subject = c.UserID
		}
		if c.Role == "" && c.ActiveRole != "" {
			c.Role = c.ActiveRole
		}
		if c.ActiveRole == "" && c.Role != "" {
			c.ActiveRole = c.Role
		}
	}
	return c
}

// GetClaims retrieves claims from context.
func GetClaims(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsCtxKey).(*Claims)
	if c != nil {
		if c.UserID == "" && c.RegisteredClaims.Subject != "" {
			c.UserID = c.RegisteredClaims.Subject
		}
		if c.Subject == "" {
			c.Subject = c.UserID
		}
		if c.Role == "" && c.ActiveRole != "" {
			c.Role = c.ActiveRole
		}
		if c.ActiveRole == "" && c.Role != "" {
			c.ActiveRole = c.Role
		}
	}
	return c
}

// EffectiveSchoolID is the JWT school_id. hr never honours a ?school_id=
// override unless superadmin.
func EffectiveSchoolID(r *http.Request) string {
	if c := FromCtx(r); c != nil {
		return c.SchoolID
	}
	return ""
}

// RequireRole admits the request only when the active role is one of roles.
// On failure it writes the error envelope and returns nil.
func RequireRole(w http.ResponseWriter, r *http.Request, roles []string) *Claims {
	c := FromCtx(r)
	if c == nil || (!slices.Contains(roles, c.ActiveRole) && !slices.Contains(roles, c.Role)) {
		WriteError(w, http.StatusForbidden, "Anda tidak memiliki akses ke fitur ini")
		return nil
	}
	return c
}

// RequireAnyRole admits requests where the caller has any of the listed roles.
func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := FromCtx(r)
			if c == nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			userRole := c.ActiveRole
			if userRole == "" {
				userRole = c.Role
			}
			for _, allowed := range roles {
				if allowed == userRole {
					next.ServeHTTP(w, r)
					return
				}
			}
			WriteError(w, http.StatusForbidden, "Akses ditolak")
		})
	}
}
