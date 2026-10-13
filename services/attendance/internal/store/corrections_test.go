package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newUUIDs(t *testing.T, db *sql.DB, ps ...*string) {
	t.Helper()
	for _, p := range ps {
		if err := db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(p); err != nil {
			t.Fatal(err)
		}
	}
}

func auditActions(t *testing.T, db *sql.DB, correlation string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT action FROM audit_outbox WHERE correlation_id = $1 ORDER BY id`, correlation)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		out = append(out, a)
	}
	return out
}

// BR-ATT-01 / AC-41-02: approved correction changes the locked row, is
// idempotent on callback retry, and both request + decision hit the audit
// outbox.
func TestApplyCorrectionDecisionApproved(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := New(db)
	var school, ay, cls, guru, student, admin, approver string
	newUUIDs(t, db, &school, &ay, &cls, &guru, &student, &admin, &approver)
	date := "2026-10-06"
	var sessionID string
	if err := db.QueryRow(`INSERT INTO class_sessions (school_id, academic_year_id, schedule_id, classroom_id, guru_id, session_date)
		VALUES ($1,$2,gen_random_uuid(),$3,$4,$5) RETURNING id`, school, ay, cls, guru, date).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO student_attendances (class_session_id, school_id, academic_year_id, classroom_id, student_id, status, session_date)
		VALUES ($1,$2,$3,$4,$5,'tanpa_keterangan',$6)`, sessionID, school, ay, cls, student, date); err != nil {
		t.Fatal(err)
	}
	from := "tanpa_keterangan"
	note := "surat dokter menyusul"
	c, err := s.CreateCorrection(ctx, CreateCorrectionInput{SchoolID: school, AcademicYearID: ay, ClassroomID: cls, StudentID: student,
		SessionDate: date, FromStatus: &from, ToStatus: "sakit", Catatan: &note, Reason: "orang tua lupa lapor", RequestedBy: admin, RequestedRole: "walikelas"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != "pending" {
		t.Fatalf("status = %s, want pending", c.Status)
	}
	// A second pending correction for the same student/day is refused.
	if _, err := s.CreateCorrection(ctx, CreateCorrectionInput{SchoolID: school, AcademicYearID: ay, ClassroomID: cls, StudentID: student,
		SessionDate: date, ToStatus: "izin", Reason: "x", RequestedBy: admin, RequestedRole: "admin"}); err != ErrCorrectionPending {
		t.Fatalf("duplicate pending err = %v, want ErrCorrectionPending", err)
	}

	got, changed, err := s.ApplyCorrectionDecision(ctx, CorrectionDecision{CorrectionID: c.ID, ApprovalID: "appr-1", Decision: "approved", DecidedBy: approver, Comment: "ok"})
	if err != nil || !changed {
		t.Fatalf("apply: changed=%v err=%v", changed, err)
	}
	if got.Status != "approved" || got.AppliedAt == nil {
		t.Fatalf("correction = %+v, want approved+applied", got)
	}
	sa, err := s.GetStudentAttendanceOn(ctx, cls, date, student)
	if err != nil {
		t.Fatal(err)
	}
	if sa.Status != "sakit" || sa.Catatan == nil || *sa.Catatan != note {
		t.Fatalf("attendance = %s/%v, want sakit with catatan", sa.Status, sa.Catatan)
	}
	// Retry of the same callback: no change, no extra audit.
	_, changed, err = s.ApplyCorrectionDecision(ctx, CorrectionDecision{CorrectionID: c.ID, Decision: "approved", DecidedBy: approver})
	if err != nil || changed {
		t.Fatalf("retry: changed=%v err=%v, want unchanged", changed, err)
	}
	acts := auditActions(t, db, "attendance_correction:"+c.ID)
	if len(acts) != 2 || acts[0] != "attendance_correction.requested" || acts[1] != "attendance_correction.approved" {
		t.Fatalf("audit actions = %v", acts)
	}
	var before, after string
	_ = db.QueryRow(`SELECT before->>'status', after->>'status' FROM audit_outbox WHERE correlation_id = $1 AND action = 'attendance_correction.approved'`,
		"attendance_correction:"+c.ID).Scan(&before, &after)
	if before != "tanpa_keterangan" || after != "sakit" {
		t.Fatalf("audit before/after = %s/%s", before, after)
	}
}

func TestApplyCorrectionDecisionRejectedAndMissingRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := New(db)
	var school, ay, cls, student, admin string
	newUUIDs(t, db, &school, &ay, &cls, &student, &admin)
	date := "2026-10-07"

	// Rejected: nothing written to student_attendances.
	c, err := s.CreateCorrection(ctx, CreateCorrectionInput{SchoolID: school, AcademicYearID: ay, ClassroomID: cls, StudentID: student,
		SessionDate: date, ToStatus: "hadir", Reason: "r", RequestedBy: admin, RequestedRole: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, changed, err := s.ApplyCorrectionDecision(ctx, CorrectionDecision{CorrectionID: c.ID, Decision: "rejected", DecidedBy: "not-a-uuid", Comment: "tidak valid"}); err != nil || !changed {
		t.Fatalf("reject: changed=%v err=%v", changed, err)
	}
	if _, err := s.GetStudentAttendanceOn(ctx, cls, date, student); err != ErrNotFound {
		t.Fatalf("rejected correction wrote attendance (err=%v)", err)
	}

	// Approved with no existing row: creates a manual session + the row.
	c2, err := s.CreateCorrection(ctx, CreateCorrectionInput{SchoolID: school, AcademicYearID: ay, ClassroomID: cls, StudentID: student,
		SessionDate: date, ToStatus: "terlambat", Reason: "r2", RequestedBy: admin, RequestedRole: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ApplyCorrectionDecision(ctx, CorrectionDecision{CorrectionID: c2.ID, Decision: "approved"}); err != nil {
		t.Fatal(err)
	}
	sa, err := s.GetStudentAttendanceOn(ctx, cls, date, student)
	if err != nil || sa.Status != "terlambat" {
		t.Fatalf("attendance = %+v err=%v, want terlambat", sa, err)
	}
	var source string
	_ = db.QueryRow(`SELECT source FROM class_sessions WHERE id = $1`, sa.ClassSessionID).Scan(&source)
	if source != "manual" {
		t.Fatalf("session source = %q, want manual", source)
	}
	if _, _, err := s.ApplyCorrectionDecision(ctx, CorrectionDecision{CorrectionID: "00000000-0000-0000-0000-000000000000", Decision: "approved"}); err != ErrNotFound {
		t.Fatalf("unknown correction err = %v, want ErrNotFound", err)
	}
}

// FR-41-004: one alpa notification claim per student per day.
func TestClaimAlpaNotificationDedupe(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := New(db)
	var school, student string
	newUUIDs(t, db, &school, &student)
	ok1, err := s.ClaimAlpaNotification(ctx, school, student, "2026-10-06")
	if err != nil || !ok1 {
		t.Fatalf("first claim = %v, %v", ok1, err)
	}
	ok2, _ := s.ClaimAlpaNotification(ctx, school, student, "2026-10-06")
	if ok2 {
		t.Fatal("second claim for same student/day must be refused")
	}
	ok3, _ := s.ClaimAlpaNotification(ctx, school, student, "2026-10-07")
	if !ok3 {
		t.Fatal("other day must be claimable")
	}
	_ = s.ReleaseAlpaNotification(ctx, student, "2026-10-06")
	if ok4, _ := s.ClaimAlpaNotification(ctx, school, student, "2026-10-06"); !ok4 {
		t.Fatal("released claim must be claimable again")
	}
}

// FR-42-002: leave approve/reject writes a core audit event (outbox).
func TestReviewLeaveRequestWritesAudit(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := New(db)
	var school, student, parent, reviewer string
	newUUIDs(t, db, &school, &student, &parent, &reviewer)
	l, err := s.CreateLeaveRequest(ctx, CreateLeaveInput{SchoolID: school, StudentID: student, RequestedBy: parent,
		Type: "izin", StartDate: "2026-10-07", EndDate: "2026-10-07", Reason: "acara keluarga"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReviewLeaveRequest(ctx, l.ID, "approved", reviewer, "walikelas", ""); err != nil {
		t.Fatal(err)
	}
	var action, role, after string
	if err := db.QueryRow(`SELECT action, actor_role, after->>'status' FROM audit_outbox WHERE correlation_id = $1`, "leave:"+l.ID).
		Scan(&action, &role, &after); err != nil {
		t.Fatal(err)
	}
	if action != "leave.approved" || role != "walikelas" || after != "approved" {
		t.Fatalf("audit = %s/%s/%s", action, role, after)
	}
}

func TestLockSettingsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	s := New(db)
	var school, admin string
	newUUIDs(t, db, &school, &admin)
	def, err := s.GetLockSettings(ctx, school)
	if err != nil || def.Enabled {
		t.Fatalf("default = %+v err=%v, want disabled", def, err)
	}
	def.Enabled, def.CutoffDays, def.CutoffTime = true, 1, "08:30"
	got, err := s.UpsertLockSettings(ctx, *def, admin, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.CutoffDays != 1 || got.CutoffTime != "08:30" || got.UpdatedBy == nil {
		t.Fatalf("saved = %+v", got)
	}
}
