package procurement

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"skci/ops/internal/common"
)

type fakeCoreExceptions struct {
	mu   sync.Mutex
	got  []map[string]any
	keys []string
}

func (f *fakeCoreExceptions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/internal/exceptions" || r.Header.Get("X-Internal-Key") != "k" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.got = append(f.got, body)
	f.keys = append(f.keys, r.Header.Get("Idempotency-Key"))
	f.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

// QA-XSVC (CORE §9): a vendor sync that exhausts its retries, by an
// accounting error or by the network, is marked failed and queued in core as
// a dead letter through the shared ExceptionReporter.
func TestVendorSyncDeadLetter(t *testing.T) {
	db, _, cleanup := setupTestDB(t)
	defer cleanup()

	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer failing.Close()
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close() // connection refused

	for _, tc := range []struct{ name, accountingURL string }{
		{"accounting 500", failing.URL},
		{"jaringan putus", downURL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := &fakeCoreExceptions{}
			coreSrv := httptest.NewServer(core)
			defer coreSrv.Close()

			school := "11111111-1111-1111-1111-111111111111"
			var vendorID string
			if err := db.QueryRow(`INSERT INTO vendors (school_id, code, name) VALUES ($1, gen_random_uuid()::text, 'Vendor Uji') RETURNING id`, school).Scan(&vendorID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO vendor_accounting_sync (vendor_id, school_id, payload_hash, attempts) VALUES ($1, $2, 'h', $3)`,
				vendorID, school, vendorSyncMaxAttempts-1); err != nil {
				t.Fatal(err)
			}
			job := NewSyncJob(db, nil, tc.accountingURL, "k")
			job.Reporter = common.NewExceptionReporter(coreSrv.URL, "k", "ops")
			if err := job.RunSyncVendors(context.Background()); err != nil {
				t.Fatal(err)
			}

			var status string
			var attempts int
			if err := db.QueryRow(`SELECT status, attempts FROM vendor_accounting_sync WHERE vendor_id=$1`, vendorID).Scan(&status, &attempts); err != nil {
				t.Fatal(err)
			}
			if status != "failed" || attempts != vendorSyncMaxAttempts {
				t.Fatalf("row = %s/%d, want failed/%d", status, attempts, vendorSyncMaxAttempts)
			}
			core.mu.Lock()
			defer core.mu.Unlock()
			var mine map[string]any
			var key string
			for i, b := range core.got {
				if s, _ := b["payload_summary"].(map[string]any); s["vendor_id"] == vendorID {
					mine, key = b, core.keys[i]
				}
			}
			if mine == nil {
				t.Fatalf("no dead-letter report for %s: %v", vendorID, core.got)
			}
			if mine["kind"] != "dead_letter" || mine["source_service"] != "ops" || mine["severity"] != "critical" ||
				key != "dead_letter:ops:vendor_accounting_sync:"+vendorID {
				t.Fatalf("report = %v key %q", mine, key)
			}
		})
	}
}
