package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/wallet"
)

// Top-up (BR-WAL-02, IF-OPS-03, FR-125-002): only through the payment
// gateway (K1/K2 with T14) or the school cashier. No endpoint credits a
// wallet directly.

// purposeWalletTopup is gateway_transactions.purpose for wallet top-ups (T14 0006).
const purposeWalletTopup = "wallet_topup"

// purposeCharger is K1 (T12 §4.1): T14 creates a gateway charge for a
// non-bill purpose. Primitive parameters keep both branches compiling alone.
type purposeCharger interface {
	CreatePurposeCharge(ctx context.Context, schoolID, payerUserID, studentUserID, methodID, purpose, purposeRef, idempotencyKey string, amountIDR int64) (gatewayTxID string, err error)
}

// ParentCreateTopup is POST /parent/children/{studentUserId}/wallets/{kind}/topups
// {method_id, amount_idr} (+Idempotency-Key) -> {topup_id, transaction_id}.
// The frontend continues to orang-tua/transaksi/:id (T6).
func (a *App) ParentCreateTopup(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireWalletChild(w, r)
	if !ok {
		return
	}
	if a.PurposeCharger == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "Top-up online belum tersedia")
		return
	}
	kind := param(r, "kind")
	if !wallet.ValidKind(kind) {
		common.WriteError(w, http.StatusNotFound, "jenis dompet tidak dikenal")
		return
	}
	var in struct {
		MethodID  string `json:"method_id"`
		AmountIDR int64  `json:"amount_idr"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.MethodID) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "metode pembayaran wajib dipilih")
		return
	}
	key := "wallet-topup:" + idempotencyKey(r)
	var topupID, txID, status string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id::text,COALESCE(gateway_transaction_id::text,''),status FROM wallet_topups WHERE school_id=$1 AND idempotency_key=$2`,
		c.SchoolID, key).Scan(&topupID, &txID, &status)
	if err != nil && err != sql.ErrNoRows {
		writeErr(w, err)
		return
	}
	if err == nil && txID != "" {
		common.WriteJSON(w, http.StatusOK, map[string]any{"topup_id": topupID, "transaction_id": txID, "status": status}, nil)
		return
	}
	if topupID == "" {
		settings, err := loadWalletSettings(r.Context(), a.DB, c.SchoolID)
		if err != nil {
			writeErr(w, err)
			return
		}
		var balance int64
		var wstatus string
		err = a.DB.QueryRowContext(r.Context(),
			`SELECT balance_idr,status FROM wallets WHERE school_id=$1 AND student_user_id::text=$2 AND kind=$3`, c.SchoolID, studentID, kind).
			Scan(&balance, &wstatus)
		if err != nil && err != sql.ErrNoRows {
			writeErr(w, err)
			return
		}
		if wstatus != "" && wstatus != "active" {
			common.WriteError(w, http.StatusUnprocessableEntity, "dompet sedang ditutup")
			return
		}
		if err := wallet.CanTopup(balance, in.AmountIDR, settings.MaxFor(kind)); err != nil {
			writeErr(w, errHTTP(err))
			return
		}
		if err = a.DB.QueryRowContext(r.Context(),
			`INSERT INTO wallet_topups(school_id,student_user_id,wallet_kind,amount_idr,channel,requested_by,idempotency_key)
			 VALUES($1,$2,$3,$4,'gateway',$5,$6) ON CONFLICT (school_id,idempotency_key) DO UPDATE SET updated_at=now() RETURNING id::text`,
			c.SchoolID, studentID, kind, in.AmountIDR, c.UserID, key).Scan(&topupID); err != nil {
			writeErr(w, err)
			return
		}
	}
	txID, err = a.PurposeCharger.CreatePurposeCharge(r.Context(), c.SchoolID, c.UserID, studentID, in.MethodID, purposeWalletTopup, topupID, key, in.AmountIDR)
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			writeErr(w, err)
			return
		}
		common.CaptureError(err)
		common.WriteError(w, http.StatusBadGateway, "gagal membuat transaksi top-up")
		return
	}
	if _, err = a.DB.ExecContext(r.Context(),
		`UPDATE wallet_topups SET gateway_transaction_id=$1::uuid,updated_at=now() WHERE id::text=$2`, txID, topupID); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), c.SchoolID, "wallet_topup", topupID, "topup_requested", common.RoleParent, c.UserID, "", "", nil,
		map[string]any{"student_user_id": studentID, "kind": kind, "amount_idr": in.AmountIDR, "transaction_id": txID})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"topup_id": topupID, "transaction_id": txID, "status": "pending"}, nil)
}

