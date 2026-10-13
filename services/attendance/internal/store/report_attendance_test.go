package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"skci/attendance/internal/model"
)

func row(school, class, student string, c model.AttendanceCounts) model.AttendanceRecapRow {
	r := model.AttendanceRecapRow{Scope: "student", SchoolID: school, ClassroomID: class, StudentID: student, ClassroomName: class, SchoolName: school, Counts: c}
	FinalizeCounts(&r)
	return r
}

// AC-43-01: Dani hadir 17, terlambat 1, sakit 1, alpa 1 in October.
func TestComputePercentagesAC4301(t *testing.T) {
	c := model.AttendanceCounts{Hadir: 17, Terlambat: 1, Sakit: 1, TanpaKeterangan: 1, BelumPresensi: 3}
	p := ComputePercentages(c)
	if got := reportCountedTotal(c); got != 20 {
		t.Fatalf("total = %d, want 20 (belum_presensi excluded)", got)
	}
	want := model.AttendancePercentages{Hadir: 85, Terlambat: 5, Sakit: 5, Izin: 0, TanpaKeterangan: 5, Kehadiran: 90, TidakHadir: 10}
	if p != want {
		t.Fatalf("percent = %+v, want %+v", p, want)
	}
}

func TestComputePercentagesRoundingAndZero(t *testing.T) {
	if p := ComputePercentages(model.AttendanceCounts{BelumPresensi: 5}); p != (model.AttendancePercentages{}) {
		t.Fatalf("all-unmarked should be zero percentages, got %+v", p)
	}
	p := ComputePercentages(model.AttendanceCounts{Hadir: 2, Izin: 1})
	if p.Hadir != 66.7 || p.Izin != 33.3 || p.Kehadiran != 66.7 || p.TidakHadir != 33.3 {
		t.Fatalf("rounding: %+v", p)
	}
}

func TestRollupRecapClassAndUnit(t *testing.T) {
	rows := []model.AttendanceRecapRow{
		row("S1", "X-A", "a", model.AttendanceCounts{Hadir: 9, Sakit: 1}),
		row("S1", "X-A", "b", model.AttendanceCounts{Hadir: 8, Terlambat: 2}),
		row("S1", "X-B", "c", model.AttendanceCounts{Hadir: 5, TanpaKeterangan: 5}),
		row("S2", "VII-A", "d", model.AttendanceCounts{Hadir: 10}),
	}
	classes := RollupRecap(rows, "class")
	if len(classes) != 3 {
		t.Fatalf("classes = %d, want 3", len(classes))
	}
	xa := classes[0]
	if xa.ClassroomID != "X-A" || xa.Students != 2 || xa.Counts.Hadir != 17 || xa.Counts.Terlambat != 2 || xa.Counts.Total != 20 {
		t.Fatalf("X-A rollup wrong: %+v", xa)
	}
	if xa.Percent.Kehadiran != 95 || xa.Percent.Hadir != 85 {
		t.Fatalf("X-A percent wrong: %+v", xa.Percent)
	}
	units := RollupRecap(rows, "unit")
	if len(units) != 2 {
		t.Fatalf("units = %d, want 2", len(units))
	}
	s1 := units[0]
	if s1.SchoolID != "S1" || s1.Students != 3 || s1.Counts.Total != 30 || s1.Counts.Hadir != 22 {
		t.Fatalf("S1 unit rollup wrong: %+v", s1)
	}
	if s1.Percent.Kehadiran != 80 || s1.Percent.TanpaKeterangan != 16.7 {
		t.Fatalf("S1 unit percent wrong: %+v", s1.Percent)
	}
	if s1.ClassroomID != "" || s1.StudentID != "" {
		t.Fatalf("unit row must not carry class/student ids: %+v", s1)
	}
	if got := RollupRecap(rows, "student"); len(got) != 4 {
		t.Fatalf("student scope should pass rows through")
	}
	sum := SumCounts(rows, "summary")
	if sum.Students != 4 || sum.Counts.Total != 40 {
		t.Fatalf("summary wrong: %+v", sum)
	}
}

