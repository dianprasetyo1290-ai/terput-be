package model

import (
	"testing"
	"time"
)

func TestReminderFor(t *testing.T) {
	jkt := time.FixedZone("WIB", 7*3600)
	p := ReminderPolicy{Enabled: true, DaysBefore: []int{3, 7}, OnDue: true, DaysAfter: []int{3}, SendHour: 8, Channels: []string{"in_app"}}
	due := time.Date(2026, 9, 10, 0, 0, 0, 0, jkt)
	cases := []struct {
		name   string
		today  time.Time
		policy ReminderPolicy
		kind   ReminderKind
		offset int
		ok     bool
	}{
		{"H-7", time.Date(2026, 9, 3, 9, 0, 0, 0, jkt), p, ReminderBeforeDue, -7, true},
		{"H-3", time.Date(2026, 9, 7, 9, 0, 0, 0, jkt), p, ReminderBeforeDue, -3, true},
		{"H-2 not scheduled", time.Date(2026, 9, 8, 9, 0, 0, 0, jkt), p, "", 0, false},
		{"H", time.Date(2026, 9, 10, 23, 59, 0, 0, jkt), p, ReminderOnDue, 0, true},
		{"H+3", time.Date(2026, 9, 13, 1, 0, 0, 0, jkt), p, ReminderOverdue, 3, true},
		{"H+4", time.Date(2026, 9, 14, 1, 0, 0, 0, jkt), p, "", 0, false},
		// 2026-09-09 18:00 UTC is already 2026-09-10 in WIB.
		{"time zone", time.Date(2026, 9, 9, 18, 0, 0, 0, time.UTC), p, ReminderOnDue, 0, true},
		{"disabled", time.Date(2026, 9, 10, 9, 0, 0, 0, jkt), ReminderPolicy{OnDue: true}, "", 0, false},
	}
	for _, tc := range cases {
		k, off, ok := ReminderFor(tc.policy, tc.today, due, jkt)
		if k != tc.kind || off != tc.offset || ok != tc.ok {
			t.Errorf("%s: got %s %d %v", tc.name, k, off, ok)
		}
	}
}

func TestReminderPolicyNormalize(t *testing.T) {
	cases := []struct {
		name string
		p    ReminderPolicy
		err  bool
	}{
		{"suggested", SuggestedReminderPolicy(), false},
		{"dedupe", ReminderPolicy{Enabled: true, DaysBefore: []int{7, 3, 3}, Channels: []string{"email", "email"}}, false},
		{"zero offset", ReminderPolicy{Enabled: true, DaysBefore: []int{0}, Channels: []string{"in_app"}}, true},
		{"too far", ReminderPolicy{Enabled: true, DaysAfter: []int{91}, Channels: []string{"in_app"}}, true},
		{"bad hour", ReminderPolicy{Enabled: true, OnDue: true, SendHour: 24, Channels: []string{"in_app"}}, true},
		{"no channel", ReminderPolicy{Enabled: true, OnDue: true}, true},
		{"unknown channel", ReminderPolicy{Enabled: true, OnDue: true, Channels: []string{"fax"}}, true},
		{"enabled without schedule", ReminderPolicy{Enabled: true, Channels: []string{"in_app"}}, true},
		{"disabled without schedule", ReminderPolicy{Enabled: false, Channels: []string{"in_app"}}, false},
	}
	for _, tc := range cases {
		p := tc.p
		err := p.Normalize()
		if (err != nil) != tc.err {
			t.Errorf("%s: err=%v", tc.name, err)
		}
	}
	p := ReminderPolicy{Enabled: true, DaysBefore: []int{7, 3, 3}, DaysAfter: []int{14, 1}, Channels: []string{"email", "email"}}
	_ = p.Normalize()
	if b, a := p.Window(); b != 7 || a != 14 || len(p.DaysBefore) != 2 || len(p.Channels) != 1 {
		t.Fatalf("normalize/window: %+v", p)
	}
}
