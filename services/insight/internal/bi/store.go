package bi

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

// Dashboards
func (s *Store) CreateDashboard(ctx context.Context, d Dashboard) (*Dashboard, error) {
	query := `INSERT INTO dashboards (school_id, owner_user_id, title, scope_kind, shared_roles)
	          VALUES ($1, $2, $3, $4, $5)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, d.SchoolID, d.OwnerUserID, d.Title, d.ScopeKind, d.SharedRoles).
		Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetDashboard(ctx context.Context, id, schoolID string) (*Dashboard, error) {
	query := `SELECT id, school_id, owner_user_id, title, scope_kind, shared_roles, created_at, updated_at
	          FROM dashboards WHERE id = $1 AND (school_id = $2 OR scope_kind = 'foundation') AND deleted_at IS NULL`
	var d Dashboard
	err := s.pool.QueryRow(ctx, query, id, schoolID).
		Scan(&d.ID, &d.SchoolID, &d.OwnerUserID, &d.Title, &d.ScopeKind, &d.SharedRoles, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}

	wQuery := `SELECT id, school_id, dashboard_id, sort, title, metric_key, chart, group_by, period, created_at, updated_at
	           FROM dashboard_widgets WHERE dashboard_id = $1 ORDER BY sort ASC`
	rows, err := s.pool.Query(ctx, wQuery, d.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var w DashboardWidget
		var periodBytes []byte
		if err := rows.Scan(&w.ID, &w.SchoolID, &w.DashboardID, &w.Sort, &w.Title, &w.MetricKey, &w.Chart, &w.GroupBy, &periodBytes, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(periodBytes, &w.Period)
		d.Widgets = append(d.Widgets, w)
	}

	return &d, nil
}

func (s *Store) ListDashboards(ctx context.Context, schoolID string) ([]Dashboard, error) {
	query := `SELECT id, school_id, owner_user_id, title, scope_kind, shared_roles, created_at, updated_at
	          FROM dashboards WHERE (school_id = $1 OR scope_kind = 'foundation') AND deleted_at IS NULL ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Dashboard
	for rows.Next() {
		var d Dashboard
		if err := rows.Scan(&d.ID, &d.SchoolID, &d.OwnerUserID, &d.Title, &d.ScopeKind, &d.SharedRoles, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, d)
	}
	return res, nil
}

func (s *Store) UpdateDashboard(ctx context.Context, d Dashboard) error {
	query := `UPDATE dashboards SET title = $2, scope_kind = $3, shared_roles = $4, updated_at = NOW()
	          WHERE id = $1 AND deleted_at IS NULL`
	cmd, err := s.pool.Exec(ctx, query, d.ID, d.Title, d.ScopeKind, d.SharedRoles)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("dashboard not found")
	}
	return nil
}

func (s *Store) DeleteDashboard(ctx context.Context, id, schoolID string) error {
	query := `UPDATE dashboards SET deleted_at = NOW() WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`
	_, err := s.pool.Exec(ctx, query, id, schoolID)
	return err
}

func (s *Store) ReplaceWidgets(ctx context.Context, dashboardID string, widgets []DashboardWidget) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "DELETE FROM dashboard_widgets WHERE dashboard_id = $1", dashboardID)
	if err != nil {
		return err
	}

	for i, w := range widgets {
		pBytes, _ := json.Marshal(w.Period)
		q := `INSERT INTO dashboard_widgets (school_id, dashboard_id, sort, title, metric_key, chart, group_by, period)
		      VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
		_, err = tx.Exec(ctx, q, w.SchoolID, dashboardID, i, w.Title, w.MetricKey, w.Chart, w.GroupBy, pBytes)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Saved Reports
func (s *Store) CreateReport(ctx context.Context, r SavedReport) (*SavedReport, error) {
	cols, _ := json.Marshal(r.Columns)
	filters, _ := json.Marshal(r.Filters)
	query := `INSERT INTO saved_reports (school_id, owner_user_id, title, columns, filters, shared_roles)
	          VALUES ($1, $2, $3, $4, $5, $6)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, r.SchoolID, r.OwnerUserID, r.Title, cols, filters, r.SharedRoles).
		Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListReports(ctx context.Context, schoolID string) ([]SavedReport, error) {
	query := `SELECT id, school_id, owner_user_id, title, columns, filters, shared_roles, created_at, updated_at
	          FROM saved_reports WHERE school_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []SavedReport
	for rows.Next() {
		var r SavedReport
		var colBytes, filterBytes []byte
		if err := rows.Scan(&r.ID, &r.SchoolID, &r.OwnerUserID, &r.Title, &colBytes, &filterBytes, &r.SharedRoles, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(colBytes, &r.Columns)
		_ = json.Unmarshal(filterBytes, &r.Filters)
		res = append(res, r)
	}
	return res, nil
}

func (s *Store) GetReport(ctx context.Context, id, schoolID string) (*SavedReport, error) {
	query := `SELECT id, school_id, owner_user_id, title, columns, filters, shared_roles, created_at, updated_at
	          FROM saved_reports WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`
	var r SavedReport
	var colBytes, filterBytes []byte
	err := s.pool.QueryRow(ctx, query, id, schoolID).
		Scan(&r.ID, &r.SchoolID, &r.OwnerUserID, &r.Title, &colBytes, &filterBytes, &r.SharedRoles, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(colBytes, &r.Columns)
	_ = json.Unmarshal(filterBytes, &r.Filters)
	return &r, nil
}

func (s *Store) UpdateReport(ctx context.Context, r SavedReport) error {
	cols, _ := json.Marshal(r.Columns)
	filters, _ := json.Marshal(r.Filters)
	query := `UPDATE saved_reports SET title = $2, columns = $3, filters = $4, shared_roles = $5, updated_at = NOW()
	          WHERE id = $1 AND deleted_at IS NULL`
	cmd, err := s.pool.Exec(ctx, query, r.ID, r.Title, cols, filters, r.SharedRoles)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("report not found")
	}
	return nil
}

func (s *Store) DeleteReport(ctx context.Context, id, schoolID string) error {
	query := `UPDATE saved_reports SET deleted_at = NOW() WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL`
	_, err := s.pool.Exec(ctx, query, id, schoolID)
	return err
}

// Scheduled Reports
func (s *Store) CreateSchedule(ctx context.Context, sched ScheduledReport) (*ScheduledReport, error) {
	query := `INSERT INTO scheduled_reports
	          (school_id, saved_report_id, frequency, weekday, month_day, send_hour, recipient_user_ids, active, next_run_at, created_by)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, sched.SchoolID, sched.SavedReportID, sched.Frequency, sched.Weekday, sched.MonthDay,
		sched.SendHour, sched.RecipientUserIDs, sched.Active, sched.NextRunAt, sched.CreatedBy).
		Scan(&sched.ID, &sched.CreatedAt, &sched.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sched, nil
}

