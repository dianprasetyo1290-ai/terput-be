package payroll

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/hr/internal/common"
)

// QA-XSVC (CORE §9): a failed payroll journal posting is queued in core's
// exception queue as kind journal_posting (critical), one item per journal.
func TestReportJournalFailure(t *testing.T) {
	var got map[string]any
	var key, internal, corr string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/exceptions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		key, internal, corr = r.Header.Get("Idempotency-Key"), r.Header.Get("X-Internal-Key"), r.Header.Get(common.CorrelationHeader)
		w.WriteHeader(http.StatusCreated)
	}))
	defer core.Close()

	s := &Store{Reporter: common.NewExceptionReporter(core.URL, "k", "hr")}
	ctx := common.WithCorrelationID(context.Background(), "payroll-run-7")
	s.reportJournalFailure(ctx, "10000000-0000-0000-0000-000000000001", "j-1", "payroll:r-1:accrual", "2026-09",
		errors.New("periode akuntansi sudah ditutup"))

	if got == nil {
		t.Fatal("no report sent")
	}
	if got["kind"] != "journal_posting" || got["severity"] != "critical" || got["source_service"] != "hr" ||
		got["school_id"] != "10000000-0000-0000-0000-000000000001" {
		t.Fatalf("report = %v", got)
	}
	if key != "journal_posting:hr:j-1" || internal != "k" || corr != "payroll-run-7" || got["correlation_id"] != "payroll-run-7" {
		t.Fatalf("headers key=%q internal=%q corr=%q body corr=%v", key, internal, corr, got["correlation_id"])
	}
	if sum, _ := got["payload_summary"].(map[string]any); sum["journal_id"] != "j-1" || sum["period"] != "2026-09" {
		t.Fatalf("summary = %v", got["payload_summary"])
	}

	// No reporter configured: nothing is sent and nothing panics.
	(&Store{}).reportJournalFailure(ctx, "s", "j-2", "r", "p", errors.New("x"))
}
