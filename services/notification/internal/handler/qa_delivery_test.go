package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"skci/notification/internal/client"
	"skci/notification/internal/model"
	"skci/notification/internal/provider"
	"skci/notification/internal/store"
)

type recordingProvider struct {
	mu   sync.Mutex
	sent []provider.Message
	err  error
}

func (p *recordingProvider) Name() string { return "recording" }
func (p *recordingProvider) Send(_ context.Context, m provider.Message) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, m)
	if p.err != nil {
		return "", p.err
	}
	return "rcpt-1", nil
}

type exceptionSink struct {
	mu   sync.Mutex
	reps []client.ExceptionReport
}

func (s *exceptionSink) report(_ context.Context, e client.ExceptionReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reps = append(s.reps, e)
	return nil
}

// newDeliveryServer wires a fake identity (contact lookup), a recording
// provider and an exception sink.
func newDeliveryServer(t *testing.T, contacts map[string][2]string) (*fakeStore, http.Handler, *Handler, *recordingProvider, *exceptionSink) {
	t.Helper()
	idSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/users/")
		c, ok := contacts[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"id": id, "email": c[0], "phone": c[1]}})
	}))
	t.Cleanup(idSrv.Close)
	fs := newFakeStore()
	p := &recordingProvider{}
	sink := &exceptionSink{}
	h := New(fs, client.New(idSrv.URL, testInternalKey), p, false)
	h.Relations = newFakeRelations()
	h.Exceptions = sink.report
	h.InternalURL = "http://notification:8089"
	router := NewRouter(h, RouterConfig{JWTSecret: testJWTSecret, InternalKey: testInternalKey})
	return fs, router, h, p, sink
}

func send(router http.Handler, key, body string) *httptest.ResponseRecorder {
	return do(router, http.MethodPost, "/internal/notifications", "", map[string]string{
		"X-Internal-Key": testInternalKey, "Idempotency-Key": key}, body)
}

func deliveryChannels(t *testing.T, fs *fakeStore, rec *httptest.ResponseRecorder) map[string]model.NotificationDelivery {
	t.Helper()
	var resp struct{ Data model.Notification }
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	ds, _ := fs.ListDeliveries(context.Background(), resp.Data.ID)
	out := map[string]model.NotificationDelivery{}
	for _, d := range ds {
		out[d.Channel] = d
	}
	return out
}

func setPref(fs *fakeStore, user, channel string, on bool) {
	_, _ = fs.UpsertNotificationPreferences(context.Background(), schoolA, user, []store.PreferenceInput{{Channel: channel, Enabled: on}})
}

