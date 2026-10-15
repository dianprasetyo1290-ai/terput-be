package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	"skci/billing/internal/client"
	"skci/billing/internal/model"
)

// pendingBillRefundSQL is the amount of refunds still requested or approved
// against bill b: refunds filed on the bill itself (Menu 25) plus the bill
// parts of split student refunds (Menu 78). $excl is a refund id to leave out.
const pendingBillRefundSQL = `(COALESCE((SELECT sum(f.amount_idr) FROM bill_refunds f
	  WHERE f.student_bill_id=b.id AND f.school_id=b.school_id AND f.status IN ('requested','approved') AND f.id::text<>$excl),0)
	+ COALESCE((SELECT sum(rp.amount_idr) FROM bill_refund_parts rp JOIN bill_refunds f ON f.id=rp.refund_id
	  WHERE rp.student_bill_id=b.id AND rp.school_id=b.school_id AND f.status IN ('requested','approved') AND f.id::text<>$excl),0))`

// errRefundSourceShort means the deposit or a bill's paid balance no longer
// covers an approved refund at processing time.
var errRefundSourceShort = errors.New("refund source no longer covers the amount")

// studentRefundablesQ returns what a student refund may still return: the
// deposit balance minus the deposit portions of pending refunds, and every
// bill's paid balance minus pending refunds, newest bill first. The caller
// holds the transaction; the deposit row and the bills are locked so two
// concurrent requests cannot both spend the same money.
func studentRefundablesQ(ctx context.Context, tx *sql.Tx, schoolID, studentID string) (int64, []model.RefundableBill, error) {
	var balance int64
	err := tx.QueryRowContext(ctx,
		`SELECT balance_idr FROM student_deposits WHERE school_id=$1 AND student_user_id::text=$2 FOR UPDATE`,
		schoolID, studentID).Scan(&balance)
	if err != nil && err != sql.ErrNoRows {
		return 0, nil, err
	}
	var pendingDeposit int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(sum(deposit_idr),0) FROM bill_refunds
		 WHERE school_id=$1 AND student_user_id::text=$2 AND wallet_id IS NULL AND status IN ('requested','approved')`,
		schoolID, studentID).Scan(&pendingDeposit); err != nil {
		return 0, nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`SELECT 1 FROM student_bills WHERE school_id=$1 AND student_user_id::text=$2 ORDER BY id FOR UPDATE`,
		schoolID, studentID); err != nil {
		return 0, nil, err
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT b.id::text,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id AND p.status='verified'),0)
		      - COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
		                  WHERE p.student_bill_id=b.id AND p.school_id=b.school_id),0)
		      - `+pendingSQL("''")+`
		 FROM student_bills b WHERE b.school_id=$1 AND b.student_user_id::text=$2
		 ORDER BY b.created_at DESC, b.id`, schoolID, studentID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	var bills []model.RefundableBill
	for rows.Next() {
		var b model.RefundableBill
		if err := rows.Scan(&b.BillID, &b.RefundableIDR); err != nil {
			return 0, nil, err
		}
		bills = append(bills, b)
	}
	return balance - pendingDeposit, bills, rows.Err()
}

// pendingSQL binds pendingBillRefundSQL's excluded refund id to expr.
func pendingSQL(expr string) string {
	return strings.ReplaceAll(pendingBillRefundSQL, "$excl", expr)
}

