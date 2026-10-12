package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Sim is the simulation branch T6 had inline in ParentCheckout ("SIM-"
// refs), moved into an adapter. It supports every capability so dev, tests
// and Playwright can exercise routing, retail and autodebit without a real
// provider. The handler never builds it in production.
type Sim struct {
	ProviderCode string
	FailCharges  bool // every charge fails like an outage (failover drills)
	FailDebits   bool // every debit is rejected like insufficient balance
}

func (s *Sim) Code() string           { return s.ProviderCode }
func (s *Sim) Capabilities() []string { return AdapterCapabilities(AdapterSim) }

// CreateCharge reproduces T6's simulated instructions byte for byte for
// VA/QRIS/e-wallet so old responses stay identical.
func (s *Sim) CreateCharge(_ context.Context, req ChargeRequest) (Charge, error) {
	if s.FailCharges {
		return Charge{}, &TransientError{Err: errors.New("simulator: provider tidak merespons")}
	}
	ch := Charge{ProviderRef: "SIM-" + req.RefNo}
	switch req.Kind {
	case CapVA:
		ch.VANumber = "8808" + strings.TrimPrefix(req.RefNo, "TX-")
	case CapQRIS:
		ch.QRContent = "00020101021226680016ID.CO.NICEPAY.WWW0118936009990000000001520454115303360540" + fmt.Sprintf("%d", req.AmountIDR) + "5802ID"
	case CapEWallet:
		ch.RedirectURL = "https://mock-ewallet.nicepay.co.id/pay?ref=" + req.RefNo
	case CapRetail:
		ch.PayCode = "SIMR" + digits(req.RefNo)
		ch.RetailOutlet = req.ProviderMethodCode
	default:
		return Charge{}, ErrUnsupported
	}
	return ch, nil
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// QueryStatus echoes what the (unsigned) simulated notification claimed;
// without a notification the charge is still pending.
func (s *Sim) QueryStatus(_ context.Context, q StatusQuery) (Status, error) {
	if q.ClaimedStatus != "" {
		return q.ClaimedStatus, nil
	}
	return StatusPending, nil
}

func (s *Sim) Cancel(context.Context, StatusQuery) error { return nil }

type simNotification struct {
	LatestTransactionStatus string `json:"latestTransactionStatus"`
	MandateRef              string `json:"mandateRef"`
	MandateStatus           string `json:"mandateStatus"`
}

// VerifyWebhook accepts any body (simulator only). Like T6 without NICEPay
// credentials, a notification without a status means "paid".
func (s *Sim) VerifyWebhook(_ http.Header, body []byte) (Notification, error) {
	ref, pRef := ParseNicepayRefs(body)
	var b simNotification
	_ = json.Unmarshal(body, &b)
	n := Notification{RefNo: ref, ProviderRef: pRef, MandateRef: b.MandateRef, MandateStatus: b.MandateStatus}
	switch b.LatestTransactionStatus {
	case "", "00":
		n.ClaimedStatus = StatusPaid
	case "05", "06":
		n.ClaimedStatus = StatusFailed
	default:
		n.ClaimedStatus = StatusPending
	}
	return n, nil
}

func (s *Sim) CreateMandate(_ context.Context, req MandateRequest) (MandateSetup, error) {
	return MandateSetup{ProviderMandateRef: "SIM-MD-" + req.RefNo, AccountMasked: "••••0000", Active: true}, nil
}

func (s *Sim) Debit(_ context.Context, req DebitRequest) (Charge, error) {
	if s.FailDebits {
		return Charge{}, errors.New("simulator: saldo tidak mencukupi")
	}
	return Charge{ProviderRef: "SIM-" + req.RefNo, Status: StatusPaid}, nil
}

func (s *Sim) CancelMandate(context.Context, string) error { return nil }

var _ Provider = (*Sim)(nil)
