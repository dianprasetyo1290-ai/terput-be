package nicepay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"testing"
	"time"
)

// testKey builds a throwaway merchant key so unit tests never need real credentials.
func testConfig(t *testing.T) Config {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return Config{ClientID: "TESTMERCHANT", PrivateKey: base64.StdEncoding.EncodeToString(der)}
}

// sandboxConfig reads NICEPay sandbox credentials from the environment
// (same names as the service config). Nothing is baked into the repo.
func sandboxConfig(t *testing.T) Config {
	t.Helper()
	cfg := Config{
		BaseURL:      os.Getenv("NICEPAY_BASE_URL"),
		ClientID:     os.Getenv("NICEPAY_CLIENT_ID"),
		ChannelID:    os.Getenv("NICEPAY_CHANNEL_ID"),
		ClientSecret: os.Getenv("NICEPAY_CLIENT_SECRET"),
		PrivateKey:   os.Getenv("NICEPAY_PRIVATE_KEY"),
		NotifPubKey:  os.Getenv("NICEPAY_PUBLIC_KEY"),
		StoreID:      os.Getenv("NICEPAY_STORE_ID"),
	}
	if cfg.BaseURL == "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.PrivateKey == "" {
		t.Skip("NICEPAY_* sandbox credentials not set")
	}
	return cfg
}

func TestVerifyNotification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	cfg.NotifPubKey = base64.StdEncoding.EncodeToString(pub)
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ts := "2026-09-11T10:00:00+07:00"
	digest := sha256.Sum256([]byte(cfg.ClientID + "|" + ts))
	raw, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := base64.StdEncoding.EncodeToString(raw)

	if !c.VerifyNotification(ts, sig) {
		t.Fatal("valid signature rejected")
	}
	if c.VerifyNotification("2026-09-11T10:00:01+07:00", sig) {
		t.Fatal("signature accepted for a different timestamp")
	}
	if c.VerifyNotification(ts, "garbage") {
		t.Fatal("garbage signature accepted")
	}
}

func TestBillingPhone(t *testing.T) {
	for in, want := range map[string]string{
		"+628121000555":  "08121000555",
		"0812-3456-7890": "081234567890",
		"":               "080000000000",
		"+62":            "080000000000",
	} {
		if got := billingPhone(in); got != want {
			t.Errorf("billingPhone(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAmount(t *testing.T) {
	if got := Amount(100000); got != "100000.00" {
		t.Errorf("Amount(100000) = %q, want 100000.00", got)
	}
	if got := Amount(0); got != "0.00" {
		t.Errorf("Amount(0) = %q, want 0.00", got)
	}
}

// TestSandbox hits NICEPay's real sandbox: NICEPAY_SANDBOX=1 go test ./internal/nicepay/
func TestSandbox(t *testing.T) {
	if os.Getenv("NICEPAY_SANDBOX") == "" {
		t.Skip("set NICEPAY_SANDBOX=1 to run against dev.nicepay.co.id")
	}
	c, err := NewClient(sandboxConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	o := func(p string) Order {
		return Order{
			RefNo:      fmt.Sprintf("%s%d", p, time.Now().UnixNano()),
			Amount:     15000,
			GoodsName:  "Test Tagihan SPP",
			BuyerName:  "Budi Santoso",
			BuyerPhone: "081234567890",
			BuyerEmail: "budi@example.com",
			NotifyURL:  "https://example.com/notify",
			ReturnURL:  "https://example.com/return",
			ClientIP:   "127.0.0.1",
			ImageURL:   "https://example.com/logo.png",
		}
	}

	for _, bank := range []string{"BMRI", "BNIN", "BRIN", "BBBA", "BNIA"} {
		ord := o("VA")
		ins, err := c.CreateVA(ord, bank)
		if err != nil {
			t.Errorf("VA %s: %v", bank, err)
			continue
		}
		st, err := c.Status(StatusQuery{Method: "va", RefNo: ord.RefNo, GatewayRef: ins.GatewayRef, PayCode: ins.PayCode, Amount: ord.Amount})
		if err != nil || st != StatusPending || ins.ExpiresAt.IsZero() {
			t.Errorf("VA %s status=%q exp=%v err=%v", bank, st, ins.ExpiresAt, err)
		}
		q := StatusQuery{Method: "va", RefNo: ord.RefNo, GatewayRef: ins.GatewayRef, PayCode: ins.PayCode, Amount: ord.Amount}
		if err := c.CancelVA(q); err != nil {
			t.Errorf("cancel VA %s: %v", bank, err)
		} else if st, _ := c.Status(q); st != StatusCanceled {
			t.Errorf("VA %s after cancel status=%q", bank, st)
		}
	}

	ord := o("QR")
	ins, err := c.CreateQRIS(ord, 15*time.Minute)
	if err != nil || ins.PayCode == "" {
		t.Fatalf("QRIS: %v", err)
	}
	if st, err := c.Status(StatusQuery{Method: "qris", RefNo: ord.RefNo, GatewayRef: ins.GatewayRef, Amount: ord.Amount}); err != nil || st != StatusPending {
		t.Errorf("QRIS status=%q err=%v", st, err)
	}

	for _, mitra := range []string{"ESHP"} {
		ord := o("EW")
		ins, err := c.CreateEWallet(ord, mitra, 15*time.Minute)
		if err != nil {
			t.Errorf("e-wallet %s: %v", mitra, err)
			continue
		}
		st, err := c.Status(StatusQuery{Method: "ewallet", RefNo: ord.RefNo, GatewayRef: ins.GatewayRef, Amount: ord.Amount})
		if err != nil || (st != StatusPending && st != StatusInitiated) {
			t.Errorf("e-wallet %s status=%q err=%v", mitra, st, err)
		}
	}
}
