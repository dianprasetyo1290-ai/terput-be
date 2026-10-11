package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

func decodeNotification(t *testing.T, rec *httptest.ResponseRecorder) model.Notification {
	t.Helper()
	var env struct {
		Data model.Notification `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v (body %s)", err, rec.Body.String())
	}
	return env.Data
}

// TestMarkReadIsIdempotent pins two rules: marking read sets read_at exactly
// once (a retry must not move it), and a user can only mark their OWN
// notification (someone else's id is a 404, not a 403 leak).
func TestMarkReadIsIdempotent(t *testing.T) {
	fs, router := newTestServer(t)
	ctx := context.Background()

	n, _, err := fs.CreateNotification(ctx, store.CreateNotificationInput{
		SchoolID: schoolA, UserID: userA, Type: "billing.invoice.paid", Title: "Tagihan baru",
	})
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}

	tokA := token(t, userA, schoolA, "siswa")
	path := "/notifications/" + n.ID + "/read"

	rec1 := do(router, http.MethodPost, path, tokA, nil, "")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first mark-read: status = %d, want 200 (body %s)", rec1.Code, rec1.Body.String())
	}
	first := decodeNotification(t, rec1)
	if first.ReadAt == nil {
		t.Fatal("first mark-read returned read_at = null")
	}

	rec2 := do(router, http.MethodPost, path, tokA, nil, "")
	if rec2.Code != http.StatusOK {
		t.Fatalf("second mark-read: status = %d, want 200 (body %s)", rec2.Code, rec2.Body.String())
	}
	second := decodeNotification(t, rec2)
	if second.ReadAt == nil || !first.ReadAt.Equal(*second.ReadAt) {
		t.Fatalf("mark-read is not idempotent: read_at moved from %v to %v", first.ReadAt, second.ReadAt)
	}

	stored, err := fs.GetNotification(ctx, n.ID, schoolA, userA)
	if err != nil {
		t.Fatalf("reload notification: %v", err)
	}
	if stored.ReadAt == nil || !stored.ReadAt.Equal(*first.ReadAt) {
		t.Fatalf("stored read_at = %v, want the first call's %v", stored.ReadAt, first.ReadAt)
	}

	// Another user's token: the notification is not theirs.
	tokB := token(t, userB, schoolA, "siswa")
	if rec := do(router, http.MethodPost, path, tokB, nil, ""); rec.Code != http.StatusNotFound {
		t.Errorf("cross-user mark-read: status = %d, want 404", rec.Code)
	}
	// Unknown id.
	if rec := do(router, http.MethodPost, "/notifications/does-not-exist/read", tokA, nil, ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown id mark-read: status = %d, want 404", rec.Code)
	}
	// No token.
	if rec := do(router, http.MethodPost, path, "", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated mark-read: status = %d, want 401", rec.Code)
	}
}

// TestUnreadCount covers the inbox badge and the ?unread=true filter.
func TestUnreadCount(t *testing.T) {
	fs, router := newTestServer(t)
	ctx := context.Background()

	var ids []string
	for i := 0; i < 3; i++ {
		n, _, err := fs.CreateNotification(ctx, store.CreateNotificationInput{
			SchoolID: schoolA, UserID: userA, Type: "chat.message", Title: "Pesan baru",
		})
		if err != nil {
			t.Fatalf("seed notification %d: %v", i, err)
		}
		ids = append(ids, n.ID)
	}
	// A notification for a DIFFERENT user must not count toward userA.
	if _, _, err := fs.CreateNotification(ctx, store.CreateNotificationInput{
		SchoolID: schoolA, UserID: userB, Type: "chat.message", Title: "Pesan user lain",
	}); err != nil {
		t.Fatalf("seed other user's notification: %v", err)
	}
	if _, err := fs.MarkNotificationRead(ctx, ids[0], schoolA, userA); err != nil {
		t.Fatalf("mark one read: %v", err)
	}

	tokA := token(t, userA, schoolA, "siswa")

	rec := do(router, http.MethodGet, "/notifications/unread-count", tokA, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("unread-count: status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode unread-count: %v", err)
	}
	if env.Data.Count != 2 {
		t.Errorf("unread count = %d, want 2 (3 seeded, 1 read, plus 1 for another user)", env.Data.Count)
	}

	// ?unread=true narrows the list and its total.
	rec = do(router, http.MethodGet, "/notifications?unread=true", tokA, nil, "")
	var listEnv struct {
		Data []model.Notification `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("decode unread list: %v", err)
	}
	if len(listEnv.Data) != 2 || listEnv.Meta.Total != 2 {
		t.Errorf("unread list len=%d total=%d, want 2 and 2", len(listEnv.Data), listEnv.Meta.Total)
	}

	// Without the filter, all three of userA's notifications are listed.
	rec = do(router, http.MethodGet, "/notifications", tokA, nil, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("decode all list: %v", err)
	}
	if listEnv.Meta.Total != 3 {
		t.Errorf("all-notifications total = %d, want 3", listEnv.Meta.Total)
	}
}

