package config

import (
	"errors"
	"os"
	"strings"
	"time"
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
	return g, nil
}
