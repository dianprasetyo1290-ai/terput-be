// Package gateway holds the payment-provider adapters (IF-ADV-03, OQ-41)
// and the pure routing, failover, reconciliation-grouping and autodebit
// rules of T14. Handlers own all I/O; this package never touches the DB.
//
// T6 decided "no Provider interface" because NICEPay was the only
// implementation. T14 has two real ones (nicepay, sim) and the PRD asks for
// an adapter per provider, so the interface is justified now.
package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Capabilities a provider may offer (payment_provider_capabilities).
const (
	CapVA        = "va"
	CapQRIS      = "qris"
	CapEWallet   = "ewallet"
	CapRetail    = "retail"
	CapAutodebit = "autodebit"
)

// AllCapabilities lists every capability in display order.
var AllCapabilities = []string{CapVA, CapQRIS, CapEWallet, CapRetail, CapAutodebit}

// Adapter names (payment_providers.adapter).
const (
	AdapterNicepay = "nicepay"
	AdapterSim     = "sim"
)

// AdapterCapabilities is what each adapter implementation can do at all. A
// provider row may enable a subset, never more.
func AdapterCapabilities(adapter string) []string {
	switch adapter {
	case AdapterNicepay:
		return []string{CapVA, CapQRIS, CapEWallet}
	case AdapterSim:
		return AllCapabilities
	}
	return nil
}

// Status is a provider-neutral transaction status.
type Status string

const (
	StatusPending Status = "pending"
	StatusPaid    Status = "paid"
	StatusFailed  Status = "failed"
	StatusExpired Status = "expired"
)

// Errors shared by adapters.
var (
	ErrUnsupported      = errors.New("fitur tidak didukung provider ini")
	ErrInvalidSignature = errors.New("signature notifikasi tidak valid")
	ErrNotRouted        = errors.New("Metode belum dirutekan")
)

// TransientError marks a provider failure worth failing over: timeout,
// network error or HTTP 5xx (FR-150-003). Anything else is a business
// rejection (amount refused, invalid account) and is not retried elsewhere.
type TransientError struct{ Err error }

func (e *TransientError) Error() string { return e.Err.Error() }
func (e *TransientError) Unwrap() error { return e.Err }

// IsTransient reports whether err is a provider outage rather than a
// business rejection.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	var te *TransientError
	if errors.As(err, &te) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	var ue *url.Error
	return errors.As(err, &ue)
}

// HTTPStatusTransient reports whether an HTTP status means "provider down".
func HTTPStatusTransient(code int) bool {
	return code >= http.StatusInternalServerError || code == http.StatusTooManyRequests
}

// ChargeRequest is one payment-code creation.
type ChargeRequest struct {
	RefNo              string // our tx_no, sent as partner reference
	AmountIDR          int64
	Kind               string // va | qris | ewallet | retail
	ProviderMethodCode string // bankCd / mitraCd / outlet code at this provider
	ValidFor           time.Duration
	GoodsName          string
	BuyerName          string
	BuyerPhone         string
	BuyerEmail         string
	ClientIP           string
}

// Charge is what the payer needs to pay.
type Charge struct {
	ProviderRef  string
	VANumber     string
	QRContent    string
	RedirectURL  string
	PayCode      string // retail payment code
	RetailOutlet string
	ExpiresAt    time.Time // zero = use the method's validity
	Status       Status    // set when the provider answers synchronously (debit)
}

// StatusQuery identifies an earlier charge at its provider.
type StatusQuery struct {
	Kind        string
	RefNo       string
	ProviderRef string
	PayCode     string // VA number, for VA queries/cancel
	AmountIDR   int64
	// ClaimedStatus is what the notification body said. Real adapters MUST
	// ignore it (never trust the body); only the simulator echoes it.
	ClaimedStatus Status
}

// Notification is a verified webhook. Refs are filled even when
// verification fails so the rejection can be logged.
type Notification struct {
	RefNo         string // partner reference (our tx_no)
	ProviderRef   string
	ClaimedStatus Status
	MandateRef    string
	MandateStatus string // active | cancelled | failed (mandate webhooks)
}

// MandateRequest registers an autodebit mandate at the provider.
type MandateRequest struct {
	RefNo        string
	BuyerName    string
	MaxAmountIDR int64
}

// MandateSetup is the provider's answer to a mandate registration.
type MandateSetup struct {
	ProviderMandateRef string
	RedirectURL        string
	AccountMasked      string
	Active             bool // provider activated it immediately
}

// DebitRequest debits an active mandate.
type DebitRequest struct {
	RefNo              string
	ProviderMandateRef string
	AmountIDR          int64
	GoodsName          string
}

// Provider is one payment-gateway adapter.
type Provider interface {
	Code() string
	Capabilities() []string
	CreateCharge(ctx context.Context, req ChargeRequest) (Charge, error)
	QueryStatus(ctx context.Context, q StatusQuery) (Status, error)
	Cancel(ctx context.Context, q StatusQuery) error
	VerifyWebhook(headers http.Header, body []byte) (Notification, error)
	CreateMandate(ctx context.Context, req MandateRequest) (MandateSetup, error)
	Debit(ctx context.Context, req DebitRequest) (Charge, error)
	CancelMandate(ctx context.Context, providerMandateRef string) error
}

// HasCapability reports whether caps contains c.
func HasCapability(caps []string, c string) bool {
	for _, x := range caps {
		if x == c {
			return true
		}
	}
	return false
}
