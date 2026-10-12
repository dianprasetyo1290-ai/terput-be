package consent

import (
	"errors"
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		from     string
		decision string
		used     bool
		want     string
		err      error
	}{
		{"pending → granted", StatusPending, StatusGranted, false, StatusGranted, nil},
		{"pending → declined", StatusPending, StatusDeclined, false, StatusDeclined, nil},
		{"wali kedua setelah granted → final", StatusGranted, StatusGranted, false, StatusGranted, ErrFinal},
		{"wali kedua setelah granted coba decline → final", StatusGranted, StatusDeclined, false, StatusGranted, ErrFinal},
		{"granted → revoked belum dipakai", StatusGranted, StatusRevoked, false, StatusRevoked, nil},
		{"granted → revoked sudah dipakai", StatusGranted, StatusRevoked, true, StatusGranted, ErrInUse},
		{"declined final", StatusDeclined, StatusGranted, false, StatusDeclined, ErrFinal},
		{"revoked final", StatusRevoked, StatusGranted, false, StatusRevoked, ErrFinal},
		{"pending → revoked tidak boleh", StatusPending, StatusRevoked, false, StatusPending, ErrFinal},
		{"keputusan asing", StatusPending, "maybe", false, StatusPending, ErrInvalidDecision},
	}
	for _, tc := range cases {
		got, err := Decide(Consent{Status: tc.from}, tc.decision, "g1", tc.used, now)
		if !errors.Is(err, tc.err) {
			t.Errorf("%s: err %v want %v", tc.name, err, tc.err)
		}
		if got.Status != tc.want {
			t.Errorf("%s: status %s want %s", tc.name, got.Status, tc.want)
		}
		if err == nil && (got.DecidedByGuardianID == nil || *got.DecidedByGuardianID != "g1" || got.DecidedAt == nil) {
			t.Errorf("%s: decision metadata not set", tc.name)
		}
	}
}
