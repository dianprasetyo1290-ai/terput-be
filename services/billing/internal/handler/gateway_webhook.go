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
	"net/http"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/model"
	"skci/billing/internal/nicepay"
)

type nicepayNotificationPayload struct {
	TrxID                      string `json:"trxId"`
	OriginalPartnerReferenceNo string `json:"originalPartnerReferenceNo"`
	OriginalReferenceNo        string `json:"originalReferenceNo"`
	LatestTransactionStatus    string `json:"latestTransactionStatus"`
}

// NicepayWebhook handles Bank Indonesia SNAP payment notification (AC-IF-01, AC-IF-02, BR-PAY-01).
func (a *App) NicepayWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"responseCode":"4000000","responseMessage":"Bad Request"}`))
		return
	}

	timestamp := r.Header.Get("X-TIMESTAMP")
	signature := r.Header.Get("X-SIGNATURE")

	var n nicepayNotificationPayload
	_ = json.Unmarshal(body, &n)

	ref := n.TrxID
	if ref == "" {
		ref = n.OriginalPartnerReferenceNo
	}
	pRef := n.OriginalReferenceNo
	if pRef == "" {
		pRef = ref
	}

	sum := sha256.Sum256(body)
	rawHash := hex.EncodeToString(sum[:])

	// Mask sensitive data in body for audit and storage (NFR-HR-01)
	maskedBody := maskJSON(body)

	// (2) Verify signature
	sigValid := true
	if a.Nicepay != nil {
		sigValid = a.Nicepay.VerifyNotification(timestamp, signature)
	}

	if !sigValid {
		// Log invalid webhook notification
		_, _ = a.DB.ExecContext(r.Context(),
			`INSERT INTO gateway_notifications (provider_ref, signature_valid, raw_hash, body_masked, result, received_at)
			 VALUES ($1, false, $2, $3, 'invalid_signature', now())
			 ON CONFLICT (provider_ref, raw_hash) DO NOTHING`,
			pRef, rawHash, jsonRaw(maskedBody))

		// Central security audit to core (AC-IF-01)
		if a.Core != nil {
			_ = a.Core.Audit(r.Context(), client.AuditEvent{
				Module:        "security",
				EntityType:    "webhook",
				EntityID:      pRef,
				Action:        "webhook_signature_rejected",
				Reason:        "invalid signature from payment gateway",
				CorrelationID: rawHash,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"responseCode":"4010000","responseMessage":"Unauthorized. Signature"}`))
		return
	}

	// (3) Deduplicate notification (BR-PAY-01)
	var notifID string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO gateway_notifications (provider_ref, signature_valid, raw_hash, body_masked, received_at)
		 VALUES ($1, true, $2, $3, now())
		 ON CONFLICT (provider_ref, raw_hash) DO NOTHING
		 RETURNING id`,
		pRef, rawHash, jsonRaw(maskedBody)).Scan(&notifID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// Duplicate notification already recorded and processed
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"responseCode":"2000000","responseMessage":"Success"}`))
		return
	}
	if notifID == "" {
		// Already processed via conflict do nothing
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"responseCode":"2000000","responseMessage":"Success"}`))
		return
	}

	// Find corresponding gateway transaction
	var txID, schoolID, status, methodKind, txNo, payerUserID, studentUserID string
	var totalIDR, billsAmountIDR, feeIDR int64
	var providerRefDB sql.NullString
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT t.id, t.school_id, t.status, m.kind, t.tx_no, t.total_idr, t.bills_amount_idr, t.fee_idr,
		        t.payer_user_id, t.student_user_id, t.provider_ref
		 FROM gateway_transactions t
		 JOIN payment_methods m ON m.id = t.method_id
		 WHERE t.tx_no = $1 OR t.provider_ref = $2 OR t.provider_ref = $1`,
		ref, pRef).Scan(&txID, &schoolID, &status, &methodKind, &txNo, &totalIDR, &billsAmountIDR, &feeIDR,
		&payerUserID, &studentUserID, &providerRefDB)
	if err != nil {
		// No matching transaction found
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"responseCode":"2000000","responseMessage":"Success"}`))
		return
	}

	// (4) Re-query status from NICEPay to never trust webhook body blindly
	expectedStatus := nicepay.StatusSuccess
	if a.Nicepay != nil && providerRefDB.Valid {
		st, statusErr := a.Nicepay.Status(nicepay.StatusQuery{
			Method:     methodKind,
			RefNo:      txNo,
			GatewayRef: providerRefDB.String,
			Amount:     totalIDR,
		})
		if statusErr != nil {
			// Internal error, queue exception to core
			if a.Core != nil {
				_ = a.Core.CreateException(r.Context(), client.ExceptionEvent{
					SchoolID:       schoolID,
					Kind:           "webhook",
					Severity:       "normal",
					CorrelationID:  rawHash,
					ErrorMessage:   statusErr.Error(),
					PayloadSummary: map[string]any{"tx_no": txNo, "ref": ref},
					MaxAttempts:    3,
					IdempotencyKey: fmt.Sprintf("webhook-err:%s", txID),
				})
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"responseCode":"5000000","responseMessage":"General Error"}`))
			return
		}
		expectedStatus = st
	}

	if expectedStatus != nicepay.StatusSuccess {
		// Not paid yet or failed/canceled
		if expectedStatus == nicepay.StatusCanceled || expectedStatus == nicepay.StatusFailed {
			_, _ = a.DB.ExecContext(r.Context(),
				`UPDATE gateway_transactions SET status = 'failed', updated_at = now() WHERE id = $1`, txID)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"responseCode":"2000000","responseMessage":"Success"}`))
		return
	}

	// (5) Process success in a DB transaction with SELECT ... FOR UPDATE (AC-IF-02)
	dbTx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"responseCode":"5000000","responseMessage":"General Error"}`))
		return
	}
	defer dbTx.Rollback()

	var currentStatus string
	err = dbTx.QueryRowContext(r.Context(),
		`SELECT status FROM gateway_transactions WHERE id = $1 FOR UPDATE`, txID).Scan(&currentStatus)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"responseCode":"5000000","responseMessage":"General Error"}`))
		return
	}

	if currentStatus == "success" {
		// Already success, idempotent return without re-allocating (AC-IF-02)
		_ = dbTx.Commit()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"responseCode":"2000000","responseMessage":"Success"}`))
		return
	}

	// Mark transaction success
	now := time.Now()
	_, err = dbTx.ExecContext(r.Context(),
		`UPDATE gateway_transactions
		 SET status = 'success', paid_at = $1, updated_at = $1
		 WHERE id = $2`, now, txID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"responseCode":"5000000","responseMessage":"General Error"}`))
		return
	}

	// Allocate to bills
	rows, err := dbTx.QueryContext(r.Context(),
		`SELECT bill_id, amount_idr FROM gateway_transaction_bills WHERE transaction_id = $1`, txID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"responseCode":"5000000","responseMessage":"General Error"}`))
		return
	}

	type billAlloc struct {
		billID string
		amount int64
	}
	var allocs []billAlloc
	for rows.Next() {
		var ba billAlloc
		if err := rows.Scan(&ba.billID, &ba.amount); err == nil {
			allocs = append(allocs, ba)
		}
	}
	rows.Close()

	for _, ba := range allocs {
		// Create verified bill_payments record
		idemKey := fmt.Sprintf("gw-alloc:%s:%s", txID, ba.billID)
		var pID string
		err = dbTx.QueryRowContext(r.Context(),
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
			) RETURNING id`,
			schoolID, ba.billID, ba.amount, now,
			txNo, txID, rawHash, idemKey,
			payerUserID).Scan(&pID)
		if err != nil && !isUniqueViolation(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"responseCode":"5000000","responseMessage":"General Error"}`))
			return
		}

		if pID != "" {
			// Insert allocation
			_, _ = dbTx.ExecContext(r.Context(),
				`INSERT INTO bill_payment_allocations (school_id, payment_id, amount_idr, created_at)
				 VALUES ($1, $2, $3, $4)`,
				schoolID, pID, ba.amount, now)

			// Recompute bill status
			recomputeBillStatusInTx(r.Context(), dbTx, ba.billID)
		}
	}

	if err := dbTx.Commit(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"responseCode":"5000000","responseMessage":"General Error"}`))
		return
	}

	// (6) Generate receipt and notify parent within ≤ 5 minutes (AC-29-02)
	receiptNo, rErr := a.atomicNextReceiptNo(r.Context(), schoolID, now.Year())
	if rErr == nil {
		docKey, _ := a.generateReceiptDocument(r.Context(), schoolID, ReceiptDetails{
			ReceiptNo:   receiptNo,
			SchoolName:  "Sekolah Teratai Putih",
			StudentName: "Siswa",
			PayerName:   "Orang Tua",
			AmountIDR:  totalIDR,
			MethodLabel: methodKind,
			Date:        now,
			Items:       []string{txNo},
		})

		_, _ = a.DB.ExecContext(r.Context(),
			`INSERT INTO receipts (school_id, receipt_no, source, source_id, object_key, sent_at, created_at)
			 VALUES ($1, $2, 'gateway', $3, $4, now(), now())`,
			schoolID, receiptNo, txID, docKey)

		if a.Notification != nil {
			_ = a.Notification.Notify(r.Context(), "receipt:"+receiptNo, client.Notification{
				SchoolID: schoolID,
				UserID:   payerUserID,
				Type:     "receipt",
				Title:    "Kuitansi Pembayaran " + receiptNo,
				Body:     fmt.Sprintf("Pembayaran tagihan sebesar Rp %d via %s telah berhasil.", totalIDR, methodKind),
				Data:     map[string]any{"receipt_no": receiptNo, "tx_no": txNo, "amount_idr": totalIDR},
				Channels: []string{"in_app", "email"},
			})
		}
	}

	// (7) Daily journal posting to accounting engine (AC-IF-03)
	if a.Accounting != nil {
		// Fetch accounting accounts
		clearingAcc := "1101-01"
		feeExpenseAcc := "5101-01"
		receivableAcc := "1102-01"
		_ = a.DB.QueryRowContext(r.Context(),
			`SELECT clearing_acc, fee_expense_acc, receivable_acc FROM gateway_accounts WHERE school_id = $1`,
			schoolID).Scan(&clearingAcc, &feeExpenseAcc, &receivableAcc)

		batch := client.JournalRequest{
			SchoolID:    schoolID,
			Source:      "gateway",
			TransDate:   now.Format("2006-01-02"),
			Reference:   fmt.Sprintf("pgw:%s:%s", schoolID, now.Format("2006-01-02")),
			Description: "Penerimaan pembayaran gateway " + txNo,
			Lines: []client.JournalLine{
				{AccCode: clearingAcc, DebitIDR: totalIDR - feeIDR, CreditIDR: 0},
				{AccCode: receivableAcc, DebitIDR: 0, CreditIDR: billsAmountIDR},
			},
		}
		if feeIDR > 0 {
			batch.Lines = append(batch.Lines, client.JournalLine{
				AccCode: feeExpenseAcc, DebitIDR: feeIDR, CreditIDR: 0,
			})
		}

		if a.Accounting != nil {
			if _, jErr := a.Accounting.PostJournal(r.Context(), batch); jErr != nil {
				// Queue exception to Core if accounting engine doesn't respond (AC-IF-03)
				if a.Core != nil {
					_ = a.Core.CreateException(r.Context(), client.ExceptionEvent{
						SchoolID:       schoolID,
						Kind:           "journal_posting",
						Severity:       "normal",
						CorrelationID:  batch.Reference,
						ErrorMessage:   jErr.Error(),
						PayloadSummary: map[string]any{"batch": batch},
						MaxAttempts:    3,
						IdempotencyKey: fmt.Sprintf("journal-post:%s:%s", schoolID, batch.Reference),
					})
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"responseCode":"2000000","responseMessage":"Success"}`))
}

