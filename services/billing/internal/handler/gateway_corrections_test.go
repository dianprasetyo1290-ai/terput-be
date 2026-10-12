package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
)

// TestCorrectionApprovalViaCore: the approval goes to core with a callback on
// BILLING_INTERNAL_URL (not localhost), and the core callback is idempotent.
func TestCorrectionApprovalViaCore(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID, cashierPaymentID, studentID, financeID := genUUID(), genUUID(), genUUID(), genUUID()
	approvalID := genUUID()

	var sent map[string]any
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"` + approvalID + `","status":"pending","current_seq":1}}`))
	}))
	defer core.Close()

	app, srv := setupTestApp(t, db, genUUID(), studentID)
	defer srv.Close()
	app.Core = client.New(core.URL, "k")
	app.Config.Gateway.BillingInternalURL = "http://billing:8090"

	if _, err := db.Exec(`INSERT INTO cashier_payments (id, school_id, student_user_id, method, amount_idr, received_by, note, status, idempotency_key)
		VALUES ($1, $2, $3, 'cash', 100000, $4, 'Uang kasir', 'recorded', $5)`,
		cashierPaymentID, schoolID, studentID, financeID, "idem-"+cashierPaymentID); err != nil {
		t.Fatalf("insert cashier payment: %v", err)
	}

	payload, _ := json.Marshal(map[string]any{"source": "cashier", "source_id": cashierPaymentID, "kind": "cancel", "reason": "Salah input"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/payment-corrections", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+signToken(t, financeID, schoolID, common.RoleFinanceManager))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("correction request: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("correction status=%d body=%s", resp.StatusCode, body)
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &out)
	corrID := out.Data.ID

	cb, _ := sent["callback_url"].(string)
	if cb != "http://billing:8090/internal/corrections/"+corrID+"/callback" || strings.Contains(cb, "localhost") {
		t.Fatalf("callback_url = %q", cb)
	}

	// Core's real callback payload, delivered twice.
	cbBody := `{"approval_id":"` + approvalID + `","source_id":"` + corrID + `","request_type":"payment_correction","decision":"approved","decided_by":"` + financeID + `","comment":"","decided_at":"2026-10-12T08:00:00+07:00"}`
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest(http.MethodPost, "/internal/corrections/"+corrID+"/callback", strings.NewReader(cbBody))
		r = withURLParam(r, "id", corrID)
		rr := httptest.NewRecorder()
		app.CorrectionCallback(rr, r)
		if rr.Code != http.StatusOK {
			t.Fatalf("callback #%d = %d %s", i+1, rr.Code, rr.Body.String())
		}
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM payment_corrections WHERE id = $1`, corrID).Scan(&status)
	if status != "approved" {
		t.Fatalf("correction status = %s, want approved", status)
	}
	// A late "rejected" must not overwrite the final state.
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Replace(cbBody, "approved", "rejected", 1)))
	rr := httptest.NewRecorder()
	app.CorrectionCallback(rr, withURLParam(r, "id", corrID))
	_ = db.QueryRow(`SELECT status FROM payment_corrections WHERE id = $1`, corrID).Scan(&status)
	if rr.Code != http.StatusOK || status != "approved" {
		t.Fatalf("late reject: code=%d status=%s", rr.Code, status)
	}
}

func withURLParam(r *http.Request, key, value string) *http.Request {
	rc := chi.NewRouteContext()
	rc.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}
