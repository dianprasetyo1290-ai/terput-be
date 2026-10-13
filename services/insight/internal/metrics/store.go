package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ListDefinitions(ctx context.Context, status string) ([]MetricDef, error) {
	query := `SELECT key, name, description, formula, unit, owner_service, data_owner, source_metric,
	                 allowed_group_by, additive, refresh_minutes, limitations, version, status
	          FROM metric_definitions`
	var rows [][]interface{}
	var err error

	if status != "" {
		query += " WHERE status = $1 ORDER BY key ASC"
		rowsI, errQ := s.pool.Query(ctx, query, status)
		if errQ != nil {
			return nil, errQ
		}
		defer rowsI.Close()
		for rowsI.Next() {
			var d MetricDef
			if err := rowsI.Scan(&d.Key, &d.Name, &d.Description, &d.Formula, &d.Unit, &d.OwnerService, &d.DataOwner, &d.SourceMetric,
				&d.AllowedGroupBy, &d.Additive, &d.RefreshMinutes, &d.Limitations, &d.Version, &d.Status); err != nil {
				return nil, err
			}
			rows = append(rows, []interface{}{d})
		}
	} else {
		query += " ORDER BY key ASC"
		rowsI, errQ := s.pool.Query(ctx, query)
		if errQ != nil {
			return nil, errQ
		}
		defer rowsI.Close()
		for rowsI.Next() {
			var d MetricDef
			if err := rowsI.Scan(&d.Key, &d.Name, &d.Description, &d.Formula, &d.Unit, &d.OwnerService, &d.DataOwner, &d.SourceMetric,
				&d.AllowedGroupBy, &d.Additive, &d.RefreshMinutes, &d.Limitations, &d.Version, &d.Status); err != nil {
				return nil, err
			}
			rows = append(rows, []interface{}{d})
		}
	}

	result := make([]MetricDef, len(rows))
	for i, r := range rows {
		result[i] = r[0].(MetricDef)
	}
	return result, err
}

func (s *Store) GetDefinition(ctx context.Context, key string) (*MetricDef, error) {
	query := `SELECT key, name, description, formula, unit, owner_service, data_owner, source_metric,
	                 allowed_group_by, additive, refresh_minutes, limitations, version, status
	          FROM metric_definitions WHERE key = $1`
	var d MetricDef
	err := s.pool.QueryRow(ctx, query, key).Scan(&d.Key, &d.Name, &d.Description, &d.Formula, &d.Unit, &d.OwnerService, &d.DataOwner, &d.SourceMetric,
		&d.AllowedGroupBy, &d.Additive, &d.RefreshMinutes, &d.Limitations, &d.Version, &d.Status)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) UpdateDescription(ctx context.Context, key string, description string, limitations *string) error {
	query := `UPDATE metric_definitions SET description = $2, limitations = $3 WHERE key = $1`
	cmd, err := s.pool.Exec(ctx, query, key, description, limitations)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("metric not found: %s", key)
	}
	return nil
}

func (s *Store) UpsertSchoolParams(ctx context.Context, schoolID, metricKey string, params map[string]interface{}, effectiveFrom string) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	query := `INSERT INTO metric_school_params (school_id, metric_key, params, effective_from, updated_at)
	          VALUES ($1, $2, $3, $4, NOW())
	          ON CONFLICT (school_id, metric_key, effective_from)
	          DO UPDATE SET params = EXCLUDED.params, updated_at = NOW()`
	_, err = s.pool.Exec(ctx, query, schoolID, metricKey, paramsJSON, effectiveFrom)
	return err
}

func (s *Store) GetSchoolParams(ctx context.Context, schoolID, metricKey string) (map[string]interface{}, error) {
	query := `SELECT params FROM metric_school_params
	          WHERE school_id = $1 AND metric_key = $2 AND effective_from <= CURRENT_DATE
	            AND (effective_to IS NULL OR effective_to >= CURRENT_DATE)
	          ORDER BY effective_from DESC LIMIT 1`
	var raw []byte
	err := s.pool.QueryRow(ctx, query, schoolID, metricKey).Scan(&raw)
	if err != nil {
		return map[string]interface{}{}, nil
	}
	var res map[string]interface{}
	_ = json.Unmarshal(raw, &res)
	return res, nil
}

