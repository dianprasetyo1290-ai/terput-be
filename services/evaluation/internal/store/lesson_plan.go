package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"skci/evaluation/internal/model"
)

type LessonPlanStore struct{ db *pgxpool.Pool }

func NewLessonPlanStore(db *pgxpool.Pool) *LessonPlanStore { return &LessonPlanStore{db: db} }

func marshalMap(m map[string]interface{}) []byte {
	if m == nil {
		m = map[string]interface{}{}
	}
	b, _ := json.Marshal(m)
	return b
}

func (s *LessonPlanStore) Create(ctx context.Context, lp *model.LessonPlan) error {
	row := s.db.QueryRow(ctx, `
		INSERT INTO lesson_plans (school_id, academic_year_id, guru_user_id, title, section_a, section_b, section_c, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, created_at, updated_at`,
		lp.SchoolID, lp.AcademicYearID, lp.GuruUserID, lp.Title,
		marshalMap(lp.SectionA), marshalMap(lp.SectionB), marshalMap(lp.SectionC), lp.Status)
	return row.Scan(&lp.ID, &lp.CreatedAt, &lp.UpdatedAt)
}

// ImportSections overwrites section_a/b/c and dimensi_lulusan for a guru's
// own RPM entry in one shot (used by the combined A/B/C/D Excel-import
// handler, rpm_template.go). Returns false if no row matched (wrong id or
// not the caller's own RPM).
func (s *LessonPlanStore) ImportSections(ctx context.Context, id, guruUserID string, sectionA, sectionB, sectionC, dimensi map[string]interface{}) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE lesson_plans SET section_a=$1, section_b=$2, section_c=$3, dimensi_lulusan=$4, updated_at=now()
		WHERE id=$5 AND guru_user_id=$6 AND deleted_at IS NULL`,
		marshalMap(sectionA), marshalMap(sectionB), marshalMap(sectionC), marshalMap(dimensi), id, guruUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *LessonPlanStore) Update(ctx context.Context, id, guruUserID string, lp *model.LessonPlan) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE lesson_plans SET title=$1, section_a=$2, section_b=$3, section_c=$4, status=$5, updated_at=now()
		WHERE id=$6 AND guru_user_id=$7 AND deleted_at IS NULL`,
		lp.Title, marshalMap(lp.SectionA), marshalMap(lp.SectionB), marshalMap(lp.SectionC), lp.Status, id, guruUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func scanLessonPlan(row interface {
	Scan(dest ...interface{}) error
}) (*model.LessonPlan, error) {
	var lp model.LessonPlan
	var a, b, c, d []byte
	if err := row.Scan(&lp.ID, &lp.SchoolID, &lp.AcademicYearID, &lp.GuruUserID, &lp.Title, &a, &b, &c, &d, &lp.Status, &lp.CreatedAt, &lp.UpdatedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(a, &lp.SectionA)
	_ = json.Unmarshal(b, &lp.SectionB)
	_ = json.Unmarshal(c, &lp.SectionC)
	_ = json.Unmarshal(d, &lp.DimensiLulusan)
	return &lp, nil
}

const lessonPlanCols = `id, school_id, academic_year_id, guru_user_id, title, section_a, section_b, section_c, dimensi_lulusan, status, created_at, updated_at`

func (s *LessonPlanStore) GetByID(ctx context.Context, id, schoolID string) (*model.LessonPlan, error) {
	row := s.db.QueryRow(ctx, `SELECT `+lessonPlanCols+` FROM lesson_plans WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	return scanLessonPlan(row)
}

func (s *LessonPlanStore) ListByGuru(ctx context.Context, guruUserID, schoolID string) ([]*model.LessonPlan, error) {
	rows, err := s.db.Query(ctx, `SELECT `+lessonPlanCols+` FROM lesson_plans WHERE guru_user_id=$1 AND school_id=$2 AND deleted_at IS NULL ORDER BY updated_at DESC`, guruUserID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.LessonPlan
	for rows.Next() {
		lp, err := scanLessonPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lp)
	}
	return out, rows.Err()
}

// ListAll is the admin read-only view: all guru's lesson plans in the school (optionally filter by guru_id).
func (s *LessonPlanStore) ListAll(ctx context.Context, schoolID string, guruUserID *string, page, perPage int) ([]*model.LessonPlan, int, error) {
	base := `FROM lesson_plans WHERE school_id=$1 AND deleted_at IS NULL`
	args := []interface{}{schoolID}
	if guruUserID != nil {
		base += ` AND guru_user_id=$2`
		args = append(args, *guruUserID)
	}
	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, perPage, (page-1)*perPage)
	limitOffset := fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.Query(ctx, `SELECT `+lessonPlanCols+` `+base+limitOffset, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.LessonPlan
	for rows.Next() {
		lp, err := scanLessonPlan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, lp)
	}
	return out, total, rows.Err()
}

func (s *LessonPlanStore) Delete(ctx context.Context, id, guruUserID string) (bool, error) {
	tag, err := s.db.Exec(ctx, `UPDATE lesson_plans SET deleted_at=now() WHERE id=$1 AND guru_user_id=$2 AND deleted_at IS NULL`, id, guruUserID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
