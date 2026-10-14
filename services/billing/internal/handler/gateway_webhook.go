package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/gateway"
)

// SNAP-style webhook answers (every provider gets the same shape).
const (
	snapOK           = `{"responseCode":"2000000","responseMessage":"Success"}`
	snapBadRequest   = `{"responseCode":"4000000","responseMessage":"Bad Request"}`
	snapUnauthorized = `{"responseCode":"4010000","responseMessage":"Unauthorized. Signature"}`
	snapNotFound     = `{"responseCode":"4040000","responseMessage":"Not Found"}`
	snapError        = `{"responseCode":"5000000","responseMessage":"General Error"}`
)

// readLimited reads a webhook body (max 64KB).
func readLimited(r *http.Request) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r.Body, 64<<10))
}

func writeSnap(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// NicepayWebhook is the T6 path POST /webhooks/nicepay, kept as an alias of
// POST /webhooks/{provider} with provider "nicepay".
func (a *App) NicepayWebhook(w http.ResponseWriter, r *http.Request) {
	a.handleProviderWebhook(w, r, gateway.DefaultProvider)
}

// ProviderWebhook handles POST /webhooks/{provider} (BR-PAY-01/03).
func (a *App) ProviderWebhook(w http.ResponseWriter, r *http.Request) {
	a.handleProviderWebhook(w, r, chi.URLParam(r, "provider"))
}

// gatewayTxState is a transaction as the webhook needs it.
type gatewayTxState struct {
	ID, SchoolID, Status, Kind, TxNo, PayerUserID, StudentUserID string
	TotalIDR, BillsAmountIDR, FeeIDR                             int64
	ProviderRef, VANumber, PurposeRef, MandateID                 sql.NullString
	Purpose                                                      string
}

// handleProviderWebhook: verify with the provider's adapter (AC-IF-01) →
// dedup per provider (BR-PAY-01) → re-query the transaction's own provider
// (never trust the body) → apply the status under FOR UPDATE (AC-IF-02).
func (a *App) handleProviderWebhook(w http.ResponseWriter, r *http.Request, code string) {
	ctx := r.Context()
	body, err := readLimited(r)
	if err != nil {
		writeSnap(w, http.StatusBadRequest, snapBadRequest)
		return
	}
	sum := sha256.Sum256(body)
	rawHash := hex.EncodeToString(sum[:])
	maskedBody := maskJSON(body)

	p, err := a.paymentProvider(ctx, code)
	if err != nil {
		// Fail closed (QA-PAY BUG-004): no verifier = nothing is trusted.
		// 503 so the provider retries once credentials are installed.
		_, pRef := gateway.ParseNicepayRefs(body)
		a.rejectWebhook(ctx, code, pRef, rawHash, maskedBody, "provider_unavailable")
		writeSnap(w, http.StatusServiceUnavailable, snapError)
		return
	}

	n, verr := p.VerifyWebhook(r.Header, body)
	if verr != nil {
		a.rejectWebhook(ctx, code, n.ProviderRef, rawHash, maskedBody, "invalid_signature")
		writeSnap(w, http.StatusUnauthorized, snapUnauthorized)
		return
	}

	var notifID string
	err = a.DB.QueryRowContext(ctx,
		`INSERT INTO gateway_notifications (provider, provider_ref, signature_valid, raw_hash, body_masked, received_at)
		 VALUES ($1, $2, true, $3, $4, now())
		 ON CONFLICT (provider, provider_ref, raw_hash) DO UPDATE
		   SET signature_valid = true, result = NULL, received_at = now()
		   WHERE gateway_notifications.signature_valid = false
		 RETURNING id`,
		code, n.ProviderRef, rawHash, jsonRaw(maskedBody)).Scan(&notifID)
	if err != nil || notifID == "" {
		// Same notification already recorded: processed once only.
		writeSnap(w, http.StatusOK, snapOK)
		return
	}

	t, err := a.loadTxForWebhook(ctx, code, n.RefNo, n.ProviderRef)
	if err != nil {
		a.finishNotification(ctx, notifID, "", "no_matching_transaction")
		writeSnap(w, http.StatusOK, snapOK)
		return
	}

	st, err := p.QueryStatus(ctx, gateway.StatusQuery{
		Kind: t.Kind, RefNo: t.TxNo, ProviderRef: t.ProviderRef.String, PayCode: t.VANumber.String,
		AmountIDR: t.TotalIDR, ClaimedStatus: n.ClaimedStatus,
	})
	if err == nil {
		err = a.applyProviderStatus(ctx, t.ID, st, rawHash)
	}
	if err != nil {
		a.webhookException(ctx, t.SchoolID, t.ID, t.TxNo, rawHash, err)
		// Notification stays unprocessed: delete it so the provider's retry
		// with the same body is processed again.
		_, _ = a.DB.ExecContext(ctx, `DELETE FROM gateway_notifications WHERE id = $1`, notifID)
		writeSnap(w, http.StatusInternalServerError, snapError)
		return
	}
	a.finishNotification(ctx, notifID, t.SchoolID, string(st))
	writeSnap(w, http.StatusOK, snapOK)
}

// rejectWebhook records a refused notification in gateway_notifications
// and the security audit log (TC-IF-005). The transaction is not touched.
func (a *App) rejectWebhook(ctx context.Context, code, providerRef, rawHash string, maskedBody map[string]any, result string) {
	_, _ = a.DB.ExecContext(ctx,
		`INSERT INTO gateway_notifications (provider, provider_ref, signature_valid, raw_hash, body_masked, result, received_at)
		 VALUES ($1, $2, false, $3, $4, $5, now())
		 ON CONFLICT (provider, provider_ref, raw_hash) DO NOTHING`,
		code, providerRef, rawHash, jsonRaw(maskedBody), result)
	if a.Core != nil {
		_ = a.Core.Audit(ctx, client.AuditEvent{
			Service:       "billing",
			Module:        "security",
			EntityType:    "webhook",
			EntityID:      providerRef,
			Action:        "webhook_" + result,
			Reason:        "notifikasi payment gateway " + code + " ditolak: " + result,
			CorrelationID: rawHash,
		})
	}
	log.Printf("billing: webhook %s rejected (%s) ref=%q hash=%s", code, result, providerRef, rawHash)
}

func (a *App) finishNotification(ctx context.Context, id, schoolID, result string) {
	_, _ = a.DB.ExecContext(ctx,
		`UPDATE gateway_notifications SET processed_at = now(), result = $2, school_id = COALESCE(NULLIF($3,'')::uuid, school_id) WHERE id = $1`,
		id, result, schoolID)
}

func (a *App) webhookException(ctx context.Context, schoolID, txID, txNo, correlation string, err error) {
	if a.Core == nil {
		return
	}
	_ = a.Core.CreateException(ctx, client.ExceptionEvent{
		SchoolID:       schoolID,
		SourceService:  "billing",
		Kind:           "webhook",
		Severity:       "normal",
		CorrelationID:  correlation,
		ErrorMessage:   err.Error(),
		PayloadSummary: map[string]any{"tx_no": txNo},
		MaxAttempts:    3,
		IdempotencyKey: fmt.Sprintf("webhook-err:%s", txID),
	})
}

const txStateColumns = `t.id, t.school_id, t.status, m.kind, t.tx_no, t.total_idr, t.bills_amount_idr, t.fee_idr,
	t.payer_user_id, t.student_user_id, t.provider_ref, t.va_number, t.purpose, t.purpose_ref, t.mandate_id`

func scanTxState(row interface{ Scan(...any) error }) (*gatewayTxState, error) {
	var t gatewayTxState
	err := row.Scan(&t.ID, &t.SchoolID, &t.Status, &t.Kind, &t.TxNo, &t.TotalIDR, &t.BillsAmountIDR, &t.FeeIDR,
		&t.PayerUserID, &t.StudentUserID, &t.ProviderRef, &t.VANumber, &t.Purpose, &t.PurposeRef, &t.MandateID)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// loadTxForWebhook finds the transaction of this provider only, so the same
// reference at two providers never collides (BR-PAY-03).
func (a *App) loadTxForWebhook(ctx context.Context, provider, ref, pRef string) (*gatewayTxState, error) {
	return scanTxState(a.DB.QueryRowContext(ctx,
		`SELECT `+txStateColumns+`
		   FROM gateway_transactions t JOIN payment_methods m ON m.id = t.method_id
		  WHERE t.provider = $3 AND (t.tx_no = $1 OR t.provider_ref = $2 OR t.provider_ref = $1)
		  ORDER BY t.created_at DESC LIMIT 1`, ref, pRef, provider))
}

// applyProviderStatus moves a transaction to what its provider reported.
// Paid → success + allocation (bills) or purpose hook (K2), receipt after
// commit. Failed/expired → closed + purpose hook. Idempotent.
func (a *App) applyProviderStatus(ctx context.Context, txID string, st gateway.Status, correlation string) error {
	switch st {
	case gateway.StatusPaid:
		return a.settleTransaction(ctx, txID, correlation)
	case gateway.StatusFailed, gateway.StatusExpired:
		return a.closeTransaction(ctx, txID, st)
	}
	return nil
}

func (a *App) lockTx(ctx context.Context, dbTx *sql.Tx, txID string) (*gatewayTxState, error) {
	return scanTxState(dbTx.QueryRowContext(ctx,
		`SELECT `+txStateColumns+`
		   FROM gateway_transactions t JOIN payment_methods m ON m.id = t.method_id
		  WHERE t.id = $1 FOR UPDATE OF t`, txID))
}

func (a *App) settleTransaction(ctx context.Context, txID, correlation string) error {
	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer dbTx.Rollback()
	t, err := a.lockTx(ctx, dbTx, txID)
	if err != nil {
		return err
	}
	if t.Status == "success" || t.Status == "corrected" {
		return dbTx.Commit() // AC-IF-02: never allocate twice
	}
	now := time.Now()
	if _, err := dbTx.ExecContext(ctx,
		`UPDATE gateway_transactions SET status = 'success', paid_at = $1, updated_at = $1 WHERE id = $2`, now, txID); err != nil {
		return err
	}
	if t.Purpose == PurposeBills {
		if err := allocateGatewayBills(ctx, dbTx, t, now, correlation); err != nil {
			return err
		}
	} else {
		h := a.PurposeHandlers[t.Purpose]
		if h == nil {
			return errPurposeHandlerMissing
		}
		if err := h.OnGatewaySuccess(ctx, dbTx, t.SchoolID, t.PurposeRef.String, t.ID); err != nil {
			return err
		}
	}
	if _, err := dbTx.ExecContext(ctx,
		`UPDATE autodebit_attempts SET status = 'success', updated_at = now()
		  WHERE gateway_transaction_id = $1 AND status IN ('processing','scheduled')`, txID); err != nil {
		return err
	}
	if err := dbTx.Commit(); err != nil {
		return err
	}
	a.issueGatewayReceipt(ctx, t, now)
	return nil
}

func (a *App) closeTransaction(ctx context.Context, txID string, st gateway.Status) error {
	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer dbTx.Rollback()
	t, err := a.lockTx(ctx, dbTx, txID)
	if err != nil {
		return err
	}
	if t.Status != "pending" {
		return dbTx.Commit()
	}
	status := "failed" // T6 recorded cancelled/failed as failed
	if st == gateway.StatusExpired {
		status = "expired"
	}
	if _, err := dbTx.ExecContext(ctx,
		`UPDATE gateway_transactions SET status = $1, updated_at = now() WHERE id = $2`, status, txID); err != nil {
		return err
	}
	if t.Purpose != PurposeBills {
		h := a.PurposeHandlers[t.Purpose]
		if h == nil {
			return errPurposeHandlerMissing
		}
		if err := h.OnGatewayClosed(ctx, dbTx, t.SchoolID, t.PurposeRef.String, t.ID, status); err != nil {
			return err
		}
	}
	if err := dbTx.Commit(); err != nil {
		return err
	}
	if t.MandateID.Valid {
		var attemptID string
		if err := a.DB.QueryRowContext(ctx,
			`SELECT id FROM autodebit_attempts WHERE gateway_transaction_id = $1 AND status = 'processing'`, txID).Scan(&attemptID); err == nil {
			a.failAttempt(ctx, attemptID, "debit "+status+" di provider", time.Now())
		}
	}
	return nil
}

// allocateGatewayBills books the transaction's bill lines as verified
// payments (T6 allocation, unchanged).
func allocateGatewayBills(ctx context.Context, dbTx *sql.Tx, t *gatewayTxState, now time.Time, correlation string) error {
	rows, err := dbTx.QueryContext(ctx,
		`SELECT bill_id, amount_idr FROM gateway_transaction_bills WHERE transaction_id = $1 ORDER BY position`, t.ID)
	if err != nil {
		return err
	}
	type billAlloc struct {
		billID string
		amount int64
	}
	var allocs []billAlloc
	for rows.Next() {
		var ba billAlloc
		if err := rows.Scan(&ba.billID, &ba.amount); err != nil {
			rows.Close()
			return err
		}
		allocs = append(allocs, ba)
	}
	rows.Close()

	for _, ba := range allocs {
		var pID string
		err := dbTx.QueryRowContext(ctx,
			`INSERT INTO bill_payments (
				school_id, student_bill_id, amount_idr, transfer_at,
				bank_name, account_name, reference, object_key, file_name,
				mime, size_bytes, sha256, status, idempotency_key,
				uploader_user_id, verified_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4,
				'NICEPay', 'Payment Gateway', $5, 'gateway/' || $6, 'gateway_receipt',
				'application/json', 100, $7, 'verified', $8,
				$9, $4, $4, $4
			) ON CONFLICT DO NOTHING RETURNING id`,
			t.SchoolID, ba.billID, ba.amount, now,
			t.TxNo, t.ID, correlation, fmt.Sprintf("gw-alloc:%s:%s", t.ID, ba.billID),
			t.PayerUserID).Scan(&pID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if pID == "" {
			continue
		}
		if _, err := dbTx.ExecContext(ctx,
			`INSERT INTO bill_payment_allocations (school_id, payment_id, amount_idr, created_at) VALUES ($1, $2, $3, $4)`,
			t.SchoolID, pID, ba.amount, now); err != nil {
			return err
		}
		if err := recomputeBillStatusInTx(ctx, dbTx, ba.billID); err != nil {
			return err
		}
	}
	return nil
}

// issueGatewayReceipt creates and sends the receipt (AC-29-02) for every
// purpose. Failures are retried by nothing here; they are logged in receipts.
func (a *App) issueGatewayReceipt(ctx context.Context, t *gatewayTxState, now time.Time) {
	receiptNo, err := a.atomicNextReceiptNo(ctx, t.SchoolID, now.Year())
	if err != nil {
		return
	}
	what := "Pembayaran tagihan"
	if t.Purpose == PurposeWalletTopup {
		what = "Top-up dompet"
	}
	docKey, _ := a.generateReceiptDocument(ctx, t.SchoolID, ReceiptDetails{
		ReceiptNo:   receiptNo,
		SchoolName:  "Sekolah Teratai Putih",
		StudentName: "Siswa",
		PayerName:   "Orang Tua",
		AmountIDR:   t.TotalIDR,
		MethodLabel: t.Kind,
		Date:        now,
		Items:       []string{t.TxNo},
	})
	_, _ = a.DB.ExecContext(ctx,
		`INSERT INTO receipts (school_id, receipt_no, source, source_id, object_key, sent_at, created_at)
		 VALUES ($1, $2, 'gateway', $3, $4, now(), now()) ON CONFLICT DO NOTHING`,
		t.SchoolID, receiptNo, t.ID, docKey)
	if a.Notification != nil {
		_ = a.Notification.Notify(ctx, "receipt:"+receiptNo, client.Notification{
			SchoolID: t.SchoolID,
			UserID:   t.PayerUserID,
			Type:     "receipt",
			Title:    "Kuitansi Pembayaran " + receiptNo,
			Body:     fmt.Sprintf("%s sebesar Rp %d via %s telah berhasil.", what, t.TotalIDR, t.Kind),
			Data:     map[string]any{"receipt_no": receiptNo, "tx_no": t.TxNo, "amount_idr": t.TotalIDR, "purpose": t.Purpose},
			Channels: []string{"in_app", "email"},
		})
	}
}

// recomputeBillStatusInTx re-derives a bill's status inside tx using the
// same school-scoped ledger rule as every other payment path
// (recomputeBillStatusQ); void bills are left alone and errors abort the tx.
func recomputeBillStatusInTx(ctx context.Context, tx *sql.Tx, billID string) error {
	var schoolID string
	if err := tx.QueryRowContext(ctx, `SELECT school_id::text FROM student_bills WHERE id = $1`, billID).Scan(&schoolID); err != nil {
		return err
	}
	return recomputeBillStatusQ(ctx, tx, schoolID, billID)
}

func maskJSON(b []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{"raw": "masked"}
	}
	sensitiveKeys := []string{"nik", "npwp", "phone", "email", "account_number", "bank_account"}
	for _, k := range sensitiveKeys {
		if v, ok := m[k]; ok {
			str := fmt.Sprintf("%v", v)
			if len(str) > 4 {
				m[k] = "••••" + str[len(str)-4:]
			} else {
				m[k] = "••••"
			}
		}
	}
	return m
}
