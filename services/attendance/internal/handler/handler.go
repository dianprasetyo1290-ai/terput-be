// Package handler implements attendance-service's REST API. Endpoints follow
// the guru daily flow (check-in -> agenda -> student attendance), the admin
// Presensi Siswa / Presensi Guru views, the "guru sedang mengajar" dashboard
// monitor, dashboard summary/graph endpoints, and the report-service
// integration endpoint. See CONVENTIONS.md for the shared JWT/response/
// multi-tenancy/academic-year conventions this all builds on.
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"skci/attendance/internal/client"
	"skci/attendance/internal/common"
	"skci/attendance/internal/store"
)

type Handler struct {
	Store  *store.Store
	Client *client.Client
}

func New(s *store.Store, c *client.Client) *Handler {
	return &Handler{Store: s, Client: c}
}

// ---- shared helpers --------------------------------------------------

func requireRole(w http.ResponseWriter, claims *common.Claims, roles ...string) bool {
	for _, r := range roles {
		if claims.ActiveRole == r {
			return true
		}
	}
	common.WriteError(w, http.StatusForbidden, "forbidden: role "+claims.ActiveRole+" cannot access this endpoint")
	return false
}

// scopeFromRequest applies the multi-tenancy rule from CONVENTIONS.md: every
// non-superadmin caller is force-scoped to their own school_id regardless of
// query params; superadmin may pass ?school_id= to view a specific school or
// omit it to see across schools.
func scopeFromRequest(claims *common.Claims, r *http.Request) (schoolID string) {
	if claims.ActiveRole == "superadmin" {
		return r.URL.Query().Get("school_id")
	}
	return claims.SchoolID
}

// academicYearFromRequest applies the academic-year scoping rule: a caller
// may pass ?academic_year_id= to look at a specific (including past) year.
// attendance-service does not itself own academic_years / know which one is
// "current" (that's academic-service's data) so when the param is omitted we
// deliberately return no year filter rather than guess — see README
// Assumptions.
func academicYearFromRequest(r *http.Request) string {
	return r.URL.Query().Get("academic_year_id")
}

var indoDayToEnglish = map[string]string{
	"senin": "Monday", "selasa": "Tuesday", "rabu": "Wednesday", "kamis": "Thursday",
	"jumat": "Friday", "jum'at": "Friday", "sabtu": "Saturday", "minggu": "Sunday",
}

// normalizeDay accepts either an English or Indonesian weekday name (case
// insensitive) and returns the English form Postgres' to_char(..,'FMDay')
// produces under the default "C" locale used inside the container.
func normalizeDay(day string) string {
	if day == "" {
		return ""
	}
	if en, ok := indoDayToEnglish[strings.ToLower(day)]; ok {
		return en
	}
	return strings.Title(strings.ToLower(day))
}

func pageParams(r *http.Request) (page, perPage int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	return
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// requireOwnClassroom is the wali-kelas classroom-ownership guard (Bug
// Report SEKOCI GPM 5 Agustus 2026, ROLE WALI KELAS): a walikelas caller may
// only ever see presensi/guru-sedang-mengajar data for the classroom they're
// homeroom teacher of, verified live against academic-service (the source
// of truth for classrooms.homeroom_teacher_user_id — attendance-service
// never stores that assignment itself) rather than trusting the
// client-supplied classroom_id. Only meaningful for active_role=="walikelas"
// — callers must check that role first (this doesn't no-op for other
// roles, unlike academic-service's variant, since every call site here
// already branches on role before reaching this check).
func (h *Handler) requireOwnClassroom(w http.ResponseWriter, r *http.Request, claims *common.Claims, classroomID string) bool {
	if classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id is required")
		return false
	}
	c, err := h.Client.GetClassroom(bg(r), classroomID)
	if err != nil || c == nil || c.HomeroomTeacherUserID == nil || *c.HomeroomTeacherUserID != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "forbidden: you are not the wali kelas of this classroom")
		return false
	}
	return true
}

// bg returns a short-lived background context for best-effort enrichment
// calls to sibling services, independent of the inbound request's lifetime
// concerns beyond what net/http already gives us via r.Context().
func bg(r *http.Request) context.Context {
	return r.Context()
}
