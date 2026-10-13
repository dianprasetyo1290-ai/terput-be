package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/quality/internal/client"
	"skci/quality/internal/common"
	"skci/quality/internal/config"
	"skci/quality/internal/govreport"
	"skci/quality/internal/qa"
	"skci/quality/internal/storage"
)

func createTestToken(secret, schoolID, userID, role string) string {
	claims := common.Claims{
		UserID:     userID,
		SchoolID:   schoolID,
		ActiveRole: role,
		Roles:      []string{role},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := t.SignedString([]byte(secret))
	return tokenStr
}

func TestHealthCheck(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:   "test-secret",
		InternalKey: "test-key",
	}
	app := &App{Config: cfg}
	router := app.Router()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestUnauthorizedWithoutToken(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:   "test-secret",
		InternalKey: "test-key",
	}
	app := &App{Config: cfg}
	router := app.Router()

	req := httptest.NewRequest(http.MethodGet, "/qa/instruments", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}
}

func TestDapodikDecideValidation_BR_GOV_01(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:   "test-secret",
		InternalKey: "test-key",
	}
	govHandler := govreport.NewHandler(nil, nil, nil, "hash-key", 1000)
	app := &App{
		Config:     cfg,
		GovHandler: govHandler,
	}

	token := createTestToken("test-secret", "sch-1", "usr-1", common.RoleOperatorDapodik)

	// Decision without reason must be rejected with 422 (BR-GOV-01)
	body := map[string]string{
		"decision": "update_dapodik",
		"reason":   "", // empty reason
	}
	b, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/gov/dapodik/diffs/diff-123/decide", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for decide without reason, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMetricDictionaryFallback(t *testing.T) {
	// Tests client.ListMetrics fallback if insight is down
	clients := client.NewClients("key", "http://127.0.0.1:59999", "http://core", "http://identity", "http://academic", "http://hr", "http://notif")
	metrics, err := clients.ListMetrics(t.Context())
	if err != nil {
		t.Fatalf("unexpected error on fallback: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatal("expected baseline metrics, got empty list")
	}
	found := false
	for _, m := range metrics {
		if m.Key == "attendance_rate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected attendance_rate in baseline metrics")
	}
}

func TestGetIndicatorValue_AC_142_01_Fallback(t *testing.T) {
	cfg := &config.Config{
		JWTSecret:   "test-secret",
		InternalKey: "test-key",
	}
	clients := client.NewClients("key", "http://127.0.0.1:59999", "http://core", "http://identity", "http://academic", "http://hr", "http://notif")
	st, _ := storage.NewLocalStorage(t.TempDir())
	qaHandler := qa.NewHandler(nil, clients, st)
	app := &App{
		Config:    cfg,
		QAHandler: qaHandler,
	}

	token := createTestToken("test-secret", "sch-1", "usr-1", common.RoleQACoordinator)
	req := httptest.NewRequest(http.MethodGet, "/qa/indicators/ind-none/value?from=2026-01-01&to=2026-06-30", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	app.Router().ServeHTTP(rec, req)
	// Expect 404 because ind-none doesn't exist in store (nil store check in handler)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 404/500, got %d", rec.Code)
	}
}
