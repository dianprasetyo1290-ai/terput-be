package handler

import (
	"net/http"
	"strconv"

	"skci/evaluation/internal/client"
	"skci/evaluation/internal/common"
	"skci/evaluation/internal/store"
)

type Handler struct {
	LessonPlans *store.LessonPlanStore
	Periods     *store.PeriodStore
	Forms       *store.FormStore
	Tasks       *store.TaskStore
	Clients     *client.Clients
}

func New(lp *store.LessonPlanStore, per *store.PeriodStore, forms *store.FormStore, tasks *store.TaskStore, c *client.Clients) *Handler {
	return &Handler{LessonPlans: lp, Periods: per, Forms: forms, Tasks: tasks, Clients: c}
}

// effectiveSchoolID applies the multi-tenancy rule from CONVENTIONS.md:
// every query is scoped to the caller's school_id, except superadmin with an
// explicit ?school_id= override.
func effectiveSchoolID(r *http.Request, claims *common.Claims) string {
	if claims != nil && claims.ActiveRole == "superadmin" {
		if q := r.URL.Query().Get("school_id"); q != "" {
			return q
		}
	}
	if claims == nil {
		return ""
	}
	return claims.SchoolID
}

func pagination(r *http.Request) (page, perPage int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}
	return
}

func hasRole(claims *common.Claims, role string) bool {
	if claims == nil {
		return false
	}
	if claims.ActiveRole == role {
		return true
	}
	for _, r := range claims.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
