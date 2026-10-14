package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/client"
	"skci/academic/internal/common"
	"skci/academic/internal/model"
	"skci/academic/internal/store"
)

// fakeApprovalCenter stands in for core: /internal/approvals answers with
// submitStatus (201 = accepted), cancel with cancelStatus.
type fakeApprovalCenter struct {
	mu           sync.Mutex
	submitStatus int
	submitMsg    string
	cancelStatus int
	submits      int
	cancels      []string
	audits       []string
}

func (f *fakeApprovalCenter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/internal/approvals":
		f.submits++
		if f.submitStatus != http.StatusCreated {
			w.WriteHeader(f.submitStatus)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil, "message": f.submitMsg})
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": fmt.Sprintf("b0000000-0000-4000-8000-%012d", f.submits), "status": "pending", "current_seq": 1,
		}})
	case strings.HasPrefix(r.URL.Path, "/internal/approvals/") && strings.HasSuffix(r.URL.Path, "/cancel"):
		f.cancels = append(f.cancels, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/internal/approvals/"), "/cancel"))
		w.WriteHeader(f.cancelStatus)
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "conflict"})
	case r.URL.Path == "/internal/audit":
		var ev client.CoreAuditEvent
		_ = json.NewDecoder(r.Body).Decode(&ev)
		f.audits = append(f.audits, ev.Action)
		w.WriteHeader(http.StatusAccepted)
	default:
		http.NotFound(w, r)
	}
}

type menu78Fixture struct {
	t        *testing.T
	core     *fakeApprovalCenter
	h        *Handler
	router   http.Handler
	schoolID string
	student  string
	admin    string
}

