package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
	"skci/billing/internal/gateway"
	"skci/billing/internal/storage"
)

// ---- fakes ----------------------------------------------------------------

type featuresOn map[string]bool

func (f featuresOn) FeatureEnabled(_ context.Context, _ string, key string) (bool, error) {
	return f[key], nil
}

func allFeatures() featuresOn {
	return featuresOn{common.FeaturePaymentGateway: true, common.FeaturePaymentMultiProvider: true, common.FeaturePaymentAutodebit: true}
}

// recorder is a fake core / notification service capturing request bodies.
type recorder struct {
	mu     sync.Mutex
	bodies map[string][]string // path -> bodies
	srv    *httptest.Server
}

func newRecorder(t *testing.T) *recorder {
	rec := &recorder{bodies: map[string][]string{}}
	rec.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.bodies[r.URL.Path] = append(rec.bodies[r.URL.Path], string(b))
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"x"}}`))
	}))
	t.Cleanup(rec.srv.Close)
	return rec
}

func (r *recorder) get(path string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.bodies[path]...)
}

func (r *recorder) count(path, substr string) int {
	n := 0
	for _, b := range r.get(path) {
		if strings.Contains(b, substr) {
			n++
		}
	}
	return n
}

// fakePurpose is a K2 handler that proves it runs inside the webhook's
// DB transaction (it reads the locked row through tx).
type fakePurpose struct {
	mu       sync.Mutex
	success  []string
	closed   []string
	failWith error
}

func (f *fakePurpose) OnGatewaySuccess(ctx context.Context, tx *sql.Tx, schoolID, purposeRef, gatewayTxID string) error {
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM gateway_transactions WHERE id = $1`, gatewayTxID).Scan(&status); err != nil {
		return err
	}
	if status != "success" {
		return errors.New("hook must see the success status of the same DB transaction")
	}
	if f.failWith != nil {
		return f.failWith
	}
	f.mu.Lock()
	f.success = append(f.success, purposeRef)
	f.mu.Unlock()
	return nil
}

func (f *fakePurpose) OnGatewayClosed(_ context.Context, _ *sql.Tx, _, purposeRef, _, status string) error {
	f.mu.Lock()
	f.closed = append(f.closed, purposeRef+":"+status)
	f.mu.Unlock()
	return nil
}

// ---- fixture --------------------------------------------------------------

type extFixture struct {
	t                                 *testing.T
	db                                *sql.DB
	app                               *App
	srv                               *httptest.Server
	core, notif                       *recorder
	schoolID, studentID, guardianID   string
	schemeID                          string
}

func newExtFixture(t *testing.T) *extFixture {
	db := getTestDB(t)
	t.Cleanup(func() { db.Close() })
	f := &extFixture{t: t, db: db, schoolID: genUUID(), studentID: genUUID(), guardianID: genUUID(), schemeID: genUUID()}
	identity := stubIdentity{allowed: map[string]bool{f.guardianID + "|" + f.studentID: true}}
	f.app = New(db, config.Config{JWTSecret: testSecret}, identity, storage.NewLocal(t.TempDir(), 10<<20))
	f.core, f.notif = newRecorder(t), newRecorder(t)
	f.app.Core = client.New(f.core.srv.URL, "k")
	f.app.Notification = client.New(f.notif.srv.URL, "k")
	f.app.PaymentFeatures = allFeatures()
	f.app.Providers = map[string]gateway.Provider{}
	f.app.PurposeHandlers = map[string]PurposeHandler{}

	r := chi.NewRouter()
	r.Post("/webhooks/nicepay", f.app.NicepayWebhook)
	r.Post("/webhooks/{provider}", f.app.ProviderWebhook)
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Get("/payment-methods", f.app.ListPaymentMethods)
		pr.Post("/parent/checkout", f.app.ParentCheckout)
		pr.Get("/parent/transactions/{id}", f.app.ParentGetTransaction)
		pr.Get("/parent/autodebit/mandates", f.app.ParentListMandates)
		pr.Post("/parent/autodebit/mandates", f.app.ParentCreateMandate)
		pr.Post("/parent/autodebit/mandates/{id}/cancel", f.app.ParentCancelMandate)
		pr.Get("/platform/payment-providers", f.app.PlatformListProviders)
		pr.Post("/platform/payment-providers", f.app.PlatformCreateProvider)
		pr.Put("/platform/payment-providers/{code}", f.app.PlatformUpdateProvider)
		pr.Post("/admin/payment-routing-rules", f.app.AdminCreateRoutingRule)
		pr.Post("/admin/payment-routing-rules/preview", f.app.AdminPreviewRouting)
		pr.Post("/admin/reconciliation/run", f.app.AdminRunReconciliation)
		pr.Put("/admin/autodebit/settings", f.app.AdminPutAutodebitSettings)
	})
	f.srv = httptest.NewServer(r)
	t.Cleanup(f.srv.Close)
	f.exec(`INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
	        VALUES ($1, $2, $3, 'SPP', 'active', true, 10)`, f.schemeID, f.schoolID, genUUID())
	return f
}

