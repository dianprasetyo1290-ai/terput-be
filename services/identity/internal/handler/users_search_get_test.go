package handler

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
)

// GET /users/search (picker twin of POST /users/search) used to fall into
// GetUser with id="search" and 500; a non-UUID id is now a 404.
func TestUsersSearchGETAndBadID(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(claimsMiddleware)
		pr.Get("/users/search", app.ListUsers)
		pr.Get("/users/{id}", app.GetUser)
	})
	admin := seedUser(t, db, common.RoleAdmin)
	cl := claimsFor(admin.userID, common.RoleAdmin, admin.schoolID)
	for _, p := range []string{"/users/search?role=guru", "/users/search?q=bu", "/users/search?role=guru&school_id=" + admin.schoolID} {
		code, body := govCall(t, r, http.MethodGet, p, cl, nil)
		if code != http.StatusOK {
			t.Fatalf("%s: %d %v", p, code, body)
		}
	}
	if code, _ := govCall(t, r, http.MethodGet, "/users/not-a-uuid", cl, nil); code != http.StatusNotFound {
		t.Fatalf("bad id %d", code)
	}
}