func subj(subject, class, student string, sessions int, c model.AttendanceCounts) model.AttendanceRecapRow {
	r := row("S1", class, student, c)
	r.Scope, r.SubjectID, r.SubjectName, r.Sessions = "subject", subject, subject, sessions
	return r
}

func TestRollupSubject(t *testing.T) {
	rows := []model.AttendanceRecapRow{
		subj("MTK", "X-A", "a", 4, model.AttendanceCounts{Hadir: 4}),
		subj("MTK", "X-A", "b", 4, model.AttendanceCounts{Hadir: 2, Sakit: 2}),
		subj("MTK", "X-B", "c", 3, model.AttendanceCounts{Hadir: 3}),
		subj("BIN", "X-A", "a", 2, model.AttendanceCounts{Hadir: 1, Izin: 1}),
	}
	byClass := RollupSubject(rows, "class")
	if len(byClass) != 3 {
		t.Fatalf("subject×class rows = %d, want 3", len(byClass))
	}
	mtkXA := byClass[0]
	if mtkXA.SubjectID != "MTK" || mtkXA.ClassroomID != "X-A" || mtkXA.Students != 2 || mtkXA.Sessions != 4 || mtkXA.Counts.Total != 8 || mtkXA.Percent.Hadir != 75 {
		t.Fatalf("MTK X-A wrong: %+v", mtkXA)
	}
	bySubject := RollupSubject(rows, "subject")
	if len(bySubject) != 2 {
		t.Fatalf("subject rows = %d, want 2", len(bySubject))
	}
	mtk := bySubject[0]
	if mtk.SubjectID != "MTK" || mtk.Students != 3 || mtk.Sessions != 7 || mtk.Counts.Total != 11 || mtk.ClassroomID != "" {
		t.Fatalf("MTK subject rollup wrong: %+v", mtk)
	}
	if mtk.Percent.Hadir != 81.8 {
		t.Fatalf("MTK percent wrong: %+v", mtk.Percent)
	}
	if got := RollupSubject(rows, "student"); len(got) != 4 {
		t.Fatalf("student grouping should pass rows through")
	}
}

func TestRankClasses(t *testing.T) {
	rows := RollupRecap([]model.AttendanceRecapRow{
		row("S1", "B", "1", model.AttendanceCounts{Hadir: 5, Sakit: 5}),
		row("S1", "A", "2", model.AttendanceCounts{Hadir: 10}),
		row("S1", "C", "3", model.AttendanceCounts{Hadir: 10}),
	}, "class")
	ranked := RankClasses(rows)
	if ranked[0].ClassroomName != "A" || ranked[1].ClassroomName != "C" || ranked[2].ClassroomName != "B" {
		t.Fatalf("order wrong: %s %s %s", ranked[0].ClassroomName, ranked[1].ClassroomName, ranked[2].ClassroomName)
	}
	if ranked[0].Rank != 1 || ranked[2].Rank != 3 {
		t.Fatalf("ranks wrong")
	}
	// overall = 25/30 = 83.3%; A = 100 → +16.7, B = 50 → -33.3
	if *ranked[0].DeltaVsAvg != 16.7 || *ranked[2].DeltaVsAvg != -33.3 {
		t.Fatalf("deltas wrong: %v %v", *ranked[0].DeltaVsAvg, *ranked[2].DeltaVsAvg)
	}
}

func TestSortRecapRows(t *testing.T) {
	rows := []model.AttendanceRecapRow{
		{ClassroomName: "X-B", StudentName: "Ani"},
		{ClassroomName: "X-A", StudentName: "budi"},
		{ClassroomName: "X-A", StudentName: "Andi"},
	}
	SortRecapRows(rows)
	if rows[0].StudentName != "Andi" || rows[1].StudentName != "budi" || rows[2].ClassroomName != "X-B" {
		t.Fatalf("sort wrong: %+v", rows)
	}
}