// refundBillShares returns the bill portions of a refund: the single bill
// of a Menu 25 refund, or the parts of a split student refund.
func refundBillShares(ctx context.Context, tx *sql.Tx, schoolID, refundID string, billID sql.NullString, billIDR int64) ([]model.BillRefundShare, error) {
	if billID.Valid {
		if billIDR <= 0 {
			return nil, nil
		}
		return []model.BillRefundShare{{BillID: billID.String, AmountIDR: billIDR}}, nil
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT student_bill_id::text, amount_idr FROM bill_refund_parts WHERE school_id=$1 AND refund_id::text=$2 ORDER BY student_bill_id`,
		schoolID, refundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.BillRefundShare
	for rows.Next() {
		var s model.BillRefundShare
		if err := rows.Scan(&s.BillID, &s.AmountIDR); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// reverseBillShare returns share.AmountIDR of the bill's verified payments,
// newest first, and re-derives the bill status.
func reverseBillShare(ctx context.Context, tx *sql.Tx, schoolID, refundID, reason, actorID string, share model.BillRefundShare) error {
	if _, err := tx.ExecContext(ctx,
		`SELECT 1 FROM student_bills WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, share.BillID, schoolID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT p.id::text,p.amount_idr-COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv WHERE rv.payment_id=p.id),0)
		 FROM bill_payments p WHERE p.student_bill_id::text=$1 AND p.school_id=$2 AND p.status='verified'
		 ORDER BY p.verified_at DESC,p.id`, share.BillID, schoolID)
	if err != nil {
		return err
	}
	var sources []model.RefundSource
	for rows.Next() {
		var s model.RefundSource
		if err := rows.Scan(&s.PaymentID, &s.Remaining); err != nil {
			rows.Close()
			return err
		}
		sources = append(sources, s)
	}
	rows.Close()
	parts, err := model.SpreadRefund(share.AmountIDR, sources)
	if err != nil {
		return errRefundSourceShort
	}
	for _, p := range parts {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bill_payment_reversals(school_id,payment_id,amount_idr,reason,actor_id,idempotency_key,refund_id)
			 VALUES($1,$2,$3,$4,$5,$6,$7)`,
			schoolID, p.PaymentID, p.AmountIDR, "Refund: "+reason, actorID, "refund:"+refundID+":"+p.PaymentID, refundID); err != nil {
			return err
		}
	}
	return recomputeBillStatusQ(ctx, tx, schoolID, share.BillID)
}

// payRefundFromDeposit takes the deposit portion of a refund out of the
// student's deposit balance with a ledger movement; bill payments are not
// touched. It returns the balance before and after.
func payRefundFromDeposit(ctx context.Context, tx *sql.Tx, schoolID, studentID, refundID, reason string, amount int64) (int64, int64, error) {
	var balance int64
	err := tx.QueryRowContext(ctx,
		`SELECT balance_idr FROM student_deposits WHERE school_id=$1 AND student_user_id::text=$2 FOR UPDATE`,
		schoolID, studentID).Scan(&balance)
	if err == sql.ErrNoRows || (err == nil && balance < amount) {
		return balance, balance, errRefundSourceShort
	}
	if err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE student_deposits SET balance_idr=balance_idr-$3, updated_at=now() WHERE school_id=$1 AND student_user_id::text=$2`,
		schoolID, studentID, amount); err != nil {
		return 0, 0, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO deposit_movements(school_id,student_user_id,amount_idr,balance_after_idr,reason,ref)
		 VALUES($1,$2::uuid,$3,$4,$5,$6)`,
		schoolID, studentID, -amount, balance-amount, "Refund deposit: "+reason, "refund:"+refundID); err != nil {
		return 0, 0, err
	}
	return balance, balance - amount, nil
}

// sortShares orders shares by bill id so concurrent processing locks bills
// in the same order.
func sortShares(s []model.BillRefundShare) {
	sort.Slice(s, func(i, j int) bool { return s[i].BillID < s[j].BillID })
}

// reportRefundApprovalFailure puts a student refund that core's Approval
// Center refused or could not take into the Integration Exception Queue
// (menu 80), so the missing 'refund' flow is fixed instead of the refund
// silently disappearing. Kind 'callback' is the matrix entry for a failed
// service-to-service hand-off; core has no 'high' severity, 'critical' is
// its highest.
func (a *App) reportRefundApprovalFailure(ctx context.Context, schoolID, studentID, refundID, idemKey, reason string, amount int64, cause error) {
	log.Printf("refund %s for student %s: approval request failed: %v", refundID, studentID, cause)
	if a.Core == nil || !a.Core.Configured() {
		return
	}
	key := idemKey
	if key == "" {
		key = refundID
	}
	if err := a.Core.CreateException(ctx, client.ExceptionEvent{
		SchoolID:      schoolID,
		Kind:          "callback",
		Severity:      "critical",
		CorrelationID: refundID,
		ErrorMessage: fmt.Sprintf("Pengajuan refund siswa %s sebesar %s tidak diterima Approval Center: %v",
			studentID, formatRupiah(amount), cause),
		PayloadSummary: map[string]any{"event": "refund_approval_failed", "student_user_id": studentID,
			"amount_idr": amount, "reason": reason, "idempotency_key": idemKey, "request_type": reqTypeRefund},
		MaxAttempts:    1,
		IdempotencyKey: "refund-approval:" + key,
	}); err != nil {
		log.Printf("refund %s: exception queue: %v", refundID, err)
	}
}
