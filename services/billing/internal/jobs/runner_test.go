package jobs

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestRunLockedReleases covers BUG-010: after a run the advisory lock must
// be free again, even with a pool of many connections, so the next tick
// (or another instance) can run the job.
func TestRunLockedReleases(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	name := "qa_bill_lock_test_" + time.Now().Format("150405.000000")
	runs := 0
	for i := 0; i < 20; i++ {
		if err := RunLocked(ctx, db, name, func(context.Context) error { runs++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if runs != 20 {
		t.Fatalf("runs=%d want 20 (lock leaked between runs)", runs)
	}
	// While held, a second caller must skip.
	inner := 0
	_ = RunLocked(ctx, db, name, func(ctx context.Context) error {
		return RunLocked(ctx, db, name, func(context.Context) error { inner++; return nil })
	})
	if inner != 0 {
		t.Fatal("concurrent holder ran the job")
	}
}