// BUG-029: billing asks for in_app only; the guardian enabled WhatsApp, so WA
// is attempted and its outcome recorded.
func TestSend_PreferenceAddsChannel(t *testing.T) {
	fs, router, _, p, _ := newDeliveryServer(t, map[string][2]string{userA: {"wali@example.com", "081234567890"}})
	setPref(fs, userA, model.ChannelWhatsApp, true)
	rec := send(router, "k1", `{"school_id":"`+schoolA+`","user_id":"`+userA+`","type":"receipt","title":"Pembayaran diterima","body":"Terima kasih","channels":["in_app"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("send: %d %s", rec.Code, rec.Body.String())
	}
	ds := deliveryChannels(t, fs, rec)
	if ds["whatsapp"].Status != model.DeliverySent || ds["in_app"].Status != model.DeliverySent {
		t.Fatalf("deliveries = %+v", ds)
	}
	if _, ok := ds["email"]; ok {
		t.Fatal("email is off and not mandatory: no delivery expected")
	}
	if len(p.sent) != 1 || p.sent[0].Recipient != "081234567890" {
		t.Fatalf("provider calls = %+v", p.sent)
	}
}

// BUG-090: a bill notice is mandatory: email goes out even when switched off.
func TestSend_MandatoryIgnoresOptOut(t *testing.T) {
	fs, router, _, _, _ := newDeliveryServer(t, map[string][2]string{userA: {"wali@example.com", "081234567890"}})
	setPref(fs, userA, model.ChannelEmail, false)
	rec := send(router, "k2", `{"school_id":"`+schoolA+`","user_id":"`+userA+`","type":"bill_issued","title":"Tagihan baru terbit","body":"Tagihan Okt","channels":["in_app"]}`)
	ds := deliveryChannels(t, fs, rec)
	if ds["email"].Status != model.DeliverySent || ds["whatsapp"].Status != model.DeliverySent {
		t.Fatalf("mandatory deliveries = %+v", ds)
	}
}

// Non-mandatory: a requested external channel the user switched off is skipped.
func TestSend_OptOutRespected(t *testing.T) {
	fs, router, _, p, _ := newDeliveryServer(t, map[string][2]string{userA: {"wali@example.com", ""}})
	setPref(fs, userA, model.ChannelEmail, false)
	rec := send(router, "k3", `{"school_id":"`+schoolA+`","user_id":"`+userA+`","type":"approval","title":"x","body":"y","channels":["in_app","email"]}`)
	if _, ok := deliveryChannels(t, fs, rec)["email"]; ok || len(p.sent) != 0 {
		t.Fatal("email opt-out must be honoured for non-mandatory types")
	}
}

// BUG-029/113: unconfigured provider -> failed delivery (never fake success)
// and one grouped exception for the channel; missing phone -> per-user item;
// gateway error -> per-delivery item with retry_url.
func TestSend_FailuresReachExceptionQueue(t *testing.T) {
	t.Run("provider belum dikonfigurasi", func(t *testing.T) {
		fs, router, h, p, sink := newDeliveryServer(t, map[string][2]string{userA: {"wali@example.com", "081234567890"}})
		_, p.err = provider.UnconfiguredProvider{}.Send(context.Background(), provider.Message{Channel: "email"})
		rec := send(router, "k4", `{"school_id":"`+schoolA+`","user_id":"`+userA+`","type":"bill_issued","title":"T","body":"B"}`)
		h.Wait()
		ds := deliveryChannels(t, fs, rec)
		if ds["email"].Status != model.DeliveryFailed || ds["whatsapp"].Status != model.DeliveryFailed {
			t.Fatalf("deliveries = %+v", ds)
		}
		keys := map[string]bool{}
		for _, r := range sink.reps {
			keys[r.IdempotencyKey] = true
			if r.Kind != "notification" || r.SchoolID != schoolA || r.RetryURL != nil {
				t.Errorf("report = %+v", r)
			}
		}
		if !keys["notify-provider:email"] || !keys["notify-provider:whatsapp"] {
			t.Fatalf("keys = %v", keys)
		}
	})
	t.Run("nomor tidak ada", func(t *testing.T) {
		_, router, h, _, sink := newDeliveryServer(t, map[string][2]string{userA: {"wali@example.com", ""}})
		send(router, "k5", `{"school_id":"`+schoolA+`","user_id":"`+userA+`","type":"bill_issued","title":"T","body":"B"}`)
		h.Wait()
		if len(sink.reps) != 1 || sink.reps[0].IdempotencyKey != "notify-recipient:"+userA+":whatsapp" {
			t.Fatalf("reports = %+v", sink.reps)
		}
	})
	t.Run("gateway gagal, bisa di-retry", func(t *testing.T) {
		fs, router, h, p, sink := newDeliveryServer(t, map[string][2]string{userA: {"", "081234567890"}})
		setPref(fs, userA, model.ChannelWhatsApp, true)
		p.err = errors.New("whatsapp gateway menjawab 503")
		rec := send(router, "k6", `{"school_id":"`+schoolA+`","user_id":"`+userA+`","type":"receipt","title":"T","body":"B"}`)
		h.Wait()
		var resp struct{ Data model.Notification }
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if len(sink.reps) != 1 || sink.reps[0].RetryURL == nil ||
			*sink.reps[0].RetryURL != "http://notification:8089/internal/notifications/"+resp.Data.ID+"/retry?school_id="+schoolA {
			t.Fatalf("reports = %+v", sink.reps)
		}
		retry := do(router, http.MethodPost, "/internal/notifications/"+resp.Data.ID+"/retry?school_id="+schoolA, "",
			map[string]string{"X-Internal-Key": testInternalKey}, "")
		if retry.Code != http.StatusBadGateway {
			t.Fatalf("retry while still failing: want 502, got %d", retry.Code)
		}
		p.err = nil
		retry = do(router, http.MethodPost, "/internal/notifications/"+resp.Data.ID+"/retry?school_id="+schoolA, "",
			map[string]string{"X-Internal-Key": testInternalKey}, "")
		if retry.Code != http.StatusOK {
			t.Fatalf("retry after recovery: want 200, got %d %s", retry.Code, retry.Body.String())
		}
	})
}
