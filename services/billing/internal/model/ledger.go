package model

import (
	"sort"
	"time"
)

// Ledger entry kinds of the student receivable card (BUG-130).
const (
	LedgerBill       = "bill"       // bill issued (net after discounts)
	LedgerAdjustment = "adjustment" // prorate / adjust / cancel
	LedgerVoid       = "void"       // legacy void without an adjustment row
	LedgerPayment    = "payment"    // verified payment
	LedgerReversal   = "reversal"   // payment correction or refund
	LedgerDeposit    = "deposit"    // deposit movement (no receivable effect)
)

// LedgerEntry is one line of the card. Debit raises the receivable, Credit
// lowers it; DepositIDR moves the student's deposit (overpayment) balance.
type LedgerEntry struct {
	At          time.Time `json:"at"`
	Kind        string    `json:"kind"`
	Ref         string    `json:"ref"`
	BillID      string    `json:"bill_id,omitempty"`
	Description string    `json:"description"`
	DebitIDR    int64     `json:"debit_idr"`
	CreditIDR   int64     `json:"credit_idr"`
	DepositIDR  int64     `json:"deposit_idr"`
	// Running balances after this line.
	BalanceIDR        int64 `json:"balance_idr"`
	DepositBalanceIDR int64 `json:"deposit_balance_idr"`
}

// LedgerTotals closes the card: receivable minus deposit is what the
// student still owes (negative = the school owes the student).
type LedgerTotals struct {
	OpeningIDR        int64 `json:"opening_idr"`
	OpeningDepositIDR int64 `json:"opening_deposit_idr"`
	DebitIDR          int64 `json:"debit_idr"`
	CreditIDR         int64 `json:"credit_idr"`
	ClosingIDR        int64 `json:"closing_idr"`
	ClosingDepositIDR int64 `json:"closing_deposit_idr"`
	NetPositionIDR    int64 `json:"net_position_idr"`
}

// kindOrder keeps same-instant lines stable: a bill before what adjusts or
// pays it.
var kindOrder = map[string]int{LedgerBill: 0, LedgerAdjustment: 1, LedgerVoid: 2, LedgerPayment: 3, LedgerReversal: 4, LedgerDeposit: 5}

// BuildLedger orders the entries chronologically and computes running
// balances. Entries before from (when set) fold into the opening balance;
// entries after to (when set) are left out.
func BuildLedger(entries []LedgerEntry, from, to *time.Time) ([]LedgerEntry, LedgerTotals) {
	sorted := append([]LedgerEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].At.Equal(sorted[j].At) {
			return sorted[i].At.Before(sorted[j].At)
		}
		return kindOrder[sorted[i].Kind] < kindOrder[sorted[j].Kind]
	})
	var t LedgerTotals
	bal, dep := int64(0), int64(0)
	out := []LedgerEntry{}
	for _, e := range sorted {
		if to != nil && e.At.After(*to) {
			break
		}
		bal += e.DebitIDR - e.CreditIDR
		dep += e.DepositIDR
		if from != nil && e.At.Before(*from) {
			t.OpeningIDR, t.OpeningDepositIDR = bal, dep
			continue
		}
		e.BalanceIDR, e.DepositBalanceIDR = bal, dep
		t.DebitIDR += e.DebitIDR
		t.CreditIDR += e.CreditIDR
		out = append(out, e)
	}
	t.ClosingIDR, t.ClosingDepositIDR = bal, dep
	t.NetPositionIDR = bal - dep
	return out, t
}

// SignedLedger turns a signed receivable change into debit/credit.
func SignedLedger(delta int64) (debit, credit int64) {
	if delta >= 0 {
		return delta, 0
	}
	return 0, -delta
}

// DepositMovement is one change of a student's deposit balance.
type DepositMovement struct {
	At        time.Time
	AmountIDR int64 // positive = in, negative = used/refunded
}

// DepositLot is the part of one deposit inflow still unused.
type DepositLot struct {
	At           time.Time `json:"received_at"`
	RemainingIDR int64     `json:"remaining_idr"`
}

// OpenDepositLots consumes outflows from the oldest inflows first (FIFO)
// and returns what is left of each inflow, oldest first (BUG-132: the age
// of unallocated funds).
func OpenDepositLots(moves []DepositMovement) []DepositLot {
	sorted := append([]DepositMovement(nil), moves...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].At.Before(sorted[j].At) })
	var lots []DepositLot
	for _, m := range sorted {
		if m.AmountIDR > 0 {
			lots = append(lots, DepositLot{At: m.At, RemainingIDR: m.AmountIDR})
			continue
		}
		use := -m.AmountIDR
		for i := 0; i < len(lots) && use > 0; i++ {
			take := min(lots[i].RemainingIDR, use)
			lots[i].RemainingIDR -= take
			use -= take
		}
	}
	out := []DepositLot{}
	for _, l := range lots {
		if l.RemainingIDR > 0 {
			out = append(out, l)
		}
	}
	return out
}

// AgeDays is the whole days between at and asOf (never negative).
func AgeDays(at, asOf time.Time) int {
	if !asOf.After(at) {
		return 0
	}
	return int(asOf.Sub(at).Hours() / 24)
}
