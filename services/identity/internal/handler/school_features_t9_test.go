package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/store"
)

// PUT /schools/{id}/features/campus3d while facility is off → 422 (T9 §2b).
func TestSetSchoolFeatureRequiresFacility(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var school string
	if err := db.QueryRow(`INSERT INTO schools (name) VALUES ('Fitur T9') RETURNING id`).Scan(&school); err != nil {
		t.Fatal(err)
	}
	// withClaims acts as this fixed user; school_features.updated_by is an FK.
	if _, err := db.Exec(`INSERT INTO users (id, name, email, password_hash) VALUES ('00000000-0000-4000-8000-000000000001','Test Actor','test-actor@test.local','x') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	app := &App{DB: db}
	put := func(key, body string) *httptest.ResponseRecorder {
		r := withClaims(httptest.NewRequest(http.MethodPut, "/schools/"+school+"/features/"+key, strings.NewReader(body)), common.RoleSuperadmin, "")
		rc := chi.NewRouteContext()
		rc.URLParams.Add("id", school)
		rc.URLParams.Add("key", key)
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
		w := httptest.NewRecorder()
		app.SetSchoolFeature(w, r)
		return w
	}
	w := put("campus3d", `{"enabled":true}`)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Aktifkan modul Fasilitas terlebih dahulu") {
		t.Fatalf("campus3d without facility: %d %s", w.Code, w.Body)
	}
	if w := put("facility", `{"enabled":true}`); w.Code != http.StatusOK {
		t.Fatalf("facility on: %d %s", w.Code, w.Body)
	}
	if w := put("campus3d", `{"enabled":true}`); w.Code != http.StatusOK {
		t.Fatalf("campus3d after facility: %d %s", w.Code, w.Body)
	}
	// Turning a feature off never needs prerequisites.
	if w := put("campus3d", `{"enabled":false}`); w.Code != http.StatusOK {
		t.Fatalf("campus3d off: %d", w.Code)
	}
}
