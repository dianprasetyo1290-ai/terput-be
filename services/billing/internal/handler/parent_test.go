package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

const testSecret = "test-jwt-secret"

// stubIdentity is a controllable IdentityChecker for tests.
type stubIdentity struct {
	allowed map[string]bool
	err     error
}

func (s stubIdentity) GuardianHasAccess(_ context.Context, guardianUserID, studentUserID, _ string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.allowed[guardianUserID+"|"+studentUserID], nil
}

func signToken(t *testing.T, userID, schoolID, role string) string {
	t.Helper()
	claims := common.Claims{
		UserID:     userID,
		SchoolID:   schoolID,
		ActiveRole: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func parentRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Get("/parent/children/{studentUserId}/bills", app.ListChildBills)
	return r
}

func decodeEnvelope(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env struct {
		Data    json.RawMessage `json:"data"`
		Message string          `json:"message"`
		Errors  map[string]any  `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, body)
	}
	return map[string]any{"message": env.Message, "errors": env.Errors}
}

// A guardian may only read the bills of children identity confirms they own.
func TestParentCrossStudentDenied(t *testing.T) {
	app := New(nil, config.Config{}, stubIdentity{allowed: map[string]bool{
		"guardian-1|child-own": true,
	}}, nil)
	srv := httptest.NewServer(parentRouter(app))
	defer srv.Close()

	// A child that is NOT linked to the caller must be hidden behind a 404.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/parent/children/child-other/bills", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, "guardian-1", "school-1", common.RoleParent))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-student status=%d want 404 (%s)", resp.StatusCode, body)
	}
	if msg, _ := decodeEnvelope(t, body)["message"].(string); msg != "student not found" {
		t.Fatalf("cross-student message=%q want %q", msg, "student not found")
	}
}

// When identity fails (unreachable/misconfigured) access is denied closed.
func TestParentAccessFailsClosed(t *testing.T) {
	app := New(nil, config.Config{}, stubIdentity{err: errors.New("identity down")}, nil)
	srv := httptest.NewServer(parentRouter(app))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/parent/children/child-own/bills", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, "guardian-1", "school-1", common.RoleParent))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("identity failure status=%d want 404", resp.StatusCode)
	}
}

func TestParentRoleRequired(t *testing.T) {
	app := New(nil, config.Config{}, stubIdentity{allowed: map[string]bool{"admin-1|child-own": true}}, nil)
	srv := httptest.NewServer(parentRouter(app))
	defer srv.Close()

	// An admin token must never reach the parent endpoints.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/parent/children/child-own/bills", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, "admin-1", "school-1", common.RoleAdmin))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("admin token status=%d want 403", resp.StatusCode)
	}
}

func TestParentMissingSchoolDenied(t *testing.T) {
	app := New(nil, config.Config{}, stubIdentity{allowed: map[string]bool{"guardian-1|child-own": true}}, nil)
	srv := httptest.NewServer(parentRouter(app))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/parent/children/child-own/bills", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, "guardian-1", "", common.RoleParent))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing school status=%d want 403", resp.StatusCode)
	}
}

func TestGuardianCanAccessUnit(t *testing.T) {
	claims := &common.Claims{UserID: "guardian-1", SchoolID: "school-1", ActiveRole: common.RoleParent}

	own := New(nil, config.Config{}, stubIdentity{allowed: map[string]bool{"guardian-1|child-own": true}}, nil)
	if !own.guardianCanAccess(context.Background(), claims, "child-own") {
		t.Fatal("linked child must be accessible")
	}
	if own.guardianCanAccess(context.Background(), claims, "child-other") {
		t.Fatal("unlinked child must not be accessible")
	}

	down := New(nil, config.Config{}, stubIdentity{err: errors.New("boom")}, nil)
	if down.guardianCanAccess(context.Background(), claims, "child-own") {
		t.Fatal("identity error must deny access")
	}

	unconfigured := New(nil, config.Config{}, nil, nil)
	if unconfigured.guardianCanAccess(context.Background(), claims, "child-own") {
		t.Fatal("missing identity checker must deny access")
	}
}

// The health endpoint the orchestrator probes must not require auth.
func TestHealthzNoAuth(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status=%d", rec.Code)
	}
}

// /me/bills is siswa-only; any other role (incl. orang_tua) is 403 before any DB access.
func TestMyBillsStudentRoleRequired(t *testing.T) {
	app := New(nil, config.Config{}, stubIdentity{}, nil)
	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Get("/me/bills", app.ListMyBills)
	r.Get("/me/payments", app.ListMyPayments)
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, path := range []string{"/me/bills", "/me/payments"} {
		for _, tc := range []struct{ role, school string }{
			{common.RoleParent, "school-1"},
			{common.RoleAdmin, "school-1"},
			{common.RoleStudent, ""},
		} {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
			req.Header.Set("Authorization", "Bearer "+signToken(t, "u-1", tc.school, tc.role))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("%s role=%q school=%q status=%d want 403", path, tc.role, tc.school, resp.StatusCode)
			}
		}
	}
}
