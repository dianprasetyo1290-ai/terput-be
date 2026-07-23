package store

import (
	"context"
	"database/sql"

	"skci/academic/internal/model"
)

func (s *Store) BootstrapTerms(ctx context.Context, schoolID, academicYearID string) ([]model.Term, error) {
	rows, err := s.DB.QueryContext(ctx, `
		INSERT INTO terms (school_id, academic_year_id, name, "order")
		VALUES ($1, $2, 'Semester 1', 1), ($1, $2, 'Semester 2', 2)
		ON CONFLICT (academic_year_id, "order") DO NOTHING
		RETURNING id, school_id, academic_year_id, name, "order", start_date, end_date, created_at, updated_at
	`, schoolID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	terms, err := scanTerms(rows)
	if err != nil {
		return nil, err
	}
	if len(terms) > 0 {
		return terms, nil
	}
	// already bootstrapped previously; return existing rows
	return s.ListTerms(ctx, schoolID, academicYearID)
}

func (s *Store) ListTerms(ctx context.Context, schoolID, academicYearID string) ([]model.Term, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, name, "order", start_date, end_date, created_at, updated_at
		FROM terms WHERE school_id = $1 AND academic_year_id = $2 ORDER BY "order"
	`, schoolID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTerms(rows)
}

func (s *Store) GetTerm(ctx context.Context, schoolID, id string) (*model.Term, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, name, "order", start_date, end_date, created_at, updated_at
		FROM terms WHERE id = $1 AND ($2 = '' OR school_id::text = $2)
	`, id, schoolID)
	t, err := scanTerm(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (s *Store) UpdateTerm(ctx context.Context, schoolID, id string, name *string, startDate, endDate *string) (*model.Term, error) {
	row := s.DB.QueryRowContext(ctx, `
		UPDATE terms SET
			name = COALESCE($3, name),
			start_date = COALESCE($4::date, start_date),
			end_date = COALESCE($5::date, end_date),
			updated_at = now()
		WHERE id = $1 AND school_id = $2
		RETURNING id, school_id, academic_year_id, name, "order", start_date, end_date, created_at, updated_at
	`, id, schoolID, name, startDate, endDate)
	t, err := scanTerm(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanTerm(row rowScanner) (*model.Term, error) {
	var t model.Term
	if err := row.Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.Name, &t.Order, &t.StartDate, &t.EndDate, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTerms(rows *sql.Rows) ([]model.Term, error) {
	terms := []model.Term{}
	for rows.Next() {
		t, err := scanTerm(rows)
		if err != nil {
			return nil, err
		}
		terms = append(terms, *t)
	}
	return terms, rows.Err()
}
