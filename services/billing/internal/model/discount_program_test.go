package model

import (
	"testing"
	"time"
)

func TestValidateDiscountProgram(t *testing.T) {
	d := func(m time.Month, day int) *time.Time { v := time.Date(2026, m, day, 0, 0, 0, 0, time.UTC); return &v }
	ok := DiscountProgramInput{Name: "Beasiswa Prestasi", Kind: DiscountPercent, Value: 5000, Scope: "student", StudentUserID: "s", Reason: "SK 12"}
	cases := []struct {
		name string
		mut  func(*DiscountProgramInput)
		ok   bool
	}{
		{"valid 50%", func(*DiscountProgramInput) {}, true},
		{"exactly 100%", func(in *DiscountProgramInput) { in.Value = 10000 }, true},
		{"over 100%", func(in *DiscountProgramInput) { in.Value = 10001 }, false},
		{"zero value", func(in *DiscountProgramInput) { in.Value = 0 }, false},
		{"nominal above 100% scale allowed", func(in *DiscountProgramInput) { in.Kind = DiscountNominal; in.Value = 750000 }, true},
		{"negative cap", func(in *DiscountProgramInput) { in.CapIDR = -1 }, false},
		{"unknown kind", func(in *DiscountProgramInput) { in.Kind = "x" }, false},
		{"no name", func(in *DiscountProgramInput) { in.Name = " " }, false},
		{"no reason", func(in *DiscountProgramInput) { in.Reason = "" }, false},
		{"student scope without student", func(in *DiscountProgramInput) { in.StudentUserID = "" }, false},
		{"grade scope", func(in *DiscountProgramInput) { in.Scope = "grade"; in.GradeLevel = "7" }, true},
		{"scheme scope missing", func(in *DiscountProgramInput) { in.Scope = "scheme" }, false},
		{"bad scope", func(in *DiscountProgramInput) { in.Scope = "class" }, false},
		{"period reversed", func(in *DiscountProgramInput) { in.ValidFrom, in.ValidTo = d(9, 1), d(8, 1) }, false},
		{"period ok", func(in *DiscountProgramInput) { in.ValidFrom, in.ValidTo = d(7, 1), d(12, 31) }, true},
	}
	for _, tc := range cases {
		in := ok
		tc.mut(&in)
		if err := ValidateDiscountProgram(in); (err == nil) != tc.ok {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestDiscountActiveInPeriod(t *testing.T) {
	d := func(m time.Month, day int) *time.Time { v := time.Date(2026, m, day, 0, 0, 0, 0, time.UTC); return &v }
	cases := []struct {
		name     string
		from, to *time.Time
		month    int
		want     bool
	}{
		{"open", nil, nil, 7, true},
		{"starts mid month counts", d(7, 15), nil, 7, true},
		{"starts next month", d(8, 1), nil, 7, false},
		{"ended last month", nil, d(6, 30), 7, false},
		{"ends first day", nil, d(7, 1), 7, true},
		{"inside window", d(7, 1), d(12, 31), 9, true},
		{"after window", d(7, 1), d(12, 31), 1, false},
	}
	for _, tc := range cases {
		year := 2026
		if tc.month == 1 {
			year = 2027
		}
		if got := DiscountActiveInPeriod(tc.from, tc.to, year, tc.month); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}
