package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"skci/quality/internal/client"
)

type Runner struct {
	pool     *pgxpool.Pool
	clients  *client.Clients
	interval time.Duration
	stopCh   chan struct{}
}

func NewRunner(pool *pgxpool.Pool, clients *client.Clients, interval time.Duration) *Runner {
	return &Runner{
		pool:     pool,
		clients:  clients,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (r *Runner) Start() {
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		r.runAll()
		for {
			select {
			case <-ticker.C:
				r.runAll()
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *Runner) Stop() {
	close(r.stopCh)
}

func (r *Runner) runAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r.runWithLock(ctx, "quality_audit_forward", r.forwardAuditEvents)
	r.runWithLock(ctx, "quality_action_plan_reminders", r.sendActionPlanReminders)
}

func (r *Runner) runWithLock(ctx context.Context, jobName string, fn func(ctx context.Context) error) {
	h := fnv.New64a()
	h.Write([]byte(jobName))
	lockKey := int64(h.Sum64())

	var acquired bool
	err := r.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
	if err != nil || !acquired {
		return
	}
	defer func() {
		_, _ = r.pool.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lockKey)
	}()

	if err := fn(ctx); err != nil {
		log.Printf("[jobs] %s failed: %v", jobName, err)
	}
}

func (r *Runner) forwardAuditEvents(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `
		SELECT id, school_id, module, entity_type, entity_id, action, actor_user_id::text, actor_role, before, after, reason, correlation_id, created_at
		FROM audit_events
		WHERE forwarded_at IS NULL
		ORDER BY id ASC
		LIMIT 50
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type auditRow struct {
		id            int64
		schoolID      string
		module        string
		entityType    string
		entityID      string
		action        string
		actorUserID   sql.NullString
		actorRole     sql.NullString
		before        []byte
		after         []byte
		reason        sql.NullString
		correlationID sql.NullString
		createdAt     time.Time
	}

	var list []auditRow
	for rows.Next() {
		var a auditRow
		if err := rows.Scan(&a.id, &a.schoolID, &a.module, &a.entityType, &a.entityID, &a.action,
			&a.actorUserID, &a.actorRole, &a.before, &a.after, &a.reason, &a.correlationID, &a.createdAt); err == nil {
			list = append(list, a)
		}
	}
	rows.Close()

	for _, a := range list {
		var bMap, aMap map[string]interface{}
		if len(a.before) > 0 {
			_ = json.Unmarshal(a.before, &bMap)
		}
		if len(a.after) > 0 {
			_ = json.Unmarshal(a.after, &aMap)
		}

		payload := map[string]interface{}{
			"school_id":      a.schoolID,
			"module":         a.module,
			"entity_type":    a.entityType,
			"entity_id":      a.entityID,
			"action":         a.action,
			"actor_user_id":  a.actorUserID.String,
			"actor_role":     a.actorRole.String,
			"before":         bMap,
			"after":          aMap,
			"reason":         a.reason.String,
			"correlation_id": a.correlationID.String,
			"created_at":     a.createdAt,
		}

		if err := r.clients.ForwardAuditEvent(ctx, payload); err == nil {
			_, _ = r.pool.Exec(ctx, "UPDATE audit_events SET forwarded_at = now() WHERE id = $1", a.id)
		}
	}
	return nil
}

func (r *Runner) sendActionPlanReminders(ctx context.Context) error {
	todayWIB := time.Now().UTC().Add(7 * time.Hour).Format("2006-01-02")
	rows, err := r.pool.Query(ctx, `
		SELECT ap.id, ap.owner_user_id::text, ap.action, ap.due_date::text
		FROM qa_action_plans ap
		WHERE ap.status IN ('open', 'in_progress')
		  AND ap.due_date <= $1::date
		  AND ap.deleted_at IS NULL
		LIMIT 20
	`, todayWIB)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, ownerID, action, dueDate string
		if err := rows.Scan(&id, &ownerID, &action, &dueDate); err == nil {
			_ = r.clients.SendNotification(ctx, ownerID, "Pengingat Rencana Tindak Lanjut Mutu",
				fmt.Sprintf("RTL '%s' jatuh tempo pada %s. Mohon perbarui status penanganannya.", action, dueDate),
				"/mutu/tindak-lanjut")
		}
	}
	return nil
}
