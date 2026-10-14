package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
	"skci/billing/internal/gateway"
)

// TestRenewOverduePending (QA-PAY round 2): the transaction page counts
// down to zero and offers "Perbarui"; there is no expiry sweeper, so the
// row is still 'pending'. Renew must close it and issue a new code instead
// of answering 400. A pending code that is still valid cannot be renewed.
func TestRenewOverduePending(t *testing.T) {
	f := newExtFixture(t)
	p := f.provider(&gateway.Sim{ProviderCode: uniqueCode("r")}, gateway.CapVA)
	f.rule(gateway.CapVA, p, "", false)
	va := f.method(uniqueCode("va"), gateway.CapVA, "CENA")
	f.exec(`INSERT INTO payment_method_provider_codes (method_id, provider_code, provider_method_code) VALUES ($1, $2, '014')`, va, p)

	code, data, raw := f.checkout(va, f.bill(2026, 7, time.Now(), 500000))
	if code != http.StatusOK {
		t.Fatalf("checkout %d %s", code, raw)
	}
	oldID, _ := data["id"].(string)

	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Post("/parent/transactions/{id}/renew", f.app.ParentRenewTransaction)
	srv := httptest.NewServer(r)
	defer srv.Close()
	renew := func() (int, string) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/parent/transactions/"+oldID+"/renew", nil)
		req.Header.Set("Authorization", "Bearer "+signToken(t, f.guardianID, f.schoolID, common.RoleParent))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	if st, body := renew(); st != http.StatusBadRequest {
		t.Fatalf("renew of a valid pending code: %d %s", st, body)
	}
	f.exec(`UPDATE gateway_transactions SET expires_at = now() - interval '1 minute' WHERE id = $1`, oldID)
	if st, body := renew(); st != http.StatusOK {
		t.Fatalf("renew of an overdue pending code: %d %s", st, body)
	}
	if s := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, oldID); s != "expired" {
		t.Fatalf("old transaction status=%s want expired", s)
	}
}
