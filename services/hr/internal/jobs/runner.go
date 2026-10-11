package jobs

import (
	"context"
	"database/sql"
	"hash/fnv"
	"log"
	"time"
)

type Runner struct {
	db       *sql.DB
	interval time.Duration
	jobs     []Job
	stopCh   chan struct{}
}

type Job struct {
	Name    string
	Handler func(ctx context.Context, db *sql.DB) error
}

func NewRunner(db *sql.DB, interval time.Duration) *Runner {
	return &Runner{
		db:       db,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (r *Runner) Register(name string, handler func(ctx context.Context, db *sql.DB) error) {
	r.jobs = append(r.jobs, Job{Name: name, Handler: handler})
}

func (r *Runner) Start() {
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		// Run once on startup
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
	for _, job := range r.jobs {
		r.runWithLock(job)
	}
}

func hashJobName(name string) int64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	return int64(h.Sum64())
}

func (r *Runner) runWithLock(job Job) {
	lockID := hashJobName(job.Name)

	var acquired bool
	err := r.db.QueryRow(`SELECT pg_try_advisory_lock($1)`, lockID).Scan(&acquired)
	if err != nil || !acquired {
		return // another instance is running this job
	}

	defer func() {
		_, _ = r.db.Exec(`SELECT pg_advisory_unlock($1)`, lockID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := job.Handler(ctx, r.db); err != nil {
		log.Printf("[job %s] error: %v", job.Name, err)
	}
}
