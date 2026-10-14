package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/config"
)

const (
	testJWTSecret   = "test-jwt-secret-4b71"
	testInternalKey = "test-internal-key"
	testSchoolA     = "11111111-1111-1111-1111-111111111111"
	testUserMaker   = "22222222-2222-2222-2222-222222222222"
	testUserFinance = "33333333-3333-3333-3333-333333333333"
	testUserKepala  = "44444444-4444-4444-4444-444444444444"
	testUserAuditor = "55555555-5555-5555-5555-555555555555"
	testUserAdmin   = "66666666-6666-6666-6666-666666666666"
)

func newTestServer(t *testing.T) (*fakeStore, http.Handler, *client.Client) {
	t.Helper()
	fs := newFakeStore()
	cfg := config.Config{
		JWTSecret:        testJWTSecret,
		InternalKey:      testInternalKey,
		CORSOrigins:      []string{"http://localhost:5173"},
		RetryLockSeconds: 300,
	}
	cl := client.New("", "", "", testInternalKey)
	h := New(fs, cfg, cl)
	router := NewRouter(h, cfg)
	return fs, router, cl
}

func token(t *testing.T, userID, schoolID, role string, roles ...string) string {
	t.Helper()
	allRoles := roles
	if len(allRoles) == 0 {
		allRoles = []string{role}
	}
	claims := common.Claims{
		UserID:     userID,
		SchoolID:   schoolID,
		ActiveRole: role,
		Roles:      allRoles,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func do(handler http.Handler, method, path, token string, headers map[string]string, body string) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != "" {
		bodyReader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
