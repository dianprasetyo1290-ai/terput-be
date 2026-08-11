package handler

import (
	"net/http"

	"skci/academic/internal/client"
	"skci/academic/internal/common"
	"skci/academic/internal/store"
)

type Handler struct {
	Store    *store.Store
	Identity *client.IdentityClient
}

func New(s *store.Store, ic *client.IdentityClient) *Handler {
	return &Handler{Store: s, Identity: ic}
}

// scopeSchoolID applies the multi-tenancy rule from CONVENTIONS.md: scope to
// the caller's school_id from the JWT, except when active_role ==
// "superadmin" and an explicit ?school_id= is given, in which case that
// wins. Returns "" with ok=false (and writes the error response itself) if
// scoping cannot be resolved.
func scopeSchoolID(w http.ResponseWriter, r *http.Request) (string, bool) {
	claims := common.FromCtx(r)
	if claims == nil {
		if common.IsInternal(r) {
			// Trusted sibling-service call (see internal_auth.go): if it
			// passes ?school_id= use it as a filter, otherwise fall back to
			// an unscoped-by-ID lookup — every Store method called from the
			// internal-only route group tolerates schoolID == "" as "no
			// school filter" (see internal/store SQL: "$N = '' OR school_id
			// = $N"), since the row is already uniquely identified by its
			// UUID primary key in a path param.
			return r.URL.Query().Get("school_id"), true
		}
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return "", false
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		if q := r.URL.Query().Get("school_id"); q != "" {
			return q, true
		}
	}
	if claims.SchoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "no school_id in token; superadmin must pass ?school_id=")
		return "", false
	}
	return claims.SchoolID, true
}

// requireOwnClassroomIfWaliKelas is the wali-kelas classroom-ownership guard
// (Bug Report SEKOCI GPM 5 Agustus 2026, ROLE WALI KELAS: "kelas yang
// menjadi tanggung jawab Wali Kelas tersebut" — never another class). Every
// other active_role is unaffected (returns true immediately) so this can be
// dropped into an existing admin-facing handler without changing admin's
// behavior at all. Resolves ownership via classrooms.homeroom_teacher_user_id
// (see store/classroom.go's WaliKelas type doc comment — that's the only
// place this assignment is stored, no separate wali_kelas table) rather than
// trusting a client-supplied classroom_id.
func (h *Handler) requireOwnClassroomIfWaliKelas(w http.ResponseWriter, r *http.Request, schoolID, classroomID string) bool {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != "walikelas" {
		return true
	}
	c, err := h.Store.GetClassroom(r.Context(), schoolID, classroomID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if c == nil || c.HomeroomTeacherUserID == nil || *c.HomeroomTeacherUserID != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "forbidden: you are not the wali kelas of this classroom")
		return false
	}
	return true
}

// scopeAcademicYearID applies the academic-year scoping rule from
// CONVENTIONS.md: an explicit ?academic_year_id= always wins; otherwise we
// try to resolve the school's current year via identity-service. If that
// can't be resolved (identity unreachable, no year marked current, etc.) we
// fail closed with a 400 rather than guessing — see
// internal/client/identity.go's doc comment for the exact contract
// assumption this relies on.
func (h *Handler) scopeAcademicYearID(w http.ResponseWriter, r *http.Request, schoolID string) (string, bool) {
	if q := r.URL.Query().Get("academic_year_id"); q != "" {
		return q, true
	}
	ayID, err := h.Identity.ResolveCurrentAcademicYear(schoolID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "could not resolve current academic year (identity-service unavailable or no current year set); pass ?academic_year_id= explicitly: "+err.Error())
		return "", false
	}
	return ayID, true
}
