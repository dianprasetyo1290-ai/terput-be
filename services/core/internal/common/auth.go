package common

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Roles issued by identity-service.
const (
	RoleSuperadmin     = "superadmin"
	RoleAdmin          = "admin"
	RoleParent         = "orang_tua"
	RoleStudent        = "siswa"
	RoleGuru           = "guru"
	RoleWalikelas      = "walikelas"
	RoleKepalaSekolah  = "kepala_sekolah"
	RoleKepalaYayasan  = "kepala_yayasan"
	RolePengawas       = "pengawas"
	RoleHRAdmin        = "hr_admin"
	RoleHRManager      = "hr_manager"
	RolePayrollOfficer = "payroll_officer"
	RoleRecruiter      = "recruiter"
	RoleTrainingAdmin  = "training_admin"
	RoleAuditor        = "auditor"
	RoleFinanceManager = "finance_manager"
	RolePegawai        = "pegawai"
	// RoleAkuntan is the school accountant (finance/accounting staff).
	RoleAkuntan = "akuntan"
)

// ApproverRoles are the roles a flow step may name (approver_role /
// escalate_to_role). Same values as identity's user_school_roles CHECK minus
// the portal roles (siswa, orang_tua) that never approve anything.
var ApproverRoles = []string{
	RoleSuperadmin, RoleAdmin, RoleGuru, RoleWalikelas, RoleKepalaSekolah, RoleKepalaYayasan, RolePengawas,
	RoleHRAdmin, RoleHRManager, RolePayrollOfficer, RoleRecruiter, RoleTrainingAdmin, RoleAuditor,
	RoleFinanceManager, RolePegawai,
}

// Claims is the identity JWT payload core-service verifies.
type Claims struct {
	UserID     string   `json:"sub"`
	UID        string   `json:"user_id,omitempty"`
	Name       string   `json:"name,omitempty"`
	SchoolID   string   `json:"school_id"`
	SchoolName string   `json:"school_name,omitempty"`
	ActiveRole string   `json:"active_role"`
	Roles      []string `json:"roles"`
	jwt.RegisteredClaims
}

func (c *Claims) GetUserID() string {
	if c.UserID != "" {
		return c.UserID
	}
	return c.UID
}

func (c *Claims) HasRole(role string) bool {
	if c.ActiveRole == role {
		return true
	}
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
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

// RequireAdmin enforces an admin/superadmin active_role.
func RequireAdmin(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || (c.ActiveRole != RoleAdmin && c.ActiveRole != RoleSuperadmin) {
		WriteError(w, http.StatusForbidden, "admin role required")
		return nil
	}
	return c
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

// TenantSchoolID is the school a request is scoped to. Only a superadmin may
// pick a school via ?school_id=; everyone else is pinned to the JWT school, so
// a token without school_id cannot reach another tenant through the query.
func TenantSchoolID(r *http.Request) string {
	c := FromCtx(r)
	if c == nil {
		return ""
	}
	if c.ActiveRole == RoleSuperadmin {
		return EffectiveSchoolID(r)
	}
	return c.SchoolID
}
