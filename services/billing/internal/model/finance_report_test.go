package model

import (
	"testing"
	"time"
)

func TestParseReportRange(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	cases := []struct {
		name, from, to string
		ok, err        bool
	}{
		{"none", "", "", false, false},
		{"only from", "2026-01-01", "", false, true},
		{"bad format", "01-01-2026", "2026-02-01", false, true},
		{"reversed", "2026-12-01", "2026-01-01", false, true},
		{"single day", "2026-03-05", "2026-03-05", true, false},
		{"one year", "2026-01-01", "2026-12-31", true, false},
		{"too long", "2025-01-01", "2026-01-02", false, true},
	}
	for _, tc := range cases {
		rg, ok, err := ParseReportRange(tc.from, tc.to, jkt)
		if ok != tc.ok || (err != nil) != tc.err {
			t.Errorf("%s: ok=%v err=%v", tc.name, ok, err)
		}
		if ok && !rg.End.Equal(rg.To.AddDate(0, 0, 1)) {
			t.Errorf("%s: end %v", tc.name, rg.End)
		}
	}
}

func TestReportRangeMonths(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	rg, _, _ := ParseReportRange("2025-11-15", "2026-02-01", jkt)
	got := rg.Months()
	want := [][2]int{{2025, 11}, {2025, 12}, {2026, 1}, {2026, 2}}
	if len(got) != len(want) {
		t.Fatalf("months %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("months %v", got)
		}
	}
	if p := PeriodRange(2026, 0, jkt); len(p.Months()) != 12 || !p.End.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, jkt)) {
		t.Fatalf("year range %+v", p)
	}
	if p := PeriodRange(2026, 2, jkt); len(p.Months()) != 1 || p.To.Day() != 28 {
		t.Fatalf("month range %+v", p)
	}
}

func TestAgingBucket(t *testing.T) {
	asOf := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	d := func(days int) *time.Time { x := asOf.AddDate(0, 0, -days); return &x }
	future := asOf.Add(time.Hour)
	cases := []struct {
		name string
		due  *time.Time
		want string
	}{
		{"no due date", nil, AgingCurrent},
		{"not yet due", &future, AgingCurrent},
		{"due now", d(0), AgingCurrent},
		{"1 day", d(1), Aging1to30},
		{"30 days", d(30), Aging1to30},
		{"31 days", d(31), Aging31to60},
		{"60 days", d(60), Aging31to60},
		{"61 days", d(61), Aging61to90},
		{"90 days", d(90), Aging61to90},
		{"91 days", d(91), AgingOver90},
	}
	for _, tc := range cases {
		if got := AgingBucket(tc.due, asOf); got != tc.want {
			t.Errorf("%s: got %s", tc.name, got)
		}
	}
}

func TestTrendPoints(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	asOf := time.Date(2026, 3, 15, 10, 0, 0, 0, jkt)
	pts := TrendPoints(asOf, 3, jkt)
	if len(pts) != 3 || pts[0].Month != 1 || pts[2].Month != 3 {
		t.Fatalf("points %+v", pts)
	}
	if !pts[0].At.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, jkt)) || !pts[2].At.Equal(asOf) {
		t.Fatalf("cut-offs %+v", pts)
	}
	if p := TrendPoints(time.Date(2026, 1, 5, 0, 0, 0, 0, jkt), 2, jkt); p[0].Year != 2025 || p[0].Month != 12 {
		t.Fatalf("year wrap %+v", p)
	}
}

func TestSplitByWeights(t *testing.T) {
	cases := []struct {
		name    string
		amount  int64
		weights []int64
		want    []int64
	}{
		{"proportional", 1000, []int64{750, 250}, []int64{750, 250}},
		{"remainder to largest fraction", 100, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{"partial payment", 500, []int64{600000, 300000}, []int64{333, 167}},
		{"zero weights split evenly", 10, []int64{0, 0}, []int64{5, 5}},
		{"negative ignored", 10, []int64{-5, 10}, []int64{0, 10}},
		{"empty", 10, nil, nil},
	}
	for _, tc := range cases {
		got := SplitByWeights(tc.amount, tc.weights)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: %v", tc.name, got)
		}
		var sum int64
		for i := range got {
			sum += got[i]
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
				break
			}
		}
		if len(got) > 0 && sum != tc.amount {
			t.Errorf("%s: sum %d", tc.name, sum)
		}
	}
}
