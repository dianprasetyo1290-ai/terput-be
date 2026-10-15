package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"skci/identity/internal/common"
)

// QA-XSVC (CORE §9): a failed import is queued in core as kind import, keyed
// per kind/unit/file so a retry of the same file bumps one item, and carries
// the request's correlation id.
func TestReportImportFailure(t *testing.T) {
	type sent struct {
		body      map[string]any
		key, corr string
		internal  string
	}
	got := make(chan sent, 1)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var b map[string]any
		_ = json.Unmarshal(raw, &b)
		got <- sent{b, r.Header.Get("Idempotency-Key"), r.Header.Get(common.CorrelationHeader), r.Header.Get("X-Internal-Key")}
		w.WriteHeader(http.StatusCreated)
	}))
	defer core.Close()

	a := &App{Exceptions: common.NewExceptionReporter(core.URL, "k", "identity")}
	school := "10000000-0000-0000-0000-000000000001"
	// Run through the router middleware so the correlation id is the one a
	// real request would carry.
	h := common.CorrelationMiddleware(StampCorrelationHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(common.CorrelationHeader) != "imp-42" {
			t.Errorf("header not stamped: %q", r.Header.Get(common.CorrelationHeader))
		}
		a.reportImportFailure(r, "orang_tua", school, "wali.xlsx", "Impor orang tua gagal disimpan: deadlock", map[string]any{"rows": 3})
	})))
	req := httptest.NewRequest(http.MethodPost, "/users/import/orang_tua", nil)
	req.Header.Set(common.CorrelationHeader, "imp-42")
	h.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case s := <-got:
		if s.body["kind"] != "import" || s.body["source_service"] != "identity" || s.body["school_id"] != school {
			t.Fatalf("report = %v", s.body)
		}
		if s.key != "import:identity:orang_tua:"+school+":wali.xlsx" || s.internal != "k" || s.corr != "imp-42" {
			t.Fatalf("key=%q internal=%q corr=%q", s.key, s.internal, s.corr)
		}
		if sum, _ := s.body["payload_summary"].(map[string]any); sum["file"] != "wali.xlsx" || sum["rows"] != float64(3) {
			t.Fatalf("summary = %v", s.body["payload_summary"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no report reached core")
	}

	// Without a reporter nothing is sent and nothing panics.
	(&App{}).reportImportFailure(req, "users", school, "x.csv", "gagal", nil)
}

func TestStampCorrelationHeaderMintsID(t *testing.T) {
	var seen string
	h := common.CorrelationMiddleware(StampCorrelationHeader(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get(common.CorrelationHeader)
	})))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if seen == "" || rec.Header().Get(common.CorrelationHeader) != seen {
		t.Fatalf("request id %q, response id %q", seen, rec.Header().Get(common.CorrelationHeader))
	}
}