func (f *extFixture) exec(q string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		f.t.Fatalf("exec %q: %v", q, err)
	}
}

// provider registers a provider row + adapter override.
func (f *extFixture) provider(p gateway.Provider, caps ...string) string {
	code := p.Code()
	f.exec(`INSERT INTO payment_providers (code, adapter, label, enabled) VALUES ($1, 'sim', $1, true)`, code)
	for _, c := range caps {
		f.exec(`INSERT INTO payment_provider_capabilities (provider_code, capability) VALUES ($1, $2)`, code, c)
	}
	f.app.Providers[code] = p
	return code
}

func uniqueCode(prefix string) string { return prefix + strings.ReplaceAll(genUUID(), "-", "")[:10] }

func (f *extFixture) method(code, kind, providerCode string) string {
	id := genUUID()
	f.exec(`INSERT INTO payment_methods (id, school_id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort)
	        VALUES ($1, $2, $3, $4, $5, $3, true, 'flat', 4000, 0, 'parent', 60, 1)`, id, f.schoolID, code, kind, providerCode)
	return id
}

func (f *extFixture) rule(kind, primary, fallback string, failover bool) {
	f.exec(`INSERT INTO payment_routing_rules (school_id, method_kind, primary_provider, fallback_provider, failover_enabled)
	        VALUES ($1, $2, $3, NULLIF($4,''), $5)`, f.schoolID, kind, primary, fallback, failover)
}

func (f *extFixture) bill(year, month int, due time.Time, amount int64) string {
	id := genUUID()
	f.exec(`INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month, base_idr, discount_idr, net_idr, status, due_at)
	        VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $7, 'unpaid', $8)`, id, f.schoolID, f.studentID, f.schemeID, year, month, amount, due)
	return id
}

func (f *extFixture) do(method, path, role, userID string, body any) (int, map[string]any, string) {
	f.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, f.srv.URL+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+signToken(f.t, userID, f.schoolID, role))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var env map[string]any
	_ = json.Unmarshal(raw, &env)
	data, _ := env["data"].(map[string]any)
	return resp.StatusCode, data, string(raw)
}

func (f *extFixture) checkout(methodID string, billIDs ...string) (int, map[string]any, string) {
	return f.do(http.MethodPost, "/parent/checkout", common.RoleParent, f.guardianID,
		map[string]any{"student_user_id": f.studentID, "bill_ids": billIDs, "method_id": methodID})
}

func (f *extFixture) scalar(q string, args ...any) string {
	f.t.Helper()
	var v sql.NullString
	if err := f.db.QueryRow(q, args...).Scan(&v); err != nil && !errors.Is(err, sql.ErrNoRows) {
		f.t.Fatalf("scalar %q: %v", q, err)
	}
	return v.String
}

// ---- AC-150-01 / AC-150-02 -----------------------------------------------

func TestAC15001_RoutingPerMethod(t *testing.T) {
	f := newExtFixture(t)
	a := f.provider(&gateway.Sim{ProviderCode: uniqueCode("a")}, gateway.CapQRIS, gateway.CapVA)
	b := f.provider(&gateway.Sim{ProviderCode: uniqueCode("b")}, gateway.CapVA, gateway.CapQRIS)
	f.rule(gateway.CapQRIS, a, "", false)
	f.rule(gateway.CapVA, b, "", false)
	va := f.method("va_bca", gateway.CapVA, "CENA")
	f.exec(`INSERT INTO payment_method_provider_codes (method_id, provider_code, provider_method_code) VALUES ($1, $2, '014')`, va, b)

	code, data, raw := f.checkout(va, f.bill(2026, 7, time.Now(), 500000))
	if code != http.StatusOK {
		t.Fatalf("checkout %d %s", code, raw)
	}
	if data["provider"] != b {
		t.Fatalf("provider=%v want %s", data["provider"], b)
	}
	if got := f.scalar(`SELECT provider FROM gateway_transactions WHERE id = $1`, data["id"]); got != b {
		t.Fatalf("db provider=%s want %s", got, b)
	}
}

