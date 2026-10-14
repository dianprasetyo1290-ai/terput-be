package model

import (
	"testing"
	"time"
)

func TestMenu78RoleMatrix(t *testing.T) {
	cases := []struct {
		role                string
		view, create, apply bool
	}{
		{"admin", true, true, true},
		{"walikelas", true, true, false},
		{"kepala_sekolah", true, false, false},
		{"superadmin", false, false, false}, // BUG-177
		{"guru", false, false, false},
		{"parent", false, false, false},
		{"", false, false, false},
	}
	for _, c := range cases {
		if got := CanViewStatusRequests(c.role); got != c.view {
			t.Errorf("view %q=%v want %v", c.role, got, c.view)
		}
		if got := CanCreateStatusRequest(c.role); got != c.create {
			t.Errorf("create %q=%v want %v", c.role, got, c.create)
		}
		if got := CanApplyStatusRequest(c.role); got != c.apply {
			t.Errorf("apply %q=%v want %v", c.role, got, c.apply)
		}
	}
}

func TestIsActiveStatusRequest(t *testing.T) {
	for status, want := range map[string]bool{
		StatusPendingApproval: true, StatusApproved: true,
		StatusRejected: false, StatusCancelled: false, StatusApplied: false,
	} {
		if got := IsActiveStatusRequest(status); got != want {
			t.Errorf("%s=%v want %v", status, got, want)
		}
	}
}

func TestValidateEffectiveDateAfterEnrollment(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	// 2026-07-15 01:00 WIB == 2026-07-14 18:00 UTC: the enrollment day is the
	// 15th in Jakarta even though the UTC date is the 14th.
	enrolled := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		eff      string
		enrolled time.Time
		wantErr  bool
	}{
		{"same day", "2026-07-15", enrolled, false},
		{"after", "2026-08-01", enrolled, false},
		{"day before (UTC date)", "2026-07-14", enrolled, true},
		{"long before", "2025-01-01", enrolled, true},
		{"unknown enrollment date", "2025-01-01", time.Time{}, false},
		{"bad format", "15-07-2026", enrolled, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateEffectiveDateAfterEnrollment(c.eff, c.enrolled, wib)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestCheckCancelStatusRequest(t *testing.T) {
	cases := []struct {
		name, status, by, actor string
		want                    error
	}{
		{"requester pending", StatusPendingApproval, "u1", "u1", nil},
		{"other user", StatusPendingApproval, "u1", "u2", ErrCancelNotRequester},
		{"approved", StatusApproved, "u1", "u1", ErrCancelNotPending},
		{"rejected", StatusRejected, "u1", "u1", ErrCancelNotPending},
		{"already cancelled", StatusCancelled, "u1", "u1", ErrCancelNotPending},
		{"applied", StatusApplied, "u1", "u1", ErrCancelNotPending},
		{"empty requester", StatusPendingApproval, "", "", ErrCancelNotRequester},
	}
	for _, c := range cases {
		if got := CheckCancelStatusRequest(c.status, c.by, c.actor); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
