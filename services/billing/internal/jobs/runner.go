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

// runWithLock runs job while holding a session-level advisory lock so only
// one instance runs it at a time. Lock and unlock must happen on the SAME
// connection: through the *sql.DB pool the unlock could land on another
// session, leaving the lock held forever and the job silently skipped on
// every later tick (BUG-010).
func (r *Runner) runWithLock(job Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := RunLocked(ctx, r.db, job.Name, func(ctx context.Context) error { return job.Handler(ctx, r.db) }); err != nil {
		log.Printf("[job %s] error: %v", job.Name, err)
	}
}

// RunLocked runs fn under the advisory lock for name on one pinned
// connection. It returns nil without running fn when another session holds
// the lock.
func RunLocked(ctx context.Context, db *sql.DB, name string, fn func(context.Context) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	lockID := hashJobName(name)
	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, lockID).Scan(&acquired); err != nil {
		return err
	}
	if !acquired {
		return nil
	}
	defer func() {
		// Unlock with a fresh context: ctx may already be cancelled.
		uctx, ucancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ucancel()
		_, _ = conn.ExecContext(uctx, `SELECT pg_advisory_unlock($1)`, lockID)
	}()
	return fn(ctx)
}
