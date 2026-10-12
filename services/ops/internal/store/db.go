// Package store holds the ops service's shared SQL plumbing (connection,
// audit outbox, document numbers). Domain packages (facility, …) keep their
// own queries and take a Q so the same code runs on *sql.DB or inside a
// *sql.Tx.
package store

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Q is satisfied by *sql.DB and *sql.Tx.
type Q interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
