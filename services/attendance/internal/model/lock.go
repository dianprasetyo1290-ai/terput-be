package model

import (
	"fmt"
	"time"
)

// LockSettings is the per-school daily attendance lock (FR-41-003, BR-ATT-01
// [per klien]). Attendance for date D is locked from (D + CutoffDays) at
// CutoffTime (HH:MM, Asia/Jakarta) onwards. Once locked, every direct write
// path (guru marking, admin import, admin/wali kelas daily input) refuses
// the change; it can only go through a correction request approved in the
// core Approval Center, which is audit-trailed.
type LockSettings struct {
	SchoolID   string     `json:"school_id"`
	Enabled    bool       `json:"enabled"`
	CutoffDays int        `json:"cutoff_days"`
	CutoffTime string     `json:"cutoff_time"` // HH:MM
	UpdatedBy  *string    `json:"updated_by,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

// DefaultLockSettings is what a school without a configured row gets:
// locking disabled, so existing clients keep their current behaviour until
// an admin turns it on.
func DefaultLockSettings(schoolID string) LockSettings {
	return LockSettings{SchoolID: schoolID, Enabled: false, CutoffDays: 0, CutoffTime: "23:59"}
}

// Validate checks the editable fields.
func (l LockSettings) Validate() error {
	if l.CutoffDays < 0 || l.CutoffDays > 31 {
		return fmt.Errorf("cutoff_days harus 0–31")
	}
	if _, err := time.Parse("15:04", l.CutoffTime); err != nil {
		return fmt.Errorf("cutoff_time harus berformat HH:MM")
	}
	return nil
}

// DateOnly trims a scanned DATE ("2026-10-06T00:00:00Z") to YYYY-MM-DD.
func DateOnly(s string) string {
	if len(s) > 10 && s[10] == 'T' {
		return s[:10]
	}
	return s
}

// LockAt returns the instant attendance for date (YYYY-MM-DD) becomes locked.
func (l LockSettings) LockAt(date string, loc *time.Location) (time.Time, error) {
	d, err := time.ParseInLocation("2006-01-02", DateOnly(date), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("tanggal harus berformat YYYY-MM-DD")
	}
	ct, err := time.Parse("15:04", l.CutoffTime)
	if err != nil {
		return time.Time{}, fmt.Errorf("cutoff_time tidak valid")
	}
	return time.Date(d.Year(), d.Month(), d.Day()+l.CutoffDays, ct.Hour(), ct.Minute(), 0, 0, loc), nil
}

// IsLocked reports whether attendance for date is locked at now. A disabled
// setting or an unparsable date never locks (the caller validates dates).
func (l LockSettings) IsLocked(date string, now time.Time, loc *time.Location) bool {
	if !l.Enabled {
		return false
	}
	at, err := l.LockAt(date, loc)
	if err != nil {
		return false
	}
	return !now.Before(at)
}