func (s *Store) ListSchedules(ctx context.Context, schoolID string) ([]ScheduledReport, error) {
	query := `SELECT id, school_id, saved_report_id, frequency, weekday, month_day, send_hour, recipient_user_ids, active,
	                 next_run_at, last_run_at, created_by, created_at, updated_at
	          FROM scheduled_reports WHERE school_id = $1 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []ScheduledReport
	for rows.Next() {
		var sr ScheduledReport
		if err := rows.Scan(&sr.ID, &sr.SchoolID, &sr.SavedReportID, &sr.Frequency, &sr.Weekday, &sr.MonthDay, &sr.SendHour,
			&sr.RecipientUserIDs, &sr.Active, &sr.NextRunAt, &sr.LastRunAt, &sr.CreatedBy, &sr.CreatedAt, &sr.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, sr)
	}
	return res, nil
}

func (s *Store) GetDueSchedules(ctx context.Context, now time.Time) ([]ScheduledReport, error) {
	query := `SELECT id, school_id, saved_report_id, frequency, weekday, month_day, send_hour, recipient_user_ids, active,
	                 next_run_at, last_run_at, created_by, created_at, updated_at
	          FROM scheduled_reports WHERE active = true AND next_run_at <= $1`
	rows, err := s.pool.Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []ScheduledReport
	for rows.Next() {
		var sr ScheduledReport
		if err := rows.Scan(&sr.ID, &sr.SchoolID, &sr.SavedReportID, &sr.Frequency, &sr.Weekday, &sr.MonthDay, &sr.SendHour,
			&sr.RecipientUserIDs, &sr.Active, &sr.NextRunAt, &sr.LastRunAt, &sr.CreatedBy, &sr.CreatedAt, &sr.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, sr)
	}
	return res, nil
}

func (s *Store) UpdateScheduleNextRun(ctx context.Context, id string, lastRunAt, nextRunAt time.Time) error {
	query := `UPDATE scheduled_reports SET last_run_at = $2, next_run_at = $3, updated_at = NOW() WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id, lastRunAt, nextRunAt)
	return err
}

func (s *Store) RecordScheduleRun(ctx context.Context, run ScheduledReportRun) error {
	dBytes, _ := json.Marshal(run.Deliveries)
	query := `INSERT INTO scheduled_report_runs (school_id, schedule_id, recipient_user_id, status, reason, deliveries, idempotency_key)
	          VALUES ($1, $2, $3, $4, $5, $6, $7)
	          ON CONFLICT (school_id, idempotency_key) DO NOTHING`
	_, err := s.pool.Exec(ctx, query, run.SchoolID, run.ScheduleID, run.RecipientUserID, run.Status, run.Reason, dBytes, run.IdempotencyKey)
	return err
}

