package model

import (
	"errors"
	"testing"
)

func TestScreenRecipients(t *testing.T) {
	enr := map[string]StudentEnrollment{
		"budi":  {Enrolled: true, GradeLevel: "10"},
		"ani":   {Enrolled: true, GradeLevel: "7"},
		"nogr":  {Enrolled: true, GradeLevel: ""},
		"admin": {Enrolled: false},
	}
	cases := []struct {
		name     string
		students []string
		grade    string
		ok       int
		rejected int
	}{
		{"no grade on scheme", []string{"budi", "ani", "admin"}, "", 2, 1},
		{"grade filter", []string{"budi", "ani"}, "10", 1, 1},
		{"case and space tolerant", []string{"budi"}, " 10 ", 1, 0},
		{"unplaced class grade", []string{"nogr"}, "10", 0, 1},
		{"unknown user", []string{"ghost"}, "", 0, 1},
		{"empty", nil, "10", 0, 0},
	}
	for _, tc := range cases {
		ok, rej := ScreenRecipients(tc.students, enr, tc.grade)
		if len(ok) != tc.ok || len(rej) != tc.rejected {
			t.Errorf("%s: ok=%v rejected=%v", tc.name, ok, rej)
		}
		for _, r := range rej {
			if r.Reason == "" || r.StudentUserID == "" {
				t.Errorf("%s: incomplete rejection %+v", tc.name, r)
			}
		}
	}
}

func TestDepositToApply(t *testing.T) {
	cases := []struct{ bal, net, want int64 }{
		{200000, 750000, 200000},
		{900000, 750000, 750000},
		{0, 750000, 0},
		{200000, 0, 0},
		{-5, 100, 0},
	}
	for _, tc := range cases {
		if got := DepositToApply(tc.bal, tc.net); got != tc.want {
			t.Errorf("DepositToApply(%d,%d)=%d", tc.bal, tc.net, got)
		}
	}
}

func TestSchemeIssueMapping(t *testing.T) {
	items := []BillItem{{Name: "SPP", ComponentCode: "SPP", AmountIDR: 500000}, {Name: "Seragam", ComponentCode: "SRG", AmountIDR: 250000}}
	full := map[string]ComponentAccount{"SPP": {Code: "SPP", RevenueAcc: "4-100"}, "SRG": {Code: "SRG", RevenueAcc: "4-200", ReceivableAcc: "1-310"}}
	cases := []struct {
		name  string
		set   JournalSettings
		comps map[string]ComponentAccount
		items []BillItem
		err   bool
	}{
		{"all mapped", JournalSettings{ReceivableAcc: "1-300"}, full, items, false},
		{"no mapping at all", JournalSettings{}, map[string]ComponentAccount{}, items, true},
		{"one component unmapped", JournalSettings{ReceivableAcc: "1-300"}, map[string]ComponentAccount{"SPP": full["SPP"]}, items, true},
		{"no receivable control", JournalSettings{}, full, items, true},
		{"component receivable is enough", JournalSettings{}, full, items[1:], false},
		{"component without code", JournalSettings{ReceivableAcc: "1-300"}, full, []BillItem{{Name: "Lain", AmountIDR: 1}}, true},
	}
	for _, tc := range cases {
		err := SchemeIssueMapping(tc.set, tc.comps, tc.items)
		if (err != nil) != tc.err {
			t.Errorf("%s: err=%v", tc.name, err)
		}
		var me *MissingAccountsError
		if err != nil && !errors.As(err, &me) {
			t.Errorf("%s: want MissingAccountsError, got %T", tc.name, err)
		}
	}
}
