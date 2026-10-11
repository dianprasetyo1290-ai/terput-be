package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/billing/internal/common"
	"skci/billing/internal/config"
	"skci/billing/internal/nicepay"
	"skci/billing/internal/storage"
)

func genUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		dsn = "postgres://localhost/skci_t6_billing?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("skipping DB test, cannot open DB: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("skipping DB test, cannot ping DB: %v", err)
	}
	return db
}

// TestReceiptAtomicSequence verifies that 50 concurrent goroutines generate
// 50 unique receipt numbers sequentially with zero duplicates or collisions.
func TestReceiptAtomicSequence(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID := genUUID()
	now := time.Now()

	const concurrency = 50
	var wg sync.WaitGroup
	wg.Add(concurrency)

	results := make(chan string, concurrency)
	errorsChan := make(chan error, concurrency)

	app := &App{DB: db}
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			no, err := app.atomicNextReceiptNo(context.Background(), schoolID, now.Year())
			if err != nil {
				errorsChan <- err
				return
			}
			results <- no
		}()
	}

	wg.Wait()
	close(results)
	close(errorsChan)

	for err := range errorsChan {
		t.Fatalf("atomicNextReceiptNo failed: %v", err)
	}

	seen := make(map[string]bool)
	count := 0
	for no := range results {
		count++
		if seen[no] {
			t.Errorf("duplicate receipt number detected: %s", no)
		}
		seen[no] = true
	}

	if count != concurrency {
		t.Fatalf("expected %d receipts, got %d", concurrency, count)
	}
}

func setupTestApp(t *testing.T, db *sql.DB, guardianID, studentID string) (*App, *httptest.Server) {
	t.Helper()
	tmpDir := t.TempDir()
	store := storage.NewLocal(tmpDir, 10*1024*1024)
	cfg := config.Config{
		JWTSecret: testSecret,
	}
	identity := stubIdentity{
		allowed: map[string]bool{
			guardianID + "|" + studentID: true,
		},
	}
	app := New(db, cfg, identity, store)

	r := chi.NewRouter()

	// Webhook (public/SNAP signed)
	r.Post("/webhooks/nicepay", app.NicepayWebhook)

	// Protected routes
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		// Parent routes
		pr.Get("/payment-methods", app.ListPaymentMethods)
		pr.Post("/parent/checkout/preview", app.ParentCheckoutPreview)
		pr.Post("/parent/checkout", app.ParentCheckout)
		pr.Get("/parent/transactions/{id}", app.ParentGetTransaction)
		// Cashier & Admin routes
		pr.Post("/cashier/payments", app.CashierCreatePayment)
		pr.Get("/cashier/payments", app.CashierListPayments)
		pr.Post("/admin/payment-corrections", app.AdminCreateCorrection)
		pr.Post("/admin/reconciliation/run", app.AdminRunReconciliation)
		pr.Get("/admin/reconciliation/exceptions", app.AdminListExceptions)
		pr.Get("/admin/payment-dashboard", app.AdminPaymentDashboard)
	})

	srv := httptest.NewServer(r)
	return app, srv
}

