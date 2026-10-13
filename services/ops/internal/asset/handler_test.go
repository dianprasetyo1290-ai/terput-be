package asset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type FakeAccountingServer struct {
	mu           sync.Mutex
	registered   map[string]map[string]any
	transfers    map[string]map[string]any
	disposals    map[string]map[string]any
	depreciations []string
}

func NewFakeAccountingServer() *FakeAccountingServer {
	return &FakeAccountingServer{
		registered: make(map[string]map[string]any),
		transfers:  make(map[string]map[string]any),
		disposals:  make(map[string]map[string]any),
	}
}

func (f *FakeAccountingServer) Server() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/internal/assets", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()

		if r.Method == http.MethodPost {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			ref, _ := body["reference"].(string)
			f.registered[ref] = body
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"asset_ref": body["asset_ref"],
				"trans_no":  "TRX-ACQ-001",
			})
			return
		}

		if r.Method == http.MethodGet {
			// Query book values
			var list []BookValueDTO
			list = append(list, BookValueDTO{
				AssetRef:       "ast-001",
				CostIDR:        12000000,
				AccumulatedIDR: 2000000,
				BookValueIDR:   10000000,
				LastPeriod:     "2026-10",
				Schedule: []DepreciationScheduleItem{
					{Period: "2026-11", AmountIDR: 250000},
				},
			})
			_ = json.NewEncoder(w).Encode(list)
			return
		}
	})

	mux.HandleFunc("/api/internal/assets/depreciate", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		period, _ := body["period"].(string)
		f.depreciations = append(f.depreciations, period)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"assets":    5,
			"total_idr": 1250000,
			"trans_no":  "TRX-DEP-001",
		})
	})

	return httptest.NewServer(mux)
}

func TestFakeAccountingIntegration(t *testing.T) {
	fake := NewFakeAccountingServer()
	srv := fake.Server()
	defer srv.Close()

	client := NewAccountingClient(srv.URL, "test-key")

	// 1. Test RegisterAsset (AC-107-01)
	payload := map[string]any{
		"school_id":  "sch-001",
		"asset_ref":  "ast-001",
		"reference":  "asset:acq:ast-001",
		"code":       "LPT-2026-0001",
		"name":       "Laptop",
		"cost_idr":   int64(12000000),
		"post_journal": false,
	}

	transNo, err := client.RegisterAsset(context.Background(), payload)
	if err != nil {
		t.Fatalf("RegisterAsset failed: %v", err)
	}
	if transNo != "TRX-ACQ-001" {
		t.Errorf("expected trans_no TRX-ACQ-001, got %s", transNo)
	}

	fake.mu.Lock()
	if _, ok := fake.registered["asset:acq:ast-001"]; !ok {
		t.Errorf("expected asset:acq:ast-001 in registered map")
	}
	fake.mu.Unlock()

	// 2. Test Depreciate (BR-AST-01)
	assets, total, depTrans, err := client.Depreciate(context.Background(), "sch-001", "2026-10")
	if err != nil {
		t.Fatalf("Depreciate failed: %v", err)
	}
	if assets != 5 || total != 1250000 || depTrans != "TRX-DEP-001" {
		t.Errorf("unexpected dep result: %d, %d, %s", assets, total, depTrans)
	}

	// 3. Test GetBookValues
	bvs, err := client.GetBookValues(context.Background(), "sch-001", []string{"ast-001"})
	if err != nil {
		t.Fatalf("GetBookValues failed: %v", err)
	}
	if len(bvs) == 0 || bvs[0].BookValueIDR != 10000000 {
		t.Errorf("unexpected book values: %v", bvs)
	}
}
