package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Refund lifecycle (PRD menu 25): requested -> approved|rejected (decided in
// core's Approval Center) -> processed (bank transfer recorded).
const (
	RefundRequested = "requested"
	RefundApproved  = "approved"
	RefundRejected  = "rejected"
	RefundProcessed = "processed"
)

// RefundDecision maps an Approval Center decision on a refund in status
// current to the next status. changed is false when the refund was already
// decided (a replayed callback), which the caller acknowledges as a no-op.
func RefundDecision(current, decision, decidedBy, requestedBy string) (next string, changed bool, err error) {
	if current != RefundRequested {
		return current, false, nil
	}
	switch decision {
	case "approved":
		next = RefundApproved
	case "rejected":
		next = RefundRejected
	default:
		return current, false, errors.New("keputusan harus approved atau rejected")
	}
	if strings.TrimSpace(decidedBy) == "" {
		return current, false, errors.New("decided_by wajib diisi")
	}
	if decidedBy == requestedBy {
		return current, false, errors.New("penyetuju harus berbeda dari pengaju refund")
	}
	return next, true, nil
}

// TransferProof is the bank transfer that pays a refund out.
type TransferProof struct {
	Ref  string
	Date time.Time
}

const maxTransferRefLen = 100

// ParseTransferProof validates the transfer reference and date entered when
// a refund is processed. date is YYYY-MM-DD (empty = today in loc) and may
// not lie in the future.
func ParseTransferProof(ref, date string, now time.Time, loc *time.Location) (TransferProof, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return TransferProof{}, errors.New("nomor referensi transfer wajib diisi")
	}
	if utf8.RuneCountInString(ref) > maxTransferRefLen {
		return TransferProof{}, errors.New("nomor referensi transfer maksimal 100 karakter")
	}
	y, m, d := now.In(loc).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if strings.TrimSpace(date) == "" {
		return TransferProof{Ref: ref, Date: today}, nil
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return TransferProof{}, errors.New("tanggal transfer harus berformat YYYY-MM-DD")
	}
	if t.After(today) {
		return TransferProof{}, errors.New("tanggal transfer tidak boleh di masa depan")
	}
	return TransferProof{Ref: ref, Date: t}, nil
}

// RefundNoticeText is the notification for a refund reaching status; an
// empty title means the status is not announced.
func RefundNoticeText(status, amount, note, transferRef string) (title, body string) {
	switch status {
	case RefundApproved:
		return "Refund disetujui", fmt.Sprintf("Refund sebesar %s telah disetujui dan siap ditransfer.", amount)
	case RefundRejected:
		body = fmt.Sprintf("Pengajuan refund sebesar %s ditolak.", amount)
		if strings.TrimSpace(note) != "" {
			body += " Alasan: " + strings.TrimSpace(note) + "."
		}
		return "Refund ditolak", body + " Saldo pembayaran tidak berubah."
	case RefundProcessed:
		return "Refund telah ditransfer", fmt.Sprintf("Refund sebesar %s telah ditransfer (referensi %s).", amount, transferRef)
	}
	return "", ""
}

// RefundableBill is one bill's paid balance that a student refund may still
// return (verified - reversed - refunds already pending against it).
type RefundableBill struct {
	BillID        string
	RefundableIDR int64
}

// BillRefundShare is the part of a student refund that reverses payments of
// one bill.
type BillRefundShare struct {
	BillID    string
	AmountIDR int64
}

// SplitStudentRefund splits a withdrawal/transfer refund (Menu 78) into the
// portion paid out of the student's overpayment deposit and the portions
// that return payments of individual bills. The deposit is used first: it is
// money the school holds without any bill behind it, so returning it never
// reopens a bill. The remainder is taken from bills in the order given. It
// fails when the deposit and the bills together cannot cover the amount.
func SplitStudentRefund(amount, depositAvailable int64, bills []RefundableBill) (int64, []BillRefundShare, error) {
	if amount <= 0 {
		return 0, nil, errors.New("refund amount must be positive")
	}
	deposit := min(max(depositAvailable, 0), amount)
	left := amount - deposit
	var shares []BillRefundShare
	for _, b := range bills {
		if left == 0 {
			break
		}
		if b.RefundableIDR <= 0 {
			continue
		}
		take := min(b.RefundableIDR, left)
		shares = append(shares, BillRefundShare{BillID: b.BillID, AmountIDR: take})
		left -= take
	}
	if left > 0 {
		return 0, nil, fmt.Errorf("refund melebihi deposit dan saldo terbayar tagihan sebesar %d", left)
	}
	return deposit, shares, nil
}
