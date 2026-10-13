package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// JournalSettings is a unit's (school's) account mapping for the billing
// auto-journals (billing_journal_settings, BR-ACC-05). Empty = unmapped.
type JournalSettings struct {
	UnitDivCode    string `json:"unit_div_code"`
	ReceivableAcc  string `json:"receivable_acc"`
	CashAcc        string `json:"cash_acc"`
	BankAcc        string `json:"bank_acc"`
	DiscountAcc    string `json:"discount_acc"`
	ScholarshipAcc string `json:"scholarship_acc"`
	RefundAcc      string `json:"refund_acc"`
	DepositAcc     string `json:"deposit_acc"`
}

// ComponentAccount maps one fee component to its accounts. ReceivableAcc
// overrides the unit's receivable control account when set.
type ComponentAccount struct {
	Code          string `json:"component_code"`
	Name          string `json:"name"`
	RevenueAcc    string `json:"revenue_acc"`
	ReceivableAcc string `json:"receivable_acc"`
}

// ComponentAmount is a signed rupiah amount for one fee component.
type ComponentAmount struct {
	Component string
	Name      string
	AmountIDR int64
}

// LabelAmount is a rupiah amount with its display label.
type LabelAmount struct {
	Label     string
	AmountIDR int64
}

// MissingAccountsError lists the mappings a journal needs but lacks. The
// queue reports it to the exception queue instead of guessing an account.
type MissingAccountsError struct{ Missing []string }

func (e *MissingAccountsError) Error() string {
	return "akun jurnal belum dipetakan: " + strings.Join(e.Missing, ", ")
}

type missing map[string]bool

func (m missing) need(acc, label string) string {
	if strings.TrimSpace(acc) == "" {
		m[label] = true
	}
	return acc
}

func (m missing) err() error {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return &MissingAccountsError{Missing: out}
}

// Posting is one debit-positive amount on an account before netting.
type Posting struct {
	Acc      string
	Desc     string
	DebitIDR int64 // negative = credit
}

