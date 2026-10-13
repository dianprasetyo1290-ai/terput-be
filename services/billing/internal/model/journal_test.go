package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func balanced(t *testing.T, lines []JournalLine) (debit, credit int64) {
	t.Helper()
	for _, l := range lines {
		debit += l.DebitIDR
		credit += l.CreditIDR
	}
	if debit != credit {
		t.Fatalf("unbalanced: debit %d credit %d (%+v)", debit, credit, lines)
	}
	return debit, credit
}

var testSet = JournalSettings{ReceivableAcc: "1-1300", CashAcc: "1-1100", BankAcc: "1-1200", DiscountAcc: "5-1100",
	ScholarshipAcc: "5-1200", RefundAcc: "4-1900", DepositAcc: "2-1500"}

var testComps = map[string]ComponentAccount{
	"SPP":    {Code: "SPP", Name: "SPP Bulanan", RevenueAcc: "4-1100"},
	"GEDUNG": {Code: "GEDUNG", Name: "Uang Gedung", RevenueAcc: "4-1200", ReceivableAcc: "1-1310"},
}

func TestBillIssuedJournalPerComponentBalanced(t *testing.T) {
	lines, err := BillIssuedJournal(testSet, testComps, []ComponentAmount{
		{Component: "SPP", AmountIDR: 900000}, {Component: "GEDUNG", AmountIDR: 2500000},
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := balanced(t, lines); d != 3400000 {
		t.Fatalf("total %d", d)
	}
	want := map[string]JournalLine{
		"1-1300": {DebitIDR: 900000}, "4-1100": {CreditIDR: 900000},
		"1-1310": {DebitIDR: 2500000}, "4-1200": {CreditIDR: 2500000},
	}
	for _, l := range lines {
		w := want[l.AccCode]
		if l.DebitIDR != w.DebitIDR || l.CreditIDR != w.CreditIDR {
			t.Fatalf("line %+v want %+v", l, w)
		}
	}
}

func TestBillIssuedJournalAdjustmentWithDeposit(t *testing.T) {
	// Paid bill of 100 adjusted down by 30: revenue reversed, overpayment parked as deposit.
	lines, err := BillIssuedJournal(testSet, testComps, []ComponentAmount{{Component: "SPP", AmountIDR: -30000}}, 30000)
	if err != nil {
		t.Fatal(err)
	}
	balanced(t, lines)
	got := map[string][2]int64{}
	for _, l := range lines {
		v := got[l.AccCode]
		got[l.AccCode] = [2]int64{v[0] + l.DebitIDR, v[1] + l.CreditIDR}
	}
	if got["4-1100"] != [2]int64{30000, 0} || got["2-1500"] != [2]int64{0, 30000} {
		t.Fatalf("lines %+v", lines)
	}
	if r := got["1-1300"]; r[0]-r[1] != 0 {
		t.Fatalf("receivable must net to zero, got %+v", r)
	}
}

func TestBillIssuedJournalUnmappedComponent(t *testing.T) {
	_, err := BillIssuedJournal(testSet, testComps, []ComponentAmount{{Component: "SERAGAM", Name: "Seragam", AmountIDR: 1}}, 0)
	var miss *MissingAccountsError
	if !errors.As(err, &miss) || !strings.Contains(err.Error(), "Seragam") {
		t.Fatalf("want missing mapping error, got %v", err)
	}
	_, err = BillIssuedJournal(JournalSettings{}, testComps, []ComponentAmount{{Component: "SPP", AmountIDR: 1}}, 0)
	if !errors.As(err, &miss) {
		t.Fatalf("missing receivable control must be reported, got %v", err)
	}
}

func TestDiscountJournal(t *testing.T) {
	lines, err := DiscountJournal(testSet, []LabelAmount{{"Saudara kandung", 50000}}, []LabelAmount{{"Beasiswa prestasi", 200000}})
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := balanced(t, lines); d != 250000 {
		t.Fatalf("total %d", d)
	}
	if lines[len(lines)-1].AccCode != "1-1300" || lines[len(lines)-1].CreditIDR != 250000 {
		t.Fatalf("receivable credit missing: %+v", lines)
	}
	if _, err := DiscountJournal(JournalSettings{ReceivableAcc: "R", DiscountAcc: "D"}, nil, []LabelAmount{{"B", 1}}); err == nil {
		t.Fatal("scholarship without account must fail")
	}
}

func TestReceiptJournal(t *testing.T) {
	lines, err := ReceiptJournal(testSet, testComps, ReceiptDay{
		CashIDR: 500000, BankIDR: 300000, FromDepositIDR: 20000, ToDepositIDR: 70000,
		Allocated: []ComponentAmount{{Component: "SPP", AmountIDR: 450000}, {Component: "GEDUNG", AmountIDR: 250000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := balanced(t, lines); d != 820000 {
		t.Fatalf("debit total %d", d)
	}
	var unallocated int64
	for _, l := range lines {
		if l.Description == "Pelunasan piutang tagihan" {
			unallocated = l.CreditIDR
		}
	}
	if unallocated != 50000 {
		t.Fatalf("unallocated remainder %d want 50000 (%+v)", unallocated, lines)
	}
	if _, err := ReceiptJournal(testSet, testComps, ReceiptDay{CashIDR: 1, Allocated: []ComponentAmount{{Component: "SPP", AmountIDR: 2}}}); err == nil {
		t.Fatal("over-allocation must fail")
	}
}

func TestRefundJournal(t *testing.T) {
	lines, err := RefundJournal(testSet, 150000)
	if err != nil {
		t.Fatal(err)
	}
	balanced(t, lines)
	if _, err := RefundJournal(JournalSettings{CashAcc: "K"}, 1); err == nil {
		t.Fatal("refund without refund account must fail")
	}
}

func TestNetJournalNetsAndDropsZero(t *testing.T) {
	lines, err := NetJournal([]Posting{{"A", "x", 100}, {"B", "y", -100}, {"A", "x", -100}, {"B", "y", 100}})
	if err != nil || lines != nil {
		t.Fatalf("all-zero must give no lines, got %+v %v", lines, err)
	}
	if _, err := NetJournal([]Posting{{"A", "x", 100}, {"B", "y", -90}}); err == nil {
		t.Fatal("unbalanced postings must fail")
	}
}

func TestJournalBackoff(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute}
	for i, w := range want {
		if got := JournalBackoff(i + 1); got != w {
			t.Fatalf("backoff(%d)=%s want %s", i+1, got, w)
		}
	}
	if JournalBackoff(30) != time.Hour {
		t.Fatal("backoff must cap at one hour")
	}
}
