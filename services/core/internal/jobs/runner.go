package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"time"

	"skci/core/internal/client"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

type Runner struct {
	DB          *sql.DB
	Store       store.Store
	Client      *client.Client
	Interval    time.Duration
	MaxAttempts int
	// ServiceDeskSLA is the nil-safe service desk SLA/escalation job (T11).
	ServiceDeskSLA func(ctx context.Context) error
}

func NewRunner(db *sql.DB, st store.Store, cl *client.Client, interval time.Duration, maxAttempts int) *Runner {
	return &Runner{DB: db, Store: st, Client: cl, Interval: interval, MaxAttempts: maxAttempts}
}

func (r *Runner) Start(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.RunOnce(ctx)
			}
		}
	}()
}

func (r *Runner) RunOnce(ctx context.Context) {
	r.withAdvisoryLock(ctx, "core_escalation_job", func() {
		if err := r.runEscalation(ctx); err != nil {
			log.Printf("escalation job error: %v", err)
		}
	})

	r.withAdvisoryLock(ctx, "core_callback_retry_job", func() {
		if err := r.runRetryCallback(ctx); err != nil {
			log.Printf("callback retry job error: %v", err)
		}
	})

	if r.ServiceDeskSLA != nil {
		r.withAdvisoryLock(ctx, "core_service_desk_sla_job", func() {
			if err := r.ServiceDeskSLA(ctx); err != nil {
				log.Printf("service desk sla job error: %v", err)
			}
		})
	}
}

func (r *Runner) withAdvisoryLock(ctx context.Context, jobName string, fn func()) {
	if r.DB == nil {
		fn()
		return
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(jobName))
	lockID := int64(h.Sum64())

	var acquired bool
	err := r.DB.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired)
	if err != nil {
		log.Printf("job %s: advisory lock: %v", jobName, err)
		return
	}
	if !acquired { // another replica is running this job
		return
	}

	defer func() {
		_, _ = r.DB.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", lockID)
	}()

	fn()
}

func (r *Runner) runEscalation(ctx context.Context) error {
	steps, err := r.Store.ListExpiredSteps(ctx)
	if err != nil {
		return err
	}
	for _, step := range steps {
		repl, err := r.Store.EscalateStep(ctx, step.ID)
		if errors.Is(err, store.ErrConflict) {
			continue // decided or escalated between list and lock
		} else if err != nil {
			log.Printf("escalate step %s: %v", step.ID, err)
			continue
		}
		req, err := r.Store.GetRequestByID(ctx, step.SchoolID, step.RequestID)
		if err != nil {
			log.Printf("escalate step %s: reload request: %v", step.ID, err)
			continue
		}
		targetRole := ""
		if repl.ApproverRole != nil {
			targetRole = *repl.ApproverRole
		}
		reason := fmt.Sprintf("tahap %d melewati SLA, dialihkan ke %s", step.Seq, targetRole)
		_ = r.Store.InsertAudit(ctx, model.AuditEvent{
			SchoolID: &step.SchoolID, Service: "core", Module: "approval", EntityType: "approval_request",
			EntityID: req.ID, Action: "escalated", Reason: &reason, CorrelationID: &req.CorrelationID,
		})
		// FR-79-006 / AC-79-04: both the overdue approver(s) and the new one hear about it.
		step.Status = model.StepStatusEscalated
		NotifyStep(ctx, r.Client, req, step, "Persetujuan Dieskalasi",
			fmt.Sprintf("Pengajuan %q dialihkan ke %s karena melewati batas waktu.", req.Title, targetRole))
		NotifyStep(ctx, r.Client, req, *repl, "Persetujuan Baru (Eskalasi)",
			fmt.Sprintf("Pengajuan %q dialihkan kepada Anda untuk diputuskan.", req.Title))
	}
	return nil
}

func (r *Runner) runRetryCallback(ctx context.Context) error {
	reqs, err := r.Store.ListPendingCallbacks(ctx, r.MaxAttempts)
	if err != nil {
		return err
	}
	for i := range reqs {
		req := &reqs[i]
		// Backoff attempts² minutes since the last try.
		if req.LastCallbackAt != nil {
			wait := time.Duration(req.CallbackAttempts*req.CallbackAttempts) * time.Minute
			if time.Since(*req.LastCallbackAt) < wait {
				continue
			}
		}
		DeliverCallback(ctx, r.Store, r.Client, req, r.MaxAttempts)
	}
	return nil
}
