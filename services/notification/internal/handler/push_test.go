package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/notification/internal/client"
	"skci/notification/internal/model"
	"skci/notification/internal/provider"
	"skci/notification/internal/store"
)

type fakePushSender struct {
	configured bool
	pubKey     string
	sent       []provider.PushPayload
	returnGone bool
	returnErr  error
}

func (f *fakePushSender) IsConfigured() bool   { return f.configured }
func (f *fakePushSender) VAPIDPublicKey() string { return f.pubKey }
func (f *fakePushSender) SendPush(_ context.Context, _ model.PushSubscription, p provider.PushPayload) (string, bool, error) {
	if f.returnGone {
		return "", true, errors.New("endpoint gone (410)")
	}
	if f.returnErr != nil {
		return "", false, f.returnErr
	}
	f.sent = append(f.sent, p)
	return fmt.Sprintf("receipt-%d", len(f.sent)), false, nil
}

func newTestPushServer(t *testing.T, pushSender provider.PushSender) (*fakeStore, *Handler, http.Handler) {
	t.Helper()
	fs := newFakeStore()
	h := New(fs, client.New("", ""), provider.NoopProvider{}, false)
	h.Relations = newFakeRelations()
	h.PushProvider = pushSender
	router := NewRouter(h, RouterConfig{
		JWTSecret:   testJWTSecret,
		InternalKey: testInternalKey,
		CORSOrigins: []string{"http://localhost:5173"},
	})
	return fs, h, router
}

