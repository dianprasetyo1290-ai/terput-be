package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"skci/evaluation/internal/model"
)

type TaskStore struct{ db *pgxpool.Pool }

func NewTaskStore(db *pgxpool.Pool) *TaskStore { return &TaskStore{db: db} }

const taskCols = `id, period_id, form_id, school_id, academic_year_id, target_teacher_user_id,
	evaluator_user_id, evaluator_type, classroom_id, status, submitted_at, raw_score, created_at, updated_at`

func scanTask(row interface {
	Scan(dest ...interface{}) error
}) (*model.EvalTask, error) {
	var t model.EvalTask
	if err := row.Scan(&t.ID, &t.PeriodID, &t.FormID, &t.SchoolID, &t.AcademicYearID, &t.TargetTeacherUserID,
		&t.EvaluatorUserID, &t.EvaluatorType, &t.ClassroomID, &t.Status, &t.SubmittedAt, &t.RawScore, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// UpsertTask creates a pending task for (period, form, target, evaluator[, classroom])
// if it doesn't already exist. Idempotent -- safe to call repeatedly when a
// period's config is edited (tasks already submitted are left untouched).
func (s *TaskStore) UpsertTask(ctx context.Context, t *model.EvalTask) (*model.EvalTask, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO teacher_evaluation_tasks
			(period_id, form_id, school_id, academic_year_id, target_teacher_user_id, evaluator_user_id, evaluator_type, classroom_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (period_id, form_id, target_teacher_user_id, evaluator_user_id, COALESCE(classroom_id, '00000000-0000-0000-0000-000000000000'::uuid))
		DO UPDATE SET updated_at = teacher_evaluation_tasks.updated_at
		RETURNING `+taskCols,
		t.PeriodID, t.FormID, t.SchoolID, t.AcademicYearID, t.TargetTeacherUserID, t.EvaluatorUserID, t.EvaluatorType, t.ClassroomID)
	return scanTask(row)
}

// FindTask looks up an existing task by its natural key.
func (s *TaskStore) FindTask(ctx context.Context, periodID, formID, targetID, evaluatorID string, classroomID *string) (*model.EvalTask, error) {
	row := s.db.QueryRow(ctx, `
		SELECT `+taskCols+` FROM teacher_evaluation_tasks
		WHERE period_id=$1 AND form_id=$2 AND target_teacher_user_id=$3 AND evaluator_user_id=$4
		  AND COALESCE(classroom_id, '00000000-0000-0000-0000-000000000000'::uuid) = COALESCE($5, '00000000-0000-0000-0000-000000000000'::uuid)`,
		periodID, formID, targetID, evaluatorID, classroomID)
	return scanTask(row)
}

func (s *TaskStore) GetByID(ctx context.Context, id, schoolID string) (*model.EvalTask, error) {
	row := s.db.QueryRow(ctx, `SELECT `+taskCols+` FROM teacher_evaluation_tasks WHERE id=$1 AND school_id=$2`, id, schoolID)
	return scanTask(row)
}

// SubmitAnswers writes item scores for a task, computes the task's
// weight-normalized raw_score, and marks it submitted. Re-submitting
// overwrites previous answers (upsert), consistent with "guru can fix a
// mistaken answer before period ends" (not explicit in spec, simplest call).
func (s *TaskStore) SubmitAnswers(ctx context.Context, taskID, schoolID string, answers map[string]int, notes map[string]string, itemWeights map[string]float64) (float64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var weightedSum, weightTotal float64
	for itemID, score := range answers {
		_, err := tx.Exec(ctx, `
			INSERT INTO teacher_evaluation_answers (task_id, item_id, school_id, score, notes)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (task_id, item_id) DO UPDATE SET score=EXCLUDED.score, notes=EXCLUDED.notes, updated_at=now()`,
			taskID, itemID, schoolID, score, notes[itemID])
		if err != nil {
			return 0, err
		}
		w := itemWeights[itemID]
		weightedSum += w * float64(score)
		weightTotal += w
	}
	rawScore := 0.0
	if weightTotal > 0 {
		rawScore = weightedSum / weightTotal
	}
	if _, err := tx.Exec(ctx, `UPDATE teacher_evaluation_tasks SET status='submitted', submitted_at=now(), raw_score=$1, updated_at=now() WHERE id=$2`, rawScore, taskID); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return rawScore, nil
}

// ListForPeriodTarget returns all tasks for a given guru within a period (for results/report).
func (s *TaskStore) ListForPeriodTarget(ctx context.Context, periodID, targetTeacherID string) ([]*model.EvalTask, error) {
	rows, err := s.db.Query(ctx, `SELECT `+taskCols+` FROM teacher_evaluation_tasks WHERE period_id=$1 AND target_teacher_user_id=$2`, periodID, targetTeacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.EvalTask
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListForEvaluator returns an evaluator's assigned tasks within a period (their to-do list).
func (s *TaskStore) ListForEvaluator(ctx context.Context, periodID, evaluatorUserID string) ([]*model.EvalTask, error) {
	rows, err := s.db.Query(ctx, `SELECT `+taskCols+` FROM teacher_evaluation_tasks WHERE period_id=$1 AND evaluator_user_id=$2`, periodID, evaluatorUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.EvalTask
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
