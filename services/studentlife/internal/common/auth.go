package common

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the identity JWT payload studentlife-service verifies. It never
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

// GetClaims returns the verified JWT claims from context, if any.
func GetClaims(ctx context.Context) *Claims {
	c, _ := ctx.Value(claimsCtxKey).(*Claims)
	if c == nil {
		return &Claims{}
	}
	return c
}

// RequireRole admits only the listed active roles and a resolvable tenant.
// On failure it writes the error envelope and returns nil, "".
func RequireRole(w http.ResponseWriter, r *http.Request, roles ...string) (*Claims, string) {
	c := FromCtx(r)
	if c == nil {
		WriteError(w, http.StatusUnauthorized, "missing token")
		return nil, ""
	}
	allowed := false
	for _, role := range roles {
		if c.ActiveRole == role {
			allowed = true
			break
		}
	}
	if !allowed {
		WriteError(w, http.StatusForbidden, "Peran Anda tidak memiliki akses ke fitur ini")
		return nil, ""
	}
	school := EffectiveSchoolID(r)
	if school == "" {
		WriteError(w, http.StatusBadRequest, "school_id wajib diisi")
		return nil, ""
	}
	return c, school
}

// EffectiveSchoolID resolves the tenant: the JWT's school_id when present,
// otherwise the ?school_id= override a superadmin sends. Non-superadmin
// callers never get the override.
func EffectiveSchoolID(r *http.Request) string {
	c := FromCtx(r)
	if c == nil {
		return ""
	}
	if c.SchoolID != "" {
		return c.SchoolID
	}
	if c.ActiveRole == RoleSuperadmin {
		return strings.TrimSpace(r.URL.Query().Get("school_id"))
	}
	return ""
}