// TestInternalSendIsIdempotent is the billing/admission retry contract: the
// same Idempotency-Key must never create a second notification or delivery,
// while a different key is a distinct message.
func TestInternalSendIsIdempotent(t *testing.T) {
	fs, router := newTestServer(t)
	ctx := context.Background()

	body := `{"school_id":"` + schoolA + `","user_id":"` + userA + `","type":"billing.invoice.paid","title":"Tagihan Lunas","body":"Pembayaran diterima","data":{"invoice_id":"inv-1"}}`

	first := do(router, http.MethodPost, "/internal/notifications", "", map[string]string{
		"X-Internal-Key":  testInternalKey,
		"Idempotency-Key": "billing:inv-1:paid",
	}, body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first internal send: status = %d, want 201 (body %s)", first.Code, first.Body.String())
	}
	n1 := decodeNotification(t, first)

	replay := do(router, http.MethodPost, "/internal/notifications", "", map[string]string{
		"X-Internal-Key":  testInternalKey,
		"Idempotency-Key": "billing:inv-1:paid",
	}, body)
	if replay.Code != http.StatusOK {
		t.Fatalf("replayed internal send: status = %d, want 200 (body %s)", replay.Code, replay.Body.String())
	}
	n2 := decodeNotification(t, replay)
	if n2.ID != n1.ID {
		t.Fatalf("replay created a second notification: %s != %s", n2.ID, n1.ID)
	}

	fs.mu.Lock()
	notifications := len(fs.notifications)
	fs.mu.Unlock()
	if notifications != 1 {
		t.Fatalf("notification rows = %d, want exactly 1 after a retry", notifications)
	}

	deliveries, err := fs.ListDeliveries(ctx, n1.ID)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("deliveries for the notification = %d, want exactly 1 (in_app)", len(deliveries))
	}
	if deliveries[0].Status != model.DeliverySent {
		t.Errorf("in_app delivery status = %q, want %q", deliveries[0].Status, model.DeliverySent)
	}

	// A different key is a genuinely new message.
	third := do(router, http.MethodPost, "/internal/notifications", "", map[string]string{
		"X-Internal-Key":  testInternalKey,
		"Idempotency-Key": "billing:inv-2:paid",
	}, body)
	if third.Code != http.StatusCreated {
		t.Fatalf("distinct key send: status = %d, want 201 (body %s)", third.Code, third.Body.String())
	}
	if n3 := decodeNotification(t, third); n3.ID == n1.ID {
		t.Fatal("a distinct idempotency key reused the same notification id")
	}

	// Contract guards.
	missingKey := do(router, http.MethodPost, "/internal/notifications", "", map[string]string{
		"X-Internal-Key": testInternalKey,
	}, body)
	if missingKey.Code != http.StatusBadRequest {
		t.Errorf("missing Idempotency-Key: status = %d, want 400", missingKey.Code)
	}
	missingAuth := do(router, http.MethodPost, "/internal/notifications", "", map[string]string{
		"Idempotency-Key": "billing:inv-3:paid",
	}, body)
	if missingAuth.Code != http.StatusUnauthorized {
		t.Errorf("missing internal key: status = %d, want 401", missingAuth.Code)
	}
	// A user JWT must not open the internal route.
	userJWT := token(t, userA, schoolA, "admin")
	userOnInternal := do(router, http.MethodPost, "/internal/notifications", userJWT, map[string]string{
		"Idempotency-Key": "billing:inv-4:paid",
	}, body)
	if userOnInternal.Code != http.StatusUnauthorized {
		t.Errorf("user JWT on internal route: status = %d, want 401", userOnInternal.Code)
	}
}
