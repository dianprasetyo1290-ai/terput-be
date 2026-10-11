package store

import (
	"context"

	"skci/hr/internal/model"
)

// Settings returns the school's hr_settings row, creating it from the column
// defaults on first use (OQ-23: geolocation off until HR switches it on).
func Settings(ctx context.Context, q Q, schoolID string) (model.HRSettings, error) {
	var s model.HRSettings
	if _, err := q.ExecContext(ctx, `INSERT INTO hr_settings (school_id) VALUES ($1) ON CONFLICT DO NOTHING`, schoolID); err != nil {
		return s, err
	}
	var geofence []byte
	err := q.QueryRowContext(ctx, `SELECT school_id, contract_reminder_days, document_reminder_days, geolocation_enabled,
		geofence, timezone, created_at, updated_at FROM hr_settings WHERE school_id = $1`, schoolID).
		Scan(&s.SchoolID, &s.ContractReminderDays, &s.DocumentReminderDays, &s.GeolocationEnabled, &geofence, &s.Timezone, &s.CreatedAt, &s.UpdatedAt)
	s.Geofence = geofence
	return s, err
}

// SaveSettings updates the editable settings.
func SaveSettings(ctx context.Context, q Q, s model.HRSettings) error {
	var geofence any
	if len(s.Geofence) > 0 && string(s.Geofence) != "null" {
		geofence = []byte(s.Geofence)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO hr_settings (school_id, contract_reminder_days, document_reminder_days, geolocation_enabled, geofence, timezone)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (school_id) DO UPDATE SET contract_reminder_days = EXCLUDED.contract_reminder_days,
			document_reminder_days = EXCLUDED.document_reminder_days, geolocation_enabled = EXCLUDED.geolocation_enabled,
			geofence = EXCLUDED.geofence, timezone = EXCLUDED.timezone, updated_at = now()`,
		s.SchoolID, s.ContractReminderDays, s.DocumentReminderDays, s.GeolocationEnabled, geofence, s.Timezone)
	return err
}

// HRSchools lists every school that has hr data (job fan-out).
func HRSchools(ctx context.Context, q Q) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT school_id::text FROM employees`)
	return collect(rows, err, func(r *sqlRows) (string, error) {
		var id string
		return id, r.Scan(&id)
	})
}

// GetHRSettings is a pointer-returning helper for Settings.
func GetHRSettings(ctx context.Context, q Q, schoolID string) (*model.HRSettings, error) {
	s, err := Settings(ctx, q, schoolID)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdateHRSettings updates the editable settings.
func UpdateHRSettings(ctx context.Context, q Q, s *model.HRSettings) error {
	if s == nil {
		return nil
	}
	return SaveSettings(ctx, q, *s)
}
