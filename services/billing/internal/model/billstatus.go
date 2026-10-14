package model

import "time"

// EffectiveBillStatus is the status shown to every reader. It is always
// derived from the ledger (net vs verified allocations minus reversals) so a
// stale stored status can never say 'paid' while money is still owed; only
// 'void' is taken from storage because voiding is an explicit decision.
func EffectiveBillStatus(stored string, netIDR, paidIDR int64) BillStatus {
	if BillStatus(stored) == BillVoid {
		return BillVoid
	}
	return DeriveBillStatus(netIDR, paidIDR)
}

// BillOverdue reports whether a bill with a remaining balance is past its
// due date. The due date is a whole calendar day in the school's time zone:
// a bill due 10 Oct is not overdue on 10 Oct and is overdue from 11 Oct.
func BillOverdue(status BillStatus, due *time.Time, outstandingIDR int64, now time.Time, loc *time.Location) bool {
	if due == nil || outstandingIDR <= 0 || status == BillVoid || status == BillPaid {
		return false
	}
	return CalendarDaysBetween(*due, now, loc) > 0
}
