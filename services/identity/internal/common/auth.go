package common

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID     string   `json:"sub"`
	Name       string   `json:"name,omitempty"`
	SchoolID   string   `json:"school_id"`
	SchoolName string   `json:"school_name,omitempty"`
	ActiveRole string   `json:"active_role"`
	Roles      []string `json:"roles"`
	// SessionID (sid) ties the token to a user_sessions row so it can be
	// revoked before it expires. Other services ignore the claim; their
	// requests are checked through identity's /auth/session-check (nginx
	// auth_request, see web/nginx.conf).
	SessionID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

type ctxKey string

const ClaimsCtxKey ctxKey = "claims"

// SessionValidator decides whether a correctly signed token is still
// acceptable (user active, role still held, session not revoked or idle).
// It returns "" to accept, otherwise the rejection reason.
type SessionValidator func(r *http.Request, c *Claims) string

// ParseToken verifies signature (HS256 only) and expiry.
func ParseToken(tokenStr, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

// BearerToken extracts the token from an Authorization header.
func BearerToken(r *http.Request) string {
	authz := r.Header.Get("Authorization")
	if len(authz) > 7 && strings.EqualFold(authz[:7], "Bearer ") {
		return strings.TrimSpace(authz[7:])
	}
	return ""
}

// JWTMiddleware verifies the token only (signature + expiry). Production
// routing uses JWTSessionMiddleware.
func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return JWTSessionMiddleware(secret, nil)
}

// JWTSessionMiddleware verifies the token and, when validate is set, that its
// session is still live — a token of a deleted/deactivated user, a revoked
// role or a revoked/idle session is refused immediately (BUG-022/024/025).
func JWTSessionMiddleware(secret string, validate SessionValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := BearerToken(r)
			if tokenStr == "" {
				WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
				return
			}
			claims, err := ParseToken(tokenStr, secret)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "Sesi tidak valid atau kedaluwarsa. Silakan masuk kembali")
				return
			}
			if validate != nil {
				if reason := validate(r, claims); reason != "" {
					WriteError(w, http.StatusUnauthorized, "Sesi berakhir, silakan masuk kembali ("+reason+")")
					return
				}
			}
			ctx := context.WithValue(r.Context(), ClaimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func FromCtx(r *http.Request) *Claims {
	c, _ := r.Context().Value(ClaimsCtxKey).(*Claims)
	return c
}
