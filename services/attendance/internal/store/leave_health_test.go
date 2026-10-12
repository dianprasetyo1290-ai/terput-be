package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestApplyHealthAbsence needs a migrated attendance DB (TEST_PG_DSN).
func TestApplyHealthAbsence(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	s := New(db)
	var school, ay, cls, guru, student, other, actor string
	for _, p := range []*string{&school, &ay, &cls, &guru, &student, &other, &actor} {
		if err := db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
	date := "2026-10-12"
	var sessionID string
	if err := db.QueryRow(`INSERT INTO class_sessions (school_id, academic_year_id, schedule_id, classroom_id, guru_id, session_date)
		VALUES ($1,$2,gen_random_uuid(),$3,$4,$5) RETURNING id`, school, ay, cls, guru, date).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{student, other} {
		if _, err := db.Exec(`INSERT INTO student_attendances (class_session_id, school_id, academic_year_id, classroom_id, student_id, status, session_date)
			VALUES ($1,$2,$3,$4,$5,'hadir',$6)`, sessionID, school, ay, cls, sid, date); err != nil {
			t.Fatal(err)
		}
	}
	since := time.Date(2026, 10, 12, 3, 15, 0, 0, time.UTC)
	visit := func() string {
		var id string
		_ = db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&id)
		return id
	}
	ref := visit()
	in := HealthAbsenceInput{SchoolID: school, StudentID: student, Date: date, SourceRef: ref, ActorUserID: actor,
		Reason: "Dipulangkan dari UKS pukul 10.15 WIB", ClassroomID: cls, SickSince: since}

	applied, _, err := s.ApplyHealthAbsence(ctx, in)
	if err != nil || !applied {
		t.Fatalf("first apply: applied=%v err=%v", applied, err)
	}
	var status, catatan string
	if err := db.QueryRow(`SELECT status, catatan FROM student_attendances WHERE student_id = $1 AND session_date = $2`, student, date).Scan(&status, &catatan); err != nil {
		t.Fatal(err)
	}
	if status != "sakit" || catatan != in.Reason {
		t.Fatalf("attendance row not updated: %s %q", status, catatan)
	}
	var otherStatus string
	_ = db.QueryRow(`SELECT status FROM student_attendances WHERE student_id = $1`, other).Scan(&otherStatus)
	if otherStatus != "hadir" {
		t.Fatalf("other student touched: %s", otherStatus)
	}

	// Idempotent: same source_ref → applied, still one leave row.
	applied, _, err = s.ApplyHealthAbsence(ctx, in)
	if err != nil || !applied {
		t.Fatalf("repeat apply: applied=%v err=%v", applied, err)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM leave_requests WHERE student_id = $1 AND source = 'uks'`, student).Scan(&n)
	if n != 1 {
		t.Fatalf("leave rows = %d, want 1", n)
	}
	l, err := s.ListLeaveRequests(ctx, LeaveFilter{SchoolID: school, StudentID: student})
	if err != nil || len(l) != 1 || l[0].Source != LeaveSourceUKS || l[0].SickSince == nil || l[0].Status != "approved" || l[0].Type != "sakit" {
		t.Fatalf("listed leave: %+v err=%v", l, err)
	}

	// Another visit the same day: day already covered → not overridden.
	in2 := in
	in2.SourceRef = visit()
	applied, why, err := s.ApplyHealthAbsence(ctx, in2)
	if err != nil || applied || why == "" {
		t.Fatalf("covered day: applied=%v why=%q err=%v", applied, why, err)
	}

	// A parent-approved izin on the day is never overridden.
	if _, err := db.Exec(`INSERT INTO leave_requests (school_id, student_id, requested_by, type, start_date, end_date, reason, status)
		VALUES ($1,$2,$3,'izin',$4,$4,'acara keluarga','approved')`, school, other, actor, date); err != nil {
		t.Fatal(err)
	}
	in3 := in
	in3.StudentID, in3.SourceRef = other, visit()
	if applied, _, err := s.ApplyHealthAbsence(ctx, in3); err != nil || applied {
		t.Fatalf("approved izin overridden: applied=%v err=%v", applied, err)
	}

	// Sessions seeded later pick up the UKS sakit via ApprovedLeaveTypes.
	nextDay := "2026-10-13"
	in4 := in
	in4.Date, in4.SourceRef = nextDay, visit()
	if applied, _, err := s.ApplyHealthAbsence(ctx, in4); err != nil || !applied {
		t.Fatalf("no-row day: applied=%v err=%v", applied, err)
	}
	m, err := s.ApprovedLeaveTypes(ctx, school, nextDay, []string{student})
	if err != nil || m[student].Status != "sakit" || m[student].Catatan != in.Reason {
		t.Fatalf("ApprovedLeaveTypes: %+v err=%v", m, err)
	}
}
