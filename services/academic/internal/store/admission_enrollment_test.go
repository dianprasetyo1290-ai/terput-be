package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"skci/academic/internal/model"
)

// academicTestDB follows the repo-wide convention exam-service's store tests
// use (see services/exam/internal/store/admission_test.go): run against
// ACADEMIC_TEST_DATABASE_URL, else the docker-compose dev Postgres on :5482,
// and skip (never fail) when no database or schema is reachable. That is the
// only way to check the real Postgres `ON CONFLICT (academic_year_id,
// student_user_id)` behavior — the in-process fake in
// internal/handler/admission_enrollment_test.go reimplements the query, not
// the constraint.
func academicTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ACADEMIC_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://sekoci:change-me@localhost:5482/skci_academic?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("skipping: could not open %s: %v", dsn, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dsn, err)
	}
	t.Cleanup(func() { db.Close() })

	var reg *string
	if err := db.QueryRow(`SELECT to_regclass('public.class_enrollments')::text`).Scan(&reg); err != nil || reg == nil {
		t.Skipf("skipping: class_enrollments table not present (migrations not applied): %v", err)
	}
	return db
}

func newUUID(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRowContext(ctx, `SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("gen_random_uuid: %v", err)
	}
	return id
}

// TestUpsertAdmissionEnrollmentIdempotentDB proves the real upsert: two calls
// for the same (academic_year_id, student_user_id) leave exactly one row, and
// the second call re-points it at the new classroom with status 'active'.
func TestUpsertAdmissionEnrollmentIdempotentDB(t *testing.T) {
	ctx := context.Background()
	db := academicTestDB(t)
	s := New(db)

	schoolID := newUUID(t, ctx, db)
	yearID := newUUID(t, ctx, db)
	classA := newUUID(t, ctx, db)
	classB := newUUID(t, ctx, db)
	studentID := newUUID(t, ctx, db)

	for _, classID := range []string{classA, classB} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO classrooms (id, school_id, academic_year_id, name) VALUES ($1, $2, $3, 'Test Class')`,
			classID, schoolID, yearID); err != nil {
			t.Fatalf("seed classroom: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id = $1`, schoolID)
		_, _ = db.ExecContext(ctx, `DELETE FROM classrooms WHERE school_id = $1`, schoolID)
	})

	first, err := s.UpsertAdmissionEnrollment(ctx, model.ClassEnrollment{
		SchoolID: schoolID, AcademicYearID: yearID, ClassroomID: classA, StudentUserID: studentID,
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.ClassroomID != classA || first.Status != "active" {
		t.Fatalf("first upsert = %+v, want classroom %s active", first, classA)
	}

	second, err := s.UpsertAdmissionEnrollment(ctx, model.ClassEnrollment{
		SchoolID: schoolID, AcademicYearID: yearID, ClassroomID: classB, StudentUserID: studentID,
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotency broken: id %s -> %s", first.ID, second.ID)
	}
	if second.ClassroomID != classB || second.Status != "active" {
		t.Fatalf("second upsert = %+v, want classroom %s active", second, classB)
	}

	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM class_enrollments WHERE school_id = $1 AND academic_year_id = $2 AND student_user_id = $3`,
		schoolID, yearID, studentID).Scan(&count); err != nil {
		t.Fatalf("count enrollments: %v", err)
	}
	if count != 1 {
		t.Fatalf("got %d enrollment rows, want 1", count)
	}
}

// TestGetClassroomIsSchoolScopedDB pins the tenant boundary the handler relies
// on to reject another school's classroom: a classroom of a different school
// resolves to (nil, nil), which the handler turns into a 404.
func TestGetClassroomIsSchoolScopedDB(t *testing.T) {
	ctx := context.Background()
	db := academicTestDB(t)
	s := New(db)

	schoolA := newUUID(t, ctx, db)
	schoolB := newUUID(t, ctx, db)
	yearID := newUUID(t, ctx, db)
	classA := newUUID(t, ctx, db)

	if _, err := db.ExecContext(ctx,
		`INSERT INTO classrooms (id, school_id, academic_year_id, name) VALUES ($1, $2, $3, 'Test Class')`,
		classA, schoolA, yearID); err != nil {
		t.Fatalf("seed classroom: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM classrooms WHERE school_id = $1`, schoolA) })

	ok, err := s.GetClassroom(ctx, schoolA, classA)
	if err != nil {
		t.Fatalf("get own classroom: %v", err)
	}
	if ok == nil {
		t.Fatalf("own classroom not found")
	}

	foreign, err := s.GetClassroom(ctx, schoolB, classA)
	if err != nil {
		t.Fatalf("get foreign classroom: %v", err)
	}
	if foreign != nil {
		t.Fatalf("classroom leaked across schools: %+v", foreign)
	}
}
