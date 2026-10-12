package common

import "net/http"

// Feature keys of the T14 payment extensions (identity school_features, T9 §2).
// Both also require FeaturePaymentGateway.
const (
	FeaturePaymentGateway       = "payment_gateway"
	FeaturePaymentMultiProvider = "payment_multi_provider"
	FeaturePaymentAutodebit     = "payment_autodebit"
)

// RequirePlatformAdmin admits only superadmin (Platform Admin of menu 150).
func RequirePlatformAdmin(w http.ResponseWriter, r *http.Request) *Claims {
	c := FromCtx(r)
	if c == nil || c.ActiveRole != RoleSuperadmin {
		WriteError(w, http.StatusForbidden, "platform admin role required")
		return nil
	}
	return c
}

// RequirePaymentConfig admits admin, finance_manager and superadmin for
// routing rules and autodebit administration. Returns the claims and the
// tenant: admin/finance_manager are pinned to their JWT school, superadmin
// may pick one with ?school_id=.
func RequirePaymentConfig(w http.ResponseWriter, r *http.Request) (*Claims, string) {
	c := FromCtx(r)
	if c == nil {
		WriteError(w, http.StatusForbidden, "finance role required")
		return nil, ""
	}
	switch c.ActiveRole {
	case RoleAdmin, RoleFinanceManager:
		if c.SchoolID == "" {
			WriteError(w, http.StatusForbidden, "school scope required")
			return nil, ""
		}
		return c, c.SchoolID
	case RoleSuperadmin:
		s := EffectiveSchoolID(r)
		if s == "" {
			WriteError(w, http.StatusBadRequest, "school_id required")
			return nil, ""
		}
		return c, s
	}
	WriteError(w, http.StatusForbidden, "finance role required")
	return nil, ""
}
