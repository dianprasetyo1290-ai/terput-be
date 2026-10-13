package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"skci/academic/internal/model"
)

// PRD menus 39-40 (FR-39-001..003, FR-40-002, AC-39-01, AC-40-01) against a
// real Postgres with migrations applied (see academicTestDB: skipped, never
// failed, when no database is reachable).

type cmFixture struct {
	ctx      context.Context
	db       *sql.DB
	s        *Store
	schoolID string
	yearID   string
}

func newCMFixture(t *testing.T) *cmFixture {
	t.Helper()
	ctx := context.Background()
	db := academicTestDB(t)
	var reg *string
	if err := db.QueryRow(`SELECT to_regclass('public.class_enrollment_history')::text`).Scan(&reg); err != nil || reg == nil {
		t.Skipf("skipping: migration 0006 not applied: %v", err)
	}
	f := &cmFixture{ctx: ctx, db: db, s: New(db), schoolID: newUUID(t, ctx, db), yearID: newUUID(t, ctx, db)}
	t.Cleanup(func() {
		for _, q := range []string{
			`DELETE FROM schedules WHERE school_id = $1`,
			`DELETE FROM schedule_periods WHERE school_id = $1`,
			`DELETE FROM classroom_subjects WHERE school_id = $1`,
			`DELETE FROM subjects WHERE school_id = $1`,
			`DELETE FROM class_enrollments WHERE school_id = $1`,
			`DELETE FROM class_enrollment_history WHERE school_id = $1`,
			`DELETE FROM classrooms WHERE school_id = $1`,
		} {
			_, _ = db.ExecContext(ctx, q, f.schoolID)
		}
	})
	return f
}

func (f *cmFixture) classroom(t *testing.T, name string, capacity *int, year string) *model.Classroom {
	t.Helper()
	if year == "" {
		year = f.yearID
	}
	c, err := f.s.CreateClassroom(f.ctx, model.Classroom{SchoolID: f.schoolID, AcademicYearID: year, Name: name, Capacity: capacity})
	if err != nil {
		t.Fatalf("create classroom %s: %v", name, err)
	}
	return c
}

func (f *cmFixture) enroll(classroom *model.Classroom, student, actor string) error {
	_, err := f.s.AddClassEnrollment(f.ctx, model.ClassEnrollment{
		SchoolID: f.schoolID, AcademicYearID: classroom.AcademicYearID, ClassroomID: classroom.ID, StudentUserID: student, Status: "active",
	}, EnrollmentChange{ActorUserID: actor})
	return err
}

func intp(v int) *int { return &v }

