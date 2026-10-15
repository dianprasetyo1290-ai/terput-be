package handler

import (
	"context"
	"database/sql"
	"net/http"

	"skci/billing/internal/wallet"
)

// syncWalletClosureQ moves the wallet closure that owns refundID (and its
// wallet) along with the refund's status (FR-125-005): rejected releases the
// wallet back to active, processed closes it. Refunds without a closure are
// left alone.
func syncWalletClosureQ(ctx context.Context, tx *sql.Tx, schoolID, refundID, refundStatus string) error {
	closure, walletStatus, ok := wallet.ClosureForRefund(refundStatus)
	if !ok {
		return nil
	}
	var walletID string
	err := tx.QueryRowContext(ctx,
		`UPDATE wallet_closures SET status=$1, updated_at=now()
		 WHERE school_id=$2 AND refund_id::text=$3 AND status IN ('pending_approval','approved')
		 RETURNING wallet_id::text`, closure, schoolID, refundID).Scan(&walletID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE wallets SET status=$1, updated_at=now() WHERE id::text=$2 AND school_id=$3`,
		walletStatus, walletID, schoolID)
	return err
}

// payRefundFromWallet pays an approved wallet refund out of the wallet: a
// refund_exit movement takes the balance to zero (journaled by the daily
// wallet journal as Dr wallet liability / Cr cash), then the closure is
// completed and the wallet closed. The balance must still equal the
// refund; a wallet whose balance changed since the request is refused.
func payRefundFromWallet(ctx context.Context, tx *sql.Tx, schoolID, refundID, walletID, actorID string, amount int64) (walletRow, int64, error) {
	wl, err := lockWallet(ctx, tx, schoolID, walletID)
	if err != nil {
		return wl, 0, err
	}
	before := wl.BalanceIDR
	if before != amount {
		return wl, before, &httpError{http.StatusUnprocessableEntity,
			"saldo dompet sudah berubah sejak refund diajukan; tolak refund ini dan ajukan penutupan ulang"}
	}
	if _, _, err = applyMovement(ctx, tx, schoolID, &wl, movement{Kind: wallet.MoveRefundExit, AmountIDR: -amount,
		RefType: "bill_refund", RefID: refundID, Actor: actorID, Key: "wallet-refund:" + refundID}); err != nil {
		return wl, before, err
	}
	return wl, before, syncWalletClosureQ(ctx, tx, schoolID, refundID, "processed")
}
