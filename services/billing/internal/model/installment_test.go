package model

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestComponentsForStudent(t *testing.T) {
	comps := []PlannedComponent{
		{Code: "SPP", Kind: KindMonthly}, {Code: "KEG", Kind: KindAnnual}, {Code: "PANGKAL", Kind: KindOneTime}, {Code: "OLD", Kind: ""},
	}
	cases := []struct {
		name       string
		year, ever map[string]bool
		want       []string
	}{
		{"new student gets everything", nil, nil, []string{"SPP", "KEG", "PANGKAL", "OLD"}},
		{"annual already billed this year", map[string]bool{"KEG": true}, map[string]bool{"KEG": true}, []string{"SPP", "PANGKAL", "OLD"}},
		{"annual billed last year only", nil, map[string]bool{"KEG": true}, []string{"SPP", "KEG", "PANGKAL", "OLD"}},
		{"one-time ever billed", nil, map[string]bool{"PANGKAL": true, "OLD": true}, []string{"SPP", "KEG"}},
		{"monthly never skipped", map[string]bool{"SPP": true}, map[string]bool{"SPP": true}, []string{"SPP", "KEG", "PANGKAL", "OLD"}},
	}
	for _, tc := range cases {
		got := ComponentsForStudent(comps, tc.year, tc.ever)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %v", tc.name, got)
		}
		for i := range got {
			if got[i].Code != tc.want[i] {
				t.Fatalf("%s: %v", tc.name, got)
			}
		}
	}
}

