package store

import (
	"database/sql"

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
	return db, nil
}
