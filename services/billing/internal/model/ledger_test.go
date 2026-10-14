package model

import (
	"testing"
	"time"
)

func TestBuildLedger(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 7, day, 9, 0, 0, 0, time.UTC) }
	entries := []LedgerEntry{
		{At: d(10), Kind: LedgerPayment, CreditIDR: 400},
		{At: d(1), Kind: LedgerBill, DebitIDR: 1000},
		{At: d(1), Kind: LedgerAdjustment, CreditIDR: 100},
		{At: d(15), Kind: LedgerPayment, CreditIDR: 700},
		{At: d(15), Kind: LedgerReversal, DebitIDR: 200},
		{At: d(15), Kind: LedgerDeposit, DepositIDR: 200},
		{At: d(20), Kind: LedgerDeposit, DepositIDR: -50},
	}
	from, to := d(5), d(16)
	cases := []struct {
		name                  string
		from, to              *time.Time
		lines                 int
		opening, closing, dep int64
		net                   int64
		firstKind             string
		firstBalance          int64
	}{
		{"whole card", nil, nil, 7, 0, 0, 150, -150, LedgerBill, 1000},
		{"from folds opening", &from, nil, 5, 900, 0, 150, -150, LedgerPayment, 500},
		{"to cuts later lines", nil, &to, 6, 0, 0, 200, -200, LedgerBill, 1000},
	}
	for _, tc := range cases {
		lines, tot := BuildLedger(entries, tc.from, tc.to)
		if len(lines) != tc.lines || tot.OpeningIDR != tc.opening || tot.ClosingIDR != tc.closing ||
			tot.ClosingDepositIDR != tc.dep || tot.NetPositionIDR != tc.net ||
			lines[0].Kind != tc.firstKind || lines[0].BalanceIDR != tc.firstBalance {
			t.Errorf("%s: lines=%+v totals=%+v", tc.name, lines, tot)
		}
	}
	// Same instant: the bill precedes its adjustment.
	lines, _ := BuildLedger(entries, nil, nil)
	if lines[1].Kind != LedgerAdjustment || lines[1].BalanceIDR != 900 {
		t.Fatalf("order: %+v", lines[:2])
	}
}

func TestSignedLedger(t *testing.T) {
	cases := []struct{ in, d, c int64 }{{500, 500, 0}, {-300, 0, 300}, {0, 0, 0}}
	for _, tc := range cases {
		if d, c := SignedLedger(tc.in); d != tc.d || c != tc.c {
			t.Errorf("%d -> %d/%d", tc.in, d, c)
		}
	}
}

func TestOpenDepositLots(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		name  string
		moves []DepositMovement
		want  []DepositLot
	}{
		{"nothing", nil, []DepositLot{}},
		{"single inflow", []DepositMovement{{d(1), 500}}, []DepositLot{{d(1), 500}}},
		{"outflow eats oldest first", []DepositMovement{{d(1), 500}, {d(5), 300}, {d(9), -600}},
			[]DepositLot{{d(5), 200}}},
		{"unsorted input", []DepositMovement{{d(9), -100}, {d(1), 500}}, []DepositLot{{d(1), 400}}},
		{"fully used", []DepositMovement{{d(1), 500}, {d(2), -500}}, []DepositLot{}},
	}
	for _, tc := range cases {
		got := OpenDepositLots(tc.moves)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %+v", tc.name, got)
		}
		for i := range got {
			if !got[i].At.Equal(tc.want[i].At) || got[i].RemainingIDR != tc.want[i].RemainingIDR {
				t.Fatalf("%s: %+v", tc.name, got)
			}
		}
	}
}

func TestAgeDays(t *testing.T) {
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		asOf time.Time
		want int
	}{
		{base, 0}, {base.Add(-time.Hour), 0}, {base.Add(47 * time.Hour), 1}, {base.AddDate(0, 0, 45), 45},
	}
	for _, tc := range cases {
		if got := AgeDays(base, tc.asOf); got != tc.want {
			t.Errorf("%v: %d want %d", tc.asOf, got, tc.want)
		}
	}
}
