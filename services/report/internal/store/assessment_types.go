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

// DeleteAssessmentType removes a tipe penilaian together with the
// formative_grades rows that reference it, in one transaction.
//
// The FK has no ON DELETE clause (= RESTRICT), so deleting the type alone
// fails with 23503 the moment a guru has entered a single score for it --
// which is every type worth deleting. Penilaian Formatif's "Hapus" already
// warns "Nilai yang sudah tersimpan untuk tipe ini akan ikut terhapus", so
// the child rows go with it rather than the guru being stuck with a type
// they can never remove (prod, 2026-09-10: "STS1" had 10 grades and could
// not be deleted at all).
func (s *Store) DeleteAssessmentType(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM formative_grades WHERE assessment_type_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM report_assessment_types WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}
