package model

import (
	"errors"
	"testing"
	"time"
)

func TestParseAuditRange(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	tests := []struct {
		name, from, to string
		wantErr        error
		wantTo         string
	}{
		{"kosong", "", "", nil, ""},
		{"rentang normal, to inklusif sehari penuh", "2026-10-01", "2026-10-14", nil, "2026-10-15T00:00:00+07:00"},
		{"hari yang sama", "2026-10-14", "2026-10-14", nil, "2026-10-15T00:00:00+07:00"},
		{"BUG-152: terbalik ditolak", "2026-10-14", "2026-10-01", ErrAuditDateRange, ""},
		{"format salah", "14/10/2026", "", ErrAuditDateFormat, ""},
		{"RFC3339 apa adanya", "", "2026-10-14T10:00:00+07:00", nil, "2026-10-14T10:00:00+07:00"},
		{"jam sama persis ditolak", "2026-10-14T10:00:00+07:00", "2026-10-14T10:00:00+07:00", ErrAuditDateRange, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, to, err := ParseAuditRange(tt.from, tt.to, loc)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantTo != "" && (to == nil || to.Format(time.RFC3339) != tt.wantTo) {
				t.Fatalf("to = %v, want %s", to, tt.wantTo)
			}
		})
	}
}
