package model

import (
	"testing"
	"time"
)

func TestLockSettingsIsLocked(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, wib)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct {
		name string
		cfg  LockSettings
		date string
		now  time.Time
		want bool
	}{
		{"disabled never locks", LockSettings{Enabled: false, CutoffDays: 0, CutoffTime: "00:00"}, "2026-10-06", at("2027-01-01 00:00"), false},
		{"same day before cutoff", LockSettings{Enabled: true, CutoffDays: 0, CutoffTime: "17:00"}, "2026-10-06", at("2026-10-06 16:59"), false},
		{"same day at cutoff", LockSettings{Enabled: true, CutoffDays: 0, CutoffTime: "17:00"}, "2026-10-06", at("2026-10-06 17:00"), true},
		{"next day cutoff not yet", LockSettings{Enabled: true, CutoffDays: 1, CutoffTime: "08:00"}, "2026-10-06", at("2026-10-07 07:59"), false},
		{"next day cutoff passed", LockSettings{Enabled: true, CutoffDays: 1, CutoffTime: "08:00"}, "2026-10-06", at("2026-10-07 08:00"), true},
		{"month rollover", LockSettings{Enabled: true, CutoffDays: 3, CutoffTime: "23:59"}, "2026-10-30", at("2026-11-02 23:59"), true},
		{"utc clock compared in WIB", LockSettings{Enabled: true, CutoffDays: 0, CutoffTime: "17:00"}, "2026-10-06", time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC), true},
		{"bad date never locks", LockSettings{Enabled: true, CutoffDays: 0, CutoffTime: "17:00"}, "06-10-2026", at("2027-01-01 00:00"), false},
	}
	for _, c := range cases {
		if got := c.cfg.IsLocked(c.date, c.now, wib); got != c.want {
			t.Errorf("%s: IsLocked = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLockSettingsValidate(t *testing.T) {
	if err := (LockSettings{CutoffDays: 1, CutoffTime: "07:30"}).Validate(); err != nil {
		t.Errorf("valid settings rejected: %v", err)
	}
	for _, bad := range []LockSettings{{CutoffDays: -1, CutoffTime: "07:30"}, {CutoffDays: 32, CutoffTime: "07:30"}, {CutoffDays: 1, CutoffTime: "7.30"}} {
		if bad.Validate() == nil {
			t.Errorf("invalid settings accepted: %+v", bad)
		}
	}
}

func TestTerlambatIsValidStatus(t *testing.T) {
	if !AttendanceStatus("terlambat").Valid() {
		t.Fatal("terlambat must be a valid status (FR-41-001)")
	}
}

func TestLockAcceptsScannedDate(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	cfg := LockSettings{Enabled: true, CutoffDays: 0, CutoffTime: "17:00"}
	if !cfg.IsLocked("2026-10-06T00:00:00Z", time.Date(2026, 10, 7, 0, 0, 0, 0, wib), wib) {
		t.Fatal("scanned DATE value must be understood")
	}
}
