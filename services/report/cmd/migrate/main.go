// cmd/migrate applies (or rolls back) report-service's SQL migrations
// using golang-migrate/v4, per CONVENTIONS.md's tech stack. Usage:
//
//	go run ./cmd/migrate up
//	go run ./cmd/migrate down
//	go run ./cmd/migrate version
//
// DATABASE_URL env var (or .env) selects the target database.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	_ = godotenv.Load()
	dbURL := getEnv("DATABASE_URL", "postgres://sekoci:change-me@localhost:5486/skci_report?sslmode=disable")

	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("postgres driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		log.Fatalf("migrate init: %v", err)
	}

	switch cmd {
	case "up":
		err = m.Up()
	case "down":
		err = m.Steps(-1)
	case "version":
		v, dirty, verr := m.Version()
		fmt.Printf("version=%d dirty=%v\n", v, dirty)
		err = verr
	default:
		log.Fatalf("unknown command %q (use up|down|version)", cmd)
	}
	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migrate %s: %v", cmd, err)
	}
	fmt.Println("migrate:", cmd, "done")
}
