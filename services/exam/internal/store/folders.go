package store

import (
	"context"
	"database/sql"

	"skci/exam/internal/model"
)

func (s *Store) CreateFolder(ctx context.Context, f *model.QuestionFolder) error {
	return s.DB.QueryRowContext(ctx, `
		INSERT INTO question_folders (school_id, academic_year_id, name, description, created_by)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, created_at, updated_at`,
		f.SchoolID, f.AcademicYearID, f.Name, f.Description, f.CreatedBy,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

func (s *Store) ListFolders(ctx context.Context, schoolID, academicYearID string, page, perPage int) ([]model.QuestionFolder, int64, error) {
	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM question_folders WHERE school_id=$1 AND academic_year_id=$2 AND deleted_at IS NULL`, schoolID, academicYearID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, name, description, created_by, created_at, updated_at
		FROM question_folders WHERE school_id=$1 AND academic_year_id=$2 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`, schoolID, academicYearID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.QuestionFolder
	for rows.Next() {
		var f model.QuestionFolder
		if err := rows.Scan(&f.ID, &f.SchoolID, &f.AcademicYearID, &f.Name, &f.Description, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, f)
	}
	return out, total, rows.Err()
}

func (s *Store) GetFolder(ctx context.Context, id, schoolID string) (*model.QuestionFolder, error) {
	var f model.QuestionFolder
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, name, description, created_by, created_at, updated_at
		FROM question_folders WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID,
	).Scan(&f.ID, &f.SchoolID, &f.AcademicYearID, &f.Name, &f.Description, &f.CreatedBy, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *Store) UpdateFolder(ctx context.Context, id, schoolID, name string, description *string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE question_folders SET name=$1, description=$2, updated_at=now()
		WHERE id=$3 AND school_id=$4 AND deleted_at IS NULL`, name, description, id, schoolID)
	return err
}

func (s *Store) DeleteFolder(ctx context.Context, id, schoolID string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE question_folders SET deleted_at=now() WHERE id=$1 AND school_id=$2`, id, schoolID)
	return err
}

// FindOrCreateImportFolder is used by the CSV import flow when no folder_id
// is supplied: reuses (or lazily creates) a per-guru "Import - <exam title>"
// folder so imported soal still land somewhere in Bank Soal.
func (s *Store) FindOrCreateImportFolder(ctx context.Context, schoolID, academicYearID, createdBy, name string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id FROM question_folders
		WHERE school_id=$1 AND academic_year_id=$2 AND created_by=$3 AND name=$4 AND deleted_at IS NULL
		LIMIT 1`, schoolID, academicYearID, createdBy, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO question_folders (school_id, academic_year_id, name, created_by)
		VALUES ($1,$2,$3,$4) RETURNING id`, schoolID, academicYearID, name, createdBy).Scan(&id)
	return id, err
}
