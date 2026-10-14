package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
)

// fakeCore stands in for core's Approval Center, audit and notification
// endpoints, recording what billing sent.
type fakeCore struct {
	mu        sync.Mutex
	refuse    bool // answer approval requests with 422 (no flow configured)
	approvals []client.ApprovalRequest
	audits    []client.AuditEvent
	notified  []client.Notification
	guardians []string // returned for every student
}

func (f *fakeCore) handler() http.Handler {
	r := chi.NewRouter()
	r.Post("/internal/approvals", func(w http.ResponseWriter, r *http.Request) {
		var req client.ApprovalRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.refuse {
			common.WriteError(w, http.StatusUnprocessableEntity, "alur approval "+req.RequestType+" belum dikonfigurasi")
			return
		}
		f.approvals = append(f.approvals, req)
		common.WriteJSON(w, http.StatusCreated, map[string]any{"id": "appr-" + req.SourceID, "status": "pending"}, nil)
	})
	r.Post("/internal/audit", func(w http.ResponseWriter, r *http.Request) {
		var e client.AuditEvent
		_ = json.NewDecoder(r.Body).Decode(&e)
		f.mu.Lock()
		f.audits = append(f.audits, e)
		f.mu.Unlock()
		common.WriteJSON(w, http.StatusCreated, map[string]string{"id": "a1"}, nil)
	})
	r.Post("/internal/notifications", func(w http.ResponseWriter, r *http.Request) {
		var n client.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		f.mu.Lock()
		f.notified = append(f.notified, n)
		f.mu.Unlock()
		common.WriteJSON(w, http.StatusCreated, map[string]string{"id": "n1"}, nil)
	})
	r.Post("/internal/exceptions", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusCreated, map[string]string{"id": "x1"}, nil)
	})
	r.Get("/internal/students/{id}/guardians", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		rows := []map[string]string{}
		for _, g := range f.guardians {
			rows = append(rows, map[string]string{"guardian_user_id": g})
		}
		f.mu.Unlock()
		common.WriteJSON(w, http.StatusOK, rows, nil)
	})
	return r
}

func (f *fakeCore) lastApproval(t *testing.T) client.ApprovalRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.approvals) == 0 {
		t.Fatal("no approval request reached core")
	}
	return f.approvals[len(f.approvals)-1]
}

// apiCaller signs every request as one user against router r.
type apiCaller struct {
	t     *testing.T
	r     http.Handler
	token string
}

func (c apiCaller) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	rec := httptest.NewRecorder()
	c.r.ServeHTTP(rec, req)
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env == nil {
		env = map[string]any{"raw": rec.Body.String()}
	}
	return rec.Code, env
}

func dataOf(env map[string]any) map[string]any {
	d, _ := env["data"].(map[string]any)
	return d
}

// notificationsTo returns the notifications sent to userID.
func (f *fakeCore) notificationsTo(userID string) []client.Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []client.Notification
	for _, n := range f.notified {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	return out
}
