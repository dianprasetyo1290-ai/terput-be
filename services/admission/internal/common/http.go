// Package common holds the shared HTTP plumbing every admission handler uses:
// the response envelope, request decoding, pagination, and the three auth
// middlewares (admin JWT, guardian session JWT, internal key).
package common

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Envelope is the standard response shape (see docs/CONVENTIONS.md).
type Envelope struct {
	Data    any    `json:"data"`
	Meta    any    `json:"meta,omitempty"`
	Message string `json:"message"`
	Errors  any    `json:"errors"`
}

// WriteJSON writes a success envelope.
func WriteJSON(w http.ResponseWriter, status int, data, meta any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: data, Meta: meta, Message: "", Errors: nil})
}

// WriteError writes an error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: map[string]string{"message": msg}})
}

// Decode reads a bounded JSON body into dst. Unknown fields are ignored on
// purpose: the SPA round-trips whole server objects (including read-only
// fields such as id/status/updated_at) back on save, and rejecting those would
// break every edit form.
func Decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// Page reads the page/per_page query params with sane defaults and a cap.
func Page(r *http.Request) (page, perPage int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 200 {
		perPage = 200
	}
	return page, perPage
}

// Offset is a convenience for LIMIT/OFFSET queries.
func Offset(page, perPage int) int {
	if page < 1 {
		page = 1
	}
	return (page - 1) * perPage
}

// AdminClaims is the identity JWT payload admission verifies. Superadmin
// tokens carry an empty SchoolID and select a school via ?school_id=.
type AdminClaims struct {
	UserID     string   `json:"sub"`
	SchoolID   string   `json:"school_id"`
	ActiveRole string   `json:"active_role"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

// GuardianClaims is the admission-issued guardian session token.
type GuardianClaims struct {
	GuardianID string `json:"sub"`
	SchoolID   string `json:"school_id"`
	Scope      string `json:"scope"`
	jwt.RegisteredClaims
}

type ctxKey string

const (
	adminKey    ctxKey = "admin"
	guardianKey ctxKey = "guardian"
	internalKey ctxKey = "internal"
)

// GuardianScope is the value of the guardian token's scope claim.
const GuardianScope = "admission:guardian"

func bearer(r *http.Request) string {
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

// guardianToken pulls the guardian session token from the Authorization
// header, falling back to X-Guardian-Token so non-bearer clients (the SPA
// stores the token separately from the identity JWT) can still authenticate.
func guardianToken(r *http.Request) string {
	if tok := bearer(r); tok != "" {
		return tok
	}
	if tok := strings.TrimSpace(r.Header.Get("X-Guardian-Token")); tok != "" {
		return tok
	}
	return strings.TrimSpace(r.URL.Query().Get("guardian_token"))
}

// AdminAuth verifies the identity JWT and requires an admin/superadmin role.
func AdminAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := new(AdminClaims)
			token, err := jwt.ParseWithClaims(bearer(r), claims, func(t *jwt.Token) (any, error) {
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !token.Valid {
				WriteError(w, http.StatusUnauthorized, "invalid admin token")
				return
			}
			if claims.ActiveRole != "admin" && claims.ActiveRole != "superadmin" {
				WriteError(w, http.StatusForbidden, "admin role required")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminKey, claims)))
		})
	}
}

// GuardianAuth verifies the admission guardian session token.
func GuardianAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := new(GuardianClaims)
			token, err := jwt.ParseWithClaims(guardianToken(r), claims, func(t *jwt.Token) (any, error) {
				return []byte(secret), nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !token.Valid || claims.Scope != GuardianScope || claims.GuardianID == "" {
				WriteError(w, http.StatusUnauthorized, "invalid guardian token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), guardianKey, claims)))
		})
	}
}

// InternalAuth verifies the shared service-to-service key.
func InternalAuth(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Internal-Key")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				WriteError(w, http.StatusUnauthorized, "invalid internal credentials")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), internalKey, true)))
		})
	}
}

// Admin returns the verified admin claims, if any.
func Admin(r *http.Request) *AdminClaims {
	c, _ := r.Context().Value(adminKey).(*AdminClaims)
	return c
}

// Guardian returns the verified guardian claims, if any.
func Guardian(r *http.Request) *GuardianClaims {
	c, _ := r.Context().Value(guardianKey).(*GuardianClaims)
	return c
}

// RequireAdmin is a defensive re-check for handlers reached inside the admin
// group.
func RequireAdmin(w http.ResponseWriter, r *http.Request) bool {
	c := Admin(r)
	if c == nil || (c.ActiveRole != "admin" && c.ActiveRole != "superadmin") {
		WriteError(w, http.StatusForbidden, "admin role required")
		return false
	}
	return true
}

// EffectiveSchoolID resolves the tenant for an admin request: the JWT's
// school_id when present, otherwise the ?school_id= override a superadmin
// sends (the SPA injects its active school on every request).
func EffectiveSchoolID(r *http.Request) string {
	if c := Admin(r); c != nil && c.SchoolID != "" {
		return c.SchoolID
	}
	return strings.TrimSpace(r.URL.Query().Get("school_id"))
}
