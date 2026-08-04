package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	"skci/evaluation/internal/model"
)

type FormStore struct{ db *pgxpool.Pool }

func NewFormStore(db *pgxpool.Pool) *FormStore { return &FormStore{db: db} }

// BootstrapDefaults idempotently creates the 6 default evaluator-category
// forms + their scoring items for a school, if not already present.
func (s *FormStore) BootstrapDefaults(ctx context.Context, schoolID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, f := range DefaultForms {
		var formID string
		err := tx.QueryRow(ctx, `
			INSERT INTO teacher_evaluation_forms (school_id, code, title, evaluator_type, priority, weight)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (school_id, evaluator_type) DO UPDATE SET title = EXCLUDED.title
			RETURNING id`, schoolID, f.Code, f.Title, f.EvaluatorType, f.Priority, f.Weight).Scan(&formID)
		if err != nil {
			return err
		}
		var keepCodes []string
		for i, it := range f.Items {
			labels, _ := json.Marshal(it.ScoreLabels)
			if _, err := tx.Exec(ctx, `
				INSERT INTO teacher_evaluation_items (form_id, school_id, code, indicator, indicator_en, weight, max_score, score_labels, sort_order)
				VALUES ($1,$2,$3,$4,$5,$6,4,$7,$8)
				ON CONFLICT (form_id, code) DO UPDATE SET
					indicator = EXCLUDED.indicator,
					indicator_en = EXCLUDED.indicator_en,
					weight = EXCLUDED.weight,
					score_labels = EXCLUDED.score_labels,
					sort_order = EXCLUDED.sort_order`,
				formID, schoolID, it.Code, it.Indicator, it.IndicatorEn, it.Weight, labels, i+1); err != nil {
				return err
			}
			keepCodes = append(keepCodes, it.Code)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM teacher_evaluation_items WHERE form_id = $1 AND code != ALL($2)`, formID, keepCodes); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *FormStore) ListForms(ctx context.Context, schoolID string) ([]*model.EvalForm, error) {
	rows, err := s.db.Query(ctx, `SELECT id, school_id, code, title, evaluator_type, priority, weight, created_at, updated_at
		FROM teacher_evaluation_forms WHERE school_id=$1 ORDER BY priority`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.EvalForm
	for rows.Next() {
		var f model.EvalForm
		if err := rows.Scan(&f.ID, &f.SchoolID, &f.Code, &f.Title, &f.EvaluatorType, &f.Priority, &f.Weight, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

func (s *FormStore) GetFormByEvaluatorType(ctx context.Context, schoolID, evaluatorType string) (*model.EvalForm, error) {
	var f model.EvalForm
	err := s.db.QueryRow(ctx, `SELECT id, school_id, code, title, evaluator_type, priority, weight, created_at, updated_at
		FROM teacher_evaluation_forms WHERE school_id=$1 AND evaluator_type=$2`, schoolID, evaluatorType).
		Scan(&f.ID, &f.SchoolID, &f.Code, &f.Title, &f.EvaluatorType, &f.Priority, &f.Weight, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *FormStore) ListItems(ctx context.Context, formID string) ([]*model.EvalItem, error) {
	rows, err := s.db.Query(ctx, `SELECT id, form_id, school_id, code, indicator, COALESCE(indicator_en, ''), weight, max_score, score_labels, sort_order
		FROM teacher_evaluation_items WHERE form_id=$1 ORDER BY sort_order`, formID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.EvalItem
	for rows.Next() {
		var it model.EvalItem
		var labels []byte
		if err := rows.Scan(&it.ID, &it.FormID, &it.SchoolID, &it.Code, &it.Indicator, &it.IndicatorEn, &it.Weight, &it.MaxScore, &labels, &it.SortOrder); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(labels, &it.ScoreLabels)
		out = append(out, &it)
	}
	return out, rows.Err()
}
