package predict

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Settings
func (s *Store) GetSettings(ctx context.Context, schoolID string) (*InsightSettings, error) {
	query := `SELECT school_id, predictive_enabled, enabled_by, enabled_at, review_note, created_at, updated_at
	          FROM insight_settings WHERE school_id = $1`
	var st InsightSettings
	err := s.pool.QueryRow(ctx, query, schoolID).
		Scan(&st.SchoolID, &st.PredictiveEnabled, &st.EnabledBy, &st.EnabledAt, &st.ReviewNote, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return &InsightSettings{SchoolID: schoolID, PredictiveEnabled: false}, nil
	}
	return &st, nil
}

func (s *Store) UpdateSettings(ctx context.Context, schoolID string, enabled bool, enabledBy *string, reviewNote *string) error {
	var enabledAt *time.Time
	if enabled {
		now := time.Now()
		enabledAt = &now
	}
	query := `INSERT INTO insight_settings (school_id, predictive_enabled, enabled_by, enabled_at, review_note, updated_at)
	          VALUES ($1, $2, $3, $4, $5, NOW())
	          ON CONFLICT (school_id)
	          DO UPDATE SET predictive_enabled = EXCLUDED.predictive_enabled,
	                        enabled_by = EXCLUDED.enabled_by,
	                        enabled_at = EXCLUDED.enabled_at,
	                        review_note = EXCLUDED.review_note,
	                        updated_at = NOW()`
	_, err := s.pool.Exec(ctx, query, schoolID, enabled, enabledBy, enabledAt, reviewNote)
	return err
}

