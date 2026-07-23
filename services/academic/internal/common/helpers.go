package common

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Meta is the pagination block that rides alongside `data` in list
// responses, per CONVENTIONS.md's paginated-list envelope shape.
type Meta struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

// WriteValidationError writes the envelope shape with a populated `errors`
// object for per-field validation problems.
func WriteValidationError(w http.ResponseWriter, status int, msg string, fieldErrors interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: fieldErrors})
}

// ParsePage reads ?page= and ?per_page= query params with sane defaults,
// used by every list endpoint per CONVENTIONS.md.
func ParsePage(r *http.Request) (page, perPage int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}
	return page, perPage
}

// Roles mirrors identity-service's role constants (kept local per
// CONVENTIONS.md's "no shared Go package" rule).
const (
	RoleSuperadmin = "superadmin"
	RoleAdmin      = "admin"
	RoleGuru       = "guru"
	RoleSiswa      = "siswa"
)