func TestAC15002_FailoverAndNoFailover(t *testing.T) {
	f := newExtFixture(t)
	a := f.provider(&gateway.Sim{ProviderCode: uniqueCode("a"), FailCharges: true}, gateway.CapQRIS)
	b := f.provider(&gateway.Sim{ProviderCode: uniqueCode("b")}, gateway.CapQRIS)
	f.rule(gateway.CapQRIS, a, b, true)
	qris := f.method("qris", gateway.CapQRIS, "QSHP")
	f.exec(`INSERT INTO payment_method_provider_codes (method_id, provider_code, provider_method_code) VALUES ($1, $2, 'Q'), ($1, $3, 'Q')`, qris, a, b)

	code, data, raw := f.checkout(qris, f.bill(2026, 7, time.Now(), 500000))
	if code != http.StatusOK {
		t.Fatalf("checkout %d %s", code, raw)
	}
	if data["provider"] != b || data["failover_from"] != a {
		t.Fatalf("provider=%v failover_from=%v", data["provider"], data["failover_from"])
	}
	if got := f.scalar(`SELECT failover_from FROM gateway_transactions WHERE id = $1`, data["id"]); got != a {
		t.Fatalf("db failover_from=%s", got)
	}
	if n := f.core.count("/internal/exceptions", `"event":"provider_failover"`); n != 1 {
		t.Fatalf("provider_failover exceptions=%d want 1", n)
	}
	if got := f.scalar(`SELECT consecutive_failures::text FROM payment_providers WHERE code = $1`, a); got != "1" {
		t.Fatalf("consecutive_failures=%s want 1", got)
	}

	// Failover off: 502 and no transaction.
	f.exec(`UPDATE payment_routing_rules SET failover_enabled = false WHERE school_id = $1`, f.schoolID)
	billID := f.bill(2026, 8, time.Now(), 300000)
	code, _, raw = f.checkout(qris, billID)
	if code != http.StatusBadGateway {
		t.Fatalf("no-failover status=%d %s", code, raw)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM gateway_transaction_bills WHERE bill_id = $1`, billID); n != "0" {
		t.Fatalf("transactions for bill=%s want 0", n)
	}
}

// Legacy configuration (T14 features off, NICEPay only): the checkout
// response keeps every T6 key and value shape; T14 only adds keys.
func TestCheckoutGoldenLegacy(t *testing.T) {
	f := newExtFixture(t)
	f.app.PaymentFeatures = nil
	f.app.Providers = nil
	va := f.method("va_bni", gateway.CapVA, "BNIN")
	billID := f.bill(2026, 7, time.Now(), 500000)
	code, data, raw := f.checkout(va, billID)
	if code != http.StatusOK {
		t.Fatalf("checkout %d %s", code, raw)
	}
	var keys []string
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := "bills,bills_amount_idr,expires_at,fee_bearer,fee_idr,id,kind,method_code,method_label,provider,purpose,status,total_idr,tx_no,va_number"
	if strings.Join(keys, ",") != want {
		t.Fatalf("keys=%s\nwant=%s", strings.Join(keys, ","), want)
	}
	txNo, _ := data["tx_no"].(string)
	golden := map[string]any{
		"bills_amount_idr": 500000.0, "fee_idr": 4000.0, "total_idr": 504000.0, "fee_bearer": "parent", "status": "pending",
		"method_code": "va_bni", "kind": "va", "method_label": "va_bni", "provider": "nicepay", "purpose": "bills",
		"va_number": "8808" + strings.TrimPrefix(txNo, "TX-"),
	}
	for k, v := range golden {
		if data[k] != v {
			t.Errorf("%s=%v want %v", k, data[k], v)
		}
	}
	if pref := f.scalar(`SELECT provider_ref FROM gateway_transactions WHERE id = $1`, data["id"]); pref != "SIM-"+txNo {
		t.Errorf("provider_ref=%s want SIM-%s", pref, txNo)
	}
}

func TestNotRoutedMethod(t *testing.T) {
	f := newExtFixture(t)
	retail := f.method("alfamart", gateway.CapRetail, "ALFA")
	code, _, raw := f.checkout(retail, f.bill(2026, 7, time.Now(), 100000))
	if code != http.StatusUnprocessableEntity || !strings.Contains(raw, "Metode belum dirutekan") {
		t.Fatalf("status=%d %s", code, raw)
	}
}

// ---- webhook per provider -------------------------------------------------

func (f *extFixture) pendingTx(provider, ref, purpose, purposeRef string, methodID string) string {
	id := genUUID()
	f.exec(`INSERT INTO gateway_transactions (id, school_id, tx_no, payer_user_id, student_user_id, method_id, bills_amount_idr, fee_idr, total_idr,
	          fee_bearer, status, provider_ref, expires_at, idempotency_key, provider, purpose, purpose_ref)
	        VALUES ($1,$2,$3,$4,$5,$6,100000,0,100000,'parent','pending',$7,now() + interval '1 hour',$8,$9,$10,NULLIF($11,'')::uuid)`,
		id, f.schoolID, "TX-"+id[:8], f.guardianID, f.studentID, methodID, ref, "idem-"+id, provider, purpose, purposeRef)
	return id
}

func (f *extFixture) webhook(provider, ref string) int {
	f.t.Helper()
	body := `{"originalReferenceNo":"` + ref + `","trxId":"` + ref + `","latestTransactionStatus":"00"}`
	resp, err := http.Post(f.srv.URL+"/webhooks/"+provider, "application/json", strings.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestWebhookDedupPerProvider(t *testing.T) {
	f := newExtFixture(t)
	a := f.provider(&gateway.Sim{ProviderCode: uniqueCode("a")}, gateway.CapVA)
	b := f.provider(&gateway.Sim{ProviderCode: uniqueCode("b")}, gateway.CapVA)
	m := f.method("va_x", gateway.CapVA, "X")
	ref := "REF-" + genUUID()[:8]
	txA := f.pendingTx(a, ref, PurposeBills, "", m)
	txB := f.pendingTx(b, ref, PurposeBills, "", m)
	if c := f.webhook(b, ref); c != http.StatusOK {
		t.Fatalf("webhook b=%d", c)
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, txB); st != "success" {
		t.Fatalf("tx B=%s want success", st)
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, txA); st != "pending" {
		t.Fatalf("tx A=%s want pending (other provider)", st)
	}
	// The same body at provider A is a different notification.
	if c := f.webhook(a, ref); c != http.StatusOK {
		t.Fatalf("webhook a=%d", c)
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, txA); st != "success" {
		t.Fatalf("tx A=%s want success", st)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM gateway_notifications WHERE provider_ref = $1`, ref); n != "2" {
		t.Fatalf("notifications=%s want 2", n)
	}
}