// Risk Rules
func (s *Store) GetActiveRule(ctx context.Context, schoolID, model string) (*RiskRule, error) {
	query := `SELECT id, school_id, model, version, params, effective_from, created_by, status, created_at, updated_at
	          FROM risk_rules WHERE school_id = $1 AND model = $2 AND status = 'active'
	          ORDER BY version DESC LIMIT 1`
	var r RiskRule
	var pBytes []byte
	var effFrom time.Time
	err := s.pool.QueryRow(ctx, query, schoolID, model).
		Scan(&r.ID, &r.SchoolID, &r.Model, &r.Version, &pBytes, &effFrom, &r.CreatedBy, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(pBytes, &r.Params)
	r.EffectiveFrom = effFrom.Format("2006-01-02")
	return &r, nil
}

func (s *Store) ListRules(ctx context.Context, schoolID, model string) ([]RiskRule, error) {
	query := `SELECT id, school_id, model, version, params, effective_from, created_by, status, created_at, updated_at
	          FROM risk_rules WHERE school_id = $1`
	args := []interface{}{schoolID}
	if model != "" {
		args = append(args, model)
		query += " AND model = $2"
	}
	query += " ORDER BY model ASC, version DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []RiskRule
	for rows.Next() {
		var r RiskRule
		var pBytes []byte
		var effFrom time.Time
		if err := rows.Scan(&r.ID, &r.SchoolID, &r.Model, &r.Version, &pBytes, &effFrom, &r.CreatedBy, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(pBytes, &r.Params)
		r.EffectiveFrom = effFrom.Format("2006-01-02")
		res = append(res, r)
	}
	return res, nil
}

func (s *Store) CreateRule(ctx context.Context, r RiskRule) (*RiskRule, error) {
	// Find current max version for model in school
	var nextVer int = 1
	var maxVer *int
	_ = s.pool.QueryRow(ctx, "SELECT MAX(version) FROM risk_rules WHERE school_id = $1 AND model = $2", r.SchoolID, r.Model).Scan(&maxVer)
	if maxVer != nil {
		nextVer = *maxVer + 1
	}
	r.Version = nextVer

	// Retire older active rules for this model
	_, _ = s.pool.Exec(ctx, "UPDATE risk_rules SET status = 'retired', updated_at = NOW() WHERE school_id = $1 AND model = $2 AND status = 'active'", r.SchoolID, r.Model)

	pBytes, _ := json.Marshal(r.Params)
	query := `INSERT INTO risk_rules (school_id, model, version, params, effective_from, created_by, status)
	          VALUES ($1, $2, $3, $4, $5, $6, 'active')
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, r.SchoolID, r.Model, r.Version, pBytes, r.EffectiveFrom, r.CreatedBy).
		Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.Status = "active"
	return &r, nil
}

// Prediction Runs & Predictions
func (s *Store) CreatePredictionRun(ctx context.Context, schoolID, model, ruleID, status string, reason *string) (*PredictionRun, error) {
	query := `INSERT INTO prediction_runs (school_id, model, rule_id, status, reason)
	          VALUES ($1, $2, $3, $4, $5)
	          RETURNING id, run_at, created_at`
	var run PredictionRun
	run.SchoolID = schoolID
	run.Model = model
	run.RuleID = ruleID
	run.Status = status
	run.Reason = reason
	err := s.pool.QueryRow(ctx, query, schoolID, model, ruleID, status, reason).
		Scan(&run.ID, &run.RunAt, &run.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *Store) SavePredictions(ctx context.Context, preds []Prediction) error {
	for _, p := range preds {
		fBytes, _ := json.Marshal(p.Factors)
		q := `INSERT INTO predictions (school_id, run_id, model, subject_kind, subject_id, score, level, projected_value, factors, review_label)
		      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
		_, err := s.pool.Exec(ctx, q, p.SchoolID, p.RunID, p.Model, p.SubjectKind, p.SubjectID, p.Score, p.Level, p.ProjectedValue, fBytes, p.ReviewLabel)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListLatestPredictions(ctx context.Context, schoolID, model, level string) ([]Prediction, error) {
	query := `SELECT DISTINCT ON (p.subject_id)
	                 p.id, p.school_id, p.run_id, p.model, p.subject_kind, p.subject_id, p.score, p.level, p.projected_value,
	                 p.factors, p.review_label, p.created_at
	          FROM predictions p
	          WHERE p.school_id = $1 AND p.model = $2`
	args := []interface{}{schoolID, model}
	if level != "" {
		args = append(args, level)
		query += " AND p.level = $3"
	}
	query += " ORDER BY p.subject_id, p.created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Prediction
	for rows.Next() {
		var p Prediction
		var fBytes []byte
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.RunID, &p.Model, &p.SubjectKind, &p.SubjectID, &p.Score, &p.Level, &p.ProjectedValue,
			&fBytes, &p.ReviewLabel, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Factors = decodeFactors(fBytes)
		res = append(res, p)
	}
	return res, nil
}

func (s *Store) AddReview(ctx context.Context, schoolID, predictionID, reviewedBy, note string) error {
	query := `INSERT INTO prediction_reviews (school_id, prediction_id, reviewed_by, note)
	          VALUES ($1, $2, $3, $4)`
	_, err := s.pool.Exec(ctx, query, schoolID, predictionID, reviewedBy, note)
	return err
}

func (s *Store) ListReviews(ctx context.Context, predictionID string) ([]PredictionReview, error) {
	query := `SELECT id, school_id, prediction_id, reviewed_by, note, created_at
	          FROM prediction_reviews WHERE prediction_id = $1 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, predictionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []PredictionReview
	for rows.Next() {
		var pr PredictionReview
		if err := rows.Scan(&pr.ID, &pr.SchoolID, &pr.PredictionID, &pr.ReviewedBy, &pr.Note, &pr.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, pr)
	}
	return res, nil
}

func (s *Store) ListAccuracy(ctx context.Context, schoolID, model string) ([]PredictionAccuracy, error) {
	query := `SELECT id, school_id, model, rule_id, period_start, period_end, measure, value_bp, sample_size, computed_at
	          FROM prediction_accuracy WHERE school_id = $1`
	args := []interface{}{schoolID}
	if model != "" {
		args = append(args, model)
		query += " AND model = $2"
	}
	query += " ORDER BY computed_at DESC LIMIT 20"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []PredictionAccuracy
	for rows.Next() {
		var a PredictionAccuracy
		var pStart, pEnd time.Time
		if err := rows.Scan(&a.ID, &a.SchoolID, &a.Model, &a.RuleID, &pStart, &pEnd, &a.Measure, &a.ValueBP, &a.SampleSize, &a.ComputedAt); err != nil {
			return nil, err
		}
		a.PeriodStart = pStart.Format("2006-01-02")
		a.PeriodEnd = pEnd.Format("2006-01-02")
		res = append(res, a)
	}
	return res, nil
}

// decodeFactors reads the predictions.factors JSONB column. Besides the
// canonical RiskFactor shape it accepts the legacy {factor, weight, note}
// objects that older seed data stored, so their labels are not lost.
func decodeFactors(b []byte) []RiskFactor {
	var raw []map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return []RiskFactor{}
	}
	str := func(m map[string]any, keys ...string) string {
		for _, k := range keys {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	num := func(m map[string]any, keys ...string) float64 {
		for _, k := range keys {
			if v, ok := m[k].(float64); ok {
				return v
			}
		}
		return 0
	}
	out := make([]RiskFactor, 0, len(raw))
	for _, m := range raw {
		out = append(out, RiskFactor{
			Key:          str(m, "key"),
			Label:        str(m, "label", "factor"),
			RawValue:     num(m, "raw_value"),
			Contribution: num(m, "contribution", "weight"),
			Note:         str(m, "note"),
		})
	}
	return out
}
