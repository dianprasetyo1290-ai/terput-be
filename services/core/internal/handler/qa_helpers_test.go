package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"skci/core/internal/client"
	"skci/core/internal/config"
)

// fakeIdentity serves the two identity reads core uses: GET /users/{id}
// (roles in a school) and GET /users?role= (holders of a role).
type fakeIdentity struct {
	school string
	roles  map[string][]string // user id -> roles in school
	// internalUsers serves GET /internal/users (and hides the public
	// /users list) to prove callers use the internal route when present.
	internalUsers bool
}

func (f *fakeIdentity) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/users" || (f.internalUsers && r.URL.Path == "/internal/users") {
		if r.URL.Path == "/internal/users" && (r.Header.Get("X-Internal-Key") == "" || r.URL.Query().Get("school_id") != f.school) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		role := r.URL.Query().Get("role")
		if f.internalUsers && r.URL.Path == "/users" {
			role = ""
		}
		var out []map[string]string
		for id, rs := range f.roles {
			for _, x := range rs {
				if x == role {
					out = append(out, map[string]string{"id": id})
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": out})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/users/")
	rs, ok := f.roles[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
		return
	}
	var roles []map[string]any
	for _, x := range rs {
		roles = append(roles, map[string]any{"school_id": f.school, "role": x, "status": "active"})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "roles": roles}})
}

// sentNotification is one POST /internal/notifications captured by fakeNotifier.
type sentNotification struct {
	Key    string
	UserID string `json:"user_id"`
	Type   string `json:"type"`
	Title  string `json:"title"`
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent []sentNotification
	fail bool
}

func (f *fakeNotifier) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var n sentNotification
	_ = json.Unmarshal(raw, &n)
	n.Key = r.Header.Get("Idempotency-Key")
	f.mu.Lock()
	f.sent = append(f.sent, n)
	fail := f.fail
	f.mu.Unlock()
	if fail {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"data":{}}`))
}

func (f *fakeNotifier) recipients(notifType string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, n := range f.sent {
		if notifType == "" || n.Type == notifType {
			out = append(out, n.UserID)
		}
	}
	return out
}

// newWiredServer is newTestServer with fake identity + notification services.
func newWiredServer(t *testing.T, roles map[string][]string) (*fakeStore, http.Handler, *Handler, *fakeNotifier) {
	t.Helper()
	return newWiredServerWith(t, &fakeIdentity{school: testSchoolA, roles: roles})
}

// newWiredServerWith wires a caller-configured fake identity.
func newWiredServerWith(t *testing.T, identity *fakeIdentity) (*fakeStore, http.Handler, *Handler, *fakeNotifier) {
	t.Helper()
	idSrv := httptest.NewServer(identity)
	t.Cleanup(idSrv.Close)
	notifier := &fakeNotifier{}
	nSrv := httptest.NewServer(notifier)
	t.Cleanup(nSrv.Close)

	fs := newFakeStore()
	cfg := config.Config{JWTSecret: testJWTSecret, InternalKey: testInternalKey, CallbackMaxAttempts: 3,
		AuditExportMaxRows: 100, RetryLockSeconds: 300}
	cl := client.New(nSrv.URL, idSrv.URL, "", testInternalKey)
	h := New(fs, cfg, cl)
	return fs, NewRouter(h, cfg), h, notifier
}