// ---- K1 + K2 ---------------------------------------------------------------

func TestPurposeChargeAndHook(t *testing.T) {
	f := newExtFixture(t)
	f.app.PaymentFeatures = nil // K1 must work on the T6 path too
	hook := &fakePurpose{}
	f.app.PurposeHandlers[PurposeWalletTopup] = hook
	f.app.Providers = nil
	m := f.method("va_topup", gateway.CapVA, "BNIN")
	ctx := context.Background()
	topupID := genUUID()

	if _, err := f.app.CreatePurposeCharge(ctx, f.schoolID, f.guardianID, f.studentID, m, PurposeWalletTopup, "", "k1", 200000); err == nil {
		t.Fatal("purpose_ref is required")
	}
	if _, err := f.app.CreatePurposeCharge(ctx, f.schoolID, f.guardianID, f.studentID, m, PurposeBills, topupID, "k1", 200000); err == nil {
		t.Fatal("bills purpose must be refused")
	}
	txID, err := f.app.CreatePurposeCharge(ctx, f.schoolID, f.guardianID, f.studentID, m, PurposeWalletTopup, topupID, "wallet-topup:"+topupID, 200000)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.app.CreatePurposeCharge(ctx, f.schoolID, f.guardianID, f.studentID, m, PurposeWalletTopup, topupID, "wallet-topup:"+topupID, 200000)
	if err != nil || again != txID {
		t.Fatalf("idempotent replay %s %v want %s", again, err, txID)
	}
	if got := f.scalar(`SELECT purpose || ':' || purpose_ref || ':' || bills_amount_idr || ':' || total_idr FROM gateway_transactions WHERE id = $1`, txID); got != "wallet_topup:"+topupID+":200000:204000" {
		t.Fatalf("row=%s", got)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM gateway_transaction_bills WHERE transaction_id = $1`, txID); n != "0" {
		t.Fatalf("bill lines=%s want 0", n)
	}
	ref := f.scalar(`SELECT provider_ref FROM gateway_transactions WHERE id = $1`, txID)
	if c := f.webhook("nicepay", ref); c != http.StatusOK {
		t.Fatalf("webhook=%d", c)
	}
	if c := f.webhook("nicepay", ref); c != http.StatusOK {
		t.Fatalf("duplicate webhook=%d", c)
	}
	if len(hook.success) != 1 || hook.success[0] != topupID {
		t.Fatalf("hook calls=%v want exactly [%s]", hook.success, topupID)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM receipts WHERE source_id = $1`, txID); n != "1" {
		t.Fatalf("receipts=%s want 1", n)
	}
}

func TestPurposeHookErrorRollsBack(t *testing.T) {
	f := newExtFixture(t)
	a := f.provider(&gateway.Sim{ProviderCode: uniqueCode("a")}, gateway.CapVA)
	m := f.method("va_y", gateway.CapVA, "Y")

	// Hook error: 500, status untouched, provider may retry.
	f.app.PurposeHandlers[PurposeWalletTopup] = &fakePurpose{failWith: errors.New("wallet locked")}
	ref := "R-" + genUUID()[:8]
	tx := f.pendingTx(a, ref, PurposeWalletTopup, genUUID(), m)
	if c := f.webhook(a, ref); c != http.StatusInternalServerError {
		t.Fatalf("hook error status=%d want 500", c)
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, tx); st != "pending" {
		t.Fatalf("status=%s want pending (rolled back)", st)
	}
	// Retry of the same notification is processed once the hook works.
	ok := &fakePurpose{}
	f.app.PurposeHandlers[PurposeWalletTopup] = ok
	if c := f.webhook(a, ref); c != http.StatusOK || len(ok.success) != 1 {
		t.Fatalf("retry status=%d calls=%v", c, ok.success)
	}

	// No handler registered: 500 + webhook exception.
	delete(f.app.PurposeHandlers, PurposeWalletTopup)
	ref2 := "R-" + genUUID()[:8]
	tx2 := f.pendingTx(a, ref2, PurposeWalletTopup, genUUID(), m)
	if c := f.webhook(a, ref2); c != http.StatusInternalServerError {
		t.Fatalf("missing handler status=%d want 500", c)
	}
	if n := f.core.count("/internal/exceptions", "webhook-err:"+tx2); n != 1 {
		t.Fatalf("webhook exceptions for %s = %d", tx2, n)
	}
}

