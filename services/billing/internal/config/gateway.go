package config

import (
	"errors"
	"os"
	"strings"
	"time"
	_ "time/tzdata"
)

// Gateway holds the payment-gateway (T6) settings. NICEPay credentials have
// no defaults: either every required NICEPAY_* variable is set or none is
// (gateway endpoints then answer 503 instead of pretending to work).
type Gateway struct {
	NicepayBaseURL      string
	NicepayClientID     string
	NicepayChannelID    string
	NicepayClientSecret string
	NicepayPrivateKey   string
	NicepayPublicKey    string
	NicepayStoreID      string // QRIS storeId; QRIS checkout is refused when empty
	NicepayCallbackURL  string // public URL of POST /webhooks/nicepay (dbProcessUrl)
	NicepayReturnURL    string // browser return base; the transaction id is appended
	NicepayImageURL     string // cartData img_url, mandatory for e-wallet

	CoreURL            string
	NotificationURL    string
	AccountingURL      string
	BillingInternalURL string // base URL core uses to call approval callbacks

	Location    *time.Location
	JobInterval time.Duration

	// QA-PAY BUG-004: the payment simulator is off unless explicitly enabled
	// with PAYMENT_SIMULATOR_ENABLED=true (refused when APP_ENV=production)
	// and is never a fallback for a provider without credentials. Its
	// webhooks are verified with HMAC-SHA256 over the body using
	// PAYMENT_SIM_WEBHOOK_SECRET (header X-SIGNATURE, hex).
	SimulatorEnabled bool
	SimWebhookSecret string

	// T14: simulator switches (non-production only) for failover and
	// failed-debit drills, and how many past days the daily gateway journal
	// job re-checks for days it has not journaled yet.
	SimFailCharges      bool
	SimFailDebits       bool
	JournalLookbackDays int
}

// NicepayConfigured reports whether NICEPay credentials are present.
func (g Gateway) NicepayConfigured() bool { return g.NicepayPrivateKey != "" }

func loadGateway() (Gateway, error) {
	g := Gateway{
		NicepayBaseURL:      strings.TrimRight(os.Getenv("NICEPAY_BASE_URL"), "/"),
		NicepayClientID:     os.Getenv("NICEPAY_CLIENT_ID"),
		NicepayChannelID:    os.Getenv("NICEPAY_CHANNEL_ID"),
		NicepayClientSecret: os.Getenv("NICEPAY_CLIENT_SECRET"),
		NicepayPrivateKey:   os.Getenv("NICEPAY_PRIVATE_KEY"),
		NicepayPublicKey:    os.Getenv("NICEPAY_PUBLIC_KEY"),
		NicepayStoreID:      os.Getenv("NICEPAY_STORE_ID"),
		NicepayCallbackURL:  os.Getenv("NICEPAY_CALLBACK_URL"),
		NicepayReturnURL:    strings.TrimRight(os.Getenv("NICEPAY_RETURN_URL"), "/"),
		NicepayImageURL:     os.Getenv("NICEPAY_IMAGE_URL"),
		CoreURL:             os.Getenv("CORE_URL"),
		NotificationURL:     os.Getenv("NOTIFICATION_URL"),
		AccountingURL:       os.Getenv("ACCOUNTING_URL"),
		BillingInternalURL:  strings.TrimRight(os.Getenv("BILLING_INTERNAL_URL"), "/"),
		JobInterval:         time.Duration(integer("GATEWAY_JOB_INTERVAL_SECONDS", 60)) * time.Second,
		SimulatorEnabled:    os.Getenv("PAYMENT_SIMULATOR_ENABLED") == "true",
		SimWebhookSecret:    os.Getenv("PAYMENT_SIM_WEBHOOK_SECRET"),
		SimFailCharges:      os.Getenv("PAYMENT_SIM_FAIL_CHARGES") == "true",
		SimFailDebits:       os.Getenv("PAYMENT_SIM_FAIL_DEBITS") == "true",
		JournalLookbackDays: integer("GATEWAY_JOURNAL_LOOKBACK_DAYS", 7),
	}
	loc, err := time.LoadLocation(env("APP_TIMEZONE", "Asia/Jakarta"))
	if err != nil {
		return g, errors.New("APP_TIMEZONE is not a valid IANA zone")
	}
	g.Location = loc

	required := map[string]string{
		"NICEPAY_BASE_URL": g.NicepayBaseURL, "NICEPAY_CLIENT_ID": g.NicepayClientID,
		"NICEPAY_CHANNEL_ID": g.NicepayChannelID, "NICEPAY_CLIENT_SECRET": g.NicepayClientSecret,
		"NICEPAY_PRIVATE_KEY": g.NicepayPrivateKey, "NICEPAY_PUBLIC_KEY": g.NicepayPublicKey,
		"NICEPAY_CALLBACK_URL": g.NicepayCallbackURL, "NICEPAY_RETURN_URL": g.NicepayReturnURL,
	}
	set, missing := 0, []string{}
	for k, v := range required {
		if v != "" {
			set++
		} else {
			missing = append(missing, k)
		}
	}
	if set > 0 && len(missing) > 0 {
		return g, errors.New("NICEPay is partially configured; missing " + strings.Join(missing, ", "))
	}
	if g.SimulatorEnabled && len(g.SimWebhookSecret) < 16 {
		return g, errors.New("PAYMENT_SIMULATOR_ENABLED requires PAYMENT_SIM_WEBHOOK_SECRET (min 16 characters)")
	}
	return g, nil
}

// NicepayCredentials are the NICEPay settings of one provider instance,
// read from <credential_ref>_* (FR-150-001: credentials live in the secret
// manager / env, never in the DB). The 'NICEPAY' prefix is the T6 set.
type NicepayCredentials struct {
	BaseURL, ClientID, ChannelID, ClientSecret, PrivateKey, PublicKey string
	StoreID, CallbackURL, ReturnURL, ImageURL                        string
}

// NicepayFromEnv reads <prefix>_BASE_URL, <prefix>_CLIENT_ID, ...
func NicepayFromEnv(prefix string) NicepayCredentials {
	get := func(k string) string { return strings.TrimSpace(os.Getenv(prefix + "_" + k)) }
	return NicepayCredentials{
		BaseURL: strings.TrimRight(get("BASE_URL"), "/"), ClientID: get("CLIENT_ID"), ChannelID: get("CHANNEL_ID"),
		ClientSecret: get("CLIENT_SECRET"), PrivateKey: get("PRIVATE_KEY"), PublicKey: get("PUBLIC_KEY"),
		StoreID: get("STORE_ID"), CallbackURL: get("CALLBACK_URL"), ReturnURL: strings.TrimRight(get("RETURN_URL"), "/"),
		ImageURL: get("IMAGE_URL"),
	}
}

// Installed reports whether every required credential is present.
func (c NicepayCredentials) Installed() bool {
	for _, v := range []string{c.BaseURL, c.ClientID, c.ChannelID, c.ClientSecret, c.PrivateKey, c.PublicKey, c.CallbackURL, c.ReturnURL} {
		if v == "" {
			return false
		}
	}
	return true
}
