package model

import (
	"errors"
	"time"
)

var (
	// ErrAuditDateFormat: from/to is not RFC3339, YYYY-MM-DDTHH:MM:SS or YYYY-MM-DD.
	ErrAuditDateFormat = errors.New("format tanggal tidak valid; gunakan YYYY-MM-DD")
	// ErrAuditDateRange: from is after to (BUG-152, TC-06-006).
	ErrAuditDateRange = errors.New("rentang tanggal tidak valid: tanggal awal melewati tanggal akhir")
)

// ParseAuditRange parses the audit filter window. A date-only "to" covers the
// whole day (the store filters created_at < to, so it becomes the next
// midnight). Empty values leave that side open.
func ParseAuditRange(from, to string, loc *time.Location) (*time.Time, *time.Time, error) {
	parse := func(s string, endOfDay bool) (*time.Time, error) {
		if s == "" {
			return nil, nil
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
			if t, err := time.ParseInLocation(layout, s, loc); err == nil {
				return &t, nil
			}
		}
		t, err := time.ParseInLocation("2006-01-02", s, loc)
		if err != nil {
			return nil, ErrAuditDateFormat
		}
		if endOfDay {
			t = t.AddDate(0, 0, 1)
		}
		return &t, nil
	}
	f, err := parse(from, false)
	if err != nil {
		return nil, nil, err
	}
	t, err := parse(to, true)
	if err != nil {
		return nil, nil, err
	}
	if f != nil && t != nil && !f.Before(*t) {
		return nil, nil, ErrAuditDateRange
	}
	return f, t, nil
}