func newMenu78Fixture(t *testing.T) *menu78Fixture {
	db := getAcademicTestDB(t)
	ctx := context.Background()
	f := &menu78Fixture{
		t:        t,
		core:     &fakeApprovalCenter{submitStatus: http.StatusCreated, cancelStatus: http.StatusOK},
		schoolID: "f7800000-0000-4000-8000-000000000001",
		student:  "f7800000-0000-4000-8000-000000000002",
		admin:    "f7800000-0000-4000-8000-000000000003",
	}
	yearID := "f7800000-0000-4000-8000-000000000004"
	classID := "f7800000-0000-4000-8000-000000000005"
	for _, q := range []string{
		`DELETE FROM student_status_requests WHERE school_id = $1`,
		`DELETE FROM class_enrollments WHERE school_id = $1`,
		`DELETE FROM classrooms WHERE school_id = $1`,
	} {
		if _, err := db.ExecContext(ctx, q, f.schoolID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO classrooms (id, school_id, academic_year_id, name, grade_level) VALUES ($1, $2, $3, 'VII A', '7')`, classID, f.schoolID, yearID); err != nil {
		t.Fatal(err)
	}
	// Entered the class on 2026-07-13 WIB (06:00 WIB = 2026-07-12 23:00 UTC).
	if _, err := db.ExecContext(ctx, `INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at) VALUES ($1, $2, $3, $4, 'active', '2026-07-13T06:00:00+07:00')`, f.schoolID, yearID, classID, f.student); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(f.core)
	t.Cleanup(srv.Close)
	f.h = &Handler{Store: store.New(db), Core: &client.CoreClient{
		BaseURL: srv.URL, InternalKey: "k", CallbackBaseURL: "http://academic", HTTPClient: srv.Client(),
	}}
	r := chi.NewRouter()
	r.Post("/student-status-requests", f.h.CreateStudentStatusRequest)
	r.Get("/student-status-requests", f.h.ListStudentStatusRequests)
	r.Get("/student-status-requests/{id}", f.h.GetStudentStatusRequest)
	r.Post("/student-status-requests/{id}/apply", f.h.ApplyStudentStatusRequest)
	r.Post("/student-status-requests/{id}/cancel", f.h.CancelStudentStatusRequest)
	r.Get("/student-status-requests/financial-summary", f.h.GetStudentFinancialSummaryProxy)
	r.Post("/internal/student-status-requests/approval-callback", f.h.ApprovalCallback)
	f.router = r
	return f
}

// do sends a request as (role, user); role "" sends no claims (internal call).
func (f *menu78Fixture) do(method, path, role, user string, body any) (int, map[string]any) {
	f.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if role != "" {
		req = req.WithContext(common.WithClaims(req.Context(), &common.Claims{UserID: user, ActiveRole: role, SchoolID: f.schoolID}))
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func (f *menu78Fixture) create(role, user, effective string) (int, map[string]any) {
	return f.do(http.MethodPost, "/student-status-requests", role, user, map[string]any{
		"student_user_id": f.student, "kind": model.KindWithdrawal,
		"effective_date": effective, "reason": "Pindah domisili",
	})
}

func (f *menu78Fixture) count() int {
	var n int
	_ = f.h.Store.DB.QueryRow(`SELECT COUNT(*) FROM student_status_requests WHERE school_id = $1`, f.schoolID).Scan(&n)
	return n
}

func dataField(env map[string]any, k string) string {
	d, _ := env["data"].(map[string]any)
	v, _ := d[k].(string)
	return v
}

// BUG-177 (TC-78-020): superadmin has no Mutasi Siswa access at the API.
func TestMenu78SuperadminForbidden(t *testing.T) {
	f := newMenu78Fixture(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/student-status-requests"},
		{http.MethodGet, "/student-status-requests/f7800000-0000-4000-8000-0000000000ff"},
		{http.MethodGet, "/student-status-requests/financial-summary?student_user_id=" + f.student},
		{http.MethodPost, "/student-status-requests/f7800000-0000-4000-8000-0000000000ff/apply"},
		{http.MethodPost, "/student-status-requests/f7800000-0000-4000-8000-0000000000ff/cancel"},
	} {
		if code, _ := f.do(c.method, c.path, common.RoleSuperadmin, f.admin, nil); code != http.StatusForbidden {
			t.Errorf("%s %s superadmin=%d want 403", c.method, c.path, code)
		}
	}
	if code, _ := f.create(common.RoleSuperadmin, f.admin, "2026-08-01"); code != http.StatusForbidden {
		t.Errorf("create superadmin=%d want 403", code)
	}
	if code, _ := f.create("kepala_sekolah", f.admin, "2026-08-01"); code != http.StatusForbidden {
		t.Errorf("create kepala_sekolah=%d want 403", code)
	}
	if code, _ := f.do(http.MethodGet, "/student-status-requests", "kepala_sekolah", f.admin, nil); code != http.StatusOK {
		t.Errorf("list kepala_sekolah=%d want 200", code)
	}
	if f.count() != 0 {
		t.Fatal("forbidden calls must not create rows")
	}
}

// BUG-057: a request the Approval Center refuses is not kept pending forever.
func TestMenu78SubmitFailClosed(t *testing.T) {
	f := newMenu78Fixture(t)

	f.core.submitStatus = http.StatusUnprocessableEntity
	f.core.submitMsg = "alur approval student_withdrawal belum dikonfigurasi"
	code, env := f.create(common.RoleAdmin, f.admin, "2026-08-01")
	if code != http.StatusUnprocessableEntity || !strings.Contains(env["message"].(string), "belum dikonfigurasi") ||
		!strings.Contains(env["message"].(string), "Alur Approval") {
		t.Fatalf("no flow: %d %v", code, env)
	}
	if f.count() != 0 {
		t.Fatal("refused request must not stay pending")
	}

	f.core.submitStatus = http.StatusInternalServerError
	if code, _ := f.create(common.RoleAdmin, f.admin, "2026-08-01"); code != http.StatusBadGateway {
		t.Fatalf("core 500: %d want 502", code)
	}
	f.core.submitStatus = http.StatusUnauthorized // wrong SEKOCI_INTERNAL_KEY
	if code, _ := f.create(common.RoleAdmin, f.admin, "2026-08-01"); code != http.StatusBadGateway {
		t.Fatalf("core 401: %d want 502", code)
	}
	f.h.Core.BaseURL = "http://127.0.0.1:1" // unreachable
	if code, _ := f.create(common.RoleAdmin, f.admin, "2026-08-01"); code != http.StatusBadGateway {
		t.Fatalf("core down: %d want 502", code)
	}
	if f.count() != 0 {
		t.Fatal("failed submits must not leave rows")
	}
	f.h.Core = nil
	if code, _ := f.create(common.RoleAdmin, f.admin, "2026-08-01"); code != http.StatusServiceUnavailable {
		t.Fatalf("no core client: %d want 503", code)
	}
}

// BUG-176 / BUG-137 / BUG-057: date guard, one active request, cancel.
func TestMenu78ActiveRequestLifecycle(t *testing.T) {
	f := newMenu78Fixture(t)

	// BUG-176: before the enrollment day (2026-07-13 WIB) is refused.
	if code, env := f.create(common.RoleAdmin, f.admin, "2026-07-12"); code != http.StatusUnprocessableEntity ||
		!strings.Contains(env["message"].(string), "2026-07-13") {
		t.Fatalf("effective before enrollment: %d %v", code, env)
	}

	code, env := f.create(common.RoleAdmin, f.admin, "2026-07-13")
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, env)
	}
	firstID := dataField(env, "id")
	if dataField(env, "approval_id") == "" || dataField(env, "status") != model.StatusPendingApproval {
		t.Fatalf("created request must carry the core approval id: %v", env)
	}

	// BUG-137: a second request for the same student is refused with a link.
	code, env = f.create(common.RoleAdmin, f.admin, "2026-08-01")
	if code != http.StatusConflict || dataField(env, "existing_request_id") != firstID ||
		dataField(env, "link") != "/akademik/mutasi-siswa/"+firstID {
		t.Fatalf("duplicate: %d %v", code, env)
	}

	// Cancel: only the requester, only while pending.
	other := "f7800000-0000-4000-8000-000000000009"
	if code, _ := f.do(http.MethodPost, "/student-status-requests/"+firstID+"/cancel", common.RoleAdmin, other, nil); code != http.StatusForbidden {
		t.Fatalf("cancel by other admin: %d want 403", code)
	}
	f.core.cancelStatus = http.StatusConflict // already decided in core
	if code, _ := f.do(http.MethodPost, "/student-status-requests/"+firstID+"/cancel", common.RoleAdmin, f.admin, nil); code != http.StatusConflict {
		t.Fatalf("cancel decided: %d want 409", code)
	}
	f.core.cancelStatus = http.StatusOK
	code, env = f.do(http.MethodPost, "/student-status-requests/"+firstID+"/cancel", common.RoleAdmin, f.admin, map[string]string{"reason": "salah input"})
	if code != http.StatusOK || dataField(env, "status") != model.StatusCancelled {
		t.Fatalf("cancel: %d %v", code, env)
	}
	if len(f.core.cancels) != 2 {
		t.Fatalf("core cancel calls=%v", f.core.cancels)
	}
	if code, _ := f.do(http.MethodPost, "/student-status-requests/"+firstID+"/cancel", common.RoleAdmin, f.admin, nil); code != http.StatusConflict {
		t.Fatalf("cancel twice: %d want 409", code)
	}
	found := false
	for _, a := range f.core.audits {
		found = found || a == "request_cancelled"
	}
	if !found {
		t.Fatalf("cancel must be audited: %v", f.core.audits)
	}

	// A late core decision on a cancelled request is refused.
	if code, _ := f.do(http.MethodPost, "/internal/student-status-requests/approval-callback", "", "", map[string]string{
		"source_id": firstID, "decision": "approved",
	}); code != http.StatusConflict {
		t.Fatalf("callback after cancel: %d want 409", code)
	}

	// After cancelling, a new request is accepted; a rejection frees it again.
	code, env = f.create(common.RoleAdmin, f.admin, "2099-01-01")
	if code != http.StatusCreated {
		t.Fatalf("create after cancel: %d %v", code, env)
	}
	secondID := dataField(env, "id")
	for i := 0; i < 2; i++ { // second call is an idempotent replay
		if code, _ := f.do(http.MethodPost, "/internal/student-status-requests/approval-callback", "", "", map[string]string{
			"source_id": secondID, "decision": "rejected",
		}); code != http.StatusOK {
			t.Fatalf("reject callback #%d: %d", i, code)
		}
	}
	if code, _ := f.do(http.MethodPost, "/internal/student-status-requests/approval-callback", "", "", map[string]string{
		"source_id": secondID, "decision": "approved",
	}); code != http.StatusConflict {
		t.Fatalf("approve after reject: %d want 409", code)
	}
	if code, env := f.create(common.RoleAdmin, f.admin, "2099-01-01"); code != http.StatusCreated {
		t.Fatalf("create after rejection: %d %v", code, env)
	}
}
