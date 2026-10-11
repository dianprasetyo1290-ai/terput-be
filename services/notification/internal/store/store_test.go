package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"skci/notification/internal/model"
)

// testDB runs against NOTIFICATION_TEST_DATABASE_URL, else the standalone dev
// Postgres on :5489, and SKIPS (never fails) when no migrated database is
// reachable — the same convention exam-service uses for its store tests.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("NOTIFICATION_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://teratai:change-me@localhost:5489/teratai_notification?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("skipping: could not open %s: %v", dsn, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dsn, err)
	}
	if _, err := db.Exec(`SELECT 1 FROM notifications LIMIT 1`); err != nil {
		db.Close()
		t.Skipf("skipping: migrations not applied to %s: %v", dsn, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testUUID(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRowContext(ctx, `SELECT gen_random_uuid()`).Scan(&id); err != nil {
		t.Fatalf("gen_random_uuid: %v", err)
	}
	return id
}

func ptr(v string) *string { return &v }

// TestCreateNotificationIdempotent pins the SQL half of the internal-send
// contract: the same (school_id, idempotency_key) converges on one row.
func TestCreateNotificationIdempotent(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID, userID := testUUID(t, ctx, db), testUUID(t, ctx, db)
	key := "billing:" + testUUID(t, ctx, db) + ":paid"

	first, created, err := s.CreateNotification(ctx, CreateNotificationInput{
		SchoolID: schoolID, UserID: userID, Type: "billing.invoice.paid",
		Title: "Tagihan", Body: "lunas", IdempotencyKey: ptr(key),
	})
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	second, created, err := s.CreateNotification(ctx, CreateNotificationInput{
		SchoolID: schoolID, UserID: userID, Type: "billing.invoice.paid",
		Title: "Tagihan", Body: "lunas", IdempotencyKey: ptr(key),
	})
	if err != nil {
		t.Fatalf("retry create: %v", err)
	}
	if created {
		t.Error("retry reported created=true; the idempotency key must deduplicate")
	}
	if second.ID != first.ID {
		t.Errorf("retry returned a different id %s (want %s)", second.ID, first.ID)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM notifications WHERE school_id=$1 AND idempotency_key=$2`, schoolID, key).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("rows for the idempotency key = %d, want 1", count)
	}
}

// TestMarkNotificationReadIdempotent pins the COALESCE: a second mark does not
// move read_at, and an unknown id is ErrNotFound.
func TestMarkNotificationReadIdempotent(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID, userID := testUUID(t, ctx, db), testUUID(t, ctx, db)

	n, _, err := s.CreateNotification(ctx, CreateNotificationInput{SchoolID: schoolID, UserID: userID, Type: "chat.message", Title: "Pesan"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	first, err := s.MarkNotificationRead(ctx, n.ID, schoolID, userID)
	if err != nil {
		t.Fatalf("first mark: %v", err)
	}
	if first.ReadAt == nil {
		t.Fatal("read_at is null after marking read")
	}
	second, err := s.MarkNotificationRead(ctx, n.ID, schoolID, userID)
	if err != nil {
		t.Fatalf("second mark: %v", err)
	}
	if second.ReadAt == nil || !first.ReadAt.Equal(*second.ReadAt) {
		t.Errorf("read_at moved on a second mark: %v -> %v", first.ReadAt, second.ReadAt)
	}
	if _, err := s.MarkNotificationRead(ctx, testUUID(t, ctx, db), schoolID, userID); !errors.Is(err, ErrNotFound) {
		t.Errorf("marking an unknown notification: err = %v, want ErrNotFound", err)
	}

	// Unread count drops to zero once read.
	if got, err := s.CountUnreadNotifications(ctx, schoolID, userID); err != nil || got != 0 {
		t.Errorf("unread count after read = %d, %v; want 0, nil", got, err)
	}
}

// TestChatParticipantScoping pins the chat isolation SQL: listing returns only
// the caller's threads, and IsChatParticipant is false for everyone else.
func TestChatParticipantScoping(t *testing.T) {
	db := testDB(t)
	s := New(db)
	ctx := context.Background()
	schoolID, otherSchool := testUUID(t, ctx, db), testUUID(t, ctx, db)
	owner, outsider := testUUID(t, ctx, db), testUUID(t, ctx, db)

	thread, err := s.CreateChatThread(ctx, CreateThreadInput{
		SchoolID: schoolID, Subject: "Wali", CreatedBy: owner,
		Participants: []ParticipantInput{{UserID: outsider, Role: model.ChatRoleMember}},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := s.CreateChatThread(ctx, CreateThreadInput{SchoolID: schoolID, Subject: "Lain", CreatedBy: testUUID(t, ctx, db)}); err != nil {
		t.Fatalf("create other thread: %v", err)
	}

	ok, err := s.IsChatParticipant(ctx, thread.ID, owner)
	if err != nil || !ok {
		t.Fatalf("owner IsChatParticipant = %v, %v; want true, nil", ok, err)
	}
	for _, stranger := range []string{testUUID(t, ctx, db), outsider + "0"} {
		if ok, err := s.IsChatParticipant(ctx, thread.ID, stranger); err != nil || ok {
			t.Errorf("IsChatParticipant(%s) = %v, %v; want false, nil", stranger, ok, err)
		}
	}

	threads, total, err := s.ListChatThreadsForUser(ctx, schoolID, owner, 1, 50)
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if total != 1 || len(threads) != 1 || threads[0].ID != thread.ID {
		t.Errorf("owner's threads total=%d len=%d, want exactly the seeded one", total, len(threads))
	}

	// A thread in another school is invisible to this tenant.
	if _, err := s.GetChatThread(ctx, thread.ID, otherSchool); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-school GetChatThread err = %v, want ErrNotFound", err)
	}
}
