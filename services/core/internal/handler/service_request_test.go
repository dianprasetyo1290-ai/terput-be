package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/config"
	"skci/core/internal/model"
)

// TestParentServiceRequestGuardianAndNotify: a parent may only file for a
// student they actively guard (identity access check), and staff holding the
// category's handler_role are notified.
func TestParentServiceRequestGuardianAndNotify(t *testing.T) {
	const (
		parent   = "77777777-7777-7777-7777-777777777777"
		child    = "88888888-8888-8888-8888-888888888888"
		stranger = "99999999-9999-9999-9999-999999999999"
		staff    = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)
	var mu sync.Mutex
	var notified []map[string]any
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/internal/guardians/"+parent+"/students/"+child+"/access" && r.URL.Query().Get("school_id") == testSchoolA:
			_, _ = w.Write([]byte(`{"data":{"allowed":true}}`))
		case strings.HasPrefix(r.URL.Path, "/internal/guardians/"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"guardian access not found"}`))
		case r.URL.Path == "/users" && r.URL.Query().Get("role") == "tata_usaha":
			_, _ = w.Write([]byte(`{"data":[{"id":"` + staff + `"}]}`))
		case r.URL.Path == "/internal/notifications":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			notified = append(notified, body)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer down.Close()

	fs := newFakeStore()
	fs.srCategories = map[string]*model.ServiceRequestCategory{
		"cat-1": {ID: "cat-1", SchoolID: testSchoolA, Name: "Surat Keterangan Aktif", HandlerRole: "tata_usaha", RequiresStudent: true, Active: true},
	}
	cfg := config.Config{JWTSecret: testJWTSecret, InternalKey: testInternalKey}
	h := New(fs, cfg, client.New(down.URL, down.URL, "", testInternalKey))
	router := NewRouter(h, cfg)
	parentTok := token(t, parent, testSchoolA, common.RoleParent)

	body := func(student string) string {
		return `{"category_id":"cat-1","student_user_id":"` + student + `","subject":"Surat aktif","description":"Untuk beasiswa"}`
	}

	if rec := do(router, http.MethodPost, "/parent/service-requests", parentTok, nil, body(stranger)); rec.Code != http.StatusForbidden {
		t.Fatalf("non-guarded student: want 403, got %d %s", rec.Code, rec.Body.String())
	}
	guruTok := token(t, staff, testSchoolA, common.RoleGuru)
	if rec := do(router, http.MethodPost, "/parent/service-requests", guruTok, nil, body(child)); rec.Code != http.StatusForbidden {
		t.Fatalf("non-parent role: want 403, got %d", rec.Code)
	}
	rec := do(router, http.MethodPost, "/parent/service-requests", parentTok, nil, body(child))
	if rec.Code != http.StatusCreated {
		t.Fatalf("guarded student: want 201, got %d %s", rec.Code, rec.Body.String())
	}
	h.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 1 || notified[0]["user_id"] != staff || notified[0]["type"] != "service_request" {
		t.Fatalf("expected one service_request notification to staff, got %v", notified)
	}
}

// TestParentServiceRequestIdentityDown fails closed when identity is not reachable.
func TestParentServiceRequestIdentityDown(t *testing.T) {
	fs := newFakeStore()
	fs.srCategories = map[string]*model.ServiceRequestCategory{
		"cat-1": {ID: "cat-1", SchoolID: testSchoolA, Name: "Surat", HandlerRole: "admin", RequiresStudent: true, Active: true},
	}
	cfg := config.Config{JWTSecret: testJWTSecret, InternalKey: testInternalKey}
	router := NewRouter(New(fs, cfg, client.New("", "", "", testInternalKey)), cfg)
	rec := do(router, http.MethodPost, "/parent/service-requests", token(t, testUserMaker, testSchoolA, common.RoleParent), nil,
		`{"category_id":"cat-1","student_user_id":"88888888-8888-8888-8888-888888888888","subject":"s","description":"d"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d %s", rec.Code, rec.Body.String())
	}
}
