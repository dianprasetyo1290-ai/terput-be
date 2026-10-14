package model

import (
	"strings"
	"testing"
	"time"
)

func TestRefundDecision(t *testing.T) {
	cases := []struct {
		name, current, decision, by string
		next                        string
		changed, err                bool
	}{
		{"approve", RefundRequested, "approved", "checker", RefundApproved, true, false},
		{"reject", RefundRequested, "rejected", "checker", RefundRejected, true, false},
		{"replay after approve", RefundApproved, "rejected", "checker", RefundApproved, false, false},
		{"replay after process", RefundProcessed, "approved", "checker", RefundProcessed, false, false},
		{"unknown decision", RefundRequested, "maybe", "checker", RefundRequested, false, true},
		{"no approver", RefundRequested, "approved", " ", RefundRequested, false, true},
		{"maker approves own", RefundRequested, "approved", "maker", RefundRequested, false, true},
	}
	for _, tc := range cases {
		next, changed, err := RefundDecision(tc.current, tc.decision, tc.by, "maker")
		if next != tc.next || changed != tc.changed || (err != nil) != tc.err {
			t.Errorf("%s: got %s %v %v", tc.name, next, changed, err)
		}
	}
}

func TestParseTransferProof(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC) // 14 Oct in WIB
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'x'
	}
	cases := []struct {
		name, ref, date string
		want            string
		err             bool
	}{
		{"today default", " TRF-001 ", "", "2026-10-14", false},
		{"past date", "TRF-002", "2026-10-01", "2026-10-01", false},
		{"today explicit (school tz)", "TRF-003", "2026-10-14", "2026-10-14", false},
		{"future", "TRF-004", "2026-10-15", "", true},
		{"missing ref", "  ", "", "", true},
		{"too long", string(long), "", "", true},
		{"bad date", "TRF-005", "14/10/2026", "", true},
	}
	for _, tc := range cases {
		p, err := ParseTransferProof(tc.ref, tc.date, now, jkt)
		if (err != nil) != tc.err {
			t.Errorf("%s: err=%v", tc.name, err)
			continue
		}
		if err == nil && (p.Date.Format("2006-01-02") != tc.want || p.Ref != strings.TrimSpace(tc.ref)) {
			t.Errorf("%s: got %+v", tc.name, p)
		}
	}
}

func TestRefundNoticeText(t *testing.T) {
	cases := []struct {
		status, note, ref string
		title, contains   string
	}{
		{RefundApproved, "", "", "Refund disetujui", "Rp 50.000"},
		{RefundRejected, "dobel", "", "Refund ditolak", "Alasan: dobel."},
		{RefundRejected, "", "", "Refund ditolak", "Saldo pembayaran tidak berubah."},
		{RefundProcessed, "", "TRF-9", "Refund telah ditransfer", "referensi TRF-9"},
		{RefundRequested, "", "", "", ""},
	}
	for _, tc := range cases {
		title, body := RefundNoticeText(tc.status, "Rp 50.000", tc.note, tc.ref)
		if title != tc.title || !strings.Contains(body, tc.contains) {
			t.Errorf("%s: %q %q", tc.status, title, body)
		}
	}
}
