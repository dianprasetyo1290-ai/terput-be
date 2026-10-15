package common

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const (
	RoleSuperadmin     = "superadmin"
	RoleAdmin          = "admin"
	RoleKepalaSekolah  = "kepala_sekolah"
	RoleKepalaYayasan  = "kepala_yayasan"
	RoleBiAnalyst      = "bi_analyst"
	RoleFinanceManager = "finance_manager"
	RoleGuru           = "guru"
	RoleWalikelas      = "walikelas"
	RolePegawai        = "pegawai"
	RoleSiswa          = "siswa"
	RoleOrangTua       = "orang_tua"
)

type Claims struct {
	UserID     string   `json:"user_id"`
	SchoolID   string   `json:"school_id"`
	Email      string   `json:"email"`
	Name       string   `json:"name"`
	ActiveRole string   `json:"active_role"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

type ctxKey string

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	claimsCtxKey   ctxKey = "claims"
	internalCtxKey ctxKey = "is_internal"
)

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				WriteError(w, http.StatusUnauthorized, "Missing or invalid authorization header")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				WriteError(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			claims, ok := token.Claims.(*Claims)
			if !ok {
				WriteError(w, http.StatusUnauthorized, "Invalid token claims")
				return
			}
			// A superadmin token carries no school; scope it to the
			// ?school_id= the web sends so handlers reading claims.SchoolID
			// see the school being viewed.
			if claims.SchoolID == "" && claims.ActiveRole == RoleSuperadmin {
				if q := strings.TrimSpace(r.URL.Query().Get("school_id")); uuidRe.MatchString(q) {
					claims.SchoolID = q
				}
			}

			ctx := context.WithValue(r.Context(), claimsCtxKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func InternalKeyMiddleware(internalKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get("X-Internal-Key")
			if provided == "" || provided != internalKey {
				WriteError(w, http.StatusUnauthorized, "Invalid or missing X-Internal-Key")
				return
			}
			ctx := context.WithValue(r.Context(), internalCtxKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func FromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsCtxKey).(*Claims)
	return claims
}

func IsInternal(ctx context.Context) bool {
	v, _ := ctx.Value(internalCtxKey).(bool)
	return v
}

func RequireRoles(allowed ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := FromContext(r.Context())
			if claims == nil {
				WriteError(w, http.StatusUnauthorized, "Authentication required")
				return
			}

			// Superadmin always has access
			if claims.ActiveRole == RoleSuperadmin {
				next.ServeHTTP(w, r)
				return
			}

			if slices.Contains(allowed, claims.ActiveRole) {
				next.ServeHTTP(w, r)
				return
			}

			WriteError(w, http.StatusForbidden, "Akses ditolak: peran tidak memiliki izin")
		})
	}
}
