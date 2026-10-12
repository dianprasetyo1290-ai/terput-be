package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"skci/notification/internal/model"
)

// t13DB runs the T13 chat SQL against TEST_PG_DSN (already migrated through
// 0003); it skips when the variable is unset.
func t13DB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT 1 FROM chat_message_reports LIMIT 1`); err != nil {
		t.Skipf("migration 0003 not applied: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTeacherParentThreadSQL(t *testing.T) {
	db := t13DB(t)
	s := New(db)
	ctx := context.Background()
	school, teacher, guardian, student := testUUID(t, ctx, db), testUUID(t, ctx, db), testUUID(t, ctx, db), testUUID(t, ctx, db)

	th, err := s.CreateChatThread(ctx, CreateThreadInput{SchoolID: school, Subject: "x", CreatedBy: teacher,
		Participants: []ParticipantInput{{UserID: guardian}}, Kind: model.ChatKindTeacherParent,
		StudentUserID: student, TeacherUserID: teacher, GuardianUserID: guardian})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if th.Kind != model.ChatKindTeacherParent || th.GuardianUserID == nil || *th.GuardianUserID != guardian {
		t.Fatalf("thread = %+v", th)
	}
	if _, err := s.CreateChatThread(ctx, CreateThreadInput{SchoolID: school, Subject: "dup", CreatedBy: teacher, Kind: model.ChatKindTeacherParent,
		StudentUserID: student, TeacherUserID: teacher, GuardianUserID: guardian}); err == nil {
		t.Fatal("second open thread for the same trio must violate the unique index")
	}
	found, err := s.FindActiveTeacherParentThread(ctx, school, teacher, guardian, student)
	if err != nil || found.ID != th.ID {
		t.Fatalf("find: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO chat_threads (school_id, subject, created_by, kind) VALUES ($1,'bad',$2,'teacher_parent')`, school, teacher); err == nil {
		t.Fatal("teacher_parent without trio must violate the CHECK")
	}

	name, mime, size := "a.pdf", "application/pdf", int64(10)
	msg, err := s.CreateChatMessage(ctx, CreateMessageInput{ThreadID: th.ID, SenderUserID: guardian, Body: "malam",
		AttachmentName: &name, AttachmentMime: &mime, AttachmentSize: &size, OutsideServiceHours: true})
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	var msgSchool string
	_ = db.QueryRow(`SELECT school_id FROM chat_messages WHERE id=$1`, msg.ID).Scan(&msgSchool)
	if msgSchool != school {
		t.Fatalf("message school_id = %q", msgSchool)
	}
	got, err := s.GetChatMessage(ctx, msg.ID, school)
	if err != nil || !got.OutsideServiceHours || got.AttachmentSize == nil || *got.AttachmentSize != 10 {
		t.Fatalf("get message = %+v, %v", got, err)
	}
	if _, err := s.GetChatMessage(ctx, msg.ID, testUUID(t, ctx, db)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-school message: %v", err)
	}

	r, err := s.CreateChatReport(ctx, school, msg.ID, teacher, "kasar")
	if err != nil || r.ThreadID != th.ID || r.MessageBody != "malam" {
		t.Fatalf("report = %+v, %v", r, err)
	}
	if _, err := s.CreateChatReport(ctx, school, msg.ID, teacher, "lagi"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate report: %v", err)
	}
	list, total, err := s.ListChatReports(ctx, school, "open", 1, 20)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("list: %d %v", total, err)
	}
	if _, err := s.ReviewChatReport(ctx, r.ID, school, model.ReportReviewed, "ok", teacher); err != nil {
		t.Fatalf("review: %v", err)
	}
	if _, err := s.ReviewChatReport(ctx, r.ID, school, model.ReportDismissed, "", teacher); !errors.Is(err, ErrConflict) {
		t.Fatalf("second review: %v", err)
	}
	if err := s.ArchiveChatThread(ctx, th.ID); err != nil {
		t.Fatal(err)
	}
	msgs, _, _ := s.ListChatMessages(ctx, th.ID, 1, 10)
	if len(msgs) != 1 || msgs[0].AttachmentName == nil {
		t.Fatalf("messages after archive = %+v", msgs)
	}
}

func TestChatSettingsSQL(t *testing.T) {
	db := t13DB(t)
	s := New(db)
	ctx := context.Background()
	school := testUUID(t, ctx, db)
	def, err := s.GetChatSettings(ctx, school)
	if err != nil || def.ServiceStart != nil || def.MaxAttachmentBytes != model.DefaultMaxAttachmentBytes {
		t.Fatalf("defaults = %+v, %v", def, err)
	}
	start, end := "07:00", "17:00"
	if err := s.UpsertChatSettings(ctx, model.ChatSettings{SchoolID: school, ServiceStart: &start, ServiceEnd: &end, ServiceDays: []int{1, 2, 3}, MaxAttachmentBytes: 1000}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetChatSettings(ctx, school)
	if err != nil || *got.ServiceStart != "07:00" || *got.ServiceEnd != "17:00" || len(got.ServiceDays) != 3 || got.MaxAttachmentBytes != 1000 {
		t.Fatalf("settings = %+v, %v", got, err)
	}
	if err := s.UpsertChatSettings(ctx, model.ChatSettings{SchoolID: school, MaxAttachmentBytes: 2000}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetChatSettings(ctx, school)
	if got.ServiceStart != nil || len(got.ServiceDays) != 0 {
		t.Fatalf("cleared settings = %+v", got)
	}
}