// ---- K3: ad-hoc bill (scheme_id NULL, T12 migration 0007) -------------------

func TestAdhocBillCheckout(t *testing.T) {
	f := newExtFixture(t)
	if f.scalar(`SELECT is_nullable FROM information_schema.columns WHERE table_name = 'student_bills' AND column_name = 'scheme_id'`) != "YES" {
		t.Skip("student_bills.scheme_id is still NOT NULL: runs after rebase on T12 migration 0007")
	}
	f.app.PaymentFeatures = nil
	m := f.method("va_adhoc", gateway.CapVA, "BNIN")
	id := genUUID()
	f.exec(`INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month, base_idr, discount_idr, net_idr, status, due_at, source_module, source_ref)
	        VALUES ($1, $2, $3, NULL, 2026, 7, 50000, 0, 50000, 'unpaid', now(), 'library', $1)`, id, f.schoolID, f.studentID)
	if code, _, raw := f.checkout(m, id); code != http.StatusOK {
		t.Fatalf("ad-hoc checkout %d %s", code, raw)
	}
}

// ---- FR-150-004 ------------------------------------------------------------

func TestReconcilePerProviderHandler(t *testing.T) {
	f := newExtFixture(t)
	a := f.provider(&gateway.Sim{ProviderCode: uniqueCode("a")}, gateway.CapVA)
	b := f.provider(&gateway.Sim{ProviderCode: uniqueCode("b")}, gateway.CapVA)
	m := f.method("va_r", gateway.CapVA, "R")
	ref := "SAME-" + genUUID()[:6]
	tx := f.pendingTx(a, ref, PurposeBills, "", m)
	f.exec(`UPDATE gateway_transactions SET status = 'success', paid_at = '2026-07-15T03:00:00Z', created_at = '2026-07-15T03:00:00Z' WHERE id = $1`, tx)
	batch := genUUID()
	f.exec(`INSERT INTO settlement_batches (id, school_id, provider, channel, settled_on, gross_idr, fee_idr, net_idr, bank_account)
	        VALUES ($1, $2, $3, 'va', '2026-07-15', 100000, 0, 100000, 'ACC')`, batch, f.schoolID, b)
	f.exec(`INSERT INTO settlement_items (school_id, batch_id, provider_ref, amount_idr) VALUES ($1, $2, $3, 100000)`, f.schoolID, batch, ref)

	code, data, raw := f.do(http.MethodPost, "/admin/reconciliation/run", common.RoleFinanceManager, genUUID(), map[string]any{"date": "2026-07-15"})
	if code != http.StatusOK {
		t.Fatalf("run %d %s", code, raw)
	}
	if data["matched_count"] != 0.0 {
		t.Fatalf("A's transaction must not match B's settlement: %s", raw)
	}
	got := f.scalar(`SELECT string_agg(provider || ':' || kind, ',' ORDER BY provider) FROM reconciliation_exceptions WHERE school_id = $1`, f.schoolID)
	if got != a+":missing_in_settlement,"+b+":missing_in_sekoci" {
		t.Fatalf("exceptions=%s", got)
	}
}

// ---- FR-150-001: credentials never leave the env --------------------------

