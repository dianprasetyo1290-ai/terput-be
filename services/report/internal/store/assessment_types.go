package store

import (
	"context"
	"database/sql"

	"skci/report/internal/model"
)

// ListAssessmentTypes returns a school's configured "tipe penilaian"
// (item 4/5), ordered for display (sort_order, then name). Used both by
// the admin config screen and the guru Penilaian Formatif dropdown.
func (s *Store) ListAssessmentTypes(ctx context.Context, schoolID string) ([]model.AssessmentType, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, code, name, bucket, weight_percent, exam_linked, sort_order, created_at, updated_at
		FROM report_assessment_types WHERE school_id = $1 ORDER BY sort_order, name`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.AssessmentType{}
	for rows.Next() {
		var t model.AssessmentType
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Code, &t.Name, &t.Bucket, &t.WeightPercent, &t.ExamLinked, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetAssessmentType(ctx context.Context, id string) (*model.AssessmentType, error) {
	var t model.AssessmentType
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, bucket, weight_percent, exam_linked, sort_order, created_at, updated_at
		FROM report_assessment_types WHERE id = $1`, id).
		Scan(&t.ID, &t.SchoolID, &t.Code, &t.Name, &t.Bucket, &t.WeightPercent, &t.ExamLinked, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) CreateAssessmentType(ctx context.Context, schoolID, code, name, bucket string, weightPercent float64, examLinked bool, sortOrder int) (*model.AssessmentType, error) {
	var t model.AssessmentType
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO report_assessment_types (school_id, code, name, bucket, weight_percent, exam_linked, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, school_id, code, name, bucket, weight_percent, exam_linked, sort_order, created_at, updated_at`,
		schoolID, code, name, bucket, weightPercent, examLinked, sortOrder).
		Scan(&t.ID, &t.SchoolID, &t.Code, &t.Name, &t.Bucket, &t.WeightPercent, &t.ExamLinked, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) UpdateAssessmentType(ctx context.Context, id, name, bucket string, weightPercent float64, examLinked bool, sortOrder int) (*model.AssessmentType, error) {
	var t model.AssessmentType
	err := s.DB.QueryRowContext(ctx, `
		UPDATE report_assessment_types SET name = $1, bucket = $2, weight_percent = $3, exam_linked = $4, sort_order = $5, updated_at = now()
		WHERE id = $6
		RETURNING id, school_id, code, name, bucket, weight_percent, exam_linked, sort_order, created_at, updated_at`,
		name, bucket, weightPercent, examLinked, sortOrder, id).
		Scan(&t.ID, &t.SchoolID, &t.Code, &t.Name, &t.Bucket, &t.WeightPercent, &t.ExamLinked, &t.SortOrder, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// DeleteAssessmentType removes a tipe penilaian. If any formative_grades
// rows still reference it, the FK (no ON DELETE clause = RESTRICT) makes
// Postgres reject the delete; the caller maps that to a 409 rather than a
// generic 500 (see handler/settings.go).
func (s *Store) DeleteAssessmentType(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM report_assessment_types WHERE id = $1`, id)
	return err
}
