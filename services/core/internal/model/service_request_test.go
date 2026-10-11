package model

import (
	"testing"
)

func TestValidateStatusTransition(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		to        string
		note      string
		expectErr bool
	}{
		{"submitted to accepted", "submitted", "accepted", "", false},
		{"submitted to rejected with note", "submitted", "rejected", "Data tidak lengkap", false},
		{"submitted to rejected without note", "submitted", "rejected", "", true},
		{"submitted to in_progress invalid", "submitted", "in_progress", "", true},
		{"submitted to done invalid", "submitted", "done", "", true},

		{"accepted to in_progress", "accepted", "in_progress", "", false},
		{"accepted to rejected with note", "accepted", "rejected", "Dibatalkan pemohon", false},
		{"accepted to rejected without note", "accepted", "rejected", "", true},
		{"accepted to done invalid", "accepted", "done", "", true},

		{"in_progress to done", "in_progress", "done", "", false},
		{"in_progress to rejected with note", "in_progress", "rejected", "Ditolak", false},
		{"in_progress to accepted invalid", "in_progress", "accepted", "", true},

		{"done to anywhere invalid", "done", "in_progress", "", true},
		{"rejected to anywhere invalid", "rejected", "submitted", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatusTransition(tt.from, tt.to, tt.note)
			if (err != nil) != tt.expectErr {
				t.Fatalf("expected error: %v, got: %v", tt.expectErr, err)
			}
		})
	}
}
