package model

import (
	"errors"
	"strings"
)

// Bill change requests (BUG-053): cancelling or re-pricing an issued bill
// waits for the Approval Center; the bill stays active until approved and
// its original values are kept.
const (
	BillChangeCancel = "cancel"
	BillChangeAdjust = "adjust"

	BillChangePending  = "pending_approval"
	BillChangeApplied  = "applied"
	BillChangeRejected = "rejected"
	BillChangeFailed   = "failed"
)

// ValidateBillChange checks a request against the bill as it is now.
func ValidateBillChange(kind string, newNet *int64, reason, billStatus string, net int64) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("alasan wajib diisi")
	}
	if billStatus == string(BillVoid) {
		return errors.New("tagihan sudah dibatalkan")
	}
	switch kind {
	case BillChangeCancel:
		if newNet != nil {
			return errors.New("pembatalan tidak memakai new_net_idr")
		}
	case BillChangeAdjust:
		if newNet == nil {
			return errors.New("new_net_idr wajib diisi untuk penyesuaian")
		}
		if *newNet <= 0 {
			return errors.New("nilai tagihan baru harus lebih dari 0; untuk menghapus seluruh tagihan ajukan pembatalan")
		}
		if *newNet == net {
			return errors.New("nilai tagihan baru sama dengan nilai sekarang")
		}
	default:
		return errors.New("kind harus cancel atau adjust")
	}
	return nil
}

// BillChangePlan is what applying an approved change does to a bill.
type BillChangePlan struct {
	NetAfterIDR int64      // net after the change (0 for a cancellation)
	DeltaIDR    int64      // NetAfter - net; the adjustment line amount
	DepositIDR  int64      // paid money above the new net, moved to the deposit
	Status      BillStatus // void for a cancellation, else derived from the ledger
}

// PlanBillChange computes the effect of an approved change given the
// bill's current net and the net amount paid on it.
func PlanBillChange(kind string, net, paid, newNet int64) (BillChangePlan, error) {
	if paid < 0 {
		paid = 0
	}
	target := newNet
	if kind == BillChangeCancel {
		target = 0
	}
	if target < 0 {
		return BillChangePlan{}, errors.New("nilai tagihan baru tidak boleh negatif")
	}
	p := BillChangePlan{NetAfterIDR: target, DeltaIDR: target - net}
	if paid > target {
		p.DepositIDR = paid - target
		paid = target
	}
	if kind == BillChangeCancel {
		p.Status = BillVoid
	} else {
		p.Status = DeriveBillStatus(target, paid)
	}
	return p, nil
}

// SplitSigned spreads a signed amount over weights (largest remainder),
// keeping the sign: used to post an adjustment line per fee component.
func SplitSigned(amount int64, weights []int64) []int64 {
	if amount >= 0 {
		return SplitByWeights(amount, weights)
	}
	parts := SplitByWeights(-amount, weights)
	for i := range parts {
		parts[i] = -parts[i]
	}
	return parts
}

// ApprovalOutcome maps an Approval Center decision on a pending request to
// the next status. A replay on a decided request is not a change; the
// approver may never be the maker.
func ApprovalOutcome(current, pending, decision, decidedBy, requestedBy string) (approved, changed bool, err error) {
	if current != pending {
		return false, false, nil
	}
	switch decision {
	case "approved":
		approved = true
	case "rejected":
	default:
		return false, false, errors.New("keputusan harus approved atau rejected")
	}
	if strings.TrimSpace(decidedBy) == "" {
		return false, false, errors.New("decided_by wajib diisi")
	}
	if decidedBy == requestedBy {
		return false, false, errors.New("penyetuju harus berbeda dari pengaju")
	}
	return approved, true, nil
}
