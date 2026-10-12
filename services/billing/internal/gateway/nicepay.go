package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"skci/billing/internal/nicepay"
)

// Nicepay wraps the T6 SNAP client (internal/nicepay) without rewriting it.
// Retail and autodebit are not offered: the SNAP client in this repo has no
// such API and its parameters are not invented here (OQ-41).
type Nicepay struct {
	ProviderCode string
	Client       *nicepay.Client
	NotifyURL    string // dbProcessUrl: public URL of POST /webhooks/{code}
	ReturnURL    string // browser return base
	ImageURL     string // cartData img_url (mandatory for e-wallet)
}

func (n *Nicepay) Code() string           { return n.ProviderCode }
func (n *Nicepay) Capabilities() []string { return AdapterCapabilities(AdapterNicepay) }

// classify turns a NICEPay client error into TransientError when it is an
// outage (network, timeout, HTTP/SNAP 5xx).
func classify(err error) error {
	if err == nil {
		return nil
	}
	var api *nicepay.APIError
	if errors.As(err, &api) && api.ServerSide() {
		return &TransientError{Err: err}
	}
	var he *nicepay.HTTPError
	if errors.As(err, &he) && HTTPStatusTransient(he.Status) {
		return &TransientError{Err: err}
	}
	if IsTransient(err) {
		return &TransientError{Err: err}
	}
	return err
}

func (n *Nicepay) order(req ChargeRequest) nicepay.Order {
	return nicepay.Order{
		RefNo:      req.RefNo,
		Amount:     req.AmountIDR,
		GoodsName:  req.GoodsName,
		BuyerName:  req.BuyerName,
		BuyerPhone: req.BuyerPhone,
		BuyerEmail: req.BuyerEmail,
		NotifyURL:  n.NotifyURL,
		ReturnURL:  n.ReturnURL,
		ClientIP:   req.ClientIP,
		ImageURL:   n.ImageURL,
	}
}

func (n *Nicepay) CreateCharge(_ context.Context, req ChargeRequest) (Charge, error) {
	o := n.order(req)
	var (
		ins *nicepay.Instruction
		err error
		ch  Charge
	)
	switch req.Kind {
	case CapVA:
		ins, err = n.Client.CreateVA(o, req.ProviderMethodCode)
		if err == nil {
			ch.VANumber = ins.PayCode
		}
	case CapQRIS:
		ins, err = n.Client.CreateQRIS(o, req.ValidFor)
		if err == nil {
			ch.QRContent = ins.PayCode
		}
	case CapEWallet:
		ins, err = n.Client.CreateEWallet(o, req.ProviderMethodCode, req.ValidFor)
		if err == nil {
			ch.RedirectURL = ins.PayCode
		}
	default:
		return Charge{}, ErrUnsupported
	}
	if err != nil {
		return Charge{}, classify(err)
	}
	ch.ProviderRef = ins.GatewayRef
	ch.ExpiresAt = ins.ExpiresAt
	return ch, nil
}

func (n *Nicepay) query(q StatusQuery) nicepay.StatusQuery {
	return nicepay.StatusQuery{Method: q.Kind, RefNo: q.RefNo, GatewayRef: q.ProviderRef, PayCode: q.PayCode, Amount: q.AmountIDR}
}

// QueryStatus re-asks NICEPay; ClaimedStatus is deliberately ignored.
func (n *Nicepay) QueryStatus(_ context.Context, q StatusQuery) (Status, error) {
	st, err := n.Client.Status(n.query(q))
	if err != nil {
		return "", classify(err)
	}
	return NicepayStatus(st), nil
}

// NicepayStatus maps latestTransactionStatus to a neutral status. "05"
// covers both cancelled and expired; T6 recorded it as failed.
func NicepayStatus(code string) Status {
	switch code {
	case nicepay.StatusSuccess:
		return StatusPaid
	case nicepay.StatusCanceled, nicepay.StatusFailed:
		return StatusFailed
	}
	return StatusPending
}

func (n *Nicepay) Cancel(_ context.Context, q StatusQuery) error {
	if q.Kind != CapVA {
		return nil
	}
	return classify(n.Client.CancelVA(n.query(q)))
}

type nicepayNotification struct {
	TrxID                      string `json:"trxId"`
	OriginalPartnerReferenceNo string `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string `json:"originalReferenceNo"`
}

// ParseNicepayRefs extracts (partner ref, provider ref) the way T6 did.
func ParseNicepayRefs(body []byte) (string, string) {
	var b nicepayNotification
	_ = json.Unmarshal(body, &b)
	ref := b.TrxID
	if ref == "" {
		ref = b.OriginalPartnerReferenceNo
	}
	pRef := b.OriginalReferenceNo
	if pRef == "" {
		pRef = ref
	}
	return ref, pRef
}

func (n *Nicepay) VerifyWebhook(h http.Header, body []byte) (Notification, error) {
	ref, pRef := ParseNicepayRefs(body)
	out := Notification{RefNo: ref, ProviderRef: pRef}
	if !n.Client.VerifyNotification(h.Get("X-TIMESTAMP"), h.Get("X-SIGNATURE")) {
		return out, ErrInvalidSignature
	}
	return out, nil
}

func (n *Nicepay) CreateMandate(context.Context, MandateRequest) (MandateSetup, error) {
	return MandateSetup{}, ErrUnsupported
}
func (n *Nicepay) Debit(context.Context, DebitRequest) (Charge, error) {
	return Charge{}, ErrUnsupported
}
func (n *Nicepay) CancelMandate(context.Context, string) error { return ErrUnsupported }

// String keeps credentials out of any accidental %v logging.
func (n *Nicepay) String() string { return fmt.Sprintf("nicepay-adapter(%s)", n.ProviderCode) }

var _ Provider = (*Nicepay)(nil)
