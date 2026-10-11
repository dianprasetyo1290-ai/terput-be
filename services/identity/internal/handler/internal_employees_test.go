package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"skci/identity/internal/store"
)

func TestFeatureHrisKnown(t *testing.T) {
	if !store.IsKnownFeature("hris") {
		t.Fatal("hris feature should be known")
	}
	statuses := store.FeatureStatuses(map[string]bool{"hris": true})
	found := false
	for _, s := range statuses {
		if s.FeatureKey == "hris" && s.Enabled {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("hris feature should be present in FeatureStatuses and enabled")
	}
}

func TestInternalCreateOrLinkValidation(t *testing.T) {
	app := &App{}
	cases := []struct {
		name string
		body string
		code int
	}{
		{"invalid json", "bad-json", http.StatusBadRequest},
		{"missing fields", `{"email":"test@example.com"}`, http.StatusBadRequest},
		{"invalid role", `{"school_id":"s1","email":"test@example.com","full_name":"Test","role":"invalid"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/internal/employees/create-or-link", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			app.InternalCreateOrLinkEmployee(w, r)
			if w.Code != tc.code {
				t.Fatalf("got status %d, want %d", w.Code, tc.code)
			}
		})
	}
}

func TestInternalDeactivateValidation(t *testing.T) {
	app := &App{}
	for _, body := range []string{`bad`, `{}`, `{"school_id":"s1"}`} {
		r := httptest.NewRequest(http.MethodPost, "/internal/users//deactivate", strings.NewReader(body))
		w := httptest.NewRecorder()
		app.InternalDeactivateUser(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: got status %d, want 400", body, w.Code)
		}
	}
}
