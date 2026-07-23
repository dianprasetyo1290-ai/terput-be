package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"skci/exam/internal/model"
)

var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrTokenInvalid      = errors.New("token invalid or expired")
)

func scanAttempt(row interface {
	Scan(dest ...interface{}) error
}) (*model.ExamAttempt, error) {
	var a model.ExamAttempt
	var qOrderJSON []byte
	err := row.Scan(&a.ID, &a.SchoolID, &a.AcademicYearID, &a.ExamID, &a.UserID, &a.ClassroomID, &a.TokenID, &a.Status,
		&qOrderJSON, &a.CurrentPosition, &a.FocusLostCount, &a.StartedAt, &a.SubmittedAt,
		&a.BlockedAt, &a.BlockedReason, &a.BlockedBy, &a.BannedAt, &a.BannedReason, &a.BannedBy,
		&a.Score, &a.CorrectCount, &a.GradedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(qOrderJSON) > 0 {
		_ = json.Unmarshal(qOrderJSON, &a.QuestionOrder)
	}
	return &a, nil
}

const attemptCols = `id, school_id, academic_year_id, exam_id, user_id, classroom_id, token_id, status,
	question_order, current_position, focus_lost_count, started_at, submitted_at,
	blocked_at, blocked_reason, blocked_by, banned_at, banned_reason, banned_by,
	score, correct_count, graded_at, created_at, updated_at`

// attemptColsQualified is attemptCols prefixed with "ea." for use in JOIN
// queries against other tables that share column names (id, created_at, ...).
const attemptColsQualified = `ea.id, ea.school_id, ea.academic_year_id, ea.exam_id, ea.user_id, ea.classroom_id, ea.token_id, ea.status,
	ea.question_order, ea.current_position, ea.focus_lost_count, ea.started_at, ea.submitted_at,
	ea.blocked_at, ea.blocked_reason, ea.blocked_by, ea.banned_at, ea.banned_reason, ea.banned_by,
	ea.score, ea.correct_count, ea.graded_at, ea.created_at, ea.updated_at`

func (s *Store) GetAttempt(ctx context.Context, examID, userID string) (*model.ExamAttempt, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts WHERE exam_id=$1 AND user_id=$2`, examID, userID)
	a, err := scanAttempt(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *Store) GetAttemptByID(ctx context.Context, id, schoolID string) (*model.ExamAttempt, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts WHERE id=$1 AND school_id=$2`, id, schoolID)
	a, err := scanAttempt(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (s *Store) ListAttemptsByClassroom(ctx context.Context, examID, classroomID string) ([]model.ExamAttempt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts WHERE exam_id=$1 AND classroom_id=$2 ORDER BY created_at`, examID, classroomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamAttempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Store) ListAttemptsByStudent(ctx context.Context, userID string) ([]model.ExamAttempt, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamAttempt
	for rows.Next() {
		a, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// MonitorCounts computes the 4 live-monitor counters fresh from current DB
// state (never cached) for a classroom's roster on this exam.
type MonitorCounts struct {
	BelumMengerjakan int `json:"belum_mengerjakan"`
	SedangUjian      int `json:"sedang_ujian"` // aktif + blokir: student is still mid-attempt either way
	Selesai          int `json:"selesai"`
	DilarangUjian    int `json:"dilarang_ujian"`
	Total            int `json:"total"`
}

func (s *Store) MonitorCounts(ctx context.Context, examID, classroomID string) (*MonitorCounts, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT status, COUNT(*) FROM exam_attempts WHERE exam_id=$1 AND classroom_id=$2 GROUP BY status`, examID, classroomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := &MonitorCounts{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		switch status {
		case model.StatusBelumMulai:
			c.BelumMengerjakan += n
		case model.StatusAktif, model.StatusBlokir:
			c.SedangUjian += n
		case model.StatusSelesai:
			c.Selesai += n
		case model.StatusDilarang:
			c.DilarangUjian += n
		}
		c.Total += n
	}
	return c, rows.Err()
}

// EnterExam validates the voucher code and starts (belum_mulai->aktif) or
// resumes (aktif stays aktif) the student's attempt, building the
// per-attempt question_order the first time it starts.
func (s *Store) EnterExam(ctx context.Context, examID, userID, code string) (*model.ExamAttempt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var tokenID string
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT id, expires_at FROM exam_tokens WHERE exam_id=$1 AND user_id=$2 AND code=$3`, examID, userID, code).
		Scan(&tokenID, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrTokenInvalid
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(expiresAt) {
		return nil, ErrTokenInvalid
	}

	row := tx.QueryRowContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts WHERE exam_id=$1 AND user_id=$2 FOR UPDATE`, examID, userID)
	a, err := scanAttempt(row)
	if err == sql.ErrNoRows {
		return nil, ErrTokenInvalid // no attempt row -> voucher was never properly issued
	}
	if err != nil {
		return nil, err
	}

	switch a.Status {
	case model.StatusAktif:
		// already in progress: resume as-is
	case model.StatusBelumMulai:
		isShuffled, examCols, err := s.examShuffleAndQuestionIDs(ctx, tx, examID)
		if err != nil {
			return nil, err
		}
		if len(examCols) == 0 {
			return nil, fmt.Errorf("exam has no questions to attempt")
		}
		order := make([]string, len(examCols))
		copy(order, examCols)
		if isShuffled {
			rand.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		}
		qJSON, _ := json.Marshal(order)
		now := time.Now()
		_, err = tx.ExecContext(ctx, `
			UPDATE exam_attempts SET status='aktif', started_at=$1, question_order=$2, current_position=0, updated_at=now()
			WHERE id=$3`, now, qJSON, a.ID)
		if err != nil {
			return nil, err
		}
		a.Status = model.StatusAktif
		a.StartedAt = &now
		a.QuestionOrder = order
		a.CurrentPosition = 0
	default:
		return nil, ErrInvalidTransition
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) examShuffleAndQuestionIDs(ctx context.Context, tx *sql.Tx, examID string) (bool, []string, error) {
	var shuffled bool
	if err := tx.QueryRowContext(ctx, `SELECT is_shuffled FROM exams WHERE id=$1`, examID).Scan(&shuffled); err != nil {
		return false, nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM exam_questions WHERE exam_id=$1 ORDER BY order_index`, examID)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return false, nil, err
		}
		ids = append(ids, id)
	}
	return shuffled, ids, rows.Err()
}

// SubmitAnswer records/updates a student's answer for one exam_question,
// auto-grades it where applicable, and advances current_position.
func (s *Store) SubmitAnswer(ctx context.Context, attemptID, examQuestionID string, selectedOptionID *string, essayAnswer *string) (*model.ExamAttemptAnswer, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status string
	var questionOrderJSON []byte
	var currentPosition int
	err = tx.QueryRowContext(ctx, `SELECT status, question_order, current_position FROM exam_attempts WHERE id=$1 FOR UPDATE`, attemptID).
		Scan(&status, &questionOrderJSON, &currentPosition)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("attempt not found")
	}
	if err != nil {
		return nil, err
	}
	if status != model.StatusAktif {
		return nil, ErrInvalidTransition
	}

	var questionType string
	var questionID string
	err = tx.QueryRowContext(ctx, `SELECT q.question_type, q.id FROM exam_questions eq JOIN questions q ON q.id=eq.question_id WHERE eq.id=$1`, examQuestionID).
		Scan(&questionType, &questionID)
	if err != nil {
		return nil, fmt.Errorf("question not found in exam: %w", err)
	}

	var pointsPossible float64
	if err := tx.QueryRowContext(ctx, `SELECT points FROM exam_questions WHERE id=$1`, examQuestionID).Scan(&pointsPossible); err != nil {
		return nil, err
	}

	ans := &model.ExamAttemptAnswer{
		AttemptID:      attemptID,
		ExamQuestionID: examQuestionID,
		SelectedOptionID: selectedOptionID,
		EssayAnswer:    essayAnswer,
	}

	switch questionType {
	case "pilihan_ganda", "benar_salah":
		if selectedOptionID == nil {
			return nil, fmt.Errorf("selected_option_id is required for %s", questionType)
		}
		var isCorrect bool
		if err := tx.QueryRowContext(ctx, `SELECT is_correct FROM answer_options WHERE id=$1 AND question_id=$2`, *selectedOptionID, questionID).Scan(&isCorrect); err != nil {
			return nil, fmt.Errorf("selected option not found: %w", err)
		}
		ans.IsCorrect = &isCorrect
		pts := 0.0
		if isCorrect {
			pts = pointsPossible
		}
		ans.PointsAwarded = &pts
	case "essay", "tebak_gambar":
		if essayAnswer == nil || strings.TrimSpace(*essayAnswer) == "" {
			return nil, fmt.Errorf("essay_answer is required for %s", questionType)
		}
		matched, err := s.matchesEssayOption(ctx, tx, questionID, *essayAnswer)
		if err != nil {
			return nil, err
		}
		ans.IsCorrect = &matched
		pts := 0.0
		if matched {
			pts = pointsPossible
		}
		ans.PointsAwarded = &pts
	default:
		return nil, fmt.Errorf("unknown question_type %q", questionType)
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO exam_attempt_answers (attempt_id, exam_question_id, selected_option_id, essay_answer, is_correct, points_awarded, answered_at)
		VALUES ($1,$2,$3,$4,$5,$6, now())
		ON CONFLICT (attempt_id, exam_question_id) DO UPDATE SET
			selected_option_id=EXCLUDED.selected_option_id, essay_answer=EXCLUDED.essay_answer,
			is_correct=EXCLUDED.is_correct, points_awarded=EXCLUDED.points_awarded,
			manually_graded=false, answered_at=now(), updated_at=now()
		RETURNING id, manually_graded, answered_at, created_at, updated_at`,
		ans.AttemptID, ans.ExamQuestionID, ans.SelectedOptionID, ans.EssayAnswer, ans.IsCorrect, ans.PointsAwarded,
	).Scan(&ans.ID, &ans.ManuallyGraded, &ans.AnsweredAt, &ans.CreatedAt, &ans.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// advance the resume pointer to just past this question's position
	var order []string
	_ = json.Unmarshal(questionOrderJSON, &order)
	pos := indexOf(order, examQuestionID)
	if pos >= 0 && pos+1 > currentPosition {
		if _, err := tx.ExecContext(ctx, `UPDATE exam_attempts SET current_position=$1, updated_at=now() WHERE id=$2`, pos+1, attemptID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return ans, nil
}

func (s *Store) matchesEssayOption(ctx context.Context, tx *sql.Tx, questionID, answer string) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT answer_text FROM essay_answer_options WHERE question_id=$1`, questionID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	norm := strings.ToLower(strings.TrimSpace(answer))
	for rows.Next() {
		var opt string
		if err := rows.Scan(&opt); err != nil {
			return false, err
		}
		if strings.ToLower(strings.TrimSpace(opt)) == norm {
			return true, nil
		}
	}
	return false, rows.Err()
}

func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

// ListAnswers returns every recorded answer for an attempt.
func (s *Store) ListAnswers(ctx context.Context, attemptID string) ([]model.ExamAttemptAnswer, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, attempt_id, exam_question_id, selected_option_id, essay_answer, is_correct, points_awarded,
			manually_graded, answered_at, created_at, updated_at
		FROM exam_attempt_answers WHERE attempt_id=$1`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.ExamAttemptAnswer
	for rows.Next() {
		var a model.ExamAttemptAnswer
		if err := rows.Scan(&a.ID, &a.AttemptID, &a.ExamQuestionID, &a.SelectedOptionID, &a.EssayAnswer, &a.IsCorrect, &a.PointsAwarded,
			&a.ManuallyGraded, &a.AnsweredAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// OverrideAnswerCorrectness lets a guru flip an essay answer's correctness
// after the fact (essay only; tebak_gambar is intentionally excluded by the
// handler layer per spec).
// OverrideAnswerCorrectness applies a guru's manual essay-correctness
// override and recomputes the attempt's score/correct_count in the same
// transaction — otherwise the attempt's cached totals (set once at submit
// time in SubmitExam) go stale and the override never visibly moves the
// score shown on the monitor or the student's own result. Only submitted
// ('selesai') attempts carry a meaningful score, so an override on an
// in-progress attempt updates the answer but doesn't touch the (not yet
// computed) attempt totals.
func (s *Store) OverrideAnswerCorrectness(ctx context.Context, answerID, attemptID string, isCorrect bool, points float64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE exam_attempt_answers SET is_correct=$1, points_awarded=$2, manually_graded=true, updated_at=now()
		WHERE id=$3 AND attempt_id=$4`, isCorrect, points, answerID, attemptID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("answer not found")
	}

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM exam_attempts WHERE id=$1`, attemptID).Scan(&status); err != nil {
		return err
	}
	if status == model.StatusSelesai {
		var totalPoints sql.NullFloat64
		var correctCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(points_awarded), 0), COUNT(*) FILTER (WHERE is_correct = true)
			FROM exam_attempt_answers WHERE attempt_id=$1`, attemptID).Scan(&totalPoints, &correctCount); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE exam_attempts SET score=$1, correct_count=$2, updated_at=now() WHERE id=$3`,
			totalPoints.Float64, correctCount, attemptID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// FocusLost is the student-facing anti-cheat event: aktif -> blokir.
func (s *Store) FocusLost(ctx context.Context, examID, userID string) (*model.ExamAttempt, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exam_attempts SET status='blokir', blocked_at=now(), blocked_reason='Terdeteksi keluar dari halaman ujian (focus lost)',
			blocked_by=NULL, focus_lost_count=focus_lost_count+1, updated_at=now()
		WHERE exam_id=$1 AND user_id=$2 AND status='aktif'`, examID, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrInvalidTransition
	}
	return s.GetAttempt(ctx, examID, userID)
}

// Blokir: admin/guru manually kicks a student mid-exam (aktif -> blokir).
func (s *Store) Blokir(ctx context.Context, attemptID, byUserID string, reason *string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exam_attempts SET status='blokir', blocked_at=now(), blocked_reason=$1, blocked_by=$2, updated_at=now()
		WHERE id=$3 AND status='aktif'`, reason, byUserID, attemptID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// BukaBlokir ("Ulangi"): blokir -> aktif, resuming at current_position
// without touching question_order or any saved answers.
func (s *Store) BukaBlokir(ctx context.Context, attemptID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exam_attempts SET status='aktif', blocked_at=NULL, blocked_reason=NULL, blocked_by=NULL, updated_at=now()
		WHERE id=$1 AND status='blokir'`, attemptID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// LarangUjian (admin only, belum_mulai -> dilarang). Alasan is required.
func (s *Store) LarangUjian(ctx context.Context, attemptID, byUserID, reason string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exam_attempts SET status='dilarang', banned_at=now(), banned_reason=$1, banned_by=$2, updated_at=now()
		WHERE id=$3 AND status='belum_mulai'`, reason, byUserID, attemptID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// BukaLarangan reverses dilarang -> belum_mulai; the student's existing
// voucher is re-enabled automatically (no regeneration needed) since the
// token row was never touched.
func (s *Store) BukaLarangan(ctx context.Context, attemptID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE exam_attempts SET status='belum_mulai', banned_at=NULL, banned_reason=NULL, banned_by=NULL, updated_at=now()
		WHERE id=$1 AND status='dilarang'`, attemptID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// SubmitExam finalizes an attempt (aktif -> selesai) and computes score/correct_count.
func (s *Store) SubmitExam(ctx context.Context, attemptID string) (*model.ExamAttempt, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM exam_attempts WHERE id=$1 FOR UPDATE`, attemptID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("attempt not found")
		}
		return nil, err
	}
	if status != model.StatusAktif {
		return nil, ErrInvalidTransition
	}

	var totalPoints sql.NullFloat64
	var correctCount int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(points_awarded), 0), COUNT(*) FILTER (WHERE is_correct = true)
		FROM exam_attempt_answers WHERE attempt_id=$1`, attemptID).Scan(&totalPoints, &correctCount)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE exam_attempts SET status='selesai', submitted_at=now(), graded_at=now(),
			score=$1, correct_count=$2, updated_at=now()
		WHERE id=$3`, totalPoints.Float64, correctCount, attemptID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	row := s.DB.QueryRowContext(ctx, `SELECT `+attemptCols+` FROM exam_attempts WHERE id=$1`, attemptID)
	return scanAttempt(row)
}

// Scores is the report-service-facing view: final scores per student for an exam.
type ScoreRow struct {
	UserID       string   `json:"user_id"`
	Status       string   `json:"status"`
	Score        *float64 `json:"score"`
	CorrectCount *int     `json:"correct_count"`
}

func (s *Store) ExamScores(ctx context.Context, examID string) ([]ScoreRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT user_id, status, score, correct_count FROM exam_attempts WHERE exam_id=$1 ORDER BY created_at`, examID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScoreRow
	for rows.Next() {
		var r ScoreRow
		if err := rows.Scan(&r.UserID, &r.Status, &r.Score, &r.CorrectCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
