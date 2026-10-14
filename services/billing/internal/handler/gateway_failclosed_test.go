package handler

import (
	"net/http"
	"strings"
	"testing"

	"skci/billing/internal/gateway"
)

// postWebhook sends body to /webhooks/<provider> with optional signature.
func (f *extFixture) postWebhook(provider, body, signature string) int {
	f.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/webhooks/"+provider, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", "2026-10-14T10:00:00+07:00")
	if signature != "" {
		req.Header.Set("X-SIGNATURE", signature)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// QA-PAY BUG-004 (TC-IF-001/005): a webhook is never trusted without a
// verifier. NICEPay without credentials is not replaced by the simulator,
// the simulator only runs when explicitly enabled, and every rejection is
// logged while the transaction stays pending.
func TestWebhookFailClosed(t *testing.T) {
	f := newExtFixture(t)
	f.app.PaymentFeatures = nil
	m := f.method("va_fc", gateway.CapVA, "BNIN")

	// 1. NICEPay without credentials, forged merchantToken: 503, pending.
	ref := "NICE-" + genUUID()[:8]
	tx := f.pendingTx(gateway.DefaultProvider, ref, PurposeBills, "", m)
	body := `{"originalReferenceNo":"` + ref + `","trxId":"` + ref + `","latestTransactionStatus":"00","merchantToken":"palsu"}`
	if c := f.postWebhook(gateway.DefaultProvider, body, ""); c != http.StatusServiceUnavailable {
		t.Fatalf("nicepay without credentials status=%d want 503", c)
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, tx); st != "pending" {
		t.Fatalf("tx=%s want pending", st)
	}
	if r := f.scalar(`SELECT result || ':' || signature_valid::text FROM gateway_notifications WHERE provider = 'nicepay' AND provider_ref = $1`, ref); r != "provider_unavailable:false" {
		t.Fatalf("notification log=%q", r)
	}
	if n := f.core.count("/internal/audit", "webhook_provider_unavailable"); n < 1 {
		t.Fatal("rejection must reach the security audit log")
	}

	// 2. A sim provider row while the simulator is not enabled: 503.
	simCode := uniqueCode("s")
	f.exec(`INSERT INTO payment_providers (code, adapter, label, enabled) VALUES ($1, 'sim', $1, true)`, simCode)
	f.exec(`INSERT INTO payment_provider_capabilities (provider_code, capability) VALUES ($1, 'va')`, simCode)
	simRef := "SIM-" + genUUID()[:8]
	simTx := f.pendingTx(simCode, simRef, PurposeBills, "", m)
	simBody := `{"originalReferenceNo":"` + simRef + `","trxId":"` + simRef + `","latestTransactionStatus":"00"}`
	if c := f.postWebhook(simCode, simBody, gateway.SimSignature(testSimSecret, []byte(simBody))); c != http.StatusServiceUnavailable {
		t.Fatalf("disabled simulator status=%d want 503", c)
	}
	if f.app.credentialsInstalled(gateway.AdapterSim, "") {
		t.Fatal("disabled simulator must not report installed credentials")
	}

	// 3. Simulator explicitly enabled: unsigned/forged rejected (401), the
	// genuine signed copy of the same body is still processed afterwards.
	f.app.Config.Gateway.SimulatorEnabled = true
	f.app.Config.Gateway.SimWebhookSecret = testSimSecret
	for _, sig := range []string{"", "merchantToken-palsu"} {
		if c := f.postWebhook(simCode, simBody, sig); c != http.StatusUnauthorized {
			t.Fatalf("sig %q status=%d want 401", sig, c)
		}
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, simTx); st != "pending" {
		t.Fatalf("tx after forged webhook=%s want pending", st)
	}
	if c := f.postWebhook(simCode, simBody, gateway.SimSignature(testSimSecret, []byte(simBody))); c != http.StatusOK {
		t.Fatalf("signed webhook status=%d want 200", c)
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, simTx); st != "success" {
		t.Fatalf("tx after signed webhook=%s want success", st)
	}

	// 4. Production never runs the simulator, even with the flag set.
	f.app.Config.AppEnv = "production"
	if f.app.Config.SimulatorAllowed() {
		t.Fatal("simulator allowed in production")
	}
	if _, err := f.app.paymentProvider(t.Context(), simCode); err == nil {
		t.Fatal("sim adapter built in production")
	}
}