func TestProviderCredentialsNotExposed(t *testing.T) {
	f := newExtFixture(t)
	secret := "SUPER-SECRET-" + genUUID()[:8]
	prefix := "T14" + strings.ToUpper(uniqueCode("x"))[1:9]
	t.Setenv(prefix+"_PRIVATE_KEY", secret)
	t.Setenv(prefix+"_CLIENT_SECRET", secret)
	code := uniqueCode("np")
	st, _, raw := f.do(http.MethodPost, "/platform/payment-providers", common.RoleSuperadmin, genUUID(),
		map[string]any{"code": code, "adapter": "nicepay", "label": "NICEPay 2", "enabled": true, "credential_ref": prefix, "capabilities": []string{"va"}})
	if st != http.StatusCreated {
		t.Fatalf("create %d %s", st, raw)
	}
	if st, _, _ := f.do(http.MethodPost, "/platform/payment-providers", common.RoleSuperadmin, genUUID(),
		map[string]any{"code": uniqueCode("np"), "adapter": "nicepay", "label": "x", "capabilities": []string{"retail"}}); st != http.StatusBadRequest {
		t.Fatalf("retail on nicepay adapter must be refused, got %d", st)
	}
	_, _, list := f.do(http.MethodGet, "/platform/payment-providers", common.RoleSuperadmin, genUUID(), nil)
	if st, _, _ := f.do(http.MethodGet, "/platform/payment-providers", common.RoleFinanceManager, genUUID(), nil); st != http.StatusForbidden {
		t.Fatalf("finance_manager on platform endpoint=%d want 403", st)
	}
	for name, s := range map[string]string{"create": raw, "list": list, "audit": strings.Join(f.core.get("/internal/audit"), "\n")} {
		if strings.Contains(s, secret) {
			t.Fatalf("secret leaked in %s", name)
		}
	}
	if !strings.Contains(list, `"credentials_installed":false`) || !strings.Contains(list, prefix) {
		t.Fatalf("list must show credential_ref and installed flag: %s", list)
	}
}

// ---- autodebit ---------------------------------------------------------------

func (f *extFixture) autodebitSetup(fail bool) (provider, mandateID string) {
	provider = f.provider(&gateway.Sim{ProviderCode: uniqueCode("ad"), FailDebits: fail}, gateway.CapAutodebit)
	f.method("autodebit", kindAutodebit, "AD")
	st, _, raw := f.do(http.MethodPut, "/admin/autodebit/settings", common.RoleFinanceManager, genUUID(),
		map[string]any{"max_retries": 2, "retry_interval_days": 3, "run_hour": 6})
	if st != http.StatusOK {
		f.t.Fatalf("settings %d %s", st, raw)
	}
	st, data, raw := f.do(http.MethodPost, "/parent/autodebit/mandates", common.RoleParent, f.guardianID,
		map[string]any{"student_user_id": f.studentID, "scheme_ids": []string{f.schemeID}, "provider": provider})
	if st != http.StatusCreated || data["status"] != gateway.MandateActive {
		f.t.Fatalf("mandate %d %s", st, raw)
	}
	return provider, data["id"].(string)
}