// AC-39-01: assigning another guru as wali kelas replaces the previous one —
// the class still has exactly one wali kelas.
func TestHomeroomExactlyOnePerClassDB(t *testing.T) {
	f := newCMFixture(t)
	guruA, guruB := newUUID(t, f.ctx, f.db), newUUID(t, f.ctx, f.db)
	c, err := f.s.CreateClassroom(f.ctx, model.Classroom{SchoolID: f.schoolID, AcademicYearID: f.yearID, Name: "7A", HomeroomTeacherUserID: &guruA})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := f.s.UpdateClassroom(f.ctx, f.schoolID, c.ID, ClassroomPatch{HomeroomTeacherUserID: &guruB})
	if err != nil || updated == nil {
		t.Fatalf("update wali kelas: %v", err)
	}
	if updated.HomeroomTeacherUserID == nil || *updated.HomeroomTeacherUserID != guruB {
		t.Fatalf("wali kelas = %v, want %s", updated.HomeroomTeacherUserID, guruB)
	}
	wali, err := f.s.ListWaliKelas(f.ctx, f.schoolID, f.yearID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, w := range wali {
		if w.ClassroomID == c.ID {
			count++
			if w.HomeroomTeacherUserID != guruB {
				t.Fatalf("wali kelas row = %s, want %s", w.HomeroomTeacherUserID, guruB)
			}
		}
	}
	if count != 1 {
		t.Fatalf("class 7A has %d wali kelas rows, want exactly 1", count)
	}
}

// FR-39-001: capacity blocks over-enrollment with a clear message; re-adding
// an already-enrolled student is idempotent; capacity cannot drop below the
// current roster; 0 clears the limit.
func TestClassroomCapacityDB(t *testing.T) {
	f := newCMFixture(t)
	c := f.classroom(t, "8A", intp(2), "")
	s1, s2, s3 := newUUID(t, f.ctx, f.db), newUUID(t, f.ctx, f.db), newUUID(t, f.ctx, f.db)
	for _, s := range []string{s1, s2} {
		if err := f.enroll(c, s, ""); err != nil {
			t.Fatalf("enroll within capacity: %v", err)
		}
	}
	err := f.enroll(c, s3, "")
	var ce *CapacityError
	if !errors.As(err, &ce) {
		t.Fatalf("enroll over capacity err = %v, want *CapacityError", err)
	}
	if ce.Capacity != 2 || ce.Enrolled != 2 {
		t.Fatalf("capacity error = %+v", ce)
	}
	if want := "Kelas 8A sudah penuh (2/2 siswa). Naikkan kapasitas kelas atau pilih kelas lain."; ce.Error() != want {
		t.Fatalf("message = %q, want %q", ce.Error(), want)
	}
	if err := f.enroll(c, s1, ""); err != nil {
		t.Fatalf("re-adding enrolled student must not count twice: %v", err)
	}
	if _, err := f.s.UpdateClassroom(f.ctx, f.schoolID, c.ID, ClassroomPatch{Capacity: intp(1)}); !errors.As(err, &ce) || !ce.Lowering {
		t.Fatalf("lowering capacity below roster err = %v, want lowering *CapacityError", err)
	}
	got, err := f.s.UpdateClassroom(f.ctx, f.schoolID, c.ID, ClassroomPatch{Capacity: intp(0)})
	if err != nil || got.Capacity != nil {
		t.Fatalf("clear capacity: %v / %v", err, got)
	}
	if err := f.enroll(c, s3, ""); err != nil {
		t.Fatalf("enroll after clearing capacity: %v", err)
	}
	got, _ = f.s.GetClassroom(f.ctx, f.schoolID, c.ID)
	if got.StudentCount != 3 {
		t.Fatalf("student_count = %d, want 3", got.StudentCount)
	}
}

// FR-39-003: add, class transfer (Menu 78 apply) and remove each leave a
// history row; the transfer no longer loses the source classroom.
func TestEnrollmentHistoryAddTransferRemoveDB(t *testing.T) {
	f := newCMFixture(t)
	a := f.classroom(t, "9A", nil, "")
	b := f.classroom(t, "9B", nil, "")
	full := f.classroom(t, "9C", intp(1), "")
	student, other, actor := newUUID(t, f.ctx, f.db), newUUID(t, f.ctx, f.db), newUUID(t, f.ctx, f.db)
	requestID := newUUID(t, f.ctx, f.db)

	if err := f.enroll(a, student, actor); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyClassTransfer(f.ctx, f.schoolID, student, a.ID, b.ID, EnrollmentChange{RequestID: requestID, Reason: "Pindah jurusan", ActorUserID: actor}); err != nil {
		t.Fatalf("apply transfer: %v", err)
	}
	// Transfer into a full class is refused and writes nothing.
	if err := f.enroll(full, other, ""); err != nil {
		t.Fatal(err)
	}
	var ce *CapacityError
	if err := f.s.ApplyClassTransfer(f.ctx, f.schoolID, student, b.ID, full.ID, EnrollmentChange{}); !errors.As(err, &ce) {
		t.Fatalf("transfer into full class err = %v, want *CapacityError", err)
	}
	if err := f.s.RemoveClassEnrollment(f.ctx, f.schoolID, b.ID, student, EnrollmentChange{ActorUserID: actor}); err != nil {
		t.Fatal(err)
	}

	hist, err := f.s.ListStudentEnrollmentHistory(f.ctx, f.schoolID, student)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 3 {
		t.Fatalf("history len = %d, want 3: %+v", len(hist), hist)
	}
	// newest first: remove, transfer, enroll
	rm, tr, en := hist[0], hist[1], hist[2]
	if en.Action != ActionEnroll || en.FromClassroomID != nil || *en.ToClassroomID != a.ID || en.ActorUserID == nil || *en.ActorUserID != actor {
		t.Fatalf("enroll entry = %+v", en)
	}
	if tr.Action != ActionTransfer || *tr.FromClassroomID != a.ID || *tr.ToClassroomID != b.ID ||
		tr.RequestID == nil || *tr.RequestID != requestID || tr.Reason == nil || *tr.Reason != "Pindah jurusan" {
		t.Fatalf("transfer entry = %+v", tr)
	}
	if tr.FromClassroomName == nil || *tr.FromClassroomName != "9A" || *tr.ToClassroomName != "9B" {
		t.Fatalf("transfer names = %v -> %v", tr.FromClassroomName, tr.ToClassroomName)
	}
	if rm.Action != ActionRemove || *rm.FromClassroomID != b.ID || rm.ToClassroomID != nil {
		t.Fatalf("remove entry = %+v", rm)
	}

	classA, err := f.s.ListClassroomEnrollmentHistory(f.ctx, f.schoolID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(classA) != 2 { // enroll into A + transfer out of A
		t.Fatalf("classroom A history len = %d, want 2", len(classA))
	}
}

// Withdrawal, promotion and an untagged direct move are logged too.
func TestEnrollmentHistoryOtherWritersDB(t *testing.T) {
	f := newCMFixture(t)
	nextYear := newUUID(t, f.ctx, f.db)
	a := f.classroom(t, "10A", nil, "")
	b := f.classroom(t, "10B", nil, "")
	next := f.classroom(t, "11A", intp(1), nextYear)
	s1, s2 := newUUID(t, f.ctx, f.db), newUUID(t, f.ctx, f.db)
	_ = f.enroll(a, s1, "")
	_ = f.enroll(a, s2, "")

	// Direct move via add into another class of the same year.
	if err := f.enroll(b, s2, ""); err != nil {
		t.Fatal(err)
	}
	// Promotion over capacity is refused atomically.
	_, err := f.s.PromoteStudents(f.ctx, f.schoolID, f.yearID, nextYear, []StudentPromotion{
		{StudentUserID: s1, FromClassroomID: a.ID, ToClassroomID: &next.ID},
		{StudentUserID: s2, FromClassroomID: b.ID, ToClassroomID: &next.ID},
	}, "")
	var ce *CapacityError
	if !errors.As(err, &ce) {
		t.Fatalf("promotion over capacity err = %v, want *CapacityError", err)
	}
	if _, err := f.s.PromoteStudents(f.ctx, f.schoolID, f.yearID, nextYear, []StudentPromotion{
		{StudentUserID: s1, FromClassroomID: a.ID, ToClassroomID: &next.ID},
	}, ""); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyWithdrawal(f.ctx, f.schoolID, s2, EnrollmentChange{Reason: "Pindah kota"}); err != nil {
		t.Fatal(err)
	}

	actions := func(student string) []string {
		h, err := f.s.ListStudentEnrollmentHistory(f.ctx, f.schoolID, student)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for i := len(h) - 1; i >= 0; i-- {
			out = append(out, h[i].Action)
		}
		return out
	}
	if got := actions(s1); len(got) != 2 || got[0] != ActionEnroll || got[1] != ActionPromotion {
		t.Fatalf("s1 actions = %v, want [enroll promotion]", got)
	}
	if got := actions(s2); len(got) != 3 || got[0] != ActionEnroll || got[1] != ActionMove || got[2] != ActionWithdrawal {
		t.Fatalf("s2 actions = %v, want [enroll move withdrawal]", got)
	}
}

// FR-40-002 / AC-40-01: hours per guru summed across classes, and kelas x
// mapel pairings without a guru are reported as unassigned.
func TestTeachingWorkloadDB(t *testing.T) {
	f := newCMFixture(t)
	c8a := f.classroom(t, "8A", nil, "")
	c8b := f.classroom(t, "8B", nil, "")
	guruT, guruU := newUUID(t, f.ctx, f.db), newUUID(t, f.ctx, f.db)
	subj := func(name string) string {
		var id string
		if err := f.db.QueryRowContext(f.ctx, `INSERT INTO subjects (school_id, name) VALUES ($1, $2) RETURNING id`, f.schoolID, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mat, ipa, seni := subj("Matematika"), subj("IPA"), subj("Seni")
	pair := func(c *model.Classroom, subjectID string, teacher *string, hours int, active bool) *model.ClassroomSubject {
		cs, err := f.s.AddClassroomSubject(f.ctx, model.ClassroomSubject{
			SchoolID: f.schoolID, AcademicYearID: f.yearID, ClassroomID: c.ID, SubjectID: subjectID, TeacherUserID: teacher, IsActive: active,
		}, &hours)
		if err != nil {
			t.Fatal(err)
		}
		return cs
	}
	mat8a := pair(c8a, mat, &guruT, 4, true)
	pair(c8a, ipa, &guruT, 3, true)
	pair(c8b, mat, nil, 4, true)    // AC-40-01: 8B has no Matematika guru
	pair(c8b, ipa, &guruU, 3, true) //
	pair(c8b, seni, nil, 2, false)  // inactive in matrix: ignored

	// One jadwal slot for Matematika 8A.
	var periodID string
	if err := f.db.QueryRowContext(f.ctx, `INSERT INTO schedule_periods (school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time)
		VALUES ($1, $2, $3, 1, 1, '07:00', '07:45') RETURNING id`, f.schoolID, f.yearID, c8a.ID).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(f.ctx, `INSERT INTO schedules (school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id) VALUES ($1, $2, $3, $4, $5)`,
		f.schoolID, f.yearID, c8a.ID, periodID, mat8a.ID); err != nil {
		t.Fatal(err)
	}

	rep, err := f.s.TeachingWorkload(f.ctx, f.schoolID, f.yearID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Summary.TotalPairs != 4 || rep.Summary.AssignedPairs != 3 || rep.Summary.UnassignedPairs != 1 ||
		rep.Summary.TotalHours != 14 || rep.Summary.UnassignedHours != 4 || rep.Summary.TeacherCount != 2 {
		t.Fatalf("summary = %+v", rep.Summary)
	}
	if len(rep.Teachers) != 2 {
		t.Fatalf("teachers = %+v", rep.Teachers)
	}
	top := rep.Teachers[0]
	if top.TeacherUserID != guruT || top.HoursPerWeek != 7 || top.ClassCount != 1 || top.SubjectCount != 2 || top.ScheduledPeriods != 1 || len(top.Assignments) != 2 {
		t.Fatalf("guru T workload = %+v", top)
	}
	if rep.Teachers[1].TeacherUserID != guruU || rep.Teachers[1].HoursPerWeek != 3 {
		t.Fatalf("guru U workload = %+v", rep.Teachers[1])
	}
	if len(rep.Unassigned) != 1 || rep.Unassigned[0].ClassroomName != "8B" || rep.Unassigned[0].SubjectName != "Matematika" {
		t.Fatalf("unassigned = %+v, want [8B Matematika]", rep.Unassigned)
	}

	// Un-assigning a guru makes the pairing show up as unassigned.
	if _, err := f.s.UpdateClassroomSubject(f.ctx, f.schoolID, mat8a.ID, nil, nil, nil, true); err != nil {
		t.Fatal(err)
	}
	rep, _ = f.s.TeachingWorkload(f.ctx, f.schoolID, f.yearID)
	if rep.Summary.UnassignedPairs != 2 || rep.Teachers[0].HoursPerWeek != 3 {
		t.Fatalf("after unassign: summary = %+v teachers = %+v", rep.Summary, rep.Teachers)
	}

}
