package jobs

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"skci/insight/internal/bi"
	"skci/insight/internal/client"
	"skci/insight/internal/config"
	"skci/insight/internal/metrics"
	"skci/insight/internal/predict"
)

type Runner struct {
	cfg          *config.Config
	pool         *pgxpool.Pool
	biStore      *bi.Store
	metricStore  *metrics.Store
	predictStore *predict.Store
	client       *client.Client
}

func NewRunner(cfg *config.Config, pool *pgxpool.Pool, biStore *bi.Store, metricStore *metrics.Store, predictStore *predict.Store, client *client.Client) *Runner {
	return &Runner{
		cfg:          cfg,
		pool:         pool,
		biStore:      biStore,
		metricStore:  metricStore,
		predictStore: predictStore,
		client:       client,
	}
}

func (r *Runner) Start(ctx context.Context) {
	interval := time.Duration(r.cfg.JobIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.runTick(ctx)
			}
		}
	}()
}

func (r *Runner) runTick(ctx context.Context) {
	// Try advisory lock
	var locked bool
	err := r.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock(8096001)").Scan(&locked)
	if err != nil || !locked {
		return
	}
	defer func() {
		_, _ = r.pool.Exec(ctx, "SELECT pg_advisory_unlock(8096001)")
	}()

	now := time.Now()
	r.dispatchScheduledReports(ctx, now)
	r.runPredictions(ctx, now)
	r.forwardAuditEvents(ctx)
}

func (r *Runner) dispatchScheduledReports(ctx context.Context, now time.Time) {
	schedules, err := r.biStore.GetDueSchedules(ctx, now)
	if err != nil {
		return
	}

	for _, s := range schedules {
		rep, errR := r.biStore.GetReport(ctx, s.SavedReportID, s.SchoolID)
		if errR != nil {
			continue
		}

		for _, recID := range s.RecipientUserIDs {
			runDate := now.Format("2006-01-02")
			idempKey := fmt.Sprintf("sched:%s:%s:%s", s.ID, recID, runDate)

			title := fmt.Sprintf("Laporan Terjadwal: %s", rep.Title)
			body := fmt.Sprintf("Laporan berkala '%s' telah siap untuk ditinjau di %s/analitik/laporan/%s", rep.Title, r.cfg.AppPublicURL, rep.ID)

			deliveries, errN := r.client.SendScheduledNotification(ctx, recID, title, body, idempKey)
			status := "sent"
			var reason *string
			if errN != nil {
				status = "failed"
				errStr := errN.Error()
				reason = &errStr
			}

			_ = r.biStore.RecordScheduleRun(ctx, bi.ScheduledReportRun{
				SchoolID:        s.SchoolID,
				ScheduleID:      s.ID,
				RecipientUserID: recID,
				Status:          status,
				Reason:          reason,
				Deliveries:      deliveries,
				IdempotencyKey:  idempKey,
			})
		}

		nextRun := bi.NextRun(s, now)
		_ = r.biStore.UpdateScheduleNextRun(ctx, s.ID, now, nextRun)
	}
}

// Zero write calls during risk prediction jobs (AC-154-01)
func (r *Runner) runPredictions(ctx context.Context, now time.Time) {
	// Find schools with predictive_enabled = true
	rows, err := r.pool.Query(ctx, "SELECT school_id FROM insight_settings WHERE predictive_enabled = true")
	if err != nil {
		return
	}
	defer rows.Close()

	var schoolIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			schoolIDs = append(schoolIDs, id)
		}
	}

	todayStr := now.Format("2006-01-02")

	for _, sID := range schoolIDs {
		// 1. Arrears Risk
		rule, errRule := r.predictStore.GetActiveRule(ctx, sID, "arrears_risk")
		if errRule == nil && rule != nil {
			params := predict.DefaultArrearsRuleParams()
			facts, errF := r.client.FetchRiskFactsArrears(ctx, sID, todayStr)
			if errF == nil && len(facts) > 0 {
				run, errRun := r.predictStore.CreatePredictionRun(ctx, sID, "arrears_risk", rule.ID, "ok", nil)
				if errRun == nil {
					preds := make([]predict.Prediction, 0, len(facts))
					for _, f := range facts {
						score, level, topFactors := predict.ArrearsRiskScore(f, params)
						preds = append(preds, predict.Prediction{
							SchoolID:    sID,
							RunID:       run.ID,
							Model:       "arrears_risk",
							SubjectKind: "student",
							SubjectID:   f.StudentUserID,
							Score:       &score,
							Level:       &level,
							Factors:     topFactors,
							ReviewLabel: "perlu_tinjauan",
						})
					}
					_ = r.predictStore.SavePredictions(ctx, preds)
				}
			}
		}

		// 2. Absence Risk
		ruleAbs, errAbs := r.predictStore.GetActiveRule(ctx, sID, "absence_risk")
		if errAbs == nil && ruleAbs != nil {
			paramsAbs := predict.DefaultAbsenceRuleParams()
			startStr := now.AddDate(0, -1, 0).Format("2006-01-02")
			factsAbs, errFA := r.client.FetchRiskFactsAbsence(ctx, sID, startStr, todayStr)
			if errFA == nil && len(factsAbs) > 0 {
				run, errRun := r.predictStore.CreatePredictionRun(ctx, sID, "absence_risk", ruleAbs.ID, "ok", nil)
				if errRun == nil {
					preds := make([]predict.Prediction, 0, len(factsAbs))
					for _, f := range factsAbs {
						score, level, topFactors := predict.AbsenceRiskScore(f, paramsAbs)
						preds = append(preds, predict.Prediction{
							SchoolID:    sID,
							RunID:       run.ID,
							Model:       "absence_risk",
							SubjectKind: "student",
							SubjectID:   f.StudentUserID,
							Score:       &score,
							Level:       &level,
							Factors:     topFactors,
							ReviewLabel: "perlu_tinjauan",
						})
					}
					_ = r.predictStore.SavePredictions(ctx, preds)
				}
			}
		}
	}
}

func (r *Runner) forwardAuditEvents(ctx context.Context) {
	query := `SELECT id, school_id, event_type, actor_user_id, entity_type, entity_id, diff
	          FROM audit_events WHERE forwarded_at IS NULL ORDER BY created_at ASC LIMIT 50`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return
	}
	defer rows.Close()

	type eventRow struct {
		ID         string
		SchoolID   string
		EventType  string
		ActorID    *string
		EntityType string
		EntityID   string
		Diff       map[string]interface{}
	}

	var events []eventRow
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.EventType, &e.ActorID, &e.EntityType, &e.EntityID, &e.Diff); err == nil {
			events = append(events, e)
		}
	}

	for _, e := range events {
		actorStr := ""
		if e.ActorID != nil {
			actorStr = *e.ActorID
		}
		errF := r.client.ForwardAudit(ctx, e.SchoolID, e.EventType, actorStr, e.EntityType, e.EntityID, e.Diff)
		if errF == nil {
			_, _ = r.pool.Exec(ctx, "UPDATE audit_events SET forwarded_at = NOW() WHERE id = $1", e.ID)
		} else {
			log.Printf("failed to forward audit event %s: %v", e.ID, errF)
		}
	}
}