func TestParentCheckoutFlow(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID := genUUID()
	studentID := genUUID()
	guardianID := genUUID()
	methodID := genUUID()
	schemeID := genUUID()
	academicYearID := genUUID()
	billID := genUUID()

	_, srv := setupTestApp(t, db, guardianID, studentID)
	defer srv.Close()

	// 1. Insert Payment Method
	_, err := db.Exec(`
		INSERT INTO payment_methods (
			id, school_id, code, kind, provider_code, label, enabled,
			fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort
		) VALUES (
			$1, $2, 'VA_BCA', 'va', '014', 'BCA Virtual Account', true,
			'flat', 4000, 0, 'parent', 1440, 1
		)
	`, methodID, schoolID)
	if err != nil {
		t.Fatalf("insert payment method: %v", err)
	}

	// 2. Insert Billing Scheme & Bill
	_, err = db.Exec(`
		INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP 2026/2027', 'active', true, 10)
	`, schemeID, schoolID, academicYearID)
	if err != nil {
		t.Fatalf("insert scheme: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO student_bills (
			id, school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at
		) VALUES (
			$1, $2, $3, $4, 2026, 7,
			500000, 0, 500000, 'unpaid', '2026-07-10T00:00:00Z'
		)
	`, billID, schoolID, studentID, schemeID)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	parentToken := signToken(t, guardianID, schoolID, common.RoleParent)

	// 3. Checkout Preview (AC-27-01)
	previewPayload, _ := json.Marshal(map[string]any{
		"student_user_id": studentID,
		"bill_ids":        []string{billID},
		"method_id":       methodID,
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/parent/checkout/preview", bytes.NewReader(previewPayload))
	req.Header.Set("Authorization", "Bearer "+parentToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("checkout preview request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d want 200, body=%s", resp.StatusCode, body)
	}

	// 4. Perform Checkout
	checkoutPayload, _ := json.Marshal(map[string]any{
		"student_user_id": studentID,
		"bill_ids":        []string{billID},
		"method_id":       methodID,
	})
	reqCheckout, _ := http.NewRequest(http.MethodPost, srv.URL+"/parent/checkout", bytes.NewReader(checkoutPayload))
	reqCheckout.Header.Set("Authorization", "Bearer "+parentToken)
	reqCheckout.Header.Set("Content-Type", "application/json")

	respCheckout, err := http.DefaultClient.Do(reqCheckout)
	if err != nil {
		t.Fatalf("checkout request: %v", err)
	}
	defer respCheckout.Body.Close()
	bodyCheckout, _ := io.ReadAll(respCheckout.Body)
	if respCheckout.StatusCode != http.StatusOK && respCheckout.StatusCode != http.StatusCreated {
		t.Fatalf("checkout status=%d want 200/201, body=%s", respCheckout.StatusCode, bodyCheckout)
	}

	// 5. Duplicate Checkout on the same pending bill must return 409 Conflict (AC-27-02)
	reqConflict, _ := http.NewRequest(http.MethodPost, srv.URL+"/parent/checkout", bytes.NewReader(checkoutPayload))
	reqConflict.Header.Set("Authorization", "Bearer "+parentToken)
	reqConflict.Header.Set("Content-Type", "application/json")

	respConflict, err := http.DefaultClient.Do(reqConflict)
	if err != nil {
		t.Fatalf("checkout conflict request: %v", err)
	}
	defer respConflict.Body.Close()
	bodyConflict, _ := io.ReadAll(respConflict.Body)
	if respConflict.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate checkout status=%d want 409, body=%s", respConflict.StatusCode, bodyConflict)
	}
}

// TestNicepayWebhookSecurity verifies that invalid signature returns 401 and
// writes an audit event (AC-IF-01).
func TestNicepayWebhookSecurity(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	app, srv := setupTestApp(t, db, genUUID(), genUUID())
	defer srv.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	niceClient, err := nicepay.NewClient(nicepay.Config{
		ClientID:    "TESTMERCHANT",
		PrivateKey:  base64.StdEncoding.EncodeToString(der),
		NotifPubKey: base64.StdEncoding.EncodeToString(pub),
	})
	if err != nil {
		t.Fatal(err)
	}
	app.Nicepay = niceClient

	payload := `{"trxId":"NICE123","status":"0000"}`
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhooks/nicepay", bytes.NewReader([]byte(payload)))
	req.Header.Set("Content-Type", "application/json")
	// Set an invalid signature header
	req.Header.Set("X-Signature", "invalid-signature")
	req.Header.Set("X-Timestamp", "2026-10-11T12:00:00Z")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 for invalid signature", resp.StatusCode)
	}

	// Verify notification record logged invalid signature (AC-IF-01)
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM gateway_notifications WHERE provider_ref='NICE123' AND signature_valid=false").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 invalid notification log, got %d", count)
	}
}

// TestCashierPaymentFlow tests Cashier recording payment with automatic receipt generation (AC-29-03).
func TestCashierPaymentFlow(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID := genUUID()
	studentID := genUUID()
	cashierID := genUUID()
	schemeID := genUUID()
	academicYearID := genUUID()
	billID := genUUID()

	_, srv := setupTestApp(t, db, genUUID(), studentID)
	defer srv.Close()

	_, err := db.Exec(`
		INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP Kasir', 'active', true, 10)
	`, schemeID, schoolID, academicYearID)
	if err != nil {
		t.Fatalf("insert scheme: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO student_bills (
			id, school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at
		) VALUES (
			$1, $2, $3, $4, 2026, 7,
			300000, 0, 300000, 'unpaid', '2026-07-10T00:00:00Z'
		)
	`, billID, schoolID, studentID, schemeID)
	if err != nil {
		t.Fatalf("insert bill: %v", err)
	}

	cashierToken := signToken(t, cashierID, schoolID, common.RoleKasir)

	payload, _ := json.Marshal(map[string]any{
		"student_user_id": studentID,
		"method":          "cash",
		"amount_idr":      300000,
		"note":            "Pembayaran tunai kasir",
	})

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/cashier/payments", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+cashierToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("cashier payment request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("cashier payment status=%d want 200/201, body=%s", resp.StatusCode, body)
	}

	var res struct {
		Data struct {
			ReceiptNo string `json:"receipt_no"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &res)
	if res.Data.ReceiptNo == "" {
		t.Fatalf("expected receipt number in response, got empty")
	}

	// Verify bill was updated to paid
	var billStatus string
	_ = db.QueryRow("SELECT status FROM student_bills WHERE id=$1", billID).Scan(&billStatus)
	if billStatus != "paid" {
		t.Fatalf("billStatus=%s want paid", billStatus)
	}
}

// TestAdminReconciliationRun verifies settlement reconciliation produces exceptions
// when items are missing or mismatched (AC-31-01).
func TestAdminReconciliationRun(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID := genUUID()
	batchID := genUUID()
	itemID := genUUID()

	_, srv := setupTestApp(t, db, genUUID(), genUUID())
	defer srv.Close()

	// Create a settlement batch
	_, err := db.Exec(`
		INSERT INTO settlement_batches (
			id, school_id, provider, channel, settled_on,
			gross_idr, fee_idr, net_idr, bank_account, status
		) VALUES (
			$1, $2, 'nicepay', 'va', '2026-07-15',
			100000, 4000, 96000, 'BCA-123456', 'reported'
		)
	`, batchID, schoolID)
	if err != nil {
		t.Fatalf("insert batch: %v", err)
	}

	// Insert item that does NOT exist in Sekoci (missing_in_sekoci)
	_, err = db.Exec(`
		INSERT INTO settlement_items (
			id, school_id, batch_id, provider_ref, amount_idr, fee_idr
		) VALUES (
			$1, $2, $3, 'ORDER-UNKNOWN-999', 100000, 4000
		)
	`, itemID, schoolID, batchID)
	if err != nil {
		t.Fatalf("insert settlement item: %v", err)
	}

	financeToken := signToken(t, genUUID(), schoolID, common.RoleFinanceManager)

	runPayload, _ := json.Marshal(map[string]any{
		"batch_id": batchID,
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/reconciliation/run", bytes.NewReader(runPayload))
	req.Header.Set("Authorization", "Bearer "+financeToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reconciliation run request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reconciliation run status=%d want 200, body=%s", resp.StatusCode, body)
	}

	// Verify exception was generated in DB
	var excCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM reconciliation_exceptions WHERE school_id=$1 AND kind='missing_in_sekoci'", schoolID).Scan(&excCount)
	if excCount != 1 {
		t.Fatalf("expected 1 missing_in_sekoci exception, got %d", excCount)
	}
}

// TestAdminCreateCorrection verifies that requesting a correction on a payment works (AC-29-04).
func TestAdminCreateCorrection(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID := genUUID()
	cashierPaymentID := genUUID()
	studentID := genUUID()
	financeID := genUUID()

	_, srv := setupTestApp(t, db, genUUID(), studentID)
	defer srv.Close()

	// Insert cashier payment
	_, err := db.Exec(`
		INSERT INTO cashier_payments (
			id, school_id, student_user_id, method, amount_idr, received_by,
			note, status, idempotency_key
		) VALUES (
			$1, $2, $3, 'cash', 100000, $4, 'Uang kasir', 'recorded', 'idem-corr-1'
		)
	`, cashierPaymentID, schoolID, studentID, financeID)
	if err != nil {
		t.Fatalf("insert cashier payment: %v", err)
	}

	financeToken := signToken(t, financeID, schoolID, common.RoleFinanceManager)

	payload, _ := json.Marshal(map[string]any{
		"source":    "cashier",
		"source_id": cashierPaymentID,
		"kind":      "cancel",
		"reason":    "Koreksi pembayaran kasir",
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/admin/payment-corrections", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+financeToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("correction request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("correction status=%d want 200/201, body=%s", resp.StatusCode, body)
	}

	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM payment_corrections WHERE school_id=$1 AND source_id=$2", schoolID, cashierPaymentID).Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 payment correction, got %d", count)
	}
}

// TestNicepayWebhookIdempotency verifies AC-IF-02 and BR-PAY-01:
// duplicate webhook calls for an already successful transaction are idempotent and do not double-allocate.
func TestNicepayWebhookIdempotency(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	schoolID := genUUID()
	studentID := genUUID()
	methodID := genUUID()
	txID := genUUID()
	billID := genUUID()

	_, srv := setupTestApp(t, db, genUUID(), studentID)
	defer srv.Close()

	// 1. Method
	_, err := db.Exec(`
		INSERT INTO payment_methods (
			id, school_id, code, kind, provider_code, label, enabled,
			fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort
		) VALUES (
			$1, $2, 'VA_BNI', 'va', '009', 'BNI Virtual Account', true,
			'flat', 4000, 0, 'parent', 1440, 1
		)
	`, methodID, schoolID)
	if err != nil {
		t.Fatalf("insert method: %v", err)
	}

	// 2. Bill
	schemeID := genUUID()
	academicYearID := genUUID()
	_, _ = db.Exec(`INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day) VALUES ($1, $2, $3, 'SPP', 'active', true, 10)`, schemeID, schoolID, academicYearID)
	_, _ = db.Exec(`INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month, base_idr, discount_idr, net_idr, status, due_at) VALUES ($1, $2, $3, $4, 2026, 8, 600000, 0, 600000, 'unpaid', '2026-08-10T00:00:00Z')`, billID, schoolID, studentID, schemeID)

	// 3. Insert transaction already success (AC-IF-02)
	now := time.Now()
	txNo := "TX-" + genUUID()[:8]
	providerRef := "NICE-" + genUUID()[:8]
	_, err = db.Exec(`
		INSERT INTO gateway_transactions (
			id, school_id, tx_no, payer_user_id, student_user_id, method_id,
			bills_amount_idr, fee_idr, total_idr, fee_bearer, status,
			provider_ref, expires_at, paid_at, idempotency_key
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			600000, 4000, 604000, 'parent', 'success',
			$7, $8, $8, $9
		)
	`, txID, schoolID, txNo, genUUID(), studentID, methodID, providerRef, now, "idem-"+txID)
	if err != nil {
		t.Fatalf("insert success tx: %v", err)
	}

	_, _ = db.Exec(`INSERT INTO gateway_transaction_bills (transaction_id, bill_id, position, amount_idr) VALUES ($1, $2, 1, 600000)`, txID, billID)

	// 4. Send webhook for TX-002
	payload, _ := json.Marshal(map[string]any{
		"trxId":                   txNo,
		"originalReferenceNo":     providerRef,
		"latestTransactionStatus": "00",
		"amount": map[string]string{
			"value":    "604000.00",
			"currency": "IDR",
		},
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhooks/nicepay", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("webhook request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("webhook status=%d want 200", resp.StatusCode)
	}

	// 5. Send duplicate webhook again (AC-IF-02)
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhooks/nicepay", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("duplicate webhook request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("duplicate webhook status=%d want 200", resp2.StatusCode)
	}

	// Verify no double payments (AC-IF-02)
	var pCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM bill_payments WHERE student_bill_id = $1", billID).Scan(&pCount)
	if pCount > 1 {
		t.Fatalf("expected at most 1 payment record, got %d", pCount)
	}
}

