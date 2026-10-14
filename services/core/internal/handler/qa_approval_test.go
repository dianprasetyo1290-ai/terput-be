package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"skci/core/internal/model"
	"skci/core/internal/store"
)

const testUserFinance2 = "77777777-7777-7777-7777-777777777777"

func seedEscalatingFlow(t *testing.T, fs *fakeStore, escalate *string) {
	t.Helper()
	if _, err := fs.CreateFlow(context.Background(), testSchoolA, testUserAdmin, model.ApprovalFlow{
		RequestType: model.ReqTypeRefund, Name: "Refund", Active: true,
		Steps: []model.FlowStep{{Seq: 1, ApproverKind: model.ApproverKindRole, ApproverRole: ptr("finance_manager"),
			SLAHours: ptr(24), EscalateToRole: escalate}},
	}); err != nil {
		t.Fatal(err)
	}
}

func createRefund(router http.Handler, maker, key string) *httpResult {
	body := `{"school_id":"` + testSchoolA + `","request_type":"refund","source_service":"billing","source_id":"` + key + `",
		"title":"Refund","summary":{},"amount_idr":1000000,"maker_user_id":"` + maker + `",
		"callback_url":"http://localhost/cb","correlation_id":"c-` + key + `","idempotency_key":"refund:` + key + `"}`
	rec := do(router, http.MethodPost, "/internal/approvals", "", map[string]string{"X-Internal-Key": testInternalKey}, body)
	var resp struct {
		Data struct{ ID string } `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return &httpResult{code: rec.Code, body: rec.Body.String(), id: resp.Data.ID}
}

type httpResult struct {
	code int
	body string
	id   string
}

// BUG-028: approvers of the current step are notified (maker excluded), the
// maker is notified of the final result.
func TestApprovalNotifications(t *testing.T) {
	fs, router, h, notifier := newWiredServer(t, map[string][]string{
		testUserFinance:  {"finance_manager"},
		testUserFinance2: {"finance_manager"},
	})
	seedEscalatingFlow(t, fs, nil)
	res := createRefund(router, testUserFinance, "n1")
	if res.code != http.StatusCreated {
		t.Fatalf("create: %d %s", res.code, res.body)
	}
	h.Wait()
	got := notifier.recipients("approval")
	if !slices.Contains(got, testUserFinance2) || slices.Contains(got, testUserFinance) {
		t.Fatalf("step recipients = %v, want the other finance manager only", got)
	}

	tok := token(t, testUserFinance2, testSchoolA, "finance_manager")
	if rec := do(router, http.MethodPost, "/approvals/"+res.id+"/approve", tok, nil, `{"comment":"ok"}`); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	h.Wait()
	if got := notifier.recipients("approval"); !slices.Contains(got, testUserFinance) {
		t.Fatalf("maker not notified of the result: %v", got)
	}
}

// BUG-028: a failed notification is not silent; it lands in the exception queue.
func TestApprovalNotificationFailure_RaisesException(t *testing.T) {
	fs, router, h, notifier := newWiredServer(t, map[string][]string{
		testUserFinance:  {"finance_manager"},
		testUserFinance2: {"finance_manager"},
	})
	notifier.fail = true
	seedEscalatingFlow(t, fs, nil)
	if res := createRefund(router, testUserFinance, "n2"); res.code != http.StatusCreated {
		t.Fatalf("create: %d %s", res.code, res.body)
	}
	h.Wait()
	items, _, _ := fs.ListExceptions(context.Background(), store.ExceptionFilter{SchoolID: testSchoolA, Kinds: model.ExceptionKinds}, 1, 50)
	if len(items) != 1 || items[0].Kind != model.ExceptionKindNotification {
		t.Fatalf("want one notification exception, got %+v", items)
	}
}

// BUG-111: when the maker is the only holder of the step's role the request
// goes to the escalation role, or is refused when none is configured.
func TestMakerOnlyApprover(t *testing.T) {
	roles := map[string][]string{testUserFinance: {"finance_manager"}, testUserKepala: {"kepala_sekolah"}}
	t.Run("tanpa peran eskalasi: 422", func(t *testing.T) {
		fs, router, _, _ := newWiredServer(t, roles)
		seedEscalatingFlow(t, fs, nil)
		res := createRefund(router, testUserFinance, "m1")
		if res.code != http.StatusUnprocessableEntity || !strings.Contains(res.body, "eskalasi") {
			t.Fatalf("want 422 mentioning eskalasi, got %d %s", res.code, res.body)
		}
	})
	t.Run("dengan peran eskalasi: dialihkan ke kepala sekolah", func(t *testing.T) {
		fs, router, h, notifier := newWiredServer(t, roles)
		seedEscalatingFlow(t, fs, ptr("kepala_sekolah"))
		res := createRefund(router, testUserFinance, "m2")
		if res.code != http.StatusCreated {
			t.Fatalf("create: %d %s", res.code, res.body)
		}
		h.Wait()
		if got := notifier.recipients("approval"); !slices.Equal(got, []string{testUserKepala}) {
			t.Fatalf("recipients = %v, want kepala sekolah", got)
		}
		tok := token(t, testUserKepala, testSchoolA, "kepala_sekolah")
		if rec := do(router, http.MethodPost, "/approvals/"+res.id+"/approve", tok, nil, `{"comment":"ok"}`); rec.Code != http.StatusOK {
			t.Fatalf("kepala approve: %d %s", rec.Code, rec.Body.String())
		}
	})
}

// ApprovalTimeline on source pages: GET /approvals/by-source returns the
// request (with steps) to people who may see it, 404 to others.
func TestGetApprovalBySource(t *testing.T) {
	fs, router, h, _ := newWiredServer(t, map[string][]string{
		testUserFinance:  {"finance_manager"},
		testUserFinance2: {"finance_manager"},
	})
	seedEscalatingFlow(t, fs, nil)
	res := createRefund(router, testUserFinance, "src-1")
	h.Wait()
	path := "/approvals/by-source?source_service=billing&source_id=src-1"
	rec := do(router, http.MethodGet, path, token(t, testUserFinance, testSchoolA, "finance_manager"), nil, "")
	var resp struct{ Data model.ApprovalRequest }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if rec.Code != http.StatusOK || resp.Data.ID != res.id || len(resp.Data.Steps) == 0 {
		t.Fatalf("maker: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(router, http.MethodGet, path, token(t, testUserKepala, testSchoolA, "guru"), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unrelated user: want 404, got %d", rec.Code)
	}
	if rec := do(router, http.MethodGet, "/approvals/by-source?source_service=billing", token(t, testUserFinance, testSchoolA, "finance_manager"), nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing source_id: want 400, got %d", rec.Code)
	}
}
