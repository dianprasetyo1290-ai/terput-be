package model

import "testing"

func TestCorrectionRequestError(t *testing.T) {
	cases := []struct {
		source, kind string
		ok           bool
	}{
		{"cashier", "cancel", true},
		{"gateway", "cancel", false},
		{"gateway", "reassign", false},
		{"manual", "cancel", false},
		{"", "cancel", false},
		{"cashier", "reassign", false},
		{"cashier", "", false},
	}
	for _, tc := range cases {
		if got := CorrectionRequestError(tc.source, tc.kind); (got == "") != tc.ok {
			t.Errorf("%s/%s: %q", tc.source, tc.kind, got)
		}
	}
}
