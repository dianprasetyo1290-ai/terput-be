package store

import (
	"context"
	"database/sql"
	"fmt"

	"skci/exam/internal/model"
)

func (s *Store) CreateExam(ctx context.Context, e *model.Exam) error {
	return s.DB.QueryRowContext(ctx, `
		INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, description,
			classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, is_published, created_at, updated_at`,
		e.SchoolID, e.AcademicYearID, e.TermID, e.ExamTypeID, e.Title, e.Description,
		e.ClassroomID, e.SubjectID, e.TeacherID, e.StartAt, e.EndAt, e.IsShuffled, e.CreatedBy,
	).Scan(&e.ID, &e.IsPublished, &e.CreatedAt, &e.UpdatedAt)
}

type ExamFilter struct {
	SchoolID       string
	AcademicYearID string
	TermID         string
	ClassroomID    string
	TeacherID      string // "mine" scoping for guru
	IsPublished    *bool
}

func (s *Store) ListExams(ctx context.Context, f ExamFilter, page, perPage int) ([]model.Exam, int64, error) {
	where := "($1 = '' OR e.school_id::text = $1) AND e.deleted_at IS NULL"
	args := []interface{}{f.SchoolID}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		where += fmt.Sprintf(" AND %s=$%d", cond, len(args))
	}
	if f.AcademicYearID != "" {
		add("e.academic_year_id", f.AcademicYearID)
	}
	if f.TermID != "" {
		add("e.term_id", f.TermID)
	}
	if f.ClassroomID != "" {
		add("e.classroom_id", f.ClassroomID)
	}
	if f.TeacherID != "" {
		add("e.teacher_id", f.TeacherID)
	}
	if f.IsPublished != nil {
		add("e.is_published", *f.IsPublished)
	}

	var total int64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM exams e WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, perPage, (page-1)*perPage)
	q := fmt.Sprintf(`
		SELECT e.id, e.school_id, e.academic_year_id, e.term_id, e.exam_type_id, e.title, e.description,
			e.classroom_id, e.subject_id, e.teacher_id, e.start_at, e.end_at, e.is_shuffled, e.is_published,
			e.published_at, e.created_by, e.created_at, e.updated_at,
			COALESCE((SELECT SUM(points) FROM exam_questions eq WHERE eq.exam_id = e.id), 0),
			COALESCE((SELECT COUNT(*) FROM exam_questions eq WHERE eq.exam_id = e.id), 0)
		FROM exams e WHERE %s ORDER BY e.created_at DESC LIMIT $%d OFFSET $%d`,
		where, len(args)-1, len(args))

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.Exam
	for rows.Next() {
		var e model.Exam
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
			&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
			&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.TotalPoints, &e.QuestionCount); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (s *Store) GetExam(ctx context.Context, id, schoolID string) (*model.Exam, error) {
	var e model.Exam
	err := s.DB.QueryRowContext(ctx, `
		SELECT e.id, e.school_id, e.academic_year_id, e.term_id, e.exam_type_id, e.title, e.description,
			e.classroom_id, e.subject_id, e.teacher_id, e.start_at, e.end_at, e.is_shuffled, e.is_published,
			e.published_at, e.created_by, e.created_at, e.updated_at,
			COALESCE((SELECT SUM(points) FROM exam_questions eq WHERE eq.exam_id = e.id), 0),
			COALESCE((SELECT COUNT(*) FROM exam_questions eq WHERE eq.exam_id = e.id), 0)
		FROM exams e WHERE e.id=$1 AND ($2 = '' OR e.school_id::text=$2) AND e.deleted_at IS NULL`, id, schoolID,
	).Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
		&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
		&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt, &e.TotalPoints, &e.QuestionCount)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) UpdateExam(ctx context.Context, e *model.Exam) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exams SET term_id=$1, exam_type_id=$2, title=$3, description=$4, classroom_id=$5, subject_id=$6,
			teacher_id=$7, start_at=$8, end_at=$9, is_shuffled=$10, updated_at=now()
		WHERE id=$11 AND school_id=$12 AND is_published=false AND deleted_at IS NULL`,
		e.TermID, e.ExamTypeID, e.Title, e.Description, e.ClassroomID, e.SubjectID,
		e.TeacherID, e.StartAt, e.EndAt, e.IsShuffled, e.ID, e.SchoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotEditable
	}
	return nil
}

var ErrNotEditable = fmt.Errorf("exam is not editable (published or not found)")

func (s *Store) DeleteExam(ctx context.Context, id, schoolID string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE exams SET deleted_at=now() WHERE id=$1 AND school_id=$2 AND is_published=false`, id, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotEditable
	}
	return nil
}

// --- exam_questions ---

