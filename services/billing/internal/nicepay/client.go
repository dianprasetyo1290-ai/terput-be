// Package nicepay wraps NICEPay's SNAP (Bank Indonesia Open API standard)
// endpoints for Virtual Account, QRIS and e-wallet direct debit.
// Docs: https://docs.nicepay.co.id/nicepay-api-snap
package nicepay

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Transaction status codes (latestTransactionStatus), shared by VA/QRIS/e-wallet.
const (
	StatusSuccess   = "00"
	StatusInitiated = "01"
	StatusPending   = "03"
	StatusRefunded  = "04"
	StatusCanceled  = "05" // also "expired"
	StatusFailed    = "06"
)

var wib = time.FixedZone("WIB", 7*3600)

type Config struct {
	BaseURL      string // https://dev.nicepay.co.id | https://www.nicepay.co.id
	ClientID     string // X-PARTNER-ID / X-CLIENT-KEY, also the merchantId (iMid)
	ChannelID    string
	ClientSecret string
	PrivateKey   string // merchant RSA private key, base64 PKCS#8 (PEM armor optional)
	NotifPubKey  string // NICEPay's RSA public key for notifications, base64 PKIX
	StoreID      string // QRIS storeId / externalStoreId
}

type Client struct {
	cfg        Config
	privKey    *rsa.PrivateKey
	notifKey   *rsa.PublicKey
	httpClient *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func NewClient(cfg Config) (*Client, error) {
	priv, err := parsePrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("nicepay: private key: %w", err)
	}
	c := &Client{cfg: cfg, privKey: priv, httpClient: &http.Client{Timeout: 20 * time.Second}}
	if cfg.NotifPubKey != "" {
		if c.notifKey, err = parsePublicKey(cfg.NotifPubKey); err != nil {
			return nil, fmt.Errorf("nicepay: notification public key: %w", err)
		}
	}
	return c, nil
}

func stripPEM(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "-----") {
			b.WriteString(strings.TrimSpace(line))
		}
	}
	return b.String()
}

func parsePrivateKey(s string) (*rsa.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(stripPEM(s))
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk, nil
		}
		return nil, errors.New("not an RSA key")
	}
	return x509.ParsePKCS1PrivateKey(der)
}

func parsePublicKey(s string) (*rsa.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(stripPEM(s))
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA key")
	}
	return rk, nil
}

func Timestamp(t time.Time) string { return t.In(wib).Format("2006-01-02T15:04:05-07:00") }

// Amount formats integer rupiah the way SNAP wants it: "10000.00".
func Amount(v int64) string { return fmt.Sprintf("%d.00", v) }

type money struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

func idr(v int64) money { return money{Value: Amount(v), Currency: "IDR"} }

// billingPhone formats a phone the way QRIS/e-wallet accept it: digits only,
// local 0-prefix ("+62 812-1000" -> "08121000"). The field is mandatory there
// but optional on our customers, so an empty one gets a placeholder.
func billingPhone(p string) string {
	d := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, p)
	if strings.HasPrefix(d, "62") {
		d = "0" + d[2:]
	}
	if len(d) < 9 {
		return "080000000000"
	}
	return d
}

