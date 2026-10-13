package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"skci/billing/internal/client"
)

func TestFormatRupiah(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "Rp 0"},
		{500, "Rp 500"},
		{5000, "Rp 5.000"},
		{500000, "Rp 500.000"},
		{1250000, "Rp 1.250.000"},
		{10000000, "Rp 10.000.000"},
		{-100000, "Rp -100.000"},
	}

	for _, c := range cases {
		got := formatRupiah(c.in)
		if got != c.want {
			t.Errorf("formatRupiah(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBillNotificationDispatch(t *testing.T) {
	var mu sync.Mutex
	notificationCalls := []map[string]any{}
	idempotencyKeys := []string{}

	// Mock notification service
	notifSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/notifications" && r.Method == http.MethodPost {
			mu.Lock()
			defer mu.Unlock()
			idemKey := r.Header.Get("Idempotency-Key")
			idempotencyKeys = append(idempotencyKeys, idemKey)

			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			notificationCalls = append(notificationCalls, body)

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"id": "notif-123"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer notifSrv.Close()

	// Mock identity service
	identSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		studentID := r.URL.Path // /internal/students/{id}/guardians
		if studentID != "" {
			w.WriteHeader(http.StatusOK)
			// Return 2 guardians for any student
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"guardian_user_id": "guard-1"},
					{"guardian_user_id": "guard-2"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer identSrv.Close()

	app := &App{
		Notification: client.New(notifSrv.URL, "test-key"),
		IdentitySvc:  client.New(identSrv.URL, "test-key"),
		NotifySync:   true,
	}

	schoolID := "school-uuid-1"
	bills := []newBillNotificationInfo{
		{
			BillID:      "bill-1",
			StudentID:   "student-1",
			NetIDR:      500000,
			PeriodYear:  2026,
			PeriodMonth: 10,
		},
		{
			BillID:      "bill-2",
			StudentID:   "student-2",
			NetIDR:      750000,
			PeriodYear:  2026,
			PeriodMonth: 10,
		},
	}

	app.notifyGuardiansForNewBills(schoolID, bills)

	mu.Lock()
	defer mu.Unlock()

	// 2 bills * 2 guardians = 4 notifications
	if len(notificationCalls) != 4 {
		t.Fatalf("expected 4 notification calls, got %d", len(notificationCalls))
	}

	expectedKeys := map[string]bool{
		"bill-issued:bill-1:guard-1": true,
		"bill-issued:bill-1:guard-2": true,
		"bill-issued:bill-2:guard-1": true,
		"bill-issued:bill-2:guard-2": true,
	}

	for _, k := range idempotencyKeys {
		if !expectedKeys[k] {
			t.Errorf("unexpected idempotency key: %s", k)
		}
		delete(expectedKeys, k)
	}
	if len(expectedKeys) > 0 {
		t.Errorf("missing idempotency keys: %+v", expectedKeys)
	}

	for _, call := range notificationCalls {
		if call["type"] != "bill_issued" {
			t.Errorf("expected type bill_issued, got %v", call["type"])
		}
		if call["title"] != "Tagihan baru terbit" {
			t.Errorf("expected title 'Tagihan baru terbit', got %v", call["title"])
		}
		channels, _ := call["channels"].([]any)
		if len(channels) != 1 || channels[0] != "in_app" {
			t.Errorf("expected channels ['in_app'], got %v", channels)
		}
		data, _ := call["data"].(map[string]any)
		if data["url"] == nil || data["bill_id"] == nil {
			t.Errorf("expected data to have url and bill_id, got %v", data)
		}
	}
}

func TestBillNotificationDispatch_ServiceDownDoesNotPanic(t *testing.T) {
	// Identity and Notification point to non-listening port
	app := &App{
		Notification: client.New("http://127.0.0.1:59999", "test-key"),
		IdentitySvc:  client.New("http://127.0.0.1:59999", "test-key"),
		NotifySync:   true,
	}

	// Should fail gracefully without panic
	app.notifyGuardiansForNewBills("school-1", []newBillNotificationInfo{
		{BillID: "b1", StudentID: "s1", NetIDR: 100000, PeriodYear: 2026, PeriodMonth: 10},
	})
}