// walletTopupHook is K2 (T12 §4.1): called by the T14 webhook inside its DB
// transaction, after FOR UPDATE on the gateway transaction and before commit.
type walletTopupHook struct{ app *App }

// WalletTopupHook returns the K2 handler registered as
// app.PurposeHandlers["wallet_topup"] at merge time.
func (a *App) WalletTopupHook() walletTopupHook { return walletTopupHook{app: a} }

// OnGatewaySuccess credits the wallet once (idempotency key
// wallet-topup:<topup_id>) and marks the top-up success. The guardian is
// notified after commit by the wallet_topup_notify job (AC-125-01).
func (h walletTopupHook) OnGatewaySuccess(ctx context.Context, tx *sql.Tx, schoolID, purposeRef, gatewayTxID string) error {
	var student, kind, status string
	var amount int64
	err := tx.QueryRowContext(ctx,
		`SELECT student_user_id::text,wallet_kind,status,amount_idr FROM wallet_topups WHERE id::text=$1 AND school_id=$2 FOR UPDATE`,
		purposeRef, schoolID).Scan(&student, &kind, &status, &amount)
	if err != nil {
		return err
	}
	if status == "success" {
		return nil
	}
	wl, err := ensureWallet(ctx, tx, schoolID, student, kind)
	if err != nil {
		return err
	}
	if _, _, err = applyMovement(ctx, tx, schoolID, &wl, movement{Kind: wallet.MoveTopupGateway, AmountIDR: amount,
		RefType: "wallet_topup", RefID: purposeRef, Actor: "gateway", Key: "wallet-topup:" + purposeRef}); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE wallet_topups SET status='success',gateway_transaction_id=COALESCE(gateway_transaction_id,NULLIF($2,'')::uuid),updated_at=now() WHERE id::text=$1`,
		purposeRef, gatewayTxID)
	return err
}

// OnGatewayClosed mirrors a failed/expired/cancelled gateway transaction.
func (h walletTopupHook) OnGatewayClosed(ctx context.Context, tx *sql.Tx, schoolID, purposeRef, gatewayTxID, status string) error {
	next := "failed"
	if status == "expired" {
		next = "expired"
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE wallet_topups SET status=$1,updated_at=now() WHERE id::text=$2 AND school_id=$3 AND status='pending'`, next, purposeRef, schoolID)
	return err
}

