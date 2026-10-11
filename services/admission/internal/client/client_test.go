package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostDecodesEnvelopeAndSendsHeaders(t *testing.T) {
	var gotKey, gotIdem, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Internal-Key")
		gotIdem = r.Header.Get("Idempotency-Key")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "user-1"}})
	}))
	defer server.Close()

	c := New(server.URL, "internal-key")
	var out struct {
		ID string `json:"id"`
	}
	if err := c.Post(context.Background(), "/internal/guardians/upsert", "idem-1", map[string]string{"name": "Ibu"}, &out); err != nil {
		t.Fatalf("Post error: %v", err)
	}
	if out.ID != "user-1" {
		t.Fatalf("decoded id=%q want user-1", out.ID)
	}
	if gotKey != "internal-key" {
		t.Fatalf("X-Internal-Key=%q", gotKey)
	}
	if gotIdem != "idem-1" {
		t.Fatalf("Idempotency-Key=%q", gotIdem)
	}
	if gotBody == "" {
		t.Fatal("request body was not sent")
	}
}

func TestNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	}))
	defer server.Close()

	c := New(server.URL, "k")
	err := c.Post(context.Background(), "/internal/admission/enrollments/upsert", "", nil, nil)
	if err == nil {
		t.Fatal("expected an error for a 404")
	}
	if !NotFound(err) {
		t.Fatalf("NotFound(err)=false for %v", err)
	}
	if NotFound(ErrNotConfigured) {
		t.Fatal("ErrNotConfigured is not a 404")
	}
}

func TestNotConfigured(t *testing.T) {
	c := New("", "k")
	if c.Configured() {
		t.Fatal("empty base URL must not be reported as configured")
	}
	if err := c.Post(context.Background(), "/x", "", nil, nil); err != ErrNotConfigured {
		t.Fatalf("err=%v want ErrNotConfigured", err)
	}
}
