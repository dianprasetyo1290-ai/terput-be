package common

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the identity-service JWT payload every service verifies. Copied
// verbatim from docs/CONVENTIONS.md.
type Claims struct {
	UserID     string   `json:"sub"`
	SchoolID   string   `json:"school_id"`
	ActiveRole string   `json:"active_role"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

type ctxKey string

const ClaimsCtxKey ctxKey = "claims"

// JWTMiddleware verifies the HS256 identity token and stores its claims in the
// request context.
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
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !token.Valid {
				WriteError(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromCtx returns the verified claims, or nil when the request was not
// authenticated with a user JWT (e.g. an internal-key call).
func FromCtx(r *http.Request) *Claims {
	c, _ := r.Context().Value(ClaimsCtxKey).(*Claims)
	return c
}
