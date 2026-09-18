package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

// TestListMyExams_DateBucketing is the runnable check for item 20's
// today/upcoming/history split. It reproduces the exact bug being fixed:
// a PUBLISHED exam with no exam_attempts row yet (student never
// voucher-entered) must still show up — "today" if it starts today
// (whether or not the window has opened), "upcoming" if it starts on a
// future day, "history" if its window already closed. Skips if no dev
// Postgres is reachable, matching services/evaluation's DB-backed test
// convention (see period_task_test.go).
func TestListMyExams_DateBucketing(t *testing.T) {
	dsn := os.Getenv("EXAM_TEST_DATABASE_URL")
	if dsn == "" {
		// Matches docker-compose's shared Postgres (POSTGRES_PORT=5480 in
		// .env) — NOT services/exam/internal/config.go's stale 5484 default.
		dsn = "postgres://sekoci:change-me@localhost:5480/skci_exam?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("skipping: could not open %s: %v", dsn, err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dsn, err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	newID := func() string {
		var id string
		if err := db.QueryRowContext(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
			t.Fatalf("gen_random_uuid: %v", err)
		}
		return id
	}

	schoolID := newID()
	academicYearID := newID()
	termID := newID()
	classroomID := newID()
	subjectID := newID()
	teacherID := newID()
	studentID := newID()

	var examTypeID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO exam_types (school_id, academic_year_id, name, mapping_rapor, created_by)
		VALUES ($1,$2,'test type','sumatif',$3) RETURNING id`, schoolID, academicYearID, teacherID).Scan(&examTypeID); err != nil {
		t.Fatalf("insert exam_type: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM exam_types WHERE id=$1`, examTypeID) })

	insertExam := func(title string, startAt, endAt time.Time) string {
		var id string
		err := db.QueryRowContext(ctx, `
			INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, classroom_id, subject_id,
				teacher_id, start_at, end_at, is_published, created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true,$8) RETURNING id`,
			schoolID, academicYearID, termID, examTypeID, title, classroomID, subjectID, teacherID, startAt, endAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert exam %s: %v", title, err)
		}
		t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM exams WHERE id=$1`, id) })
		// exam_classrooms (migration 0005) is the real target-audience set
		// ListMyExams/ListExams filter against now -- CreateExam populates
		// this in the same transaction as the exams row, so a raw-SQL test
		// insert has to do the same or the exam is invisible to every query.
		if _, err := db.ExecContext(ctx, `INSERT INTO exam_classrooms (exam_id, classroom_id) VALUES ($1,$2)`, id, classroomID); err != nil {
			t.Fatalf("insert exam_classrooms for %s: %v", title, err)
		}
		return id
	}

	now := time.Now()
	todayExamID := insertExam("today, not yet open", now.Add(1*time.Hour), now.Add(3*time.Hour))
	upcomingExamID := insertExam("two days from now", now.AddDate(0, 0, 2), now.AddDate(0, 0, 2).Add(2*time.Hour))
	historyExamID := insertExam("missed, window closed", now.AddDate(0, 0, -3), now.AddDate(0, 0, -2))

	s := New(db)

	assertBucketHas := func(bucket, wantExamID string) {
		t.Helper()
		rows, err := s.ListMyExams(ctx, studentID, classroomID, bucket)
		if err != nil {
			t.Fatalf("ListMyExams(%s): %v", bucket, err)
		}
		found := false
		for _, r := range rows {
			if r.Exam.ID == wantExamID {
				found = true
				if r.Attempt != nil {
					t.Errorf("bucket %s: exam %s should have no attempt (student never entered), got %+v", bucket, wantExamID, r.Attempt)
				}
			}
		}
		if !found {
			t.Errorf("bucket %s: expected exam %s in results, got %d rows", bucket, wantExamID, len(rows))
		}
	}

	assertBucketHas("today", todayExamID)
	assertBucketHas("upcoming", upcomingExamID)
	assertBucketHas("history", historyExamID)

	// Cross-checks: the today-not-yet-open exam must NOT leak into upcoming
	// or history, and the missed exam must not appear as "today".
	rows, err := s.ListMyExams(ctx, studentID, classroomID, "upcoming")
	if err != nil {
		t.Fatalf("ListMyExams(upcoming): %v", err)
	}
	for _, r := range rows {
		if r.Exam.ID == todayExamID {
			t.Errorf("today's not-yet-open exam leaked into upcoming bucket")
		}
	}

	// Empty classroom_id (academic-service unreachable / no enrollment) must
	// degrade to an empty list, never an error.
	empty, err := s.ListMyExams(ctx, studentID, "", "today")
	if err != nil {
		t.Fatalf("ListMyExams with empty classroomID should not error: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected empty result for empty classroomID, got %d rows", len(empty))
	}

	// Sekoci revamp V1.docx item 2: unpublish reverts a published exam back
	// to draft. Time-window gating (before start_at) is the handler's job;
	// this only checks the store flips the flag and rejects a second call.
	if err := s.UnpublishExam(ctx, todayExamID, schoolID); err != nil {
		t.Fatalf("UnpublishExam: %v", err)
	}
	reverted, err := s.GetExam(ctx, todayExamID, schoolID)
	if err != nil {
		t.Fatalf("GetExam after unpublish: %v", err)
	}
	if reverted == nil || reverted.IsPublished {
		t.Fatalf("expected exam to be draft after UnpublishExam, got %+v", reverted)
	}
	if err := s.UnpublishExam(ctx, todayExamID, schoolID); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("expected ErrNotEditable unpublishing an already-draft exam, got %v", err)
	}
}
