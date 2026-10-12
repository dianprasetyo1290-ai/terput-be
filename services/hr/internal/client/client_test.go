package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCreateApprovalRequestContract checks the T1 §3 body hr sends to core:
// subject = requester, callback = hr's own internal URL, idempotency <type>:<id>.
func TestCreateApprovalRequestContract(t *testing.T) {
	var got map[string]any
	var idemHeader string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/approvals" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		idemHeader = r.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"a1","status":"pending","current_seq":1}}`))
	}))
	defer core.Close()

	c := New("", core.URL, "", "k")
	c.CallbackURL = "http://hr:8092/internal/approval-callbacks"
	id, err := c.CreateApprovalRequest(context.Background(), "s1", "u-emp", "u-emp", ReqTypeLeave, "lr1", "Cuti")
	if err != nil || id != "a1" {
		t.Fatalf("CreateApprovalRequest = %q, %v", id, err)
	}
	want := map[string]any{
		"school_id": "s1", "request_type": "leave", "source_service": "hr", "source_id": "lr1",
		"maker_user_id": "u-emp", "subject_user_id": "u-emp",
		"callback_url": "http://hr:8092/internal/approval-callbacks", "idempotency_key": "leave:lr1",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if idemHeader != "leave:lr1" {
		t.Errorf("Idempotency-Key header = %q", idemHeader)
	}
}

func TestApprovalErrorMessage(t *testing.T) {
	status, msg := ApprovalErrorMessage(&HTTPError{Status: 422, Body: `{"message":"alur approval leave belum dikonfigurasi"}`})
	if status != http.StatusUnprocessableEntity || !strings.Contains(msg, "belum dikonfigurasi") {
		t.Fatalf("422 mapping = %d %q", status, msg)
	}
	if status, _ := ApprovalErrorMessage(errors.New("dial tcp: refused")); status != http.StatusBadGateway {
		t.Fatalf("transport error mapping = %d", status)
	}
}
