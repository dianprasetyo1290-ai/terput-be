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

func TestSplitStudentRefund(t *testing.T) {
	bills := []RefundableBill{{"b-new", 300000}, {"b-zero", 0}, {"b-old", 200000}}
	cases := []struct {
		name            string
		amount, deposit int64
		bills           []RefundableBill
		wantDeposit     int64
		wantShares      []BillRefundShare
		err             bool
	}{
		{"deposit only", 150000, 400000, bills, 150000, nil, false},
		{"deposit exactly", 400000, 400000, bills, 400000, nil, false},
		{"deposit then overpaid bill", 500000, 400000, bills, 400000, []BillRefundShare{{"b-new", 100000}}, false},
		{"spans bills skipping empty", 800000, 400000, bills, 400000,
			[]BillRefundShare{{"b-new", 300000}, {"b-old", 100000}}, false},
		{"no deposit", 250000, 0, bills, 0, []BillRefundShare{{"b-new", 250000}}, false},
		{"negative deposit treated as none", 100000, -5, bills, 0, []BillRefundShare{{"b-new", 100000}}, false},
		{"everything", 900000, 400000, bills, 400000,
			[]BillRefundShare{{"b-new", 300000}, {"b-old", 200000}}, false},
		{"exceeds deposit and bills", 900001, 400000, bills, 0, nil, true},
		{"exceeds deposit without bills", 1, 0, nil, 0, nil, true},
		{"zero amount", 0, 400000, bills, 0, nil, true},
	}
	for _, tc := range cases {
		dep, shares, err := SplitStudentRefund(tc.amount, tc.deposit, tc.bills)
		if (err != nil) != tc.err {
			t.Errorf("%s: err %v", tc.name, err)
			continue
		}
		if dep != tc.wantDeposit || len(shares) != len(tc.wantShares) {
			t.Errorf("%s: got deposit %d shares %+v", tc.name, dep, shares)
			continue
		}
		var sum int64
		for i, s := range shares {
			if s != tc.wantShares[i] {
				t.Errorf("%s: share %d = %+v want %+v", tc.name, i, s, tc.wantShares[i])
			}
			sum += s.AmountIDR
		}
		if !tc.err && dep+sum != tc.amount {
			t.Errorf("%s: parts %d+%d do not add up to %d", tc.name, dep, sum, tc.amount)
		}
	}
}