func TestAddMonthsClamped(t *testing.T) {
	cases := []struct {
		in   time.Time
		n    int
		want time.Time
	}{
		{day(2026, 1, 31), 1, day(2026, 2, 28)},
		{day(2028, 1, 31), 1, day(2028, 2, 29)},
		{day(2026, 11, 10), 2, day(2027, 1, 10)},
		{day(2026, 3, 15), 0, day(2026, 3, 15)},
	}
	for _, tc := range cases {
		if got := AddMonthsClamped(tc.in, tc.n); !got.Equal(tc.want) {
			t.Errorf("%v+%d = %v want %v", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestDefaultInstallments(t *testing.T) {
	due := day(2026, 7, 10)
	cases := []struct {
		name  string
		net   int64
		comps []PlannedComponent
		want  []Installment
	}{
		{"all at once", 500, []PlannedComponent{{AmountIDR: 500, InstallmentTerms: 1}}, nil},
		{"three monthly terms", 9000000, []PlannedComponent{{AmountIDR: 9000000, InstallmentTerms: 3, IntervalMonths: 1}},
			[]Installment{{1, day(2026, 7, 10), 3000000}, {2, day(2026, 8, 10), 3000000}, {3, day(2026, 9, 10), 3000000}}},
		{"mixed components merge by date", 1300, []PlannedComponent{
			{AmountIDR: 300, InstallmentTerms: 1}, {AmountIDR: 1000, InstallmentTerms: 2, IntervalMonths: 3}},
			[]Installment{{1, day(2026, 7, 10), 800}, {2, day(2026, 10, 10), 500}}},
		{"scaled to discounted net", 650, []PlannedComponent{
			{AmountIDR: 300, InstallmentTerms: 1}, {AmountIDR: 1000, InstallmentTerms: 2, IntervalMonths: 3}},
			[]Installment{{1, day(2026, 7, 10), 400}, {2, day(2026, 10, 10), 250}}},
		{"zero net", 0, []PlannedComponent{{AmountIDR: 900, InstallmentTerms: 3}}, nil},
	}
	for _, tc := range cases {
		got := DefaultInstallments(tc.net, due, tc.comps)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %+v", tc.name, got)
		}
		var sum int64
		for i := range got {
			sum += got[i].AmountIDR
			if got[i].Seq != tc.want[i].Seq || !got[i].DueAt.Equal(tc.want[i].DueAt) || got[i].AmountIDR != tc.want[i].AmountIDR {
				t.Fatalf("%s: %+v", tc.name, got)
			}
		}
		if len(got) > 0 && sum != tc.net {
			t.Fatalf("%s: sum %d", tc.name, sum)
		}
	}
}

func TestValidateInstallments(t *testing.T) {
	a, b, c := day(2026, 7, 10), day(2026, 8, 10), day(2026, 9, 10)
	cases := []struct {
		name string
		net  int64
		in   []Installment
		ok   bool
	}{
		{"valid", 900, []Installment{{0, a, 300}, {0, b, 300}, {0, c, 300}}, true},
		{"single term", 900, []Installment{{0, a, 900}}, false},
		{"sum mismatch", 900, []Installment{{0, a, 300}, {0, b, 500}}, false},
		{"zero amount", 300, []Installment{{0, a, 300}, {0, b, 0}}, false},
		{"dates not increasing", 600, []Installment{{0, b, 300}, {0, a, 300}}, false},
		{"same date", 600, []Installment{{0, a, 300}, {0, a, 300}}, false},
		{"missing date", 600, []Installment{{0, a, 300}, {0, time.Time{}, 300}}, false},
	}
	for _, tc := range cases {
		out, err := ValidateInstallments(tc.net, tc.in)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v", tc.name, err)
		}
		if tc.ok && (out[0].Seq != 1 || out[len(out)-1].Seq != len(out)) {
			t.Errorf("%s: seq %+v", tc.name, out)
		}
	}
}

func TestInstallmentProgressAndNextDue(t *testing.T) {
	ins := []Installment{{1, day(2026, 7, 10), 300}, {2, day(2026, 8, 10), 300}, {3, day(2026, 9, 10), 300}}
	cases := []struct {
		name     string
		paid     int64
		statuses []string
		next     *time.Time
	}{
		{"nothing paid", 0, []string{"unpaid", "unpaid", "unpaid"}, &ins[0].DueAt},
		{"first and part of second", 450, []string{"paid", "partial", "unpaid"}, &ins[1].DueAt},
		{"exactly two", 600, []string{"paid", "paid", "unpaid"}, &ins[2].DueAt},
		{"all", 900, []string{"paid", "paid", "paid"}, nil},
		{"negative paid", -5, []string{"unpaid", "unpaid", "unpaid"}, &ins[0].DueAt},
	}
	for _, tc := range cases {
		st := InstallmentProgress(ins, tc.paid)
		for i := range st {
			if st[i].Status != tc.statuses[i] {
				t.Fatalf("%s: %+v", tc.name, st)
			}
		}
		n := NextDue(ins, tc.paid)
		if (n == nil) != (tc.next == nil) || (n != nil && !n.Equal(*tc.next)) {
			t.Fatalf("%s: next %v", tc.name, n)
		}
	}
}

func TestRescaleInstallments(t *testing.T) {
	ins := []Installment{{1, day(2026, 7, 10), 600}, {2, day(2026, 8, 10), 300}}
	cases := []struct {
		name string
		net  int64
		want []int64
	}{
		{"halved", 450, []int64{300, 150}},
		{"grown", 1800, []int64{1200, 600}},
		{"too small for two terms", 1, nil},
		{"zero", 0, nil},
	}
	for _, tc := range cases {
		got := RescaleInstallments(ins, tc.net)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %+v", tc.name, got)
		}
		for i := range got {
			if got[i].AmountIDR != tc.want[i] {
				t.Fatalf("%s: %+v", tc.name, got)
			}
		}
	}
}

func TestAnnualPeriods(t *testing.T) {
	jul, jun := day(2026, 7, 1), day(2027, 6, 30)
	cases := []struct {
		name, from, to string
		effFrom, effTo *time.Time
		n              int
		first, last    [2]int
		err            bool
	}{
		{"scheme window", "", "", &jul, &jun, 12, [2]int{2026, 7}, [2]int{2027, 6}, false},
		{"explicit", "2026-11", "2027-02", nil, nil, 4, [2]int{2026, 11}, [2]int{2027, 2}, false},
		{"from explicit to window", "2027-01", "", &jul, &jun, 6, [2]int{2027, 1}, [2]int{2027, 6}, false},
		{"no bound", "", "", nil, nil, 0, [2]int{}, [2]int{}, true},
		{"reversed", "2027-02", "2026-11", nil, nil, 0, [2]int{}, [2]int{}, true},
		{"too long", "2026-01", "2027-01", nil, nil, 0, [2]int{}, [2]int{}, true},
		{"bad format", "07-2026", "2027-01", nil, nil, 0, [2]int{}, [2]int{}, true},
	}
	for _, tc := range cases {
		got, err := AnnualPeriods(tc.from, tc.to, tc.effFrom, tc.effTo)
		if (err != nil) != tc.err {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if err == nil && (len(got) != tc.n || got[0] != tc.first || got[len(got)-1] != tc.last) {
			t.Fatalf("%s: %v", tc.name, got)
		}
	}
}