// NetJournal nets postings per (account, description) in first-seen order
// and emits each on the side its sign says. All-zero input gives no lines.
func NetJournal(ps []Posting) ([]JournalLine, error) {
	type key struct{ acc, desc string }
	net := map[key]int64{}
	var order []key
	for _, p := range ps {
		k := key{p.Acc, p.Desc}
		if _, ok := net[k]; !ok {
			order = append(order, k)
		}
		net[k] += p.DebitIDR
	}
	var lines []JournalLine
	for _, k := range order {
		switch v := net[k]; {
		case v > 0:
			lines = append(lines, JournalLine{AccCode: k.acc, Description: k.desc, DebitIDR: v})
		case v < 0:
			lines = append(lines, JournalLine{AccCode: k.acc, Description: k.desc, CreditIDR: -v})
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	if err := ValidateJournal(lines); err != nil {
		return nil, err
	}
	return lines, nil
}

func componentLabel(c ComponentAmount, acc ComponentAccount) string {
	switch {
	case acc.Name != "":
		return acc.Name
	case c.Name != "":
		return c.Name
	case c.Component != "":
		return c.Component
	}
	return "tanpa kode"
}

func receivableFor(set JournalSettings, acc ComponentAccount) string {
	if strings.TrimSpace(acc.ReceivableAcc) != "" {
		return acc.ReceivableAcc
	}
	return set.ReceivableAcc
}

// BillIssuedJournal is the day's journal for bills issued (FR-34-001):
// receivable debit / revenue credit per component, gross of discounts
// (discounts are their own journal). Negative amounts (ad-hoc adjustments)
// reverse the pair. depositIDR is the part of downward adjustments that
// exceeded what was still owed and was parked as student deposit.
func BillIssuedJournal(set JournalSettings, comps map[string]ComponentAccount, items []ComponentAmount, depositIDR int64) ([]JournalLine, error) {
	m := missing{}
	var ps []Posting
	for _, it := range items {
		acc, ok := comps[it.Component]
		label := componentLabel(it, acc)
		if !ok || it.Component == "" {
			m["pendapatan komponen "+label] = true
			continue
		}
		recv := m.need(receivableFor(set, acc), "piutang komponen "+label)
		ps = append(ps,
			Posting{Acc: recv, Desc: "Piutang " + label, DebitIDR: it.AmountIDR},
			Posting{Acc: acc.RevenueAcc, Desc: "Pendapatan " + label, DebitIDR: -it.AmountIDR})
	}
	if depositIDR > 0 {
		ps = append(ps,
			Posting{Acc: m.need(set.ReceivableAcc, "piutang (kontrol)"), Desc: "Piutang tagihan", DebitIDR: depositIDR},
			Posting{Acc: m.need(set.DepositAcc, "deposit siswa"), Desc: "Deposit kelebihan bayar", DebitIDR: -depositIDR})
	}
	if err := m.err(); err != nil {
		return nil, err
	}
	return NetJournal(ps)
}

// DiscountJournal books the day's discounts and scholarships (FR-34-003):
// expense per discount label, credited to the receivable control account.
func DiscountJournal(set JournalSettings, discounts, scholarships []LabelAmount) ([]JournalLine, error) {
	m := missing{}
	var ps []Posting
	var total int64
	for _, d := range discounts {
		ps = append(ps, Posting{Acc: m.need(set.DiscountAcc, "potongan"), Desc: "Potongan: " + d.Label, DebitIDR: d.AmountIDR})
		total += d.AmountIDR
	}
	for _, s := range scholarships {
		ps = append(ps, Posting{Acc: m.need(set.ScholarshipAcc, "beasiswa"), Desc: "Beasiswa: " + s.Label, DebitIDR: s.AmountIDR})
		total += s.AmountIDR
	}
	if total != 0 {
		ps = append(ps, Posting{Acc: m.need(set.ReceivableAcc, "piutang (kontrol)"), Desc: "Pengurang piutang tagihan", DebitIDR: -total})
	}
	if err := m.err(); err != nil {
		return nil, err
	}
	return NetJournal(ps)
}

// ReceiptDay sums one day of non-gateway receipts (FR-34-002).
type ReceiptDay struct {
	CashIDR        int64             // cashier cash
	BankIDR        int64             // cashier transfer + verified transfer proofs
	FromDepositIDR int64             // bills paid from the student deposit
	ToDepositIDR   int64             // overpayment parked as deposit
	Allocated      []ComponentAmount // bill allocations per component
}

// ReceiptJournal debits cash / bank / deposit and credits the receivable per
// component (override or control). Money received but not allocated to a
// component item is credited to the receivable control account.
func ReceiptJournal(set JournalSettings, comps map[string]ComponentAccount, d ReceiptDay) ([]JournalLine, error) {
	m := missing{}
	var ps []Posting
	if d.CashIDR != 0 {
		ps = append(ps, Posting{Acc: m.need(set.CashAcc, "kas"), Desc: "Penerimaan tunai", DebitIDR: d.CashIDR})
	}
	if d.BankIDR != 0 {
		ps = append(ps, Posting{Acc: m.need(set.BankAcc, "bank"), Desc: "Penerimaan transfer bank", DebitIDR: d.BankIDR})
	}
	if d.FromDepositIDR != 0 {
		ps = append(ps, Posting{Acc: m.need(set.DepositAcc, "deposit siswa"), Desc: "Pemakaian deposit siswa", DebitIDR: d.FromDepositIDR})
	}
	if d.ToDepositIDR != 0 {
		ps = append(ps, Posting{Acc: m.need(set.DepositAcc, "deposit siswa"), Desc: "Deposit kelebihan bayar", DebitIDR: -d.ToDepositIDR})
	}
	rest := d.CashIDR + d.BankIDR + d.FromDepositIDR - d.ToDepositIDR
	for _, a := range d.Allocated {
		acc := comps[a.Component]
		label := componentLabel(a, acc)
		ps = append(ps, Posting{Acc: m.need(receivableFor(set, acc), "piutang (kontrol)"), Desc: "Pelunasan piutang " + label, DebitIDR: -a.AmountIDR})
		rest -= a.AmountIDR
	}
	if rest < 0 {
		return nil, fmt.Errorf("alokasi penerimaan melebihi uang diterima (%d)", -rest)
	}
	if rest > 0 {
		ps = append(ps, Posting{Acc: m.need(set.ReceivableAcc, "piutang (kontrol)"), Desc: "Pelunasan piutang tagihan", DebitIDR: -rest})
	}
	if err := m.err(); err != nil {
		return nil, err
	}
	return NetJournal(ps)
}

// RefundJournal books processed bill refunds: refund account debit, cash
// credit (FR-34-003).
func RefundJournal(set JournalSettings, totalIDR int64) ([]JournalLine, error) {
	m := missing{}
	ps := []Posting{
		{Acc: m.need(set.RefundAcc, "refund"), Desc: "Refund tagihan", DebitIDR: totalIDR},
		{Acc: m.need(set.CashAcc, "kas"), Desc: "Pembayaran refund", DebitIDR: -totalIDR},
	}
	if err := m.err(); err != nil {
		return nil, err
	}
	return NetJournal(ps)
}

// JournalBackoff is the wait before outbox attempt n+1 after n failures:
// 1, 2, 4, 8 ... minutes, capped at one hour.
func JournalBackoff(failures int) time.Duration {
	if failures < 1 {
		failures = 1
	}
	d := time.Minute
	for i := 1; i < failures && d < time.Hour; i++ {
		d *= 2
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
