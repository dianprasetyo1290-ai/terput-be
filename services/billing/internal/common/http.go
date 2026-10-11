package common

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Decode reads a bounded JSON body into dst. Unknown fields are ignored on
// purpose: the SPA round-trips whole server objects (including read-only
// fields such as id/status) back on save, and rejecting those would break
// every edit form.
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

// IntParam parses an optional non-negative integer query param.
func IntParam(r *http.Request, key string) int {
	v, _ := strconv.Atoi(r.URL.Query().Get(key))
	return v
}