// CashierWalletTopup is POST /cashier/wallet-topups {student_user_id,
// wallet_kind, amount_idr, idempotency_key} (admin, kasir, finance_manager):
// cash received at the counter, receipt source='wallet'.
func (a *App) CashierWalletTopup(w http.ResponseWriter, r *http.Request) {
	c := common.RequireCashier(w, r)
	if c == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	if !a.requireFeature(w, r, schoolID, featureWallet) {
		return
	}
	var in struct {
		StudentUserID  string `json:"student_user_id"`
		WalletKind     string `json:"wallet_kind"`
		AmountIDR      int64  `json:"amount_idr"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.StudentUserID = strings.TrimSpace(in.StudentUserID)
	if in.WalletKind == "" {
		in.WalletKind = wallet.KindAllowance
	}
	if in.StudentUserID == "" || !wallet.ValidKind(in.WalletKind) {
		common.WriteError(w, http.StatusUnprocessableEntity, "siswa dan jenis dompet wajib diisi")
		return
	}
	if in.IdempotencyKey == "" {
		in.IdempotencyKey = idempotencyKey(r)
	}
	key := "cashier-topup:" + in.IdempotencyKey
	var existingID, receiptNo string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT t.id::text,COALESCE(rc.receipt_no,'') FROM wallet_topups t LEFT JOIN receipts rc ON rc.id=t.receipt_id
		 WHERE t.school_id=$1 AND t.idempotency_key=$2`, schoolID, key).Scan(&existingID, &receiptNo); err == nil {
		common.WriteJSON(w, http.StatusOK, map[string]any{"topup_id": existingID, "receipt_no": receiptNo, "status": "success"}, nil)
		return
	} else if err != sql.ErrNoRows {
		writeErr(w, err)
		return
	}
	settings, err := loadWalletSettings(r.Context(), a.DB, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	now := a.now()
	receiptNo, err = a.atomicNextReceiptNo(r.Context(), schoolID, now.In(a.loc()).Year())
	if err != nil {
		writeErr(w, err)
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	wl, err := ensureWallet(r.Context(), tx, schoolID, in.StudentUserID, in.WalletKind)
	if err != nil {
		writeErr(w, err)
		return
	}
	if wl.Status != "active" {
		common.WriteError(w, http.StatusUnprocessableEntity, "dompet sedang ditutup")
		return
	}
	if err := wallet.CanTopup(wl.BalanceIDR, in.AmountIDR, settings.MaxFor(in.WalletKind)); err != nil {
		writeErr(w, errHTTP(err))
		return
	}
	var topupID, receiptID string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO wallet_topups(school_id,student_user_id,wallet_kind,amount_idr,channel,status,requested_by,idempotency_key)
		 VALUES($1,$2,$3,$4,'cashier','success',$5,$6) RETURNING id::text`,
		schoolID, in.StudentUserID, in.WalletKind, in.AmountIDR, c.UserID, key).Scan(&topupID); err != nil {
		writeErr(w, err)
		return
	}
	if _, _, err = applyMovement(r.Context(), tx, schoolID, &wl, movement{Kind: wallet.MoveTopupCashier, AmountIDR: in.AmountIDR,
		RefType: "wallet_topup", RefID: topupID, Actor: c.UserID, Key: key}); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO receipts(school_id,receipt_no,source,source_id) VALUES($1,$2,'wallet',$3) RETURNING id::text`,
		schoolID, receiptNo, topupID).Scan(&receiptID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE wallet_topups SET receipt_id=$1 WHERE id::text=$2`, receiptID, topupID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	schoolName := ""
	if a.IdentitySvc.Configured() {
		schoolName, _ = a.IdentitySvc.SchoolName(r.Context(), schoolID)
	}
	label := map[string]string{wallet.KindAllowance: "Top-up uang saku", wallet.KindSavings: "Top-up tabungan"}[in.WalletKind]
	if docKey, err := a.generateReceiptDocument(r.Context(), schoolID, ReceiptDetails{
		ReceiptNo: receiptNo, SchoolName: schoolName, StudentName: in.StudentUserID, PayerName: "Kasir sekolah", AmountIDR: in.AmountIDR,
		MethodLabel: "Tunai (kasir)", Date: now.In(a.loc()), Items: []string{label},
	}); err == nil {
		_, _ = a.DB.ExecContext(r.Context(), `UPDATE receipts SET object_key=$1,updated_at=now() WHERE id::text=$2`, docKey, receiptID)
	} else {
		common.CaptureError(err)
	}
	a.audit(r.Context(), schoolID, "wallet_topup", topupID, "topup_cashier", c.ActiveRole, c.UserID, "", "", nil,
		map[string]any{"student_user_id": in.StudentUserID, "kind": in.WalletKind, "amount_idr": in.AmountIDR, "receipt_no": receiptNo})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"topup_id": topupID, "receipt_no": receiptNo, "receipt_id": receiptID,
		"status": "success", "balance_idr": wl.BalanceIDR, "created_at": now.Format(time.RFC3339)}, nil)
}
