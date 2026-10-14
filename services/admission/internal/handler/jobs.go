package handler

import (
	"context"
	"log"
	"time"
)

// RunJobs runs the PPDB background jobs every interval until ctx ends. Each
// job is idempotent (row locks + dedupe keys), so running several replicas
// or re-running after a crash is safe.
func (a *App) RunJobs(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		a.runJobsOnce(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *App) runJobsOnce(ctx context.Context, now time.Time) {
	jobs := []struct {
		name string
		run  func(context.Context, time.Time) error
	}{
		{"notification_emails", func(ctx context.Context, _ time.Time) error { return a.deliverNotificationEmails(ctx) }},
	}
	for _, j := range jobs {
		if err := j.run(ctx, now); err != nil {
			log.Printf("admission job %s: %v", j.name, err)
		}
	}
}
