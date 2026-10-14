package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
	"skci/billing/internal/gateway"
	"skci/billing/internal/model"
	"skci/billing/internal/nicepay"
)

// Transaction purposes (gateway_transactions.purpose, contract K1/K2 with T12).
const (
	PurposeBills       = "bills"
	PurposeWalletTopup = "wallet_topup"
)

// PurposeHandler is implemented by the module that owns a non-bills
// purpose (T12: wallet top-up). The webhook calls it inside the same DB
// transaction that locked the gateway transaction (contract K2).
type PurposeHandler interface {
	OnGatewaySuccess(ctx context.Context, tx *sql.Tx, schoolID, purposeRef, gatewayTxID string) error
	OnGatewayClosed(ctx context.Context, tx *sql.Tx, schoolID, purposeRef, gatewayTxID, status string) error
}

// FeatureChecker reads a school's feature switches. *client.Client
// satisfies it (identity /schools/{id}/features, fails closed).
type FeatureChecker interface {
	FeatureEnabled(ctx context.Context, schoolID, featureKey string) (bool, error)
}

var errPurposeHandlerMissing = errors.New("purpose handler not registered")

// paymentFeature reports whether payment_gateway and key are both on for
// the school. Without a feature reader every T14 feature is off, which keeps
// T6 behaviour unchanged.
func (a *App) paymentFeature(ctx context.Context, schoolID, key string) bool {
	if a.PaymentFeatures == nil || schoolID == "" {
		return false
	}
	for _, k := range []string{common.FeaturePaymentGateway, key} {
		ok, err := a.PaymentFeatures.FeatureEnabled(ctx, schoolID, k)
		if err != nil || !ok {
			return false
		}
	}
	return true
}

// requirePaymentFeature writes 403 when the feature is off.
func (a *App) requirePaymentFeature(w http.ResponseWriter, r *http.Request, schoolID, key string) bool {
	if a.paymentFeature(r.Context(), schoolID, key) {
		return true
	}
	common.WriteError(w, http.StatusForbidden, "Fitur belum aktif untuk sekolah ini")
	return false
}

// location is the school timezone used for "today" (config, default WIB).
func (a *App) location() *time.Location {
	if a.Config.Gateway.Location != nil {
		return a.Config.Gateway.Location
	}
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.UTC
	}
	return loc
}

// paymentProvider returns the adapter for a provider code: an explicit
// override, else one built from payment_providers + <credential_ref>_* env.
// Fail-closed (QA-PAY BUG-004): a NICEPay provider without credentials is
// unavailable, never silently replaced by the simulator; the sim adapter
// itself only runs when PAYMENT_SIMULATOR_ENABLED=true outside production.
func (a *App) paymentProvider(ctx context.Context, code string) (gateway.Provider, error) {
	if p, ok := a.Providers[code]; ok {
		return p, nil
	}
	unavailable := &gateway.TransientError{Err: fmt.Errorf("provider %s tidak tersedia", code)}
	var adapter string
	var credRef sql.NullString
	err := a.DB.QueryRowContext(ctx, `SELECT adapter, credential_ref FROM payment_providers WHERE code = $1`, code).Scan(&adapter, &credRef)
	if err != nil {
		return nil, unavailable
	}
	gw := a.Config.Gateway
	switch adapter {
	case gateway.AdapterNicepay:
		ref := credRef.String
		if ref == "" || ref == "NICEPAY" {
			if a.Nicepay != nil {
				return &gateway.Nicepay{ProviderCode: code, Client: a.Nicepay, NotifyURL: gw.NicepayCallbackURL, ReturnURL: gw.NicepayReturnURL, ImageURL: gw.NicepayImageURL}, nil
			}
			return nil, unavailable
		}
		if v, ok := a.providerCache.Load(code); ok {
			return v.(gateway.Provider), nil
		}
		creds := config.NicepayFromEnv(ref)
		if !creds.Installed() {
			return nil, unavailable
		}
		c, err := nicepay.NewClient(nicepay.Config{BaseURL: creds.BaseURL, ClientID: creds.ClientID, ChannelID: creds.ChannelID,
			ClientSecret: creds.ClientSecret, PrivateKey: creds.PrivateKey, NotifPubKey: creds.PublicKey, StoreID: creds.StoreID})
		if err != nil {
			return nil, unavailable
		}
		p := &gateway.Nicepay{ProviderCode: code, Client: c, NotifyURL: creds.CallbackURL, ReturnURL: creds.ReturnURL, ImageURL: creds.ImageURL}
		a.providerCache.Store(code, p)
		return p, nil
	case gateway.AdapterSim:
		if !a.Config.SimulatorAllowed() {
			return nil, unavailable
		}
		return &gateway.Sim{ProviderCode: code, FailCharges: gw.SimFailCharges, FailDebits: gw.SimFailDebits, WebhookSecret: gw.SimWebhookSecret}, nil
	}
	return nil, unavailable
}

