package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"skci/core/internal/model"
)

const testUserYayasan = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

// identityCallbacks stands in for identity's
// POST /internal/role-conflict-exceptions/approval-callback.
type identityCallbacks struct {
	mu   sync.Mutex
	got  []map[string]any
	keys []string
}

func (f *identityCallbacks) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/internal/role-conflict-exceptions/approval-callback" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Header.Get("X-Internal-Key") != testInternalKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.got = append(f.got, body)
	f.keys = append(f.keys, r.Header.Get("Idempotency-Key"))
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"data":{"status":"` + body["decision"].(string) + `"}}`))
}

func (f *identityCallbacks) last() (map[string]any, string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.got) == 0 {
		return nil, "", 0
	}
	return f.got[len(f.got)-1], f.keys[len(f.keys)-1], len(f.got)
}

// submitRoleConflict posts the body identity's submitApproval sends
// (services/identity/internal/handler/sod_exceptions.go).
func submitRoleConflict(router http.Handler, source, exceptionID, callbackBase string) *httpResult {
	body := map[string]any{
		"school_id":      testSchoolA,
		"request_type":   "role_conflict_exception",
		"source_service": source,
		"source_id":      exceptionID,
		"title":          "Pengecualian SoD: Budi mendapat peran finance_manager",
		"summary": map[string]any{
			"user": "Budi", "email": "budi@example.sch.id", "role": "finance_manager",
			"conflicting_roles": []string{"auditor"}, "reason": "Unit kecil, belum ada pegawai lain", "valid_until": nil,
		},
		"maker_user_id":   testUserAdmin,
		"subject_user_id": testUserMaker,
		"callback_url":    callbackBase + "/internal/role-conflict-exceptions/approval-callback",
		"correlation_id":  exceptionID,
		"idempotency_key": "role-conflict-exception:" + exceptionID,
	}
	raw, _ := json.Marshal(body)
	rec := do(router, http.MethodPost, "/internal/approvals", "", map[string]string{"X-Internal-Key": testInternalKey}, string(raw))
	var resp struct {
		Data struct{ ID string } `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return &httpResult{code: rec.Code, body: rec.Body.String(), id: resp.Data.ID}
}

// QA-XSVC: identity's SoD exception goes through the Approval Center and the
// decision (approved or rejected) is called back to identity with
// X-Internal-Key and the QA-IDN §9 payload.
func TestRoleConflictExceptionApproval(t *testing.T) {
	cb := &identityCallbacks{}
	cbSrv := httptest.NewServer(cb)
	t.Cleanup(cbSrv.Close)

	fs, router, h, _ := newWiredServer(t, map[string][]string{
		testUserAdmin:   {"admin"},
		testUserYayasan: {"kepala_yayasan"},
	})
	if _, err := fs.CreateFlow(context.Background(), testSchoolA, testUserAdmin, model.ApprovalFlow{
		RequestType: model.ReqTypeRoleConflictException, Name: "Pengecualian SoD", Active: true,
		Steps: []model.FlowStep{{Seq: 1, ApproverKind: model.ApproverKindRole, ApproverRole: ptr("kepala_yayasan"),
			SLAHours: ptr(48), EscalateToRole: ptr("superadmin")}},
	}); err != nil {
		t.Fatal(err)
	}
	yayasan := token(t, testUserYayasan, testSchoolA, "kepala_yayasan")

	t.Run("jenis terdaftar dengan label", func(t *testing.T) {
		rec := do(router, http.MethodGet, "/approval-request-types", yayasan, nil, "")
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role_conflict_exception"`) ||
			!strings.Contains(rec.Body.String(), "Pengecualian Konflik Peran") {
			t.Fatalf("request types: %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("hanya identity yang boleh mengajukan", func(t *testing.T) {
		res := submitRoleConflict(router, "billing", "c0000000-0000-0000-0000-000000000001", cbSrv.URL)
		if res.code != http.StatusUnprocessableEntity || !strings.Contains(res.body, "identity") {
			t.Fatalf("want 422 for non-identity source, got %d %s", res.code, res.body)
		}
	})

	t.Run("disetujui: callback approved", func(t *testing.T) {
		exID := "c0000000-0000-0000-0000-000000000002"
		res := submitRoleConflict(router, "identity", exID, cbSrv.URL)
		if res.code != http.StatusCreated {
			t.Fatalf("submit: %d %s", res.code, res.body)
		}
		// Replay of identity's retry returns the same request.
		if again := submitRoleConflict(router, "identity", exID, cbSrv.URL); again.code != http.StatusOK || again.id != res.id {
			t.Fatalf("replay: %d %s", again.code, again.body)
		}
		h.Wait()
		// The admin who asked cannot decide it.
		admin := token(t, testUserAdmin, testSchoolA, "admin")
		if rec := do(router, http.MethodPost, "/approvals/"+res.id+"/approve", admin, nil, `{"comment":"ok"}`); rec.Code == http.StatusOK {
			t.Fatalf("maker approved own request")
		}
		if rec := do(router, http.MethodPost, "/approvals/"+res.id+"/approve", yayasan, nil, `{"comment":"Disetujui sementara"}`); rec.Code != http.StatusOK {
			t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
		}
		h.Wait()
		body, key, n := cb.last()
		if n != 1 {
			t.Fatalf("callbacks = %d, want 1", n)
		}
		want := map[string]any{"approval_id": res.id, "source_id": exID, "request_type": "role_conflict_exception",
			"decision": "approved", "decided_by": testUserYayasan, "comment": "Disetujui sementara"}
		for k, v := range want {
			if body[k] != v {
				t.Errorf("callback %s = %v, want %v", k, body[k], v)
			}
		}
		if key != "approval:"+res.id {
			t.Errorf("Idempotency-Key = %q", key)
		}
		got, err := fs.GetRequestByID(context.Background(), testSchoolA, res.id)
		if err != nil || got.CallbackStatus != "done" {
			t.Fatalf("callback status = %+v, %v", got, err)
		}
	})

	t.Run("ditolak: callback rejected dengan komentar", func(t *testing.T) {
		exID := "c0000000-0000-0000-0000-000000000003"
		res := submitRoleConflict(router, "identity", exID, cbSrv.URL)
		if res.code != http.StatusCreated {
			t.Fatalf("submit: %d %s", res.code, res.body)
		}
		h.Wait()
		if rec := do(router, http.MethodPost, "/approvals/"+res.id+"/reject", yayasan, nil, `{"comment":"Konflik terlalu berisiko"}`); rec.Code != http.StatusOK {
			t.Fatalf("reject: %d %s", rec.Code, rec.Body.String())
		}
		h.Wait()
		body, _, n := cb.last()
		if n != 2 || body["decision"] != "rejected" || body["source_id"] != exID || body["comment"] != "Konflik terlalu berisiko" {
			t.Fatalf("callback #%d = %v", n, body)
		}
	})
}