// markTransactionSuccess marks transaction as success and allocates bills
func (a *App) markTransactionSuccess(ctx context.Context, txID string) error {
	now := time.Now()
	_, err := a.DB.ExecContext(ctx,
		`UPDATE gateway_transactions SET status = 'success', paid_at = $1, updated_at = $1 WHERE id = $2`,
		now, txID)
	return err
}

func recomputeBillStatusInTx(ctx context.Context, tx *sql.Tx, billID string) {
	var netIDR int64
	err := tx.QueryRowContext(ctx,
		`SELECT net_idr FROM student_bills WHERE id = $1`, billID).Scan(&netIDR)
	if err != nil {
		return
	}

	var verifiedSum, reversalSum int64
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(a.amount_idr), 0)
		 FROM bill_payment_allocations a
		 JOIN bill_payments p ON p.id = a.payment_id
		 WHERE p.student_bill_id = $1 AND p.status = 'verified'`, billID).Scan(&verifiedSum)
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(r.amount_idr), 0)
		 FROM bill_payment_reversals r
		 JOIN bill_payments p ON p.id = r.payment_id
		 WHERE p.student_bill_id = $1`, billID).Scan(&reversalSum)

	paid := verifiedSum - reversalSum
	st := model.DeriveBillStatus(netIDR, paid)

	_, _ = tx.ExecContext(ctx,
		`UPDATE student_bills SET status = $1, updated_at = now() WHERE id = $2`, string(st), billID)
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
