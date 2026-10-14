package gateway

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"skci/billing/internal/model"
	"skci/billing/internal/nicepay"
)

func providersAB() map[string]ProviderInfo {
	return map[string]ProviderInfo{
		"nicepay": {Code: "nicepay", Enabled: true, Capabilities: []string{CapVA, CapQRIS, CapEWallet}},
		"a":       {Code: "a", Enabled: true, Capabilities: []string{CapQRIS, CapVA}},
		"b":       {Code: "b", Enabled: true, Capabilities: []string{CapVA, CapQRIS, CapRetail}},
		"off":     {Code: "off", Enabled: false, Capabilities: AllCapabilities},
	}
}

func TestRoute(t *testing.T) {
	ruleQRIS := Rule{ID: "r1", MethodKind: CapQRIS, Primary: "a"}
	ruleVA := Rule{ID: "r2", MethodKind: CapVA, Primary: "b"}
	tests := []struct {
		name      string
		rules     []Rule
		method    MethodRef
		providers map[string]ProviderInfo
		want      Decision
		wantErr   bool
	}{
		{"AC-150-01 VA goes to B", []Rule{ruleQRIS, ruleVA}, MethodRef{CapVA, "va_bca"}, providersAB(), Decision{RuleID: "r2", Primary: "b"}, false},
		{"AC-150-01 QRIS goes to A", []Rule{ruleQRIS, ruleVA}, MethodRef{CapQRIS, "qris"}, providersAB(), Decision{RuleID: "r1", Primary: "a"}, false},
		{"method code rule beats kind rule", []Rule{ruleVA, {ID: "r3", MethodKind: CapVA, MethodCode: "va_bni", Primary: "a"}}, MethodRef{CapVA, "va_bni"}, providersAB(), Decision{RuleID: "r3", Primary: "a"}, false},
		{"kind rule for other code", []Rule{ruleVA, {ID: "r3", MethodKind: CapVA, MethodCode: "va_bni", Primary: "a"}}, MethodRef{CapVA, "va_bca"}, providersAB(), Decision{RuleID: "r2", Primary: "b"}, false},
		{"no rule -> nicepay (T6)", nil, MethodRef{CapVA, "va_bca"}, providersAB(), Decision{Primary: "nicepay"}, false},
		{"no rule, nicepay disabled", nil, MethodRef{CapVA, "va_bca"}, map[string]ProviderInfo{"nicepay": {Code: "nicepay"}}, Decision{}, true},
		{"no rule, retail not at nicepay", nil, MethodRef{CapRetail, "alfamart"}, providersAB(), Decision{}, true},
		{"target disabled", []Rule{{ID: "r", MethodKind: CapVA, Primary: "off"}}, MethodRef{CapVA, "x"}, providersAB(), Decision{}, true},
		{"target lacks capability", []Rule{{ID: "r", MethodKind: CapRetail, Primary: "a"}}, MethodRef{CapRetail, "x"}, providersAB(), Decision{}, true},
		{"fallback kept when failover on", []Rule{{ID: "r", MethodKind: CapQRIS, Primary: "a", Fallback: "b", FailoverEnabled: true}}, MethodRef{CapQRIS, "qris"}, providersAB(), Decision{RuleID: "r", Primary: "a", Fallback: "b"}, false},
		{"fallback dropped when failover off", []Rule{{ID: "r", MethodKind: CapQRIS, Primary: "a", Fallback: "b"}}, MethodRef{CapQRIS, "qris"}, providersAB(), Decision{RuleID: "r", Primary: "a"}, false},
		{"fallback without capability dropped", []Rule{{ID: "r", MethodKind: CapEWallet, Primary: "nicepay", Fallback: "a", FailoverEnabled: true}}, MethodRef{CapEWallet, "dana"}, providersAB(), Decision{RuleID: "r", Primary: "nicepay"}, false},
		{"primary disabled, failover to fallback", []Rule{{ID: "r", MethodKind: CapVA, Primary: "off", Fallback: "b", FailoverEnabled: true}}, MethodRef{CapVA, "x"}, providersAB(), Decision{RuleID: "r", Primary: "b", FailoverFrom: "off"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Route(tc.rules, tc.method, tc.providers)
			if tc.wantErr {
				if !errors.Is(err, ErrNotRouted) {
					t.Fatalf("err=%v want ErrNotRouted", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %+v, %v want %+v", got, err, tc.want)
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestShouldFailover(t *testing.T) {
	d := Decision{Primary: "a", Fallback: "b"}
	var _ net.Error = timeoutErr{}
	tests := []struct {
		name string
		err  error
		d    Decision
		want bool
	}{
		{"timeout", timeoutErr{}, d, true},
		{"deadline", context.DeadlineExceeded, d, true},
		{"transient 5xx", &TransientError{Err: errors.New("HTTP 503")}, d, true},
		{"business 4xx", errors.New("nominal ditolak"), d, false},
		{"no fallback", timeoutErr{}, Decision{Primary: "a"}, false},
		{"fallback same provider", timeoutErr{}, Decision{Primary: "a", Fallback: "a"}, false},
		{"no error", nil, d, false},
	}
	for _, tc := range tests {
		if got := ShouldFailover(tc.err, tc.d); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestNicepayErrorClassification(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{&nicepay.APIError{Path: "/x", Code: "5002400", Message: "General Error"}, true},
		{&nicepay.APIError{Path: "/x", Code: "4002401", Message: "Invalid amount"}, false},
		{&nicepay.HTTPError{Status: 502, Body: "bad gateway"}, true},
		{&nicepay.HTTPError{Status: 404, Body: "nope"}, false},
	}
	for _, tc := range tests {
		if got := IsTransient(classify(tc.err)); got != tc.want {
			t.Errorf("%v: got %v want %v", tc.err, got, tc.want)
		}
	}
}

// An unreachable NICEPay must classify as an outage end to end.
func TestNicepayAdapterOutageIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("maintenance"))
	}))
	defer srv.Close()
	c, err := nicepay.NewClient(nicepay.Config{BaseURL: srv.URL, ClientID: "M", PrivateKey: testPrivateKey(t)})
	if err != nil {
		t.Fatal(err)
	}
	a := &Nicepay{ProviderCode: "nicepay", Client: c}
	_, err = a.CreateCharge(context.Background(), ChargeRequest{RefNo: "TX-1", AmountIDR: 1000, Kind: CapVA, ProviderMethodCode: "BNIN"})
	if !IsTransient(err) {
		t.Fatalf("err=%v want transient", err)
	}
	if _, err := a.CreateCharge(context.Background(), ChargeRequest{Kind: CapRetail}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("retail err=%v want ErrUnsupported", err)
	}
	if _, err := a.Debit(context.Background(), DebitRequest{}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("debit err=%v want ErrUnsupported", err)
	}
}

func TestProviderCode(t *testing.T) {
	m := model.GatewayMethod{Code: "va_bca", ProviderCode: "CENA"}
	tests := []struct {
		provider string
		mappings map[string]string
		want     string
		wantErr  bool
	}{
		{"nicepay", nil, "CENA", false},
		{"nicepay", map[string]string{"nicepay": "BCA2"}, "BCA2", false},
		{"b", map[string]string{"b": "014"}, "014", false},
		{"b", map[string]string{"nicepay": "CENA"}, "", true},
	}
	for _, tc := range tests {
		got, err := ProviderCode(m, tc.provider, tc.mappings)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s %v: got %q,%v want %q err=%v", tc.provider, tc.mappings, got, err, tc.want, tc.wantErr)
		}
	}
}

// FR-150-004: the same provider_ref at two providers never cross-matches.
func TestGroupForReconcile(t *testing.T) {
	txs := []ProviderTx{
		{"a", model.ReconcileTx{TXNo: "TX-1", ProviderRef: "R1", AmountIDR: 100}},
		{"b", model.ReconcileTx{TXNo: "TX-2", ProviderRef: "R2", AmountIDR: 200}},
	}
	items := []ProviderItem{
		{"b", model.ReconcileItem{ProviderRef: "R1", AmountIDR: 100}},
		{"b", model.ReconcileItem{ProviderRef: "R2", AmountIDR: 200}},
	}
	groups := GroupForReconcile(txs, items)
	if len(groups) != 2 || groups[0].Provider != "a" || groups[1].Provider != "b" {
		t.Fatalf("groups=%+v", groups)
	}
	ra := model.Reconcile(groups[0].Txs, groups[0].Items)
	if len(ra.Matched) != 0 || len(ra.Exceptions) != 1 || ra.Exceptions[0].Kind != model.ExceptionMissingInSettlement {
		t.Fatalf("provider a: %+v", ra)
	}
	rb := model.Reconcile(groups[1].Txs, groups[1].Items)
	if len(rb.Matched) != 1 || len(rb.Exceptions) != 1 || rb.Exceptions[0].Kind != model.ExceptionMissingInSekoci {
		t.Fatalf("provider b: %+v", rb)
	}
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestCanDebit(t *testing.T) {
	cancel := day("2026-10-20")
	tests := []struct {
		name string
		m    Mandate
		at   time.Time
		want bool
	}{
		{"active", Mandate{Status: MandateActive}, day("2026-11-10"), true},
		{"pending activation", Mandate{Status: MandatePending}, day("2026-11-10"), false},
		{"AC-151-02 cancelled 20 Okt, November bill", Mandate{Status: MandateCancelled, CancelledAt: &cancel}, day("2026-11-10"), false},
		{"cancelled moment itself", Mandate{Status: MandateActive, CancelledAt: &cancel}, cancel, false},
		{"failed", Mandate{Status: MandateFailed}, day("2026-11-10"), false},
	}
	for _, tc := range tests {
		if got := CanDebit(tc.m, tc.at); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestDueDebits(t *testing.T) {
	s := Settings{MaxRetries: 2, RetryIntervalDays: 3, RunHour: 6}
	cancel := day("2026-10-20")
	mandates := []Mandate{
		{ID: "m1", StudentUserID: "s1", Status: MandateActive, SchemeIDs: []string{"spp"}, MaxAmountIDR: 600000},
		{ID: "m2", StudentUserID: "s2", Status: MandateCancelled, SchemeIDs: []string{"spp"}, CancelledAt: &cancel},
	}
	bills := []DueBill{
		{ID: "b-due", StudentUserID: "s1", SchemeID: "spp", DueDate: day("2026-11-01"), OutstandingIDR: 500000},
		{ID: "b-future", StudentUserID: "s1", SchemeID: "spp", DueDate: day("2026-12-01"), OutstandingIDR: 500000},
		{ID: "b-other-scheme", StudentUserID: "s1", SchemeID: "gedung", DueDate: day("2026-10-01"), OutstandingIDR: 500000},
		{ID: "b-adhoc", StudentUserID: "s1", SchemeID: "", DueDate: day("2026-10-01"), OutstandingIDR: 50000},
		{ID: "b-paid", StudentUserID: "s1", SchemeID: "spp", DueDate: day("2026-10-01"), OutstandingIDR: 0},
		{ID: "b-big", StudentUserID: "s1", SchemeID: "spp", DueDate: day("2026-10-15"), OutstandingIDR: 700000},
		{ID: "b-done", StudentUserID: "s1", SchemeID: "spp", DueDate: day("2026-09-01"), OutstandingIDR: 500000},
		{ID: "b-nov-cancelled", StudentUserID: "s2", SchemeID: "spp", DueDate: day("2026-11-01"), OutstandingIDR: 500000},
	}
	got := DueDebits(mandates, bills, map[string]bool{"b-done": true}, day("2026-11-01"), s)
	want := []PlannedAttempt{
		{MandateID: "m1", BillID: "b-big", AttemptNo: 1, AmountIDR: 700000, Status: AttemptSkipped, SkipReason: SkipMaxAmount},
		{MandateID: "m1", BillID: "b-due", AttemptNo: 1, AmountIDR: 500000, Status: AttemptScheduled},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if DueDebits(mandates, bills, nil, day("2026-11-01"), Settings{MaxRetries: 2, RunHour: -1}) != nil {
		t.Fatal("unconfigured settings must plan nothing")
	}
}

func TestNextRetry(t *testing.T) {
	s := Settings{MaxRetries: 2, RetryIntervalDays: 3, RunHour: 6}
	n, on, ok := NextRetry(1, day("2026-11-01"), s)
	if !ok || n != 2 || !on.Equal(day("2026-11-04")) {
		t.Fatalf("attempt 1 -> %d %v %v", n, on, ok)
	}
	n, on, ok = NextRetry(2, day("2026-11-04"), s)
	if !ok || n != 3 || !on.Equal(day("2026-11-07")) {
		t.Fatalf("attempt 2 -> %d %v %v", n, on, ok)
	}
	if _, _, ok := NextRetry(3, day("2026-11-07"), s); ok {
		t.Fatal("attempt 3 (1 + 2 retries) must stop")
	}
	if _, _, ok := NextRetry(1, day("2026-11-01"), Settings{MaxRetries: 0, RetryIntervalDays: 3}); ok {
		t.Fatal("max_retries 0 must stop after the first attempt")
	}
}

func TestRetailExpiry(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	exp := RetailExpiry(1440, now)
	if !exp.Equal(now.Add(24 * time.Hour)) {
		t.Fatalf("exp=%v", exp)
	}
	if RetailExpired(exp, now) || !RetailExpired(exp, exp) || !RetailExpired(exp, exp.Add(time.Second)) {
		t.Fatal("RetailExpired boundary wrong")
	}
}

func TestSimMatchesT6Instructions(t *testing.T) {
	s := &Sim{ProviderCode: "nicepay"}
	ctx := context.Background()
	va, _ := s.CreateCharge(ctx, ChargeRequest{RefNo: "TX-2610121200001", Kind: CapVA})
	if va.ProviderRef != "SIM-TX-2610121200001" || va.VANumber != "88082610121200001" {
		t.Fatalf("va=%+v", va)
	}
	qr, _ := s.CreateCharge(ctx, ChargeRequest{RefNo: "TX-1", Kind: CapQRIS, AmountIDR: 504000})
	if !strings.Contains(qr.QRContent, "504000") {
		t.Fatalf("qr=%+v", qr)
	}
	rt, _ := s.CreateCharge(ctx, ChargeRequest{RefNo: "TX-77", Kind: CapRetail, ProviderMethodCode: "alfamart"})
	if rt.PayCode != "SIMR77" || rt.RetailOutlet != "alfamart" {
		t.Fatalf("retail=%+v", rt)
	}
	s.WebhookSecret = "sim-secret-for-tests-0001"
	body := []byte(`{"trxId":"TX-1","latestTransactionStatus":"06"}`)
	n, err := s.VerifyWebhook(http.Header{"X-Signature": {SimSignature(s.WebhookSecret, body)}}, body)
	if err != nil || n.RefNo != "TX-1" || n.ClaimedStatus != StatusFailed {
		t.Fatalf("notification=%+v", n)
	}
	if st, _ := s.QueryStatus(ctx, StatusQuery{}); st != StatusPending {
		t.Fatalf("status without notification=%s", st)
	}
	if _, err := (&Sim{FailCharges: true}).CreateCharge(ctx, ChargeRequest{Kind: CapVA}); !IsTransient(err) {
		t.Fatalf("FailCharges err=%v", err)
	}
	if _, err := (&Sim{FailDebits: true}).Debit(ctx, DebitRequest{}); err == nil || IsTransient(err) {
		t.Fatalf("FailDebits err=%v", err)
	}
}

func TestAdapterCapabilities(t *testing.T) {
	if HasCapability(AdapterCapabilities(AdapterNicepay), CapRetail) || HasCapability(AdapterCapabilities(AdapterNicepay), CapAutodebit) {
		t.Fatal("nicepay must not claim retail/autodebit (OQ-41)")
	}
	if AdapterCapabilities("unknown") != nil {
		t.Fatal("unknown adapter has no capabilities")
	}
}

func testPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

// BUG-004: the simulator never trusts an unsigned or wrongly signed body.
func TestSimWebhookRequiresSignature(t *testing.T) {
	body := []byte(`{"trxId":"TX-9","latestTransactionStatus":"00"}`)
	const secret = "sim-secret-for-tests-0001"
	good := SimSignature(secret, body)
	cases := []struct {
		name, secret, sig string
		ok                bool
	}{
		{"no secret configured", "", good, false},
		{"no signature header", secret, "", false},
		{"forged signature", secret, "merchantToken-palsu", false},
		{"signature of other body", secret, SimSignature(secret, []byte(`{}`)), false},
		{"signature with other secret", secret, SimSignature("other-secret-000000000", body), false},
		{"valid signature", secret, good, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := http.Header{}
			if c.sig != "" {
				h.Set("X-SIGNATURE", c.sig)
			}
			n, err := (&Sim{WebhookSecret: c.secret}).VerifyWebhook(h, body)
			if c.ok != (err == nil) {
				t.Fatalf("err=%v want ok=%v", err, c.ok)
			}
			if !c.ok && !errors.Is(err, ErrInvalidSignature) {
				t.Fatalf("err=%v want ErrInvalidSignature", err)
			}
			if n.RefNo != "TX-9" {
				t.Fatalf("refs must be parsed even when rejected: %+v", n)
			}
		})
	}
}
