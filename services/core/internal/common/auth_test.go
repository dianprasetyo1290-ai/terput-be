package common

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestRequireAdmin(t *testing.T) {
	cases := []struct {
		role string
		ok   bool
	}{
		{RoleSuperadmin, true},
		{RoleAdmin, true},
		{RoleGuru, false},
		{RoleHRAdmin, false},
		{RoleStudent, false},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/approval-flows", nil)
		r = r.WithContext(context.WithValue(r.Context(), claimsCtxKey, &Claims{ActiveRole: tc.role, SchoolID: "s1"}))
		w := httptest.NewRecorder()
		c := RequireAdmin(w, r)
		if (c != nil) != tc.ok {
			t.Errorf("role %s: got ok=%v, want %v", tc.role, c != nil, tc.ok)
		}
	}
}

func TestEffectiveSchoolID(t *testing.T) {
	// With claims school_id
	r1 := httptest.NewRequest("GET", "/approval-flows?school_id=override", nil)
	r1 = r1.WithContext(context.WithValue(r1.Context(), claimsCtxKey, &Claims{SchoolID: "jwt-school"}))
	if got := EffectiveSchoolID(r1); got != "jwt-school" {
		t.Errorf("got %q, want 'jwt-school'", got)
	}

	// Superadmin without school_id in claims
	r2 := httptest.NewRequest("GET", "/approval-flows?school_id=override", nil)
	r2 = r2.WithContext(context.WithValue(r2.Context(), claimsCtxKey, &Claims{ActiveRole: RoleSuperadmin, SchoolID: ""}))
	if got := EffectiveSchoolID(r2); got != "override" {
		t.Errorf("got %q, want 'override'", got)
	}
}

func TestJWTMiddleware(t *testing.T) {
	secret := "test-secret"
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		UserID:     "u1",
		SchoolID:   "s1",
		ActiveRole: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	mw := JWTMiddleware(secret)
	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		c := FromCtx(r)
		if c == nil || c.GetUserID() != "u1" {
			t.Fatalf("expected claims with UserID u1, got %+v", c)
		}
	}))

	r := httptest.NewRequest("GET", "/test", nil)
	r.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler was not called")
	}
}