// credentialsInstalled reports, without revealing anything, whether a
// provider's credentials are present (for the simulator: whether it is
// allowed to run at all).
func (a *App) credentialsInstalled(adapter, credRef string) bool {
	switch adapter {
	case gateway.AdapterSim:
		return a.Config.SimulatorAllowed()
	case gateway.AdapterNicepay:
		if credRef == "" || credRef == "NICEPAY" {
			return a.Nicepay != nil
		}
		return config.NicepayFromEnv(credRef).Installed()
	}
	return false
}

// loadProviders reads every provider with its capabilities.
func (a *App) loadProviders(ctx context.Context) (map[string]gateway.ProviderInfo, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT p.code, p.enabled, COALESCE(array_to_string(array_agg(c.capability ORDER BY c.capability) FILTER (WHERE c.capability IS NOT NULL), ','), '')
		   FROM payment_providers p LEFT JOIN payment_provider_capabilities c ON c.provider_code = p.code
		  GROUP BY p.code, p.enabled`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]gateway.ProviderInfo{}
	for rows.Next() {
		var p gateway.ProviderInfo
		var caps string
		if err := rows.Scan(&p.Code, &p.Enabled, &caps); err != nil {
			return nil, err
		}
		if caps != "" {
			p.Capabilities = strings.Split(caps, ",")
		}
		out[p.Code] = p
	}
	return out, rows.Err()
}

// loadRules reads a school's active routing rules.
func (a *App) loadRules(ctx context.Context, schoolID string) ([]gateway.Rule, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id, method_kind, COALESCE(method_code,''), primary_provider, COALESCE(fallback_provider,''), failover_enabled
		   FROM payment_routing_rules WHERE school_id = $1 AND active`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []gateway.Rule
	for rows.Next() {
		var r gateway.Rule
		if err := rows.Scan(&r.ID, &r.MethodKind, &r.MethodCode, &r.Primary, &r.Fallback, &r.FailoverEnabled); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// methodMappings reads provider-specific codes of a method.
func (a *App) methodMappings(ctx context.Context, methodID string) (map[string]string, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT provider_code, provider_method_code FROM payment_method_provider_codes WHERE method_id = $1`, methodID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var p, c string
		if err := rows.Scan(&p, &c); err != nil {
			return nil, err
		}
		out[p] = c
	}
	return out, rows.Err()
}

// decideRoute picks the provider. With both T14 features off this is T6:
// always nicepay, no rule or provider table consulted.
func (a *App) decideRoute(ctx context.Context, schoolID string, method *model.GatewayMethod) (gateway.Decision, error) {
	if !a.paymentFeature(ctx, schoolID, common.FeaturePaymentMultiProvider) &&
		!a.paymentFeature(ctx, schoolID, common.FeaturePaymentAutodebit) {
		return gateway.Decision{Primary: gateway.DefaultProvider}, nil
	}
	rules, err := a.loadRules(ctx, schoolID)
	if err != nil {
		return gateway.Decision{}, err
	}
	providers, err := a.loadProviders(ctx)
	if err != nil {
		return gateway.Decision{}, err
	}
	return gateway.Route(rules, gateway.MethodRef{Kind: method.Kind, Code: method.Code}, providers)
}

// chargeInput is everything createCharge needs; one path for bill checkout
// (ParentCheckout) and purpose charges (CreatePurposeCharge).
type chargeInput struct {
	SchoolID, PayerUserID, StudentUserID string
	Method                               *model.GatewayMethod
	Totals                               model.TransactionTotals
	Bills                                []BillSummaryItem
	Purpose, PurposeRef                  string
	IdemKey, GoodsName, BuyerName        string
	ClientIP                             string
}

type chargeResult struct {
	TxID, TxNo, Provider, FailoverFrom string
	Charge                             gateway.Charge
	ExpiresAt                          time.Time
}

// newTxNo is TX-<yymmddhhmmss><6 random digits>. The old suffix
// (Nanosecond()%1000) is always 0 on platforms with microsecond clocks, so
// two checkouts in the same second collided on UNIQUE(school_id, tx_no).
func newTxNo() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		n = big.NewInt(int64(time.Now().Nanosecond() / 1000))
	}
	return fmt.Sprintf("TX-%s%06d", time.Now().Format("060102150405"), n.Int64())
}

// chargeFailureMessage keeps T6's 502 wording per method kind.
func chargeFailureMessage(kind string, err error) string {
	switch kind {
	case gateway.CapVA:
		return "Gagal membuat kode VA: " + err.Error()
	case gateway.CapQRIS:
		return "Gagal membuat kode QRIS: " + err.Error()
	case gateway.CapEWallet:
		return "Gagal memproses e-wallet: " + err.Error()
	}
	return "Gagal membuat kode bayar gerai: " + err.Error()
}

// chargeAt creates the payment code at one provider.
func (a *App) chargeAt(ctx context.Context, code string, in chargeInput, txNo string, mappings map[string]string) (gateway.Charge, error) {
	p, err := a.paymentProvider(ctx, code)
	if err != nil {
		return gateway.Charge{}, err
	}
	methodCode, err := gateway.ProviderCode(*in.Method, code, mappings)
	if err != nil {
		return gateway.Charge{}, err
	}
	return p.CreateCharge(ctx, gateway.ChargeRequest{
		RefNo: txNo, AmountIDR: in.Totals.TotalIDR, Kind: in.Method.Kind, ProviderMethodCode: methodCode,
		ValidFor: time.Duration(in.Method.ValidMinutes) * time.Minute, GoodsName: in.GoodsName,
		BuyerName: in.BuyerName, ClientIP: in.ClientIP,
	})
}

// createCharge routes, charges (with failover, FR-150-002/003, AC-150-02)
// and records exactly one gateway transaction at one provider (BR-PAY-03).
func (a *App) createCharge(ctx context.Context, in chargeInput) (*chargeResult, error) {
	d, err := a.decideRoute(ctx, in.SchoolID, in.Method)
	if errors.Is(err, gateway.ErrNotRouted) {
		return nil, &httpError{status: http.StatusUnprocessableEntity, msg: gateway.ErrNotRouted.Error()}
	}
	if err != nil {
		return nil, err
	}
	mappings, err := a.methodMappings(ctx, in.Method.ID)
	if err != nil {
		return nil, err
	}
	txNo := newTxNo()
	used, failoverFrom := d.Primary, d.FailoverFrom
	charge, err := a.chargeAt(ctx, d.Primary, in, txNo, mappings)
	if err != nil && gateway.ShouldFailover(err, d) {
		a.recordProviderFailure(ctx, d.Primary)
		used, failoverFrom = d.Fallback, d.Primary
		charge, err = a.chargeAt(ctx, d.Fallback, in, txNo, mappings)
	}
	if err != nil {
		return nil, &httpError{status: http.StatusBadGateway, msg: chargeFailureMessage(in.Method.Kind, err)}
	}

	expiresAt := gateway.RetailExpiry(in.Method.ValidMinutes, time.Now())
	if !charge.ExpiresAt.IsZero() {
		expiresAt = charge.ExpiresAt
	}
	txID, err := a.insertGatewayTx(ctx, gatewayTxRow{
		SchoolID: in.SchoolID, TxNo: txNo, PayerUserID: in.PayerUserID, StudentUserID: in.StudentUserID,
		MethodID: in.Method.ID, Totals: in.Totals, Provider: used, FailoverFrom: failoverFrom,
		Purpose: in.Purpose, PurposeRef: in.PurposeRef, Charge: charge, ExpiresAt: expiresAt,
		IdemKey: in.IdemKey, Bills: in.Bills,
	})
	if err != nil {
		return nil, err
	}
	if failoverFrom != "" {
		a.reportFailover(ctx, in.SchoolID, txNo, failoverFrom, used)
	}
	return &chargeResult{TxID: txID, TxNo: txNo, Provider: used, FailoverFrom: failoverFrom, Charge: charge, ExpiresAt: expiresAt}, nil
}

type gatewayTxRow struct {
	SchoolID, TxNo, PayerUserID, StudentUserID, MethodID string
	Totals                                               model.TransactionTotals
	Provider, FailoverFrom, Purpose, PurposeRef          string
	MandateID                                            string
	Charge                                               gateway.Charge
	ExpiresAt                                            time.Time
	IdemKey                                              string
	Bills                                                []BillSummaryItem
}

// insertGatewayTx writes the transaction and its bill lines atomically.
func (a *App) insertGatewayTx(ctx context.Context, row gatewayTxRow) (string, error) {
	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer dbTx.Rollback()
	purpose := row.Purpose
	if purpose == "" {
		purpose = PurposeBills
	}
	var txID string
	err = dbTx.QueryRowContext(ctx,
		`INSERT INTO gateway_transactions (
			school_id, tx_no, payer_user_id, student_user_id, method_id,
			bills_amount_idr, fee_idr, total_idr, fee_bearer, status,
			provider_ref, va_number, qr_content, redirect_url, expires_at, idempotency_key,
			provider, failover_from, purpose, purpose_ref, mandate_id, pay_code, retail_outlet
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'pending',NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),$14,$15,
		          $16,NULLIF($17,''),$18,$19,$20,NULLIF($21,''),NULLIF($22,''))
		RETURNING id`,
		row.SchoolID, row.TxNo, row.PayerUserID, row.StudentUserID, row.MethodID,
		row.Totals.BillsAmountIDR, row.Totals.FeeIDR, row.Totals.TotalIDR, row.Totals.FeeBearer,
		row.Charge.ProviderRef, row.Charge.VANumber, row.Charge.QRContent, row.Charge.RedirectURL, row.ExpiresAt, row.IdemKey,
		row.Provider, row.FailoverFrom, purpose, nullString(row.PurposeRef), nullString(row.MandateID),
		row.Charge.PayCode, row.Charge.RetailOutlet).Scan(&txID)
	if err != nil {
		if isUniqueViolation(err) {
			return "", &httpError{status: http.StatusConflict, msg: "transaksi dengan Idempotency-Key ini sedang diproses"}
		}
		return "", err
	}
	for i, b := range row.Bills {
		if _, err := dbTx.ExecContext(ctx,
			`INSERT INTO gateway_transaction_bills (transaction_id, bill_id, position, amount_idr) VALUES ($1, $2, $3, $4)`,
			txID, b.ID, i+1, b.OutstandingIDR); err != nil {
			return "", err
		}
	}
	return txID, dbTx.Commit()
}

// recordProviderFailure counts a failed charge (dashboard; the provider is
// never disabled automatically).
func (a *App) recordProviderFailure(ctx context.Context, code string) {
	_, _ = a.DB.ExecContext(ctx,
		`UPDATE payment_providers SET consecutive_failures = consecutive_failures + 1, last_failure_at = now(), updated_at = now() WHERE code = $1`, code)
}

// reportFailover records the event in the Integration Exception Queue
// (menu 80, AC-150-02), idempotent per transaction.
func (a *App) reportFailover(ctx context.Context, schoolID, txNo, from, to string) {
	if a.Core == nil {
		return
	}
	_ = a.Core.CreateException(ctx, client.ExceptionEvent{
		SchoolID:       schoolID,
		SourceService:  "billing",
		Kind:           "webhook",
		Severity:       "normal",
		CorrelationID:  txNo,
		ErrorMessage:   fmt.Sprintf("Provider %s tidak tersedia, transaksi %s dialihkan ke %s", from, txNo, to),
		PayloadSummary: map[string]any{"event": "provider_failover", "tx_no": txNo, "from": from, "to": to},
		MaxAttempts:    1,
		IdempotencyKey: "failover:" + txNo,
	})
}

// CreatePurposeCharge creates a gateway charge for a non-bills purpose
// (contract K1, used by T12 wallet top-up). Same routing, failover and fee
// rules as bill checkout; bills_amount_idr is the purpose amount and no
// gateway_transaction_bills rows are written. Idempotent per key.
func (a *App) CreatePurposeCharge(ctx context.Context, schoolID, payerUserID, studentUserID, methodID, purpose, purposeRef, idempotencyKey string, amountIDR int64) (string, error) {
	if purpose != PurposeWalletTopup {
		return "", &httpError{status: http.StatusBadRequest, msg: "purpose tidak dikenal"}
	}
	if strings.TrimSpace(purposeRef) == "" || strings.TrimSpace(idempotencyKey) == "" || schoolID == "" {
		return "", &httpError{status: http.StatusBadRequest, msg: "purpose_ref, idempotency_key, dan school_id wajib diisi"}
	}
	if amountIDR <= 0 {
		return "", &httpError{status: http.StatusBadRequest, msg: "nominal harus lebih dari 0"}
	}
	var existing string
	err := a.DB.QueryRowContext(ctx,
		`SELECT id FROM gateway_transactions WHERE school_id = $1 AND idempotency_key = $2`, schoolID, idempotencyKey).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	method, err := a.fetchMethod(ctx, schoolID, methodID)
	if err != nil {
		return "", &httpError{status: http.StatusBadRequest, msg: err.Error()}
	}
	if err := a.checkMethodAllowed(ctx, schoolID, method); err != nil {
		return "", err
	}
	totals, err := model.Totals(amountIDR, *method)
	if err != nil {
		return "", &httpError{status: http.StatusBadRequest, msg: err.Error()}
	}
	res, err := a.createCharge(ctx, chargeInput{
		SchoolID: schoolID, PayerUserID: payerUserID, StudentUserID: studentUserID, Method: method, Totals: totals,
		Purpose: purpose, PurposeRef: purposeRef, IdemKey: idempotencyKey,
		GoodsName: "Top-up dompet siswa", BuyerName: "Orang Tua Siswa",
	})
	if err != nil {
		return "", err
	}
	return res.TxID, nil
}

// checkMethodAllowed rejects methods a payer cannot check out with:
// autodebit methods (mandate-only) and retail when payment_autodebit is off.
func (a *App) checkMethodAllowed(ctx context.Context, schoolID string, m *model.GatewayMethod) error {
	switch m.Kind {
	case kindAutodebit:
		return &httpError{status: http.StatusBadRequest, msg: "metode autodebit hanya dipakai lewat mandat"}
	case gateway.CapRetail:
		if !a.paymentFeature(ctx, schoolID, common.FeaturePaymentAutodebit) {
			return &httpError{status: http.StatusBadRequest, msg: "pembayaran gerai retail belum aktif"}
		}
	}
	return nil
}

// kindAutodebit is the payment_methods.kind carrying autodebit fees.
const kindAutodebit = "autodebit"