func wib(t *testing.T, s string) time.Time {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestAC15101_FailedDebitRetriesAndNotifies(t *testing.T) {
	f := newExtFixture(t)
	_, mandate := f.autodebitSetup(true)
	bill := f.bill(2026, 11, wib(t, "2026-11-01 00:00"), 500000)
	ctx := context.Background()

	if err := f.app.runAutodebitSchedule(ctx, wib(t, "2026-11-01 05:00")); err != nil {
		t.Fatal(err)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM autodebit_attempts WHERE mandate_id = $1`, mandate); n != "0" {
		t.Fatalf("before run_hour attempts=%s want 0", n)
	}
	attempts := func() string {
		return f.scalar(`SELECT string_agg(attempt_no || ':' || status || ':' || to_char(scheduled_for, 'MM-DD'), ',' ORDER BY attempt_no)
		                   FROM autodebit_attempts WHERE bill_id = $1`, bill)
	}
	if err := f.app.runAutodebitSchedule(ctx, wib(t, "2026-11-01 07:00")); err != nil {
		t.Fatal(err)
	}
	if got := attempts(); got != "1:failed:11-01,2:scheduled:11-04" {
		t.Fatalf("after day 1: %s", got)
	}
	if n := f.notif.count("/internal/notifications", "Akan dicoba ulang"); n != 1 {
		t.Fatalf("retry notifications=%d want 1", n)
	}
	for _, day := range []string{"2026-11-04 07:00", "2026-11-07 07:00"} {
		if err := f.app.runAutodebitSchedule(ctx, wib(t, day)); err != nil {
			t.Fatal(err)
		}
	}
	if got := attempts(); got != "1:failed:11-01,2:failed:11-04,3:failed:11-07" {
		t.Fatalf("after retries: %s", got)
	}
	if n := f.notif.count("/internal/notifications", "bayar manual"); n != 1 {
		t.Fatalf("final notifications=%d want 1", n)
	}
	if st := f.scalar(`SELECT status FROM student_bills WHERE id = $1`, bill); st != "unpaid" {
		t.Fatalf("bill=%s", st)
	}
}

func TestAutodebitSuccessAllocates(t *testing.T) {
	f := newExtFixture(t)
	_, mandate := f.autodebitSetup(false)
	bill := f.bill(2026, 11, wib(t, "2026-11-01 00:00"), 500000)
	if err := f.app.runAutodebitSchedule(context.Background(), wib(t, "2026-11-01 07:00")); err != nil {
		t.Fatal(err)
	}
	if got := f.scalar(`SELECT status FROM autodebit_attempts WHERE mandate_id = $1`, mandate); got != "success" {
		t.Fatalf("attempt=%s", got)
	}
	if st := f.scalar(`SELECT status FROM student_bills WHERE id = $1`, bill); st != "paid" {
		t.Fatalf("bill=%s want paid", st)
	}
	if got := f.scalar(`SELECT mandate_id::text FROM gateway_transactions WHERE mandate_id = $1`, mandate); got != mandate {
		t.Fatalf("gateway tx mandate=%s", got)
	}
}

func TestAC15102_CancelledMandateNotDebited(t *testing.T) {
	f := newExtFixture(t)
	_, mandate := f.autodebitSetup(false)
	octBill := f.bill(2026, 10, wib(t, "2026-10-10 00:00"), 500000)
	// An already scheduled retry for October must be skipped on cancel.
	f.exec(`INSERT INTO autodebit_attempts (school_id, mandate_id, bill_id, attempt_no, scheduled_for, amount_idr, idempotency_key)
	        VALUES ($1, $2, $3, 2, '2026-10-22', 504000, $4)`, f.schoolID, mandate, octBill, "ad:"+mandate+":"+octBill+":2")
	if _, err := f.app.cancelMandate(context.Background(), nil, f.schoolID, mandate, f.guardianID, "ganti rekening", wib(t, "2026-10-20 09:00")); err != nil {
		t.Fatal(err)
	}
	if got := f.scalar(`SELECT status || ':' || skip_reason FROM autodebit_attempts WHERE mandate_id = $1`, mandate); got != "skipped:mandate_cancelled" {
		t.Fatalf("scheduled attempt=%s", got)
	}
	novBill := f.bill(2026, 11, wib(t, "2026-11-01 00:00"), 500000)
	if err := f.app.runAutodebitSchedule(context.Background(), wib(t, "2026-11-01 07:00")); err != nil {
		t.Fatal(err)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM autodebit_attempts WHERE bill_id = $1`, novBill); n != "0" {
		t.Fatalf("November attempts=%s want 0", n)
	}
	// Other guardians cannot cancel or see it.
	if st, _, _ := f.do(http.MethodPost, "/parent/autodebit/mandates/"+mandate+"/cancel", common.RoleParent, genUUID(), map[string]any{}); st != http.StatusNotFound {
		t.Fatalf("foreign cancel=%d want 404", st)
	}
}

func TestMandateRequiresSettingsAndGuardian(t *testing.T) {
	f := newExtFixture(t)
	p := f.provider(&gateway.Sim{ProviderCode: uniqueCode("ad")}, gateway.CapAutodebit)
	body := map[string]any{"student_user_id": f.studentID, "scheme_ids": []string{f.schemeID}, "provider": p}
	if st, _, raw := f.do(http.MethodPost, "/parent/autodebit/mandates", common.RoleParent, f.guardianID, body); st != http.StatusConflict {
		t.Fatalf("without settings=%d %s", st, raw)
	}
	if st, _, _ := f.do(http.MethodPost, "/parent/autodebit/mandates", common.RoleParent, genUUID(), body); st != http.StatusForbidden {
		t.Fatalf("foreign child=%d want 403", st)
	}
	f.app.PaymentFeatures = featuresOn{common.FeaturePaymentGateway: true}
	if st, _, _ := f.do(http.MethodGet, "/parent/autodebit/mandates", common.RoleParent, f.guardianID, nil); st != http.StatusForbidden {
		t.Fatalf("feature off=%d want 403", st)
	}
}

// ---- FR-151-004 retail -------------------------------------------------------

func TestRetailPayCodeAndExpiry(t *testing.T) {
	f := newExtFixture(t)
	r := f.provider(&gateway.Sim{ProviderCode: uniqueCode("rt")}, gateway.CapRetail)
	f.rule(gateway.CapRetail, r, "", false)
	m := f.method("alfamart", gateway.CapRetail, "ALFA")
	f.exec(`INSERT INTO payment_method_provider_codes (method_id, provider_code, provider_method_code) VALUES ($1, $2, 'Alfamart')`, m, r)
	code, data, raw := f.checkout(m, f.bill(2026, 7, time.Now(), 100000))
	if code != http.StatusOK || data["pay_code"] == nil || data["retail_outlet"] != "Alfamart" {
		t.Fatalf("retail checkout %d %s", code, raw)
	}
	exp, _ := time.Parse(time.RFC3339Nano, data["expires_at"].(string))
	if d := time.Until(exp); d < 55*time.Minute || d > 61*time.Minute {
		t.Fatalf("expires in %v, want valid_minutes=60", d)
	}
	if err := f.app.runRetailExpiry(context.Background(), exp.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st := f.scalar(`SELECT status FROM gateway_transactions WHERE id = $1`, data["id"]); st != "expired" {
		t.Fatalf("status=%s want expired", st)
	}
	// Feature off: retail refused and hidden from parents.
	f.app.PaymentFeatures = featuresOn{common.FeaturePaymentGateway: true, common.FeaturePaymentMultiProvider: true}
	if code, _, _ := f.checkout(m, f.bill(2026, 8, time.Now(), 100000)); code != http.StatusBadRequest {
		t.Fatalf("retail with autodebit feature off=%d want 400", code)
	}
}

// ---- daily journal (W2 §4) ---------------------------------------------------

func TestGatewayJournalQueuedDailyWithoutHardcodedAccounts(t *testing.T) {
	f := newExtFixture(t)
	m := f.method("va_j", gateway.CapVA, "J")
	for i := 0; i < 2; i++ {
		tx := f.pendingTx("nicepay", "J-"+genUUID()[:8], PurposeBills, "", m)
		f.exec(`UPDATE gateway_transactions SET status = 'success', paid_at = $2 WHERE id = $1`, tx, wib(t, "2026-10-10 10:00").Add(time.Duration(i)*time.Hour))
	}
	topup := f.pendingTx("nicepay", "J-"+genUUID()[:8], PurposeWalletTopup, genUUID(), m)
	f.exec(`UPDATE gateway_transactions SET status = 'success', paid_at = $2 WHERE id = $1`, topup, wib(t, "2026-10-10 11:30"))
	ref := "pgw:" + f.schoolID + ":2026-10-10"
	ctx := context.Background()

	if err := f.app.runGatewayJournal(ctx, wib(t, "2026-10-11 01:00")); err != nil {
		t.Fatal(err)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM gateway_journal_postings WHERE reference = $1`, ref); n != "0" {
		t.Fatal("no journal may be guessed without gateway_accounts")
	}
	if n := f.core.count("/internal/exceptions", `"kind":"journal_posting"`); n < 1 {
		t.Fatal("missing mapping must raise a journal_posting exception")
	}
	f.exec(`INSERT INTO gateway_accounts (school_id, clearing_acc, fee_expense_acc, receivable_acc, deposit_acc) VALUES ($1, 'C', 'F', 'R', 'D')`, f.schoolID)
	for i := 0; i < 2; i++ {
		if err := f.app.runGatewayJournal(ctx, wib(t, "2026-10-11 01:00")); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.ReplaceAll(f.scalar(`SELECT lines::text FROM gateway_journal_postings WHERE reference = $1`, ref), " ", "")
	if !strings.Contains(lines, `"debit_idr":200000`) || !strings.Contains(lines, `"credit_idr":200000`) {
		t.Fatalf("one journal for both bills transactions (top-up excluded), got %s", lines)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM gateway_journal_postings WHERE school_id = $1`, f.schoolID); n != "1" {
		t.Fatalf("postings=%s want 1", n)
	}
}

// ---- routing endpoints ---------------------------------------------------------

func TestRoutingRuleEndpoints(t *testing.T) {
	f := newExtFixture(t)
	b := f.provider(&gateway.Sim{ProviderCode: uniqueCode("b")}, gateway.CapVA)
	va := f.method("va_p", gateway.CapVA, "P")
	user := genUUID()
	if st, _, raw := f.do(http.MethodPost, "/admin/payment-routing-rules", common.RoleFinanceManager, user,
		map[string]any{"method_kind": "va", "primary_provider": b, "fallback_provider": b, "failover_enabled": true}); st != http.StatusBadRequest {
		t.Fatalf("same fallback=%d %s", st, raw)
	}
	if st, _, raw := f.do(http.MethodPost, "/admin/payment-routing-rules", common.RoleFinanceManager, user,
		map[string]any{"method_kind": "qris", "primary_provider": b}); st != http.StatusBadRequest {
		t.Fatalf("provider without capability=%d %s", st, raw)
	}
	if st, _, raw := f.do(http.MethodPost, "/admin/payment-routing-rules", common.RoleFinanceManager, user,
		map[string]any{"method_kind": "va", "primary_provider": b}); st != http.StatusCreated {
		t.Fatalf("create=%d %s", st, raw)
	}
	_, data, raw := f.do(http.MethodPost, "/admin/payment-routing-rules/preview", common.RoleFinanceManager, user, map[string]any{"method_id": va})
	dec, _ := data["decision"].(map[string]any)
	if dec == nil || dec["primary"] != b {
		t.Fatalf("preview=%s", raw)
	}
	if st, _, _ := f.do(http.MethodPost, "/admin/payment-routing-rules", common.RoleParent, f.guardianID,
		map[string]any{"method_kind": "va", "primary_provider": b}); st != http.StatusForbidden {
		t.Fatalf("parent create=%d want 403", st)
	}
	if n := f.scalar(`SELECT COUNT(*)::text FROM billing_audit_events WHERE school_id = $1 AND aggregate_type = 'payment_routing_rule'`, f.schoolID); n != "1" {
		t.Fatalf("audit rows=%s want 1", n)
	}
}
