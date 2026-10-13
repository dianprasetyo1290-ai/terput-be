package common

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Roles issued by identity-service that the ops service cares about
// (identity migration 0017 for the Bagian 3–4 roles).
const (
	RoleSuperadmin       = "superadmin"
	RoleAdmin            = "admin"
	RoleGuru             = "guru"
	RoleWalikelas        = "walikelas"
	RoleKepalaSekolah    = "kepala_sekolah"
	RoleKepalaYayasan    = "kepala_yayasan"
	RolePengawas         = "pengawas"
	RoleHRAdmin          = "hr_admin"
	RoleHRManager        = "hr_manager"
	RolePayrollOfficer   = "payroll_officer"
	RoleRecruiter        = "recruiter"
	RoleTrainingAdmin    = "training_admin"
	RoleAuditor          = "auditor"
	RoleFinanceManager   = "finance_manager"
	RolePegawai          = "pegawai"
	RoleInventoryOfficer = "inventory_officer"
	RoleProcurement      = "procurement_officer"
	RoleAssetOfficer     = "asset_officer"
	RolePustakawan       = "pustakawan"
	RoleFacilityOfficer  = "facility_officer"
	RoleTeknisi          = "teknisi"
	RoleTransportOfficer = "transport_officer"
	RolePendamping       = "pendamping_transport"
	RolePengemudi        = "pengemudi"
	RoleCanteenAdmin     = "canteen_admin"
	RoleCanteenOperator  = "canteen_operator"
	RolePetugasUKS       = "petugas_uks"
	RoleKonselor         = "konselor"
	RoleKesiswaan        = "kesiswaan"
	RoleEventCoordinator = "event_coordinator"
	RolePembinaEkskul    = "pembina_ekskul"
	RoleServiceDeskAgent = "servicedesk_agent"
	RoleServiceDeskMgr   = "servicedesk_manager"
	RoleKoordinatorPKL   = "koordinator_pkl"
	RolePembimbingPKL    = "pembimbing_pkl"
	RoleQACoordinator    = "qa_coordinator"
	RoleOperatorDapodik  = "operator_dapodik"
	RoleHumas            = "humas"
	RoleBIAnalyst        = "bi_analyst"
	RoleSiswa            = "siswa"
	RoleOrangTua         = "orang_tua"
)

// StaffRoles are every internal school staff role (all roles except siswa,
// orang_tua and the external pembimbing_industri). Mirrors ALL_STAFF_ROLES in
// web/src/router/domains/core.routes.ts.
var StaffRoles = []string{
	RoleSuperadmin, RoleAdmin, RoleGuru, RoleWalikelas, RoleKepalaSekolah, RoleKepalaYayasan, RolePengawas,
	RoleHRAdmin, RoleHRManager, RolePayrollOfficer, RoleRecruiter, RoleTrainingAdmin, RoleAuditor,
	RoleFinanceManager, RolePegawai,
	RoleInventoryOfficer, RoleProcurement, RoleAssetOfficer, RolePustakawan, RoleFacilityOfficer, RoleTeknisi,
	RoleTransportOfficer, RolePendamping, RolePengemudi, RoleCanteenAdmin, RoleCanteenOperator, RolePetugasUKS,
	RoleKonselor, RoleKesiswaan, RoleEventCoordinator, RolePembinaEkskul, RoleServiceDeskAgent, RoleServiceDeskMgr,
	RoleKoordinatorPKL, RolePembimbingPKL, RoleQACoordinator, RoleOperatorDapodik, RoleHumas, RoleBIAnalyst,
}

// Claims is the identity JWT payload. ops never issues tokens.
type Claims struct {
	UserID     string   `json:"sub"`
	Name       string   `json:"name,omitempty"`
	SchoolID   string   `json:"school_id"`
	ActiveRole string   `json:"active_role"`
	Role       string   `json:"role,omitempty"`
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
			if claims.UserID == "" {
				claims.UserID = claims.RegisteredClaims.Subject
			}
			if claims.ActiveRole == "" {
				claims.ActiveRole = claims.Role
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsCtxKey, claims)))
		})
	}
}

// FromCtx returns the verified JWT claims, if any.
func FromCtx(r *http.Request) *Claims {
	c, _ := r.Context().Value(claimsCtxKey).(*Claims)
	return c
}

// EffectiveSchoolID is the JWT school_id. Only superadmin (whose JWT carries
// no school) may pick a school with ?school_id=.
func EffectiveSchoolID(r *http.Request) string {
	c := FromCtx(r)
	if c == nil {
		return ""
	}
	if c.ActiveRole == RoleSuperadmin {
		if q := strings.TrimSpace(r.URL.Query().Get("school_id")); q != "" {
			return q
		}
	}
	return c.SchoolID
}

// HasActiveRole reports whether the caller's active role is one of roles.
func HasActiveRole(c *Claims, roles ...string) bool {
	return c != nil && slices.Contains(roles, c.ActiveRole)
}

// RequireAnyRole admits requests whose active role is one of roles.
func RequireAnyRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := FromCtx(r)
			if c == nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !HasActiveRole(c, roles...) {
				WriteError(w, http.StatusForbidden, "Anda tidak memiliki akses ke fitur ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
