package handler

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// AdminDashboard is GET /admin/dashboard (BUG-030). PPDB admins and the
// read-only foundation / principal roles see the funnel per stage, the
// conversion rate, the breakdown per wave and track, the daily trend and the
// money collected per stage. Work-queue tasks are only shown to admins.
// Optional filters: period_id (defaults to the latest open period), wave_id,
// track_id.
func (a *App) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	if !common.CanViewDashboard(r) {
		common.WriteError(w, http.StatusForbidden, "akses dashboard PPDB ditolak")
		return
	}
	ctx := r.Context()
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query()
	summary := map[string]any{"tasks": []map[string]any{}, "recent_applications": []map[string]any{}}

	periodID := q.Get("period_id")
	var (
		periodName, periodStatus, periodYear string
		open, close                          sql.NullTime
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT id,name,status,academic_year,
		        COALESCE(registration_open_at,(SELECT min(registration_start) FROM admission_waves w WHERE w.period_id=p.id AND w.deleted_at IS NULL)),
		        COALESCE(registration_close_at,(SELECT max(registration_end) FROM admission_waves w WHERE w.period_id=p.id AND w.deleted_at IS NULL))
		 FROM admission_periods p WHERE school_id=$1 AND deleted_at IS NULL AND ($2='' OR id::text=$2)
		 ORDER BY (status='open') DESC,created_at DESC LIMIT 1`, schoolID, periodID).
		Scan(&periodID, &periodName, &periodStatus, &periodYear, &open, &close)
	switch {
	case err == sql.ErrNoRows:
		summary["totals"] = map[string]int{"total": 0}
		summary["funnel"] = model.BuildFunnel(nil)
		common.WriteJSON(w, http.StatusOK, summary, nil)
		return
	case err != nil:
		writeErr(w, err)
		return
	}
	period := map[string]any{"id": periodID, "name": periodName, "status": periodStatus, "academic_year": periodYear}
	timeIfSet(period, "registration_open_at", nullTimePtr(open))
	timeIfSet(period, "registration_close_at", nullTimePtr(close))
	summary["active_period"] = period

	f := dashboardFilter{school: schoolID, period: periodID, wave: q.Get("wave_id"), track: q.Get("track_id")}
	totals, err := a.dashboardTotals(ctx, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	summary["totals"] = totals
	counts, err := a.dashboardFunnel(ctx, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	summary["funnel"] = model.BuildFunnel(&counts)
	summary["conversion_rate_bp"] = model.ConversionBP(counts)
	for _, dim := range []string{"wave", "track"} {
		list, err := a.dashboardBreakdown(ctx, f, dim)
		if err != nil {
			writeErr(w, err)
			return
		}
		summary["by_"+dim] = list
	}
	trend, err := a.dashboardTrend(ctx, f, time.Now())
	if err != nil {
		writeErr(w, err)
		return
	}
	summary["daily_trend"] = trend
	money, err := a.dashboardMoney(ctx, f)
	if err != nil {
		writeErr(w, err)
		return
	}
	summary["finance"] = money

	if c := common.Admin(r); c != nil && (c.ActiveRole == "admin" || c.ActiveRole == "superadmin") {
		recent, err := a.recentApplications(r, schoolID)
		if err != nil {
			writeErr(w, err)
			return
		}
		summary["recent_applications"] = recent
		tasks, err := a.dashboardTasks(r, schoolID)
		if err != nil {
			writeErr(w, err)
			return
		}
		summary["tasks"] = tasks
	}
	common.WriteJSON(w, http.StatusOK, summary, nil)
}

type dashboardFilter struct{ school, period, wave, track string }

// scope is the shared WHERE clause; $1 school, $2 period, $3 wave, $4 track.
const dashboardScope = `a.school_id=$1 AND a.period_id::text=$2 AND ($3='' OR a.wave_id::text=$3) AND ($4='' OR a.track_id::text=$4)`

func (f dashboardFilter) args() []any { return []any{f.school, f.period, f.wave, f.track} }

func (a *App) dashboardTotals(ctx context.Context, f dashboardFilter) (map[string]int, error) {
	rows, err := a.DB.QueryContext(ctx, `SELECT a.status,count(*) FROM applications a WHERE `+dashboardScope+` GROUP BY a.status`, f.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	totals := map[string]int{}
	total := 0
	for rows.Next() {
		var (
			status string
			n      int
		)
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		totals[status] = n
		total += n
	}
	totals["total"] = total
	return totals, rows.Err()
}

func (a *App) dashboardFunnel(ctx context.Context, f dashboardFilter) (model.FunnelCounts, error) {
	var c model.FunnelCounts
	err := a.DB.QueryRowContext(ctx,
		`SELECT count(*),
		        count(*) FILTER (WHERE a.submitted_at IS NOT NULL OR `+reachedStatus(`'{submitted}'`)+`),
		        count(*) FILTER (WHERE EXISTS(SELECT 1 FROM charges c WHERE c.application_id=a.id AND c.stage='registration'
		                 AND c.net_idr<=(SELECT COALESCE(sum(pa.amount_idr),0) FROM payment_allocations pa
		                     JOIN payment_submissions ps ON ps.id=pa.payment_id JOIN charges c2 ON c2.id=ps.charge_id
		                     WHERE ps.application_id=a.id AND ps.status='verified' AND c2.stage='registration'))),
		        count(*) FILTER (WHERE `+reachedStatus(`'{eligible_for_test}'`)+`),
		        count(*) FILTER (WHERE `+reachedStatus(`'{offered}'`)+`),
		        count(*) FILTER (WHERE `+reachedStatus(`'{reregistration,converted}'`)+`)
		 FROM applications a WHERE `+dashboardScope, f.args()...).
		Scan(&c.Registered, &c.Submitted, &c.FormPaid, &c.Verified, &c.Accepted, &c.Reregistered)
	return c, err
}

// reachedStatus is true when the application ever entered one of the
// statuses (a literal text[] such as '{offered}').
func reachedStatus(set string) string {
	return `EXISTS(SELECT 1 FROM application_state_events e WHERE e.application_id=a.id AND e.to_status=ANY(` + set + `::text[]))`
}

func (a *App) dashboardBreakdown(ctx context.Context, f dashboardFilter, dim string) ([]map[string]any, error) {
	join, col := `JOIN admission_waves d ON d.id=a.wave_id`, `d.registration_start`
	if dim == "track" {
		join, col = `JOIN admission_tracks d ON d.id=a.track_id`, `d.name`
	}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT d.id,d.name,count(*),count(*) FILTER (WHERE a.submitted_at IS NOT NULL),
		        count(*) FILTER (WHERE a.status IN('offered','reregistration','converted')),
		        count(*) FILTER (WHERE a.status IN('reregistration','converted'))
		 FROM applications a `+join+` WHERE `+dashboardScope+`
		 GROUP BY d.id,d.name,`+col+` ORDER BY `+col, f.args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name                                 string
			total, submitted, accepted, reregistered int
		)
		if err := rows.Scan(&id, &name, &total, &submitted, &accepted, &reregistered); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "total": total, "submitted": submitted,
			"accepted": accepted, "reregistered": reregistered})
	}
	return out, rows.Err()
}

