package common

import "net/http"

// Canteen roles (T9 §2, created by identity migration 0017).
const (
	RoleCanteenAdmin    = "canteen_admin"
	RoleCanteenOperator = "canteen_operator"
)

// HasRole reports whether the active role is one of roles.
func HasRole(c *Claims, roles ...string) bool {
	if c == nil {
		return false
	}
	for _, r := range roles {
		if c.ActiveRole == r {
			return true
		}
	}
	return false
}

// RequireRoles admits any of roles (superadmin always) and resolves the
// tenant: superadmin may pass ?school_id=, everyone else is pinned to the
// JWT school. On failure it writes the error and returns nil.
func RequireRoles(w http.ResponseWriter, r *http.Request, roles ...string) (*Claims, string) {
	c := FromCtx(r)
	if c == nil || !(c.ActiveRole == RoleSuperadmin || HasRole(c, roles...)) {
		WriteError(w, http.StatusForbidden, "Peran Anda tidak memiliki akses ke fitur ini")
		return nil, ""
	}
	school := c.SchoolID
	if c.ActiveRole == RoleSuperadmin {
		school = EffectiveSchoolID(r)
	}
	if school == "" {
		WriteError(w, http.StatusBadRequest, "school_id is required")
		return nil, ""
	}
	return c, school
}