func (s *Store) AddExamQuestion(ctx context.Context, examID, questionID string, points float64) (*model.ExamQuestion, error) {
	var eq model.ExamQuestion
	var nextIdx int
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(order_index)+1, 0) FROM exam_questions WHERE exam_id=$1`, examID).Scan(&nextIdx); err != nil {
		return nil, err
	}
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		examID, questionID, nextIdx, points).Scan(&eq.ID, &eq.CreatedAt)
	if err != nil {
		return nil, err
	}
	eq.ExamID = examID
	eq.QuestionID = questionID
	eq.OrderIndex = nextIdx
	eq.Points = points
	return &eq, nil
}

func (s *Store) RemoveExamQuestion(ctx context.Context, examID, examQuestionID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM exam_questions WHERE id=$1 AND exam_id=$2`, examQuestionID, examID)
	return err
}

// ListExamQuestions returns exam_questions joined with their question, ordered
// by order_index (the natural/base sequence; display numbering follows this
// unless a per-attempt shuffle order overrides it).
func (s *Store) ListExamQuestions(ctx context.Context, examID string) ([]model.ExamQuestion, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT eq.id, eq.exam_id, eq.question_id, eq.order_index, eq.points, eq.created_at,
			q.id, q.school_id, q.academic_year_id, q.folder_id, q.created_by, q.question_type, q.content, q.image_url,
			q.no_kd, q.konten_kd, q.indikator_pencapaian, q.level_kognitif, q.poin, q.is_published, q.created_at, q.updated_at
		FROM exam_questions eq JOIN questions q ON q.id = eq.question_id
		WHERE eq.exam_id=$1 ORDER BY eq.order_index`, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamQuestion
	for rows.Next() {
		var eq model.ExamQuestion
		var q model.Question
		if err := rows.Scan(&eq.ID, &eq.ExamID, &eq.QuestionID, &eq.OrderIndex, &eq.Points, &eq.CreatedAt,
			&q.ID, &q.SchoolID, &q.AcademicYearID, &q.FolderID, &q.CreatedBy, &q.QuestionType, &q.Content, &q.ImageURL,
			&q.NoKD, &q.KontenKD, &q.IndikatorPencapaian, &q.LevelKognitif, &q.Poin, &q.IsPublished, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		eq.Question = &q
		out = append(out, eq)
	}
	return out, rows.Err()
}

// StudentExamRow is the siswa-dashboard view: an exam joined with that
// student's own attempt (if any voucher was ever issued to them for it).
type StudentExamRow struct {
	Exam    model.Exam        `json:"exam"`
	Attempt *model.ExamAttempt `json:"attempt"`
}

// ListMyExams buckets by "today" (window currently open), "upcoming"
// (start_at in the future), or "history" (window closed / attempt selesai).
// Only exams the student holds a voucher (hence an exam_attempts row) for
// are returned; see README Assumptions.
func (s *Store) ListMyExams(ctx context.Context, userID, bucket string) ([]StudentExamRow, error) {
	where := "ea.user_id = $1 AND e.deleted_at IS NULL"
	switch bucket {
	case "today":
		where += " AND e.start_at <= now() AND e.end_at >= now() AND ea.status != 'selesai'"
	case "upcoming":
		where += " AND e.start_at > now()"
	case "history":
		where += " AND (ea.status = 'selesai' OR e.end_at < now())"
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT e.id, e.school_id, e.academic_year_id, e.term_id, e.exam_type_id, e.title, e.description,
			e.classroom_id, e.subject_id, e.teacher_id, e.start_at, e.end_at, e.is_shuffled, e.is_published,
			e.published_at, e.created_by, e.created_at, e.updated_at,
			`+attemptColsQualified+`
		FROM exam_attempts ea
		JOIN exams e ON e.id = ea.exam_id
		WHERE `+where+`
		ORDER BY e.start_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StudentExamRow
	for rows.Next() {
		var e model.Exam
		var qOrderJSON []byte
		var a model.ExamAttempt
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
			&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
			&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt,
			&a.ID, &a.SchoolID, &a.AcademicYearID, &a.ExamID, &a.UserID, &a.ClassroomID, &a.TokenID, &a.Status,
			&qOrderJSON, &a.CurrentPosition, &a.FocusLostCount, &a.StartedAt, &a.SubmittedAt,
			&a.BlockedAt, &a.BlockedReason, &a.BlockedBy, &a.BannedAt, &a.BannedReason, &a.BannedBy,
			&a.Score, &a.CorrectCount, &a.GradedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, StudentExamRow{Exam: e, Attempt: &a})
	}
	return out, rows.Err()
}

// GetExamQuestion fetches one exam_questions row joined with its question
// (used when validating answers / manual grading overrides).
func (s *Store) GetExamQuestion(ctx context.Context, examQuestionID string) (*model.ExamQuestion, error) {
	var eq model.ExamQuestion
	var q model.Question
	err := s.DB.QueryRowContext(ctx, `
		SELECT eq.id, eq.exam_id, eq.question_id, eq.order_index, eq.points, eq.created_at,
			q.id, q.school_id, q.academic_year_id, q.folder_id, q.created_by, q.question_type, q.content, q.image_url,
			q.no_kd, q.konten_kd, q.indikator_pencapaian, q.level_kognitif, q.poin, q.is_published, q.created_at, q.updated_at
		FROM exam_questions eq JOIN questions q ON q.id = eq.question_id
		WHERE eq.id=$1`, examQuestionID,
	).Scan(&eq.ID, &eq.ExamID, &eq.QuestionID, &eq.OrderIndex, &eq.Points, &eq.CreatedAt,
		&q.ID, &q.SchoolID, &q.AcademicYearID, &q.FolderID, &q.CreatedBy, &q.QuestionType, &q.Content, &q.ImageURL,
		&q.NoKD, &q.KontenKD, &q.IndikatorPencapaian, &q.LevelKognitif, &q.Poin, &q.IsPublished, &q.CreatedAt, &q.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	eq.Question = &q

	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, question_id, content, image_url, is_correct, order_index
		FROM answer_options WHERE question_id=$1 ORDER BY order_index`, q.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var o model.AnswerOption
		if err := rows.Scan(&o.ID, &o.QuestionID, &o.Content, &o.ImageURL, &o.IsCorrect, &o.OrderIndex); err != nil {
			rows.Close()
			return nil, err
		}
		q.AnswerOptions = append(q.AnswerOptions, o)
	}
	rows.Close()

	rows2, err := s.DB.QueryContext(ctx, `
		SELECT id, question_id, answer_text FROM essay_answer_options WHERE question_id=$1`, q.ID)
	if err != nil {
		return nil, err
	}
	for rows2.Next() {
		var e model.EssayAnswerOption
		if err := rows2.Scan(&e.ID, &e.QuestionID, &e.AnswerText); err != nil {
			rows2.Close()
			return nil, err
		}
		q.EssayOptions = append(q.EssayOptions, e)
	}
	rows2.Close()

	return &eq, nil
}

