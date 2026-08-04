package store

import (
	"context"
	"database/sql"
)

type FormativeRow struct {
	StudentID string    `json:"student_id"`
	Scores    []float64 `json:"scores"`
}

type FormativeHeader struct {
	OrderIndex int    `json:"order_index"`
	HeaderName string `json:"header_name"`
}

// ListFormativeGrades returns every student's formative score columns for a
// (term, classroom_subject), ordered by order_index — the guru's
// spreadsheet-style grid (see old app's TeacherFormativeLite).
func (s *Store) ListFormativeGrades(ctx context.Context, termID, classroomSubjectID string) (map[string][]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT student_id, order_index, score FROM formative_grades
		WHERE term_id = $1 AND classroom_subject_id = $2
		ORDER BY student_id, order_index`, termID, classroomSubjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][]float64{}
	for rows.Next() {
		var studentID string
		var orderIndex int
		var score float64
		if err := rows.Scan(&studentID, &orderIndex, &score); err != nil {
			return nil, err
		}
		out[studentID] = append(out[studentID], score)
	}
	return out, rows.Err()
}

// UpsertFormativeGrades replaces every formative score for one student in
// one (term, classroom_subject) with the given ordered list — matching the
// old app's PUT .../grades/assignments "replace-all" semantics (guru edits
// the whole row of columns at once, not one cell at a time).
func (s *Store) UpsertFormativeGrades(ctx context.Context, schoolID, termID, classroomSubjectID, studentID string, scores []float64, createdBy string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM formative_grades WHERE term_id = $1 AND classroom_subject_id = $2 AND student_id = $3`,
		termID, classroomSubjectID, studentID); err != nil {
		return err
	}
	for i, score := range scores {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO formative_grades (school_id, term_id, classroom_subject_id, student_id, order_index, score, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			schoolID, termID, classroomSubjectID, studentID, i+1, score, createdBy); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SumFormativeGrades pools every formative score for a student in a
// classroom_subject across all terms sharing that classroom_subject_id's
// term (a classroom_subject_id already implies one term's assignment, so
// this scopes by termID too for clarity/defensiveness). Used by
// GenerateReportCards to blend formative scores into computed_score,
// matching the old app's flat pooled-average (sum of everything / count of
// everything, not average-of-averages).
func (s *Store) SumFormativeGrades(ctx context.Context, termID, classroomSubjectID, studentID string) (sum float64, count int) {
	var s2 sql.NullFloat64
	var c int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(score), 0), COUNT(*) FROM formative_grades
		WHERE term_id = $1 AND classroom_subject_id = $2 AND student_id = $3`,
		termID, classroomSubjectID, studentID).Scan(&s2, &c)
	if err != nil {
		return 0, 0
	}
	return s2.Float64, c
}

func (s *Store) ListFormativeHeaders(ctx context.Context, termID, classroomSubjectID string) ([]FormativeHeader, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT order_index, header_name FROM formative_headers
		WHERE term_id = $1 AND classroom_subject_id = $2
		ORDER BY order_index`, termID, classroomSubjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var headers []FormativeHeader
	for rows.Next() {
		var h FormativeHeader
		if err := rows.Scan(&h.OrderIndex, &h.HeaderName); err != nil {
			return nil, err
		}
		headers = append(headers, h)
	}
	return headers, rows.Err()
}

func (s *Store) UpsertFormativeHeader(ctx context.Context, termID, classroomSubjectID string, orderIndex int, headerName string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO formative_headers (term_id, classroom_subject_id, order_index, header_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (classroom_subject_id, term_id, order_index) DO UPDATE SET header_name = EXCLUDED.header_name, updated_at = NOW()`,
		termID, classroomSubjectID, orderIndex, headerName)
	return err
}