func TestPushSubscriptionEndpoints(t *testing.T) {
	fps := &fakePushSender{configured: true, pubKey: "test-vapid-pub-key-123"}
	_, _, router := newTestPushServer(t, fps)

	userTok := token(t, userA, schoolA, "orang_tua")

	// 1. GET /push/config
	req := httptest.NewRequest("GET", "/push/config", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var cfgResp struct {
		Data struct {
			Enabled        bool   `json:"enabled"`
			VAPIDPublicKey string `json:"vapid_public_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfgResp); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if !cfgResp.Data.Enabled || cfgResp.Data.VAPIDPublicKey != "test-vapid-pub-key-123" {
		t.Fatalf("unexpected config: %+v", cfgResp.Data)
	}

	// 2. POST /push/subscriptions
	subPayload := map[string]any{
		"endpoint": "https://push.example.com/sub/1",
		"keys": map[string]string{
			"p256dh": "dummy-p256dh-key",
			"auth":   "dummy-auth-key",
		},
	}
	b, _ := json.Marshal(subPayload)
	req = httptest.NewRequest("POST", "/push/subscriptions", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+userTok)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// 3. DELETE /push/subscriptions
	req = httptest.NewRequest("DELETE", "/push/subscriptions?endpoint=https://push.example.com/sub/1", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAutoDispatchPush_DefaultAndExemptions(t *testing.T) {
	fps := &fakePushSender{configured: true, pubKey: "test-pub-key"}
	fs, _, router := newTestPushServer(t, fps)

	// User has an active subscription
	_, err := fs.UpsertPushSubscription(context.Background(), model.PushSubscription{
		SchoolID: schoolA,
		UserID:   userA,
		Endpoint: "https://push.example.com/userA",
		P256dh:   "p256dh",
		Auth:     "auth",
	})
	if err != nil {
		t.Fatalf("upsert sub: %v", err)
	}

	// Case 1: Send notification with channels: ['in_app']. Push should be AUTO-DISPATCHED!
	sendPayload := map[string]any{
		"school_id": schoolA,
		"user_id":   userA,
		"type":      "bill_issued",
		"title":     "Tagihan baru terbit",
		"body":      "SPP Bulan Oktober Rp 500.000",
		"data":      map[string]any{"bill_id": "bill-1", "url": "/orang-tua/tagihan/bill-1"},
		"channels":  []string{"in_app"},
	}
	bodyBytes, _ := json.Marshal(sendPayload)
	req := httptest.NewRequest("POST", "/internal/notifications", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Internal-Key", testInternalKey)
	req.Header.Set("Idempotency-Key", "bill-issued:bill-1:userA")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if len(fps.sent) != 1 {
		t.Fatalf("expected 1 push sent, got %d", len(fps.sent))
	}
	p := fps.sent[0]
	if p.Title != "Tagihan baru terbit" || p.Body != "SPP Bulan Oktober Rp 500.000" || p.URL != "/orang-tua/tagihan/bill-1" {
		t.Errorf("unexpected push payload: %+v", p)
	}

	// Verify subscription last_success_at was touched
	subs, _ := fs.ListActivePushSubscriptions(context.Background(), schoolA, userA)
	if len(subs) != 1 || subs[0].LastSuccessAt == nil {
		t.Errorf("expected LastSuccessAt to be set after delivery")
	}

	// Case 2: Campaign notification exemption (BR-COM-01) - NO auto-push!
	fps.sent = nil
	campPayload := map[string]any{
		"school_id": schoolA,
		"user_id":   userA,
		"type":      "campaign",
		"title":     "Diskon SPP Awal Tahun",
		"body":      "Dapatkan potongan 10%",
		"channels":  []string{"in_app"},
	}
	campBytes, _ := json.Marshal(campPayload)
	req = httptest.NewRequest("POST", "/internal/notifications", bytes.NewReader(campBytes))
	req.Header.Set("X-Internal-Key", testInternalKey)
	req.Header.Set("Idempotency-Key", "camp-1:userA")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(fps.sent) != 0 {
		t.Fatalf("campaign notification should NOT auto-dispatch push, but got %d sent", len(fps.sent))
	}

	// Case 3: Explicitly disabled push preference (CC-08)
	fps.sent = nil
	_, _ = fs.UpsertNotificationPreferences(context.Background(), schoolA, userA, []store.PreferenceInput{
		{Channel: "push", Enabled: false},
	})
	sendPayload2 := map[string]any{
		"school_id": schoolA,
		"user_id":   userA,
		"type":      "academic_announcement",
		"title":     "Pengumuman Ujian",
		"body":      "Jadwal PTS sudah tersedia",
		"channels":  []string{"in_app"},
	}
	body2, _ := json.Marshal(sendPayload2)
	req = httptest.NewRequest("POST", "/internal/notifications", bytes.NewReader(body2))
	req.Header.Set("X-Internal-Key", testInternalKey)
	req.Header.Set("Idempotency-Key", "anno-1:userA")
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(fps.sent) != 0 {
		t.Fatalf("disabled push preference should NOT receive push, but got %d", len(fps.sent))
	}
}

func TestAutoDispatchPush_GoneEndpointRevocation(t *testing.T) {
	fps := &fakePushSender{configured: true, pubKey: "test-pub-key", returnGone: true}
	fs, _, router := newTestPushServer(t, fps)

	_, err := fs.UpsertPushSubscription(context.Background(), model.PushSubscription{
		SchoolID: schoolA,
		UserID:   userB,
		Endpoint: "https://push.example.com/userB-expired",
		P256dh:   "p256dh",
		Auth:     "auth",
	})
	if err != nil {
		t.Fatalf("upsert sub: %v", err)
	}

	sendPayload := map[string]any{
		"school_id": schoolA,
		"user_id":   userB,
		"type":      "bill_issued",
		"title":     "Tagihan",
		"body":      "Tagihan terbit",
		"channels":  []string{"in_app"},
	}
	bodyBytes, _ := json.Marshal(sendPayload)
	req := httptest.NewRequest("POST", "/internal/notifications", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Internal-Key", testInternalKey)
	req.Header.Set("Idempotency-Key", "bill-issued:gone-test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify the subscription is now revoked (revoked_at != nil)
	activeSubs, _ := fs.ListActivePushSubscriptions(context.Background(), schoolA, userB)
	if len(activeSubs) != 0 {
		t.Errorf("expected subscription to be revoked after 410 Gone, but found %d active", len(activeSubs))
	}
}

func TestAutoDispatchPush_UnconfiguredProvider(t *testing.T) {
	// PushProvider is unconfigured
	fps := &fakePushSender{configured: false}
	fs, _, router := newTestPushServer(t, fps)

	_, _ = fs.UpsertPushSubscription(context.Background(), model.PushSubscription{
		SchoolID: schoolA,
		UserID:   userC,
		Endpoint: "https://push.example.com/userC",
		P256dh:   "p256dh",
		Auth:     "auth",
	})

	sendPayload := map[string]any{
		"school_id": schoolA,
		"user_id":   userC,
		"type":      "bill_issued",
		"title":     "Tagihan",
		"body":      "Tagihan terbit",
		"channels":  []string{"in_app"},
	}
	bodyBytes, _ := json.Marshal(sendPayload)
	req := httptest.NewRequest("POST", "/internal/notifications", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Internal-Key", testInternalKey)
	req.Header.Set("Idempotency-Key", "bill-issued:unconf-test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Deliveries []model.NotificationDelivery `json:"deliveries"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	foundPush := false
	for _, d := range resp.Data.Deliveries {
		if d.Channel == "push" {
			foundPush = true
			if d.Status != model.DeliveryFailed {
				t.Errorf("expected failed status for unconfigured push, got %s", d.Status)
			}
		}
	}
	if !foundPush {
		t.Errorf("expected push delivery row even when unconfigured")
	}
}