// NormalizeAndPublish proportionally scales exam_questions.points so they sum
// to exactly 100, then marks the exam published. See README "Score
// normalization" for the approach.
func (s *Store) NormalizeAndPublish(ctx context.Context, examID, schoolID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id, points FROM exam_questions WHERE exam_id=$1 ORDER BY order_index`, examID)
	if err != nil {
		return err
	}
	type row struct {
		id     string
		points float64
	}
	var items []row
	var sum float64
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.points); err != nil {
			rows.Close()
			return err
		}
		items = append(items, r)
		sum += r.points
	}
	rows.Close()

	if len(items) == 0 {
		return fmt.Errorf("exam has no questions")
	}
	if sum <= 0 {
		return fmt.Errorf("exam total raw points must be greater than zero")
	}

	// Scale proportionally to sum to 100; last item absorbs rounding residue
	// so the total is always exactly 100.00.
	var running float64
	for i, it := range items {
		var scaled float64
		if i == len(items)-1 {
			scaled = round2(100 - running)
		} else {
			scaled = round2(it.points / sum * 100)
			running += scaled
		}
		if _, err := tx.ExecContext(ctx, `UPDATE exam_questions SET points=$1 WHERE id=$2`, scaled, it.id); err != nil {
			return err
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE exams SET is_published=true, published_at=now(), updated_at=now()
		WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, examID, schoolID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotEditable
	}

	return tx.Commit()
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

// DuplicateExam copies an exam's settings and its exam_questions into a new,
// always-unpublished exam row (see ErrNotEditable-adjacent lifecycle rule:
// a duplicate is a fresh draft, not a clone of the source's publish state).
// Tokens/attempts are intentionally not copied — those belong to the source
// exam's live exam-taking data only.
func (s *Store) DuplicateExam(ctx context.Context, id, schoolID, actorUserID string) (*model.Exam, error) {
	src, err := s.GetExam(ctx, id, schoolID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, nil
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	e := &model.Exam{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, description,
			classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, school_id, academic_year_id, term_id, exam_type_id, title, description,
			classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, is_published,
			published_at, created_by, created_at, updated_at`,
		src.SchoolID, src.AcademicYearID, src.TermID, src.ExamTypeID, src.Title+" (Salinan)", src.Description,
		src.ClassroomID, src.SubjectID, src.TeacherID, src.StartAt, src.EndAt, src.IsShuffled, actorUserID,
	).Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.TermID, &e.ExamTypeID, &e.Title, &e.Description,
		&e.ClassroomID, &e.SubjectID, &e.TeacherID, &e.StartAt, &e.EndAt, &e.IsShuffled, &e.IsPublished,
		&e.PublishedAt, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO exam_questions (exam_id, question_id, order_index, points)
		SELECT $1, question_id, order_index, points FROM exam_questions WHERE exam_id=$2`,
		e.ID, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	e.QuestionCount = src.QuestionCount
	e.TotalPoints = src.TotalPoints
	return e, nil
}
