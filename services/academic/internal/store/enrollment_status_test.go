package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TEST_PG_DSN must point at a database with the academic migrations applied.
func TestListEnrollmentsByStatusSQL(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`SELECT 1 FROM class_enrollments LIMIT 1`); err != nil {
		t.Skipf("academic migrations not applied: %v", err)
	}
	var school string
	_ = db.QueryRow(`SELECT gen_random_uuid()`).Scan(&school)
	rows, err := New(db).ListEnrollmentsByStatus(context.Background(), school, "graduated", "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}
