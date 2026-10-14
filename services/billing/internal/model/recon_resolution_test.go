package model

import "testing"

func TestValidateReconResolution(t *testing.T) {
	bal := []JournalLine{{AccCode: "1-1200", DebitIDR: 2500}, {AccCode: "6-1100", CreditIDR: 2500}}
	cases := []struct {
		name  string
		note  string
		lines []JournalLine
		total int64
		err   bool
	}{
		{"note only", "Selisih waktu settlement, cair H+1", nil, 0, false},
		{"balanced adjustment", "Biaya admin bank", bal, 2500, false},
		{"missing note", " ", bal, 0, true},
		{"unbalanced", "x", []JournalLine{{AccCode: "1", DebitIDR: 100}, {AccCode: "2", CreditIDR: 90}}, 0, true},
		{"single line", "x", []JournalLine{{AccCode: "1", DebitIDR: 100}}, 0, true},
		{"two-sided line", "x", []JournalLine{{AccCode: "1", DebitIDR: 100, CreditIDR: 100}, {AccCode: "2", CreditIDR: 0}}, 0, true},
	}
	for _, tc := range cases {
		total, err := ValidateReconResolution(tc.note, tc.lines)
		if total != tc.total || (err != nil) != tc.err {
			t.Errorf("%s: total=%d err=%v", tc.name, total, err)
		}
	}
}

func TestReconDecision(t *testing.T) {
	cases := []struct {
		name, current, decision, by, next string
		changed, err                      bool
	}{
		{"approve", ReconPendingApproval, "approved", "checker", ReconResolved, true, false},
		{"reject reopens", ReconPendingApproval, "rejected", "checker", ReconOpen, true, false},
		{"replay", ReconResolved, "rejected", "checker", ReconResolved, false, false},
		{"self approval", ReconPendingApproval, "approved", "maker", ReconPendingApproval, false, true},
		{"no approver", ReconPendingApproval, "approved", "", ReconPendingApproval, false, true},
		{"bad decision", ReconPendingApproval, "ok", "checker", ReconPendingApproval, false, true},
	}
	for _, tc := range cases {
		next, changed, err := ReconDecision(tc.current, tc.decision, tc.by, "maker")
		if next != tc.next || changed != tc.changed || (err != nil) != tc.err {
			t.Errorf("%s: %s %v %v", tc.name, next, changed, err)
		}
	}
}