// accessToken returns a cached B2B token, refreshing it a minute before its
// 15-minute expiry. Signature: SHA256withRSA(privateKey, clientId|timestamp).
func (c *Client) accessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}

	ts := Timestamp(time.Now())
	digest := sha256.Sum256([]byte(c.cfg.ClientID + "|" + ts))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest(http.MethodPost, c.cfg.BaseURL+"/nicepay/v1.0/access-token/b2b",
		strings.NewReader(`{"grantType":"client_credentials","additionalInfo":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-CLIENT-KEY", c.cfg.ClientID)
	req.Header.Set("X-SIGNATURE", base64.StdEncoding.EncodeToString(sig))

	var out struct {
		ResponseCode    string `json:"responseCode"`
		ResponseMessage string `json:"responseMessage"`
		AccessToken     string `json:"accessToken"`
	}
	if err := c.send(req, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("nicepay: token %s %s", out.ResponseCode, out.ResponseMessage)
	}
	c.token, c.tokenExp = out.AccessToken, time.Now().Add(14*time.Minute)
	return c.token, nil
}

// SymmetricSignature is the X-SIGNATURE for transactional calls:
// base64(HMAC-SHA512(clientSecret, METHOD:path:token:hex(sha256(body)):timestamp)).
// path is the endpoint WITHOUT the /nicepay prefix.
func SymmetricSignature(secret, method, path, token string, body []byte, ts string) string {
	sum := sha256.Sum256(body)
	m := hmac.New(sha512.New, []byte(secret))
	m.Write([]byte(method + ":" + path + ":" + token + ":" + hex.EncodeToString(sum[:]) + ":" + ts))
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

type snapResponse interface{ code() (string, string) }

type baseResp struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
}

func (b baseResp) code() (string, string) { return b.ResponseCode, b.ResponseMessage }

func (c *Client) post(path string, body any, out snapResponse) error {
	return c.call(http.MethodPost, path, body, out)
}

func (c *Client) call(method, path string, body any, out snapResponse) error {
	token, err := c.accessToken()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ts := Timestamp(time.Now())
	req, _ := http.NewRequest(method, c.cfg.BaseURL+"/nicepay"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-SIGNATURE", SymmetricSignature(c.cfg.ClientSecret, method, path, token, raw, ts))
	req.Header.Set("X-PARTNER-ID", c.cfg.ClientID)
	req.Header.Set("X-EXTERNAL-ID", fmt.Sprintf("%d", time.Now().UnixNano()))
	req.Header.Set("CHANNEL-ID", c.cfg.ChannelID)

	if err := c.send(req, out); err != nil {
		return err
	}
	if rc, msg := out.code(); !strings.HasPrefix(rc, "200") {
		return &APIError{Path: path, Code: rc, Message: msg}
	}
	return nil
}

// APIError is a SNAP response whose responseCode is not 200xxxx. The first
// three digits of a SNAP code are the HTTP status class.
type APIError struct {
	Path, Code, Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("nicepay %s: %s %s", e.Path, e.Code, e.Message) }

// ServerSide reports a 5xx SNAP code (NICEPay-side outage).
func (e *APIError) ServerSide() bool { return strings.HasPrefix(e.Code, "5") }

// HTTPError is a response that was not SNAP JSON at all.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("nicepay: HTTP %d: %s", e.Status, e.Body) }

func (c *Client) send(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return &HTTPError{Status: resp.StatusCode, Body: string(b)}
	}
	return nil
}

// Order is what every create call needs to know about the purchase.
type Order struct {
	RefNo      string // our partnerReferenceNo / trxId, unique per attempt
	Amount     int64 // integer rupiah
	GoodsName  string
	BuyerName  string
	BuyerPhone string
	BuyerEmail string
	NotifyURL  string // dbProcessUrl
	ReturnURL  string // browser redirect after e-wallet payment
	ClientIP   string
	ImageURL   string // cartData img_url, mandatory non-empty for e-wallet
}

// Instruction is what the customer needs to actually pay.
type Instruction struct {
	GatewayRef string    // NICEPay's own id (tXidVA / referenceNo) for status queries
	PayCode    string    // VA number | QRIS content | e-wallet redirect URL
	ExpiresAt  time.Time // zero if unknown
}

func (c *Client) cartData(o Order) string {
	b, _ := json.Marshal(map[string]any{"count": "1", "item": []map[string]string{{
		"img_url": o.ImageURL, "goods_name": o.GoodsName, "goods_detail": o.GoodsName,
		"goods_amt": Amount(o.Amount), "goods_quantity": "1",
	}}})
	return string(b)
}

// CreateVA registers a one-off virtual account at bankCd (BMRI, CENA, BNIN, BRIN, ...).
func (c *Client) CreateVA(o Order, bankCd string) (*Instruction, error) {
	var out struct {
		baseResp
		VirtualAccountData struct {
			VirtualAccountNo string `json:"virtualAccountNo"`
			AdditionalInfo   struct {
				TXidVA       string `json:"tXidVA"`
				VacctValidDt string `json:"vacctValidDt"`
				VacctValidTm string `json:"vacctValidTm"`
			} `json:"additionalInfo"`
		} `json:"virtualAccountData"`
	}
	err := c.post("/api/v1.0/transfer-va/create-va", map[string]any{
		"partnerServiceId": "", "customerNo": "", "virtualAccountNo": "",
		"virtualAccountName": truncate(o.BuyerName, 100), "trxId": o.RefNo,
		"totalAmount": idr(o.Amount),
		"additionalInfo": map[string]string{
			"bankCd": bankCd, "goodsNm": o.GoodsName, "dbProcessUrl": o.NotifyURL,
			"vacctValidDt": "", "vacctValidTm": "",
		},
	}, &out)
	if err != nil {
		return nil, err
	}
	d := out.VirtualAccountData
	exp, _ := time.ParseInLocation("20060102150405", d.AdditionalInfo.VacctValidDt+d.AdditionalInfo.VacctValidTm, wib)
	return &Instruction{GatewayRef: d.AdditionalInfo.TXidVA, PayCode: d.VirtualAccountNo, ExpiresAt: exp}, nil
}

// CreateQRIS generates a dynamic QRIS valid for validFor (NICEPay caps it at 20 minutes).
func (c *Client) CreateQRIS(o Order, validFor time.Duration) (*Instruction, error) {
	exp := time.Now().Add(validFor)
	var out struct {
		baseResp
		ReferenceNo string `json:"referenceNo"`
		QRContent   string `json:"qrContent"`
	}
	err := c.post("/api/v1.0/qr/qr-mpm-generate", map[string]any{
		"partnerReferenceNo": o.RefNo, "merchantId": c.cfg.ClientID, "storeId": c.cfg.StoreID,
		"amount": idr(o.Amount), "validityPeriod": Timestamp(exp),
		"additionalInfo": map[string]string{
			"goodsNm": o.GoodsName, "billingNm": o.BuyerName, "billingPhone": billingPhone(o.BuyerPhone),
			"billingEmail": o.BuyerEmail, "billingCity": "Jakarta", "billingState": "DKI Jakarta",
			"billingPostCd": "10000", "billingCountry": "Indonesia",
			"callBackUrl": o.ReturnURL, "dbProcessUrl": o.NotifyURL, "userIP": o.ClientIP,
			"cartData": c.cartData(o), "mitraCd": "QSHP",
		},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &Instruction{GatewayRef: out.ReferenceNo, PayCode: out.QRContent, ExpiresAt: exp}, nil
}

// CreateEWallet starts a direct-debit payment with mitraCd (DANA, ESHP = ShopeePay)
// and returns the wallet's checkout URL to send the customer to.
func (c *Client) CreateEWallet(o Order, mitraCd string, validFor time.Duration) (*Instruction, error) {
	exp := time.Now().Add(validFor)
	var out struct {
		baseResp
		ReferenceNo    string `json:"referenceNo"`
		WebRedirectURL string `json:"webRedirectUrl"`
	}
	err := c.post("/api/v1.0/debit/payment-host-to-host", map[string]any{
		"partnerReferenceNo": o.RefNo, "merchantId": c.cfg.ClientID, "subMerchantId": "", "externalStoreId": "",
		"validUpTo": Timestamp(exp), "amount": idr(o.Amount),
		"urlParams": []map[string]string{
			{"url": o.NotifyURL, "type": "PAY_NOTIFY", "isDeeplink": "N"},
			{"url": o.ReturnURL, "type": "PAY_RETURN", "isDeeplink": "N"},
		},
		"additionalInfo": map[string]string{
			"mitraCd": mitraCd, "goodsNm": o.GoodsName, "billingNm": o.BuyerName,
			"billingPhone": billingPhone(o.BuyerPhone), "billingEmail": o.BuyerEmail,
			"callBackUrl": o.ReturnURL, "dbProcessUrl": o.NotifyURL, "cartData": c.cartData(o),
		},
	}, &out)
	if err != nil {
		return nil, err
	}
	if out.WebRedirectURL == "" {
		return nil, fmt.Errorf("nicepay: %s returned no redirect URL", mitraCd)
	}
	return &Instruction{GatewayRef: out.ReferenceNo, PayCode: out.WebRedirectURL, ExpiresAt: exp}, nil
}

// CancelVA closes an unpaid VA so it can't be paid after the customer switched
// to another bank/method (its notification alone can't be verified: no tXidVA).
func (c *Client) CancelVA(q StatusQuery) error {
	var out baseResp
	return c.call(http.MethodDelete, "/api/v1.0/transfer-va/delete-va", map[string]any{
		"partnerServiceId": "", "customerNo": "", "virtualAccountNo": q.PayCode, "trxId": q.RefNo,
		"additionalInfo": map[string]any{"totalAmount": idr(q.Amount), "tXidVA": q.GatewayRef, "cancelMessage": "replaced"},
	}, &out)
}

// StatusQuery identifies one earlier create call.
type StatusQuery struct {
	Method     string // "va" | "qris" | "ewallet"
	RefNo      string
	GatewayRef string
	PayCode    string // VA number (VA only)
	Amount     int64 // integer rupiah
}

// Status returns latestTransactionStatus ("00" paid, "03" pending, ...).
func (c *Client) Status(q StatusQuery) (string, error) {
	switch q.Method {
	case "va":
		var out struct {
			baseResp
			AdditionalInfo struct {
				LatestTransactionStatus string `json:"latestTransactionStatus"`
			} `json:"additionalInfo"`
			VirtualAccountData struct {
				AdditionalInfo struct {
					LatestTransactionStatus string `json:"latestTransactionStatus"`
				} `json:"additionalInfo"`
			} `json:"virtualAccountData"`
		}
		err := c.post("/api/v1.0/transfer-va/status", map[string]any{
			"partnerServiceId": "", "customerNo": "", "virtualAccountNo": q.PayCode,
			"inquiryRequestId": fmt.Sprintf("%d", time.Now().UnixNano()),
			"additionalInfo":   map[string]any{"totalAmount": idr(q.Amount), "trxId": q.RefNo, "tXidVA": q.GatewayRef},
		}, &out)
		if err != nil {
			return "", err
		}
		// The docs put it at the top level, the sandbox nests it under virtualAccountData.
		if s := out.VirtualAccountData.AdditionalInfo.LatestTransactionStatus; s != "" {
			return s, nil
		}
		return out.AdditionalInfo.LatestTransactionStatus, nil
	case "qris":
		var out struct {
			baseResp
			LatestTransactionStatus string `json:"latestTransactionStatus"`
		}
		err := c.post("/api/v1.0/qr/qr-mpm-query", map[string]any{
			"originalReferenceNo": q.GatewayRef, "originalPartnerReferenceNo": q.RefNo,
			"merchantId": c.cfg.ClientID, "externalStoreId": c.cfg.StoreID, "serviceCode": "47",
			"additionalInfo": map[string]string{},
		}, &out)
		return out.LatestTransactionStatus, err
	case "ewallet":
		var out struct {
			baseResp
			LatestTransactionStatus string `json:"latestTransactionStatus"`
		}
		err := c.post("/api/v1.0/debit/status", map[string]any{
			"merchantId": c.cfg.ClientID, "subMerchantId": "", "externalStoreId": "",
			"originalReferenceNo": q.GatewayRef, "originalPartnerReferenceNo": q.RefNo,
			"serviceCode": "54", "amount": idr(q.Amount), "additionalInfo": map[string]string{},
		}, &out)
		return out.LatestTransactionStatus, err
	}
	return "", fmt.Errorf("nicepay: unsupported method %q", q.Method)
}

// VerifyNotification checks a notification's X-SIGNATURE:
// SHA256withRSA(NICEPay public key, clientId|X-TIMESTAMP). It does NOT cover
// the body, so callers must re-confirm the status via Status() before trusting it.
func (c *Client) VerifyNotification(timestamp, signature string) bool {
	if c.notifKey == nil {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(c.cfg.ClientID + "|" + timestamp))
	return rsa.VerifyPKCS1v15(c.notifKey, crypto.SHA256, digest[:], sig) == nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