func (s *Store) ListScheduleRuns(ctx context.Context, scheduleID string) ([]ScheduledReportRun, error) {
	query := `SELECT id, school_id, schedule_id, recipient_user_id, status, reason, deliveries, idempotency_key, created_at
	          FROM scheduled_report_runs WHERE schedule_id = $1 ORDER BY created_at DESC LIMIT 50`
	rows, err := s.pool.Query(ctx, query, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []ScheduledReportRun
	for rows.Next() {
		var r ScheduledReportRun
		var dBytes []byte
		if err := rows.Scan(&r.ID, &r.SchoolID, &r.ScheduleID, &r.RecipientUserID, &r.Status, &r.Reason, &dBytes, &r.IdempotencyKey, &r.CreatedAt); err != nil {
			return nil, err
		}
		if len(dBytes) > 0 {
			_ = json.Unmarshal(dBytes, &r.Deliveries)
		}
		res = append(res, r)
	}
	return res, nil
}

// Scope Grants
func (s *Store) ListScopeGrants(ctx context.Context, userID string) ([]AnalyticsScopeGrant, error) {
	query := `SELECT id, school_id, user_id, granted_school_id, granted_by, revoked_at, created_at, updated_at
	          FROM analytics_scope_grants WHERE user_id = $1 AND revoked_at IS NULL`
	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []AnalyticsScopeGrant
	for rows.Next() {
		var g AnalyticsScopeGrant
		if err := rows.Scan(&g.ID, &g.SchoolID, &g.UserID, &g.GrantedSchoolID, &g.GrantedBy, &g.RevokedAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, g)
	}
	return res, nil
}

func (s *Store) ListSchoolScopeGrants(ctx context.Context, schoolID string) ([]AnalyticsScopeGrant, error) {
	query := `SELECT id, school_id, user_id, granted_school_id, granted_by, revoked_at, created_at, updated_at
	          FROM analytics_scope_grants WHERE school_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []AnalyticsScopeGrant
	for rows.Next() {
		var g AnalyticsScopeGrant
		if err := rows.Scan(&g.ID, &g.SchoolID, &g.UserID, &g.GrantedSchoolID, &g.GrantedBy, &g.RevokedAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, g)
	}
	return res, nil
}

func (s *Store) CreateScopeGrant(ctx context.Context, grant AnalyticsScopeGrant) (*AnalyticsScopeGrant, error) {
	query := `INSERT INTO analytics_scope_grants (school_id, user_id, granted_school_id, granted_by)
	          VALUES ($1, $2, $3, $4)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, grant.SchoolID, grant.UserID, grant.GrantedSchoolID, grant.GrantedBy).
		Scan(&grant.ID, &grant.CreatedAt, &grant.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &grant, nil
}

func (s *Store) RevokeScopeGrant(ctx context.Context, id, schoolID string) error {
	query := `UPDATE analytics_scope_grants SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1 AND school_id = $2 AND revoked_at IS NULL`
	_, err := s.pool.Exec(ctx, query, id, schoolID)
	return err
}

// Export Tokens
func (s *Store) CreateExportToken(ctx context.Context, token BiExportToken) (*BiExportToken, error) {
	query := `INSERT INTO bi_export_tokens (school_id, name, token_hash, metric_keys, school_ids, expires_at, created_by)
	          VALUES ($1, $2, $3, $4, $5, $6, $7)
	          RETURNING id, created_at, updated_at`
	err := s.pool.QueryRow(ctx, query, token.SchoolID, token.Name, token.TokenHash, token.MetricKeys, token.SchoolIDs, token.ExpiresAt, token.CreatedBy).
		Scan(&token.ID, &token.CreatedAt, &token.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (s *Store) GetExportTokenByHash(ctx context.Context, hash string) (*BiExportToken, error) {
	query := `SELECT id, school_id, name, token_hash, metric_keys, school_ids, expires_at, revoked_at, last_used_at, created_by, created_at, updated_at
	          FROM bi_export_tokens WHERE token_hash = $1`
	var t BiExportToken
	err := s.pool.QueryRow(ctx, query, hash).
		Scan(&t.ID, &t.SchoolID, &t.Name, &t.TokenHash, &t.MetricKeys, &t.SchoolIDs, &t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) TouchExportToken(ctx context.Context, id string) error {
	query := `UPDATE bi_export_tokens SET last_used_at = NOW() WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, id)
	return err
}

func (s *Store) ListExportTokens(ctx context.Context, schoolID string) ([]BiExportToken, error) {
	query := `SELECT id, school_id, name, token_hash, metric_keys, school_ids, expires_at, revoked_at, last_used_at, created_by, created_at, updated_at
	          FROM bi_export_tokens WHERE school_id = $1 ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []BiExportToken
	for rows.Next() {
		var t BiExportToken
		if err := rows.Scan(&t.ID, &t.SchoolID, &t.Name, &t.TokenHash, &t.MetricKeys, &t.SchoolIDs, &t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		res = append(res, t)
	}
	return res, nil
}

func (s *Store) RevokeExportToken(ctx context.Context, id, schoolID string) error {
	query := `UPDATE bi_export_tokens SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1 AND school_id = $2 AND revoked_at IS NULL`
	_, err := s.pool.Exec(ctx, query, id, schoolID)
	return err
}
