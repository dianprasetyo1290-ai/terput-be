package store

import (
	"context"
	"database/sql"

	"skci/report/internal/model"
)

// GetGradeBands returns a school's custom predikat bands if it has any,
// else falls back to model.DefaultGradeBands.
func (s *Store) GetGradeBands(ctx context.Context, schoolID string) ([]model.GradeBand, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, min_score, max_score, label, description
		FROM report_grade_bands WHERE school_id = $1 ORDER BY min_score DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.GradeBand
	for rows.Next() {
		var b model.GradeBand
		var sid sql.NullString
		var desc sql.NullString
		if err := rows.Scan(&b.ID, &sid, &b.MinScore, &b.MaxScore, &b.Label, &desc); err != nil {
			return nil, err
		}
		if sid.Valid {
			b.SchoolID = &sid.String
		}
		if desc.Valid {
			b.Description = &desc.String
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return model.DefaultGradeBands, nil
	}
	return out, nil
}