// TestAttendanceReportQueries needs a migrated attendance DB (TEST_PG_DSN).
// It widens the status CHECK to include 'terlambat' if the terlambat
// migration has not landed in that DB yet, so AC-43-01 can be asserted
// end-to-end.
func TestAttendanceReportQueries(t *testing.T) {
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

	if _, err := db.Exec(`ALTER TABLE student_attendances DROP CONSTRAINT IF EXISTS student_attendances_status_check;
		ALTER TABLE student_attendances ADD CONSTRAINT student_attendances_status_check
		CHECK (status IN ('belum_presensi','hadir','terlambat','sakit','izin','tanpa_keterangan'))`); err != nil {
		t.Fatal(err)
	}
	uuid := func() string {
		var id string
		if err := db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	school, ay, term, cls, guru, dani, eka, mtk := uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid(), uuid()

	// 20 school days in October 2026 for Dani: 17 hadir, 1 terlambat, 1 sakit, 1 alpa.
	statuses := make([]string, 0, 20)
	for i := 0; i < 17; i++ {
		statuses = append(statuses, "hadir")
	}
	statuses = append(statuses, "terlambat", "sakit", "tanpa_keterangan")
	for i, st := range statuses {
		date := fmt.Sprintf("2026-10-%02d", i+1)
		var subject interface{}
		if i < 4 { // Matematika held on the first 4 days only
			subject = mtk
		}
		var sessionID string
		if err := db.QueryRow(`INSERT INTO class_sessions (school_id, academic_year_id, term_id, schedule_id, classroom_id, subject_id, guru_id, session_date)
			VALUES ($1,$2,$3,gen_random_uuid(),$4,$5,$6,$7) RETURNING id`, school, ay, term, cls, subject, guru, date).Scan(&sessionID); err != nil {
			t.Fatal(err)
		}
		for _, stu := range []struct{ id, status string }{{dani, st}, {eka, "hadir"}} {
			if _, err := db.Exec(`INSERT INTO student_attendances (class_session_id, school_id, academic_year_id, term_id, classroom_id, student_id, status, session_date)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, sessionID, school, ay, term, cls, stu.id, stu.status, date); err != nil {
				t.Fatal(err)
			}
		}
	}

	f := AttendanceReportFilter{SchoolID: school, From: "2026-10-01", To: "2026-10-31"}
	rows, err := s.AttendanceRecapStudentRows(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	var dr model.AttendanceRecapRow
	for _, r := range rows {
		if r.StudentID == dani {
			dr = r
		}
	}
	if dr.Counts.Hadir != 17 || dr.Counts.Terlambat != 1 || dr.Counts.Sakit != 1 || dr.Counts.TanpaKeterangan != 1 || dr.Counts.Total != 20 {
		t.Fatalf("AC-43-01 counts wrong: %+v", dr.Counts)
	}
	unit := RollupRecap(rows, "unit")
	if len(unit) != 1 || unit[0].Students != 2 || unit[0].Counts.Total != 40 || unit[0].Percent.Kehadiran != 95 {
		t.Fatalf("unit rollup wrong: %+v", unit)
	}

	// daily window + term filter
	daily, err := s.AttendanceRecapStudentRows(ctx, AttendanceReportFilter{SchoolID: school, TermID: term, From: "2026-10-18", To: "2026-10-18"})
	if err != nil || len(daily) != 2 {
		t.Fatalf("daily: %v %d", err, len(daily))
	}

	trend, err := s.AttendanceTrend(ctx, f, "week")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, p := range trend {
		total += p.Counts.Total
	}
	if total != 40 || len(trend) < 3 {
		t.Fatalf("trend wrong: %d points, total %d", len(trend), total)
	}
	if _, err := s.AttendanceTrend(ctx, f, "year; DROP"); err == nil {
		t.Fatal("invalid granularity must be rejected")
	}

	subjects, err := s.AttendanceSubjectRows(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	byClass := RollupSubject(subjects, "class")
	if len(byClass) != 1 || byClass[0].SubjectID != mtk || byClass[0].Sessions != 4 || byClass[0].Counts.Total != 8 || byClass[0].Students != 2 {
		t.Fatalf("subject rollup wrong: %+v", byClass)
	}
}
