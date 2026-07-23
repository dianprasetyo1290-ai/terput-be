package store

import (
	"context"
	"database/sql"

	"skci/exam/internal/model"
)

func (s *Store) CreateExamType(ctx context.Context, t *model.ExamType) error {
	return s.DB.QueryRowContext(ctx, `
		INSERT INTO exam_types (school_id, academic_year_id, name, mapping_rapor, created_by)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, created_at, updated_at`,
		t.SchoolID, t.AcademicYearID, t.Name, t.MappingRapor, t.CreatedBy,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (s *Store) ListExamTypes(ctx context.Context, schoolID, academicYearID string, page, perPage int) ([]model.ExamType, int64, error) {
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM exam_types WHERE ($1 = '' OR school_id::text = $1) AND academic_year_id=$2 AND deleted_at IS NULL`, schoolID, academicYearID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, name, mapping_rapor, created_by, created_at, updated_at
		FROM exam_types WHERE ($1 = '' OR school_id::text = $1) AND academic_year_id=$2 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`,
		schoolID, academicYearID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.ExamType
	for rows.Next() {
		var t model.ExamType
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.Name, &t.MappingRapor, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

func (s *Store) GetExamType(ctx context.Context, id, schoolID string) (*model.ExamType, error) {
	var t model.ExamType
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, name, mapping_rapor, created_by, created_at, updated_at
		FROM exam_types WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID,
	).Scan(&t.ID, &t.SchoolID, &t.AcademicYearID, &t.Name, &t.MappingRapor, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) UpdateExamType(ctx context.Context, id, schoolID, name, mappingRapor string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE exam_types SET name=$1, mapping_rapor=$2, updated_at=now()
		WHERE id=$3 AND school_id=$4 AND deleted_at IS NULL`, name, mappingRapor, id, schoolID)
	return err
}

func (s *Store) DeleteExamType(ctx context.Context, id, schoolID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE exam_types SET deleted_at=now() WHERE id=$1 AND school_id=$2`, id, schoolID)
	return err
}
