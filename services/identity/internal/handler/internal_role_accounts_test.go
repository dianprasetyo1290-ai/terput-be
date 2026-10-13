package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInternalCreateOrLinkRoleAccountValidation(t *testing.T) {
	app := &App{}
	cases := []struct {
		name string
		body string
		code int
	}{
		{"invalid json", "bad-json", http.StatusBadRequest},
		{"missing fields", `{"email":"mentor@example.com"}`, http.StatusBadRequest},
		{"invalid role not whitelisted", `{"school_id":"s1","email":"mentor@example.com","full_name":"Mentor Test","role":"guru"}`, http.StatusBadRequest},
		{"invalid role arbitrary", `{"school_id":"s1","email":"mentor@example.com","full_name":"Mentor Test","role":"superadmin"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/internal/role-accounts/create-or-link", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			app.InternalCreateOrLinkRoleAccount(w, r)
			if w.Code != tc.code {
				t.Fatalf("got status %d, want %d", w.Code, tc.code)
			}
		})
	}
}
