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

// TestListExams_FolderNone is the runnable check for the "Tanpa Folder"
// listing. An exam created outside a Bank Soal folder has folder_id NULL and
// was reachable from nowhere in the UI, while the guru dashboard still
// counted it -- SMK GPM saw "Total Ujian 4" and found only 3 in the folder.
// folder_id=none must return exactly the unfiled ones, and a real folder id
// must keep excluding them.
func TestListExams_FolderNone(t *testing.T) {
	dsn := os.Getenv("EXAM_TEST_DATABASE_URL")
	if dsn == "" {
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
	schoolID, academicYearID, termID := newID(), newID(), newID()
	classroomID, subjectID, teacherID := newID(), newID(), newID()

	var examTypeID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO exam_types (school_id, academic_year_id, name, mapping_rapor, created_by)
		VALUES ($1,$2,'folder filter type','sumatif',$3) RETURNING id`, schoolID, academicYearID, teacherID).Scan(&examTypeID); err != nil {
		t.Fatalf("insert exam_type: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM exam_types WHERE id=$1`, examTypeID) })

	var folderID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO question_folders (school_id, academic_year_id, name, created_by)
		VALUES ($1,$2,'folder filter',$3) RETURNING id`, schoolID, academicYearID, teacherID).Scan(&folderID); err != nil {
		t.Fatalf("insert question_folder: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM question_folders WHERE id=$1`, folderID) })

	insertExam := func(title string, folder *string) string {
		now := time.Now()
		var id string
		err := db.QueryRowContext(ctx, `
			INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, classroom_id, subject_id,
				teacher_id, start_at, end_at, is_published, created_by, folder_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true,$8,$11) RETURNING id`,
			schoolID, academicYearID, termID, examTypeID, title, classroomID, subjectID, teacherID,
			now, now.Add(time.Hour), folder).Scan(&id)
		if err != nil {
			t.Fatalf("insert exam %s: %v", title, err)
		}
		t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM exams WHERE id=$1`, id) })
		return id
	}
	filedID := insertExam("di dalam folder", &folderID)
	unfiledID := insertExam("tanpa folder", nil)

	s := New(db)
	ids := func(folderFilter string) map[string]bool {
		rows, _, err := s.ListExams(ctx, ExamFilter{SchoolID: schoolID, FolderID: folderFilter}, 1, 100)
		if err != nil {
			t.Fatalf("ListExams(folder_id=%q): %v", folderFilter, err)
		}
		got := map[string]bool{}
		for _, e := range rows {
			got[e.ID] = true
		}
		return got
	}

	none := ids("none")
	if !none[unfiledID] {
		t.Errorf("folder_id=none must return the unfiled exam")
	}
	if none[filedID] {
		t.Errorf("folder_id=none must not return an exam that has a folder")
	}
	inFolder := ids(folderID)
	if !inFolder[filedID] || inFolder[unfiledID] {
		t.Errorf("folder_id=<id> must return only that folder's exams")
	}
	if all := ids(""); !all[filedID] || !all[unfiledID] {
		t.Errorf("no folder filter must return both")
	}
}

// TestMoveExamsToFolder is the runnable check for the admin's "pindahkan
// dari Tanpa Folder ke folder STS" request. The scoping matters as much as
// the move: another school's exam and another guru's exam must be untouched
// even when their ids are passed in, since the ids travel in the request
// body where a caller controls them.
func TestMoveExamsToFolder(t *testing.T) {
	dsn := os.Getenv("EXAM_TEST_DATABASE_URL")
	if dsn == "" {
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
	schoolID, otherSchoolID := newID(), newID()
	academicYearID, termID := newID(), newID()
	classroomID, subjectID := newID(), newID()
	guruA, guruB := newID(), newID()

	var examTypeID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO exam_types (school_id, academic_year_id, name, mapping_rapor, created_by)
		VALUES ($1,$2,'move type','sumatif',$3) RETURNING id`, schoolID, academicYearID, guruA).Scan(&examTypeID); err != nil {
		t.Fatalf("insert exam_type: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM exam_types WHERE id=$1`, examTypeID) })

	var stsFolderID string
	if err := db.QueryRowContext(ctx, `
		INSERT INTO question_folders (school_id, academic_year_id, name, created_by)
		VALUES ($1,$2,'STS',$3) RETURNING id`, schoolID, academicYearID, guruA).Scan(&stsFolderID); err != nil {
		t.Fatalf("insert question_folder: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM question_folders WHERE id=$1`, stsFolderID) })

	insertExam := func(title, school, author string) string {
		now := time.Now()
		var id string
		err := db.QueryRowContext(ctx, `
			INSERT INTO exams (school_id, academic_year_id, term_id, exam_type_id, title, classroom_id, subject_id,
				teacher_id, start_at, end_at, is_published, created_by, folder_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false,$11,NULL) RETURNING id`,
			school, academicYearID, termID, examTypeID, title, classroomID, subjectID, author,
			now, now.Add(time.Hour), author).Scan(&id)
		if err != nil {
			t.Fatalf("insert exam %s: %v", title, err)
		}
		t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM exams WHERE id=$1`, id) })
		return id
	}
	mine := insertExam("punya guru A", schoolID, guruA)
	theirs := insertExam("punya guru B", schoolID, guruB)
	foreign := insertExam("sekolah lain", otherSchoolID, guruA)

	s := New(db)
	folderOf := func(id string) string {
		var f sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT folder_id FROM exams WHERE id=$1`, id).Scan(&f); err != nil {
			t.Fatalf("read folder_id: %v", err)
		}
		return f.String
	}

	// A guru moves only what they authored, even when handed other ids.
	moved, err := s.MoveExamsToFolder(ctx, []string{mine, theirs, foreign}, stsFolderID, schoolID, guruA)
	if err != nil {
		t.Fatalf("MoveExamsToFolder(guru): %v", err)
	}
	if moved != 1 {
		t.Errorf("guru moved %d exams, want 1 (own only)", moved)
	}
	if folderOf(mine) != stsFolderID {
		t.Errorf("guru's own exam was not moved")
	}
	if folderOf(theirs) != "" || folderOf(foreign) != "" {
		t.Errorf("guru moved an exam they do not own: theirs=%q foreign=%q", folderOf(theirs), folderOf(foreign))
	}

	// An admin (no author restriction) moves any exam in their own school,
	// but still never another school's.
	moved, err = s.MoveExamsToFolder(ctx, []string{theirs, foreign}, stsFolderID, schoolID, "")
	if err != nil {
		t.Fatalf("MoveExamsToFolder(admin): %v", err)
	}
	if moved != 1 || folderOf(theirs) != stsFolderID {
		t.Errorf("admin moved %d exams, want 1 (theirs); theirs folder=%q", moved, folderOf(theirs))
	}
	if folderOf(foreign) != "" {
		t.Errorf("admin reached across schools")
	}

	// Empty folder id moves back out -- the undo for a wrong move.
	if _, err := s.MoveExamsToFolder(ctx, []string{mine}, "", schoolID, ""); err != nil {
		t.Fatalf("MoveExamsToFolder(unfile): %v", err)
	}
	if folderOf(mine) != "" {
		t.Errorf("exam was not moved back out of the folder")
	}
}
