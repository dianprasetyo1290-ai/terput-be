package store

import (
	"context"
	"database/sql"

	"skci/academic/internal/model"
)

func (s *Store) CreateSubject(ctx context.Context, sub model.Subject) (*model.Subject, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO subjects (school_id, name, description, subject_group, kelompok)
		VALUES ($1, $2, $3, $4, NULLIF($5::text, ''))
		RETURNING id, school_id, name, description, subject_group, kelompok, created_at, updated_at
	`, sub.SchoolID, sub.Name, sub.Description, sub.SubjectGroup, sub.Kelompok)
	return scanSubject(row)
}

func (s *Store) ListSubjects(ctx context.Context, schoolID string, page, perPage int) ([]model.Subject, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM subjects WHERE school_id = $1 AND deleted_at IS NULL`, schoolID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, name, description, subject_group, kelompok, created_at, updated_at
		FROM subjects WHERE school_id = $1 AND deleted_at IS NULL ORDER BY name LIMIT $2 OFFSET $3
	`, schoolID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.Subject{}
	for rows.Next() {
		sub, err := scanSubjectRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *sub)
	}
	return out, total, rows.Err()
}

func (s *Store) GetSubject(ctx context.Context, schoolID, id string) (*model.Subject, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, name, description, subject_group, kelompok, created_at, updated_at
		FROM subjects WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`, id, schoolID)
	sub, err := scanSubject(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

func (s *Store) UpdateSubject(ctx context.Context, schoolID, id string, name, description, subjectGroup, kelompok *string) (*model.Subject, error) {
	row := s.DB.QueryRowContext(ctx, `
		UPDATE subjects SET
			name = COALESCE($3, name),
			description = COALESCE($4, description),
			subject_group = COALESCE($5, subject_group),
			kelompok = CASE WHEN $6::text IS NULL THEN kelompok ELSE NULLIF($6::text, '') END, -- "" clears it
			updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
		RETURNING id, school_id, name, description, subject_group, kelompok, created_at, updated_at
	`, id, schoolID, name, description, subjectGroup, kelompok)
	sub, err := scanSubject(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sub, err
}

func (s *Store) DeleteSubject(ctx context.Context, schoolID, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE subjects SET deleted_at = now() WHERE id = $1 AND school_id = $2`, id, schoolID)
	return err
}

func scanSubject(row rowScanner) (*model.Subject, error) {
	var sub model.Subject
	if err := row.Scan(&sub.ID, &sub.SchoolID, &sub.Name, &sub.Description, &sub.SubjectGroup, &sub.Kelompok, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
		return nil, err
	}
	return &sub, nil
}

func scanSubjectRow(rows *sql.Rows) (*model.Subject, error) {
	return scanSubject(rows)
}
