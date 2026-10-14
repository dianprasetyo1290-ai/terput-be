package model

import "testing"

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"", SeverityNormal, true},
		{"low", SeverityNormal, true},
		{"medium", SeverityNormal, true},
		{"Warning", SeverityNormal, true},
		{"normal", SeverityNormal, true},
		{"high", SeverityCritical, true},
		{" critical ", SeverityCritical, true},
		{"panic", "", false},
	}
	for _, tt := range tests {
		got, ok := NormalizeSeverity(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("NormalizeSeverity(%q) = %q,%v want %q,%v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}
