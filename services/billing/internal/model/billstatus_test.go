package model

import (
	"testing"
	"time"
)

func TestEffectiveBillStatus(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		net    int64
		paid   int64
		want   BillStatus
	}{
		{"stale paid with nothing paid", "paid", 750000, 0, BillUnpaid},
		{"stale paid partly paid", "paid", 750000, 250000, BillPartial},
		{"stale unpaid fully paid", "unpaid", 750000, 750000, BillPaid},
		{"overpaid", "partial", 750000, 800000, BillPaid},
		{"fully reversed", "paid", 750000, 0, BillUnpaid},
		{"void stays void", "void", 750000, 0, BillVoid},
		{"void with payments stays void", "void", 750000, 750000, BillVoid},
	}
	for _, tc := range cases {
		if got := EffectiveBillStatus(tc.stored, tc.net, tc.paid); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}

func TestBillOverdue(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	// Stored as midnight WIB on 10 Oct (17:00 UTC 9 Oct).
	due := time.Date(2026, 10, 10, 0, 0, 0, 0, jkt).UTC()
	cases := []struct {
		name        string
		status      BillStatus
		due         *time.Time
		outstanding int64
		now         time.Time
		want        bool
	}{
		{"on due date", BillUnpaid, &due, 100, time.Date(2026, 10, 10, 23, 30, 0, 0, jkt), false},
		{"day after", BillUnpaid, &due, 100, time.Date(2026, 10, 11, 0, 5, 0, 0, jkt), true},
		{"partial overdue", BillPartial, &due, 100, time.Date(2026, 11, 1, 9, 0, 0, 0, jkt), true},
		{"paid never overdue", BillPaid, &due, 0, time.Date(2026, 11, 1, 9, 0, 0, 0, jkt), false},
		{"no balance", BillUnpaid, &due, 0, time.Date(2026, 11, 1, 9, 0, 0, 0, jkt), false},
		{"void", BillVoid, &due, 100, time.Date(2026, 11, 1, 9, 0, 0, 0, jkt), false},
		{"no due date", BillUnpaid, nil, 100, time.Date(2026, 11, 1, 9, 0, 0, 0, jkt), false},
		{"utc clock still 10 Oct WIB", BillUnpaid, &due, 100, time.Date(2026, 10, 10, 16, 0, 0, 0, time.UTC), false},
		{"utc clock already 11 Oct WIB", BillUnpaid, &due, 100, time.Date(2026, 10, 10, 17, 30, 0, 0, time.UTC), true},
	}
	for _, tc := range cases {
		if got := BillOverdue(tc.status, tc.due, tc.outstanding, tc.now, jkt); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}
