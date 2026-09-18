package common

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Pagination reads ?page=&per_page= with sane defaults/bounds.
func Pagination(r *http.Request) (page, perPage int) {
	page = 1
	perPage = 20
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
			if perPage > 200 {
				perPage = 200
			}
		}
	}
	return
}

type Meta struct {
	Page    int   `json:"page"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
}

// DecodeJSON decodes the request body into v, writing a 400 envelope on failure.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}

// EffectiveSchoolID resolves the school_id to scope queries by: the caller's
// JWT school_id, unless the caller is superadmin AND passes ?school_id=.
func EffectiveSchoolID(r *http.Request, c *Claims) string {
	if c == nil {
		// Trusted sibling-service call via X-Internal-Key (see
		// internal_auth.go) — behave like an unscoped superadmin: an
		// explicit ?school_id= filters, omitting it means "unscoped
		// lookup by ID" (safe since callers always pass a specific
		// resource UUID, not a list query, on internal-only routes).
		return r.URL.Query().Get("school_id")
	}
	if c.ActiveRole == "superadmin" {
		if v := r.URL.Query().Get("school_id"); v != "" {
			return v
		}
	}
	return c.SchoolID
}

func Contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
