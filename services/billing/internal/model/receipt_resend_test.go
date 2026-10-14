package model

import (
	"testing"
	"time"
)

func TestReceiptResendWait(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	cases := []struct {
		name     string
		last     *time.Time
		cooldown time.Duration
		want     time.Duration
	}{
		{"never sent", nil, 10 * time.Minute, 0},
		{"just sent", ago(time.Minute), 10 * time.Minute, 9 * time.Minute},
		{"exactly at cooldown", ago(10 * time.Minute), 10 * time.Minute, 0},
		{"long ago", ago(time.Hour), 10 * time.Minute, 0},
		{"cooldown disabled", ago(time.Second), 0, 0},
	}
	for _, tc := range cases {
		if got := ReceiptResendWait(tc.last, tc.cooldown, now); got != tc.want {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}
