package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/notification/internal/client"
	"skci/notification/internal/common"
	"skci/notification/internal/provider"
)

const (
	testJWTSecret   = "test-jwt-secret-4b71"
	testInternalKey = "test-internal-key"
)

const (
	schoolA = "11111111-1111-1111-1111-111111111111"
	schoolB = "22222222-2222-2222-2222-222222222222"
	userA   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	userB   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	userC   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

// newTestServer builds the real router over an in-memory store.
func newTestServer(t *testing.T) (*fakeStore, http.Handler) {
	t.Helper()
	fs := newFakeStore()
	h := New(fs, client.New("", ""), provider.NoopProvider{}, false)
	router := NewRouter(h, RouterConfig{
		JWTSecret:   testJWTSecret,
		InternalKey: testInternalKey,
		CORSOrigins: []string{"http://localhost:5173"},
	})
	return fs, router
}

// token mints an identity JWT exactly the way identity-service would.
func token(t *testing.T, userID, schoolID, role string) string {
	t.Helper()
	claims := common.Claims{
		UserID:     userID,
		SchoolID:   schoolID,
		ActiveRole: role,
		Roles:      []string{role},
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

// do issues a request through the router.
func do(router http.Handler, method, path, bearer string, headers map[string]string, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
