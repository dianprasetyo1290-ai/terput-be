package campaign_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/notification/internal/campaign"
	"skci/notification/internal/client"
	"skci/notification/internal/handler"
	"skci/notification/internal/provider"
	"skci/notification/internal/store"
)

func regressionDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// BR-COM-01 regression: transactional notifications ignore marketing consent.
func TestTransactionalNotificationIgnoresMarketingOptOut(t *testing.T) {
	db := regressionDB(t)
	ctx := context.Background()
	var school, user string
	_ = db.QueryRow(`SELECT gen_random_uuid(), gen_random_uuid()`).Scan(&school, &user)
	if err := campaign.NewStore(db).SetConsent(ctx, school, campaign.UserSubject(user), user, campaign.ChannelInApp, campaign.ConsentOptedOut, campaign.SourcePreferences, user); err != nil {
		t.Fatal(err)
	}
	h := handler.New(store.New(db), client.New("", ""), provider.NoopProvider{}, false)
	router := handler.NewRouter(h, handler.RouterConfig{JWTSecret: "x", InternalKey: "ik"})
	req := httptest.NewRequest(http.MethodPost, "/internal/notifications", strings.NewReader(
		`{"school_id":"`+school+`","user_id":"`+user+`","type":"billing.invoice.paid","title":"Tagihan lunas"}`))
	req.Header.Set("X-Internal-Key", "ik")
	req.Header.Set("Idempotency-Key", "tx-"+user)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("transactional send to opted-out user: %d %s", rec.Code, rec.Body.String())
	}
}
