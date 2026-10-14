package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"skci/core/internal/model"
)

func seedException(t *testing.T, fs *fakeStore, kind, source string, retryURL *string) *model.IntegrationException {
	t.Helper()
	exc, _, err := fs.UpsertException(context.Background(), model.IntegrationException{
		SchoolID: testSchoolA, SourceService: source, Kind: kind, Severity: model.SeverityNormal,
		CorrelationID: "corr-" + kind, ErrorMessage: "gagal", PayloadSummary: map[string]any{"x": 1},
		RetryURL: retryURL, IdempotencyKey: "seed:" + kind + ":" + source, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return exc
}

// BUG-114 / FR-80-005: admin (Finance Admin) does not see journal postings.
func TestListExceptions_AdminCannotSeeJournalPosting(t *testing.T) {
	fs, router, _ := newTestServer(t)
	seedException(t, fs, model.ExceptionKindJournalPosting, "billing", nil)
	seedException(t, fs, model.ExceptionKindWebhook, "billing", nil)

	tok := token(t, testUserAdmin, testSchoolA, "admin")
	rec := do(router, http.MethodGet, "/exceptions", tok, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct{ Data []model.IntegrationException }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	for _, e := range resp.Data {
		if e.Kind == model.ExceptionKindJournalPosting {
			t.Fatal("admin must not see journal_posting items")
		}
	}
	if len(resp.Data) != 1 {
		t.Fatalf("want only the webhook item, got %d", len(resp.Data))
	}
	if rec := do(router, http.MethodGet, "/exceptions?kind=journal_posting", tok, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("explicit journal_posting filter: want 403, got %d", rec.Code)
	}
}

// Acting on a kind outside the role's matrix is refused (assign/resolve/retry).
func TestExceptionActions_KindAccess(t *testing.T) {
	fs, router, _ := newTestServer(t)
	journal := seedException(t, fs, model.ExceptionKindJournalPosting, "billing", nil)
	tests := []struct {
		name, role, path, body string
		want                   int
	}{
		{"admin resolve jurnal ditolak", "admin", "/exceptions/" + journal.ID + "/resolve", `{"reason":"x"}`, http.StatusForbidden},
		{"admin assign jurnal ditolak", "admin", "/exceptions/" + journal.ID + "/assign", `{"user_id":"` + testUserAdmin + `"}`, http.StatusForbidden},
		{"guru resolve ditolak", "guru", "/exceptions/" + journal.ID + "/resolve", `{"reason":"x"}`, http.StatusForbidden},
		{"auditor resolve ditolak", "auditor", "/exceptions/" + journal.ID + "/resolve", `{"reason":"x"}`, http.StatusForbidden},
		{"payroll officer item billing ditolak", "payroll_officer", "/exceptions/" + journal.ID + "/resolve", `{"reason":"x"}`, http.StatusForbidden},
		{"finance manager resolve jurnal diterima", "finance_manager", "/exceptions/" + journal.ID + "/resolve", `{"reason":"sudah diposting manual"}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodPost, tt.path, token(t, testUserFinance, testSchoolA, tt.role), nil, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("want %d, got %d (%s)", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// BUG-167: two concurrent retries call the downstream once; the other gets 409.
func TestRetryException_ConcurrentClicksLocked(t *testing.T) {
	fs, router, _ := newTestServer(t)
	var calls int32
	release := make(chan struct{})
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer downstream.Close()
	exc := seedException(t, fs, model.ExceptionKindWebhook, "billing", &downstream.URL)
	tok := token(t, testUserAdmin, testSchoolA, "admin")

	first := make(chan int)
	go func() {
		first <- do(router, http.MethodPost, "/exceptions/"+exc.ID+"/retry", tok, nil, "").Code
	}()
	// wait until the first retry holds the lock
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&calls) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	second := do(router, http.MethodPost, "/exceptions/"+exc.ID+"/retry", tok, nil, "")
	close(release)
	if code := <-first; code != http.StatusOK {
		t.Fatalf("first retry: want 200, got %d", code)
	}
	if second.Code != http.StatusConflict {
		t.Fatalf("second retry: want 409, got %d (%s)", second.Code, second.Body.String())
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("downstream called %d times, want 1", n)
	}
}

// A failed retry releases the lock (item back to open) so it can be retried.
func TestRetryException_FailureReopens(t *testing.T) {
	fs, router, _ := newTestServer(t)
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer downstream.Close()
	exc := seedException(t, fs, model.ExceptionKindWebhook, "billing", &downstream.URL)
	tok := token(t, testUserAdmin, testSchoolA, "admin")
	for i := 0; i < 2; i++ {
		if rec := do(router, http.MethodPost, "/exceptions/"+exc.ID+"/retry", tok, nil, ""); rec.Code != http.StatusBadGateway {
			t.Fatalf("retry %d: want 502, got %d", i, rec.Code)
		}
	}
	got, _ := fs.GetExceptionByID(context.Background(), testSchoolA, exc.ID)
	if got.Status != model.ExceptionStatusOpen || got.Attempts != 3 {
		t.Fatalf("want open with 3 attempts, got %s/%d", got.Status, got.Attempts)
	}
}

// BUG-115 / FR-80-006: a new critical item notifies the roles responsible for
// its kind (journal posting -> finance_manager, not admin); assignment
// notifies the assignee.
func TestCriticalException_NotifiesResponsibleRoles(t *testing.T) {
	_, router, h, notifier := newWiredServer(t, map[string][]string{
		testUserFinance: {"finance_manager"},
		testUserAdmin:   {"admin"},
	})
	body := `{"school_id":"` + testSchoolA + `","source_service":"billing","kind":"journal_posting","severity":"critical",
		"correlation_id":"c-1","error_message":"accounting down","payload_summary":{"bill":"b1"},"max_attempts":3,"idempotency_key":"jp:b1"}`
	rec := do(router, http.MethodPost, "/internal/exceptions", "", map[string]string{"X-Internal-Key": testInternalKey}, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct{ Data model.IntegrationException }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	h.Wait()
	got := notifier.recipients("integration_exception")
	if !slices.Contains(got, testUserFinance) || slices.Contains(got, testUserAdmin) {
		t.Fatalf("critical journal_posting recipients = %v, want finance manager only", got)
	}

	tok := token(t, testUserFinance, testSchoolA, "finance_manager")
	if rec := do(router, http.MethodPost, "/exceptions/"+resp.Data.ID+"/assign", tok, nil, `{"user_id":"`+testUserKepala+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body.String())
	}
	h.Wait()
	if got := notifier.recipients("integration_exception"); !slices.Contains(got, testUserKepala) {
		t.Fatalf("assignee not notified: %v", got)
	}
}