func (a *App) dashboardTrend(ctx context.Context, f dashboardFilter, now time.Time) ([]map[string]any, error) {
	args := append(f.args(), now)
	rows, err := a.DB.QueryContext(ctx,
		`SELECT day::date,(SELECT count(*) FROM applications a WHERE `+dashboardScope+`
		          AND (a.created_at AT TIME ZONE 'Asia/Jakarta')::date=day::date)
		 FROM generate_series(($5::timestamptz AT TIME ZONE 'Asia/Jakarta')::date-29,($5::timestamptz AT TIME ZONE 'Asia/Jakarta')::date,'1 day') day
		 ORDER BY day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			day time.Time
			n   int
		)
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"date": day.Format("2006-01-02"), "count": n})
	}
	return out, rows.Err()
}

func (a *App) dashboardMoney(ctx context.Context, f dashboardFilter) (map[string]any, error) {
	out := map[string]any{}
	for _, stage := range []string{stageRegistration, stageReregistration} {
		var billed, paid int64
		err := a.DB.QueryRowContext(ctx,
			`SELECT COALESCE(sum(c.net_idr),0),
			        COALESCE((SELECT sum(pa.amount_idr) FROM payment_allocations pa JOIN payment_submissions ps ON ps.id=pa.payment_id
			                  JOIN charges c2 ON c2.id=ps.charge_id JOIN applications a ON a.id=ps.application_id
			                  WHERE `+dashboardScope+` AND ps.status='verified' AND c2.stage=$5),0)
			 FROM applications a JOIN LATERAL (SELECT net_idr FROM charges c WHERE c.application_id=a.id AND c.stage=$5
			      AND c.cancelled_at IS NULL ORDER BY c.revision DESC LIMIT 1) c ON true
			 WHERE `+dashboardScope, append(f.args(), stage)...).Scan(&billed, &paid)
		if err != nil {
			return nil, err
		}
		outstanding := billed - paid
		if outstanding < 0 {
			outstanding = 0
		}
		out[stage] = map[string]any{"billed_idr": billed, "paid_idr": paid, "outstanding_idr": outstanding}
	}
	return out, nil
}
