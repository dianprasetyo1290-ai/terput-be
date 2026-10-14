package model

import (
	"testing"
	"time"
)

func d(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

func TestValidateComponents(t *testing.T) {
	cases := []struct {
		name  string
		comps []SchemeComponent
		err   bool
	}{
		{"ok", []SchemeComponent{{Code: "SPP", Name: "SPP", AmountIDR: 500000}}, false},
		{"duplicate code", []SchemeComponent{{Code: "SPP", Name: "a", AmountIDR: 1}, {Code: "spp ", Name: "b", AmountIDR: 2}}, true},
		{"negative", []SchemeComponent{{Code: "SPP", Name: "a", AmountIDR: -1}}, true},
		{"zero not free", []SchemeComponent{{Code: "SPP", Name: "a"}}, true},
		{"zero free", []SchemeComponent{{Code: "SPP", Name: "a", IsFree: true}}, false},
		{"free with amount", []SchemeComponent{{Code: "SPP", Name: "a", AmountIDR: 5, IsFree: true}}, true},
		{"missing name", []SchemeComponent{{Code: "SPP", AmountIDR: 5}}, true},
		{"bad kind", []SchemeComponent{{Code: "SPP", Name: "a", AmountIDR: 5, Kind: "weekly"}}, true},
		{"annual", []SchemeComponent{{Code: "KEG", Name: "a", AmountIDR: 5, Kind: KindAnnual}}, false},
		{"three terms", []SchemeComponent{{Code: "PGK", Name: "a", AmountIDR: 9, InstallmentTerms: 3, IntervalMonths: 1}}, false},
		{"too many terms", []SchemeComponent{{Code: "PGK", Name: "a", AmountIDR: 9, InstallmentTerms: 13}}, true},
		{"bad interval", []SchemeComponent{{Code: "PGK", Name: "a", AmountIDR: 9, InstallmentTerms: 2, IntervalMonths: 13}}, true},
		{"free in terms", []SchemeComponent{{Code: "PGK", Name: "a", IsFree: true, InstallmentTerms: 2}}, true},
	}
	for _, tc := range cases {
		if err := ValidateComponents(tc.comps); (err != nil) != tc.err {
			t.Errorf("%s: err=%v", tc.name, err)
		}
	}
}

func TestTariffChanged(t *testing.T) {
	base := []SchemeComponent{{Code: "SPP", Name: "SPP", AmountIDR: 500000}, {Code: "SRG", Name: "Seragam", AmountIDR: 250000, Kind: "one_time"}}
	cases := []struct {
		name  string
		after []SchemeComponent
		want  bool
	}{
		{"same reordered and renamed", []SchemeComponent{{Code: "SRG", Name: "Baju", AmountIDR: 250000}, {Code: "spp", Name: "SPP Bulanan", AmountIDR: 500000}}, false},
		{"amount", []SchemeComponent{{Code: "SPP", AmountIDR: 550000}, {Code: "SRG", AmountIDR: 250000}}, true},
		{"added", append(append([]SchemeComponent{}, base...), SchemeComponent{Code: "KEG", AmountIDR: 1}), true},
		{"removed", base[:1], true},
		{"kind", []SchemeComponent{{Code: "SPP", AmountIDR: 500000, Kind: "monthly"}, {Code: "SRG", AmountIDR: 250000}}, true},
		{"installment plan", []SchemeComponent{{Code: "SPP", AmountIDR: 500000}, {Code: "SRG", AmountIDR: 250000, InstallmentTerms: 2}}, true},
		{"explicit single term equals default", []SchemeComponent{{Code: "SPP", AmountIDR: 500000, InstallmentTerms: 1, IntervalMonths: 3}, {Code: "SRG", AmountIDR: 250000}}, false},
	}
	for _, tc := range cases {
		if got := TariffChanged(base, tc.after); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestSchemeConflict(t *testing.T) {
	b := SchemeWindow{ID: "b", AcademicYearID: "Y1", GradeLevel: "99", Codes: []string{"SPP"}}
	cases := []struct {
		name string
		c    SchemeWindow
		want int
	}{
		{"same dimension same code", SchemeWindow{ID: "c", AcademicYearID: "Y1", GradeLevel: "99", Codes: []string{"spp"}}, 1},
		{"same scheme", SchemeWindow{ID: "b", AcademicYearID: "Y1", GradeLevel: "99", Codes: []string{"SPP"}}, 0},
		{"other grade", SchemeWindow{ID: "c", AcademicYearID: "Y1", GradeLevel: "10", Codes: []string{"SPP"}}, 0},
		{"other year", SchemeWindow{ID: "c", AcademicYearID: "Y2", GradeLevel: "99", Codes: []string{"SPP"}}, 0},
		{"different component", SchemeWindow{ID: "c", AcademicYearID: "Y1", GradeLevel: "99", Codes: []string{"SRG"}}, 0},
		{"classroom specific", SchemeWindow{ID: "c", AcademicYearID: "Y1", GradeLevel: "99", ClassroomID: "K1", Codes: []string{"SPP"}}, 0},
		{"other program", SchemeWindow{ID: "c", AcademicYearID: "Y1", GradeLevel: "99", Program: "IPA", Codes: []string{"SPP"}}, 0},
		{"other cohort", SchemeWindow{ID: "c", AcademicYearID: "Y1", GradeLevel: "99", Cohort: "2025", Codes: []string{"SPP"}}, 0},
		{"other track", SchemeWindow{ID: "c", AcademicYearID: "Y1", GradeLevel: "99", Track: "prestasi", Codes: []string{"SPP"}}, 0},
	}
	for _, tc := range cases {
		if got := SchemeConflict(tc.c, b); len(got) != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
	// Same program/cohort/track, case-insensitive, is a double tariff.
	bp := SchemeWindow{ID: "b", AcademicYearID: "Y1", Program: "IPA", Cohort: "2026", Track: "Reguler", Codes: []string{"SPP"}}
	if SchemeConflict(SchemeWindow{ID: "c", AcademicYearID: "Y1", Program: "ipa", Cohort: "2026", Track: "reguler", Codes: []string{"SPP"}}, bp) == nil {
		t.Error("same program/cohort/track must conflict")
	}
	// Consecutive windows (new tariff from 1 Jan) do not conflict.
	old := SchemeWindow{ID: "o", AcademicYearID: "Y1", To: d("2026-12-31"), Codes: []string{"SPP"}}
	nw := SchemeWindow{ID: "n", AcademicYearID: "Y1", From: d("2027-01-01"), Codes: []string{"SPP"}}
	if SchemeConflict(old, nw) != nil {
		t.Error("consecutive windows must not conflict")
	}
	nw.From = d("2026-12-31")
	if SchemeConflict(old, nw) == nil {
		t.Error("one shared day is a conflict")
	}
}

func TestPeriodInWindow(t *testing.T) {
	cases := []struct {
		y, m     int
		from, to *time.Time
		want     bool
	}{
		{2026, 7, nil, nil, true},
		{2026, 7, d("2026-07-15"), nil, true},
		{2026, 6, d("2026-07-01"), nil, false},
		{2026, 12, nil, d("2026-12-31"), true},
		{2027, 1, nil, d("2026-12-31"), false},
		{2027, 1, d("2027-01-01"), d("2027-06-30"), true},
	}
	for _, tc := range cases {
		if got := PeriodInWindow(tc.y, tc.m, tc.from, tc.to); got != tc.want {
			t.Errorf("%d-%d: got %v", tc.y, tc.m, got)
		}
	}
}