func (s *Store) GetSnapshot(ctx context.Context, schoolID, metricKey, periodKind, periodStart, groupBy, dimValue string) (*MetricSnapshot, error) {
	query := `SELECT school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value,
	                 numerator, denominator, value, source_as_of, computed_at
	          FROM metric_snapshots
	          WHERE school_id = $1 AND metric_key = $2 AND period_kind = $3 AND period_start = $4 AND group_by = $5 AND dim_value = $6`
	var snap MetricSnapshot
	var pStart, pEnd time.Time
	err := s.pool.QueryRow(ctx, query, schoolID, metricKey, periodKind, periodStart, groupBy, dimValue).Scan(
		&snap.SchoolID, &snap.MetricKey, &snap.PeriodKind, &pStart, &pEnd, &snap.GroupBy, &snap.DimValue,
		&snap.Numerator, &snap.Denominator, &snap.Value, &snap.SourceAsOf, &snap.ComputedAt,
	)
	if err != nil {
		return nil, err
	}
	snap.PeriodStart = pStart.Format("2006-01-02")
	snap.PeriodEnd = pEnd.Format("2006-01-02")
	return &snap, nil
}

func (s *Store) ListSnapshots(ctx context.Context, schoolIDs []string, metricKey, periodKind, periodStart, groupBy string) ([]MetricSnapshot, error) {
	query := `SELECT school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value,
	                 numerator, denominator, value, source_as_of, computed_at
	          FROM metric_snapshots
	          WHERE school_id = ANY($1) AND metric_key = $2 AND period_kind = $3 AND period_start = $4 AND group_by = $5
	          ORDER BY school_id ASC, dim_value ASC`
	rows, err := s.pool.Query(ctx, query, schoolIDs, metricKey, periodKind, periodStart, groupBy)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []MetricSnapshot
	for rows.Next() {
		var snap MetricSnapshot
		var pStart, pEnd time.Time
		if err := rows.Scan(
			&snap.SchoolID, &snap.MetricKey, &snap.PeriodKind, &pStart, &pEnd, &snap.GroupBy, &snap.DimValue,
			&snap.Numerator, &snap.Denominator, &snap.Value, &snap.SourceAsOf, &snap.ComputedAt,
		); err != nil {
			return nil, err
		}
		snap.PeriodStart = pStart.Format("2006-01-02")
		snap.PeriodEnd = pEnd.Format("2006-01-02")
		res = append(res, snap)
	}
	return res, nil
}

func (s *Store) UpsertSnapshot(ctx context.Context, snap MetricSnapshot) error {
	query := `INSERT INTO metric_snapshots
	          (school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value, numerator, denominator, value, source_as_of, computed_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
	          ON CONFLICT (school_id, metric_key, period_kind, period_start, group_by, dim_value)
	          DO UPDATE SET period_end = EXCLUDED.period_end, numerator = EXCLUDED.numerator,
	                        denominator = EXCLUDED.denominator, value = EXCLUDED.value,
	                        source_as_of = EXCLUDED.source_as_of, computed_at = NOW()`
	_, err := s.pool.Exec(ctx, query, snap.SchoolID, snap.MetricKey, snap.PeriodKind, snap.PeriodStart, snap.PeriodEnd,
		snap.GroupBy, snap.DimValue, snap.Numerator, snap.Denominator, snap.Value, snap.SourceAsOf)
	return err
}

func (s *Store) RecordRefreshRun(ctx context.Context, schoolID, metricKey, periodStart, status string, errStr *string, startedAt, finishedAt time.Time) error {
	query := `INSERT INTO metric_refresh_runs (school_id, metric_key, period_start, status, error, started_at, finished_at)
	          VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := s.pool.Exec(ctx, query, schoolID, metricKey, periodStart, status, errStr, startedAt, finishedAt)
	return err
}
