package store

import (
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Store wraps the shared *sql.DB pool. All entity-specific queries live in
// sibling files in this package (term.go, classroom.go, ...).
type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{DB: db}
}

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	// database/sql defaults MaxOpenConns to UNLIMITED. Without this cap a
	// busy moment -- 800 siswa starting an exam at once -- lets this one
	// service open connections until Postgres hits max_connections and
	// starts refusing, which takes down every other service too, not just
	// this one. Same 20/5/30m shape as identity, exam and report.
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
