package store

// Attendance Report (PRD menu 43, FR-43-001..003). Lives in its own file so
// it never conflicts with concurrent edits to store.go. The SQL side only
// ever returns the finest grain (per student, or per subject+student); every
// coarser view (class, unit, subject-per-class) is rolled up in Go by the
// pure functions at the bottom, which is what the unit tests exercise.

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"skci/attendance/internal/model"
)

// AttendanceReportFilter narrows every report query. Empty fields mean "no
// filter"; From/To are inclusive YYYY-MM-DD bounds on session_date.
type AttendanceReportFilter struct {
	SchoolID       string
	AcademicYearID string
	TermID         string
	ClassroomID    string
	StudentID      string
	SubjectID      string // subject report only
	From, To       string
}

func (f AttendanceReportFilter) where() ([]string, []interface{}) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.SchoolID != "" {
		add("sa.school_id = $%d", f.SchoolID)
	}
	if f.AcademicYearID != "" {
		add("sa.academic_year_id = $%d", f.AcademicYearID)
	}
	if f.TermID != "" {
		add("sa.term_id = $%d", f.TermID)
	}
	if f.ClassroomID != "" {
		add("sa.classroom_id = $%d", f.ClassroomID)
	}
	if f.StudentID != "" {
		add("sa.student_id = $%d", f.StudentID)
	}
	if f.From != "" {
		add("sa.session_date >= $%d::date", f.From)
	}
	if f.To != "" {
		add("sa.session_date <= $%d::date", f.To)
	}
	return where, args
}

// statusCountCols is the shared per-status tally. 'terlambat' is compared as
// plain text, so it simply counts 0 until that status is allowed by the
// CHECK constraint — no dependency on the migration landing first.
const statusCountCols = `
	count(*) FILTER (WHERE sa.status = 'hadir'),
	count(*) FILTER (WHERE sa.status = 'terlambat'),
	count(*) FILTER (WHERE sa.status = 'sakit'),
	count(*) FILTER (WHERE sa.status = 'izin'),
	count(*) FILTER (WHERE sa.status = 'tanpa_keterangan'),
	count(*) FILTER (WHERE sa.status = 'belum_presensi')`

func countDest(c *model.AttendanceCounts) []interface{} {
	return []interface{}{&c.Hadir, &c.Terlambat, &c.Sakit, &c.Izin, &c.TanpaKeterangan, &c.BelumPresensi}
}

// AttendanceRecapStudentRows returns one row per (school, classroom,
// student) over the filter window — the base grain for FR-43-001. No row
// cap: the result is bounded by the number of enrolled students.
func (s *Store) AttendanceRecapStudentRows(ctx context.Context, f AttendanceReportFilter) ([]model.AttendanceRecapRow, error) {
	where, args := f.where()
	query := fmt.Sprintf(`
		SELECT sa.school_id::text, sa.classroom_id::text, sa.student_id::text, %s
		FROM student_attendances sa
		WHERE %s
		GROUP BY sa.school_id, sa.classroom_id, sa.student_id
	`, statusCountCols, strings.Join(where, " AND "))
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AttendanceRecapRow{}
	for rows.Next() {
		r := model.AttendanceRecapRow{Scope: "student"}
		dest := append([]interface{}{&r.SchoolID, &r.ClassroomID, &r.StudentID}, countDest(&r.Counts)...)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		FinalizeCounts(&r)
		out = append(out, r)
	}
	return out, rows.Err()
}

// TrendGranularities are the accepted date_trunc buckets for the trend view.
var TrendGranularities = map[string]bool{"day": true, "week": true, "month": true}

// AttendanceTrend buckets the filter window by day / ISO week / month
// (FR-43-002). Buckets without any record are omitted (e.g. weekends).
func (s *Store) AttendanceTrend(ctx context.Context, f AttendanceReportFilter, granularity string) ([]model.AttendanceTrendPoint, error) {
	if !TrendGranularities[granularity] {
		return nil, fmt.Errorf("invalid granularity %q", granularity)
	}
	where, args := f.where()
	// granularity is whitelisted above, so inlining it is safe.
	query := fmt.Sprintf(`
		SELECT date_trunc('%s', sa.session_date)::date, %s
		FROM student_attendances sa
		WHERE %s
		GROUP BY 1 ORDER BY 1
	`, granularity, statusCountCols, strings.Join(where, " AND "))
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AttendanceTrendPoint{}
	for rows.Next() {
		var p model.AttendanceTrendPoint
		var bucket time.Time
		dest := append([]interface{}{&bucket}, countDest(&p.Counts)...)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		p.Period = bucket.Format("2006-01-02")
		p.Counts.Total = reportCountedTotal(p.Counts)
		p.Percent = ComputePercentages(p.Counts)
		out = append(out, p)
	}
	return out, rows.Err()
}

// AttendanceSubjectRows returns one row per (subject, school, classroom,
// student) — FR-43-003. student_attendances is one row per student per day
// (migration 0003), so a subject's attendance is the student's daily status
// on each day a session of that subject was held in their classroom: every
// matching class_session counts as one meeting. Import-created sessions carry
// no subject and are excluded by the subject_id IS NOT NULL join condition.
func (s *Store) AttendanceSubjectRows(ctx context.Context, f AttendanceReportFilter) ([]model.AttendanceRecapRow, error) {
	where, args := f.where()
	if f.SubjectID != "" {
		args = append(args, f.SubjectID)
		where = append(where, fmt.Sprintf("cs.subject_id = $%d", len(args)))
	}
	query := fmt.Sprintf(`
		SELECT cs.subject_id::text, sa.school_id::text, sa.classroom_id::text, sa.student_id::text,
			count(DISTINCT cs.id), %s
		FROM student_attendances sa
		JOIN class_sessions cs
			ON cs.classroom_id = sa.classroom_id
			AND cs.session_date = sa.session_date
			AND cs.school_id = sa.school_id
			AND cs.subject_id IS NOT NULL
		WHERE %s
		GROUP BY cs.subject_id, sa.school_id, sa.classroom_id, sa.student_id
	`, statusCountCols, strings.Join(where, " AND "))
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.AttendanceRecapRow{}
	for rows.Next() {
		r := model.AttendanceRecapRow{Scope: "subject"}
		dest := append([]interface{}{&r.SubjectID, &r.SchoolID, &r.ClassroomID, &r.StudentID, &r.Sessions}, countDest(&r.Counts)...)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		FinalizeCounts(&r)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- pure aggregation (unit-tested without a DB) --------------------------

func reportCountedTotal(c model.AttendanceCounts) int {
	return c.Hadir + c.Terlambat + c.Sakit + c.Izin + c.TanpaKeterangan
}

func reportPct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(n)*1000/float64(total)) / 10
}

// ComputePercentages derives the per-status shares. belum_presensi is not
// part of the denominator: an unmarked day is neither present nor absent.
func ComputePercentages(c model.AttendanceCounts) model.AttendancePercentages {
	t := reportCountedTotal(c)
	return model.AttendancePercentages{
		Hadir:           reportPct(c.Hadir, t),
		Terlambat:       reportPct(c.Terlambat, t),
		Sakit:           reportPct(c.Sakit, t),
		Izin:            reportPct(c.Izin, t),
		TanpaKeterangan: reportPct(c.TanpaKeterangan, t),
		Kehadiran:       reportPct(c.Hadir+c.Terlambat, t),
		TidakHadir:      reportPct(c.Sakit+c.Izin+c.TanpaKeterangan, t),
	}
}

// FinalizeCounts recomputes Total and Percent from the raw status counts.
func FinalizeCounts(r *model.AttendanceRecapRow) {
	r.Counts.Total = reportCountedTotal(r.Counts)
	r.Percent = ComputePercentages(r.Counts)
}

// SumCounts totals any set of rows into one summary row.
func SumCounts(rows []model.AttendanceRecapRow, scope string) model.AttendanceRecapRow {
	out := model.AttendanceRecapRow{Scope: scope}
	students := map[string]bool{}
	for _, r := range rows {
		out.Counts.Add(r.Counts)
		if r.StudentID != "" {
			students[r.StudentID] = true
		} else {
			out.Students += r.Students
		}
	}
	out.Students += len(students)
	FinalizeCounts(&out)
	return out
}

type rollupBucket struct {
	row      model.AttendanceRecapRow
	students map[string]bool
}

func rollup(rows []model.AttendanceRecapRow, scope string, key func(model.AttendanceRecapRow) string, seed func(model.AttendanceRecapRow) model.AttendanceRecapRow) []model.AttendanceRecapRow {
	buckets := map[string]*rollupBucket{}
	var order []string
	for _, r := range rows {
		k := key(r)
		b, ok := buckets[k]
		if !ok {
			s := seed(r)
			s.Scope = scope
			b = &rollupBucket{row: s, students: map[string]bool{}}
			buckets[k] = b
			order = append(order, k)
		}
		b.row.Counts.Add(r.Counts)
		if r.StudentID != "" {
			b.students[r.StudentID] = true
		}
		if r.Sessions > b.row.Sessions {
			// Every student of a class sits the same meetings; the max is the
			// class's meeting count (summing would multiply by headcount).
			b.row.Sessions = r.Sessions
		}
	}
	out := make([]model.AttendanceRecapRow, 0, len(order))
	for _, k := range order {
		b := buckets[k]
		b.row.Students = len(b.students)
		FinalizeCounts(&b.row)
		out = append(out, b.row)
	}
	return out
}

// RollupRecap turns per-student rows into per-class ("class") or per-unit
// ("unit" = school) rows; "student" returns the rows unchanged. Names already
// set on the input rows carry over.
func RollupRecap(rows []model.AttendanceRecapRow, scope string) []model.AttendanceRecapRow {
	switch scope {
	case "class":
		return rollup(rows, "class",
			func(r model.AttendanceRecapRow) string { return r.SchoolID + "|" + r.ClassroomID },
			func(r model.AttendanceRecapRow) model.AttendanceRecapRow {
				return model.AttendanceRecapRow{SchoolID: r.SchoolID, SchoolName: r.SchoolName, ClassroomID: r.ClassroomID, ClassroomName: r.ClassroomName}
			})
	case "unit":
		return rollup(rows, "unit",
			func(r model.AttendanceRecapRow) string { return r.SchoolID },
			func(r model.AttendanceRecapRow) model.AttendanceRecapRow {
				return model.AttendanceRecapRow{SchoolID: r.SchoolID, SchoolName: r.SchoolName}
			})
	default:
		return rows
	}
}

// RollupSubject groups per-subject-per-student rows: "student" keeps them,
// "class" = one row per subject per classroom, "subject" = one row per
// subject across every classroom in the filter.
func RollupSubject(rows []model.AttendanceRecapRow, by string) []model.AttendanceRecapRow {
	switch by {
	case "student":
		return rows
	case "subject":
		out := rollup(rows, "subject",
			func(r model.AttendanceRecapRow) string { return r.SubjectID },
			func(r model.AttendanceRecapRow) model.AttendanceRecapRow {
				return model.AttendanceRecapRow{SubjectID: r.SubjectID, SubjectName: r.SubjectName}
			})
		// Sessions across classes do add up (each class has its own meetings),
		// so recompute as the sum of per-class maxima.
		perClass := RollupSubject(rows, "class")
		sessions := map[string]int{}
		for _, c := range perClass {
			sessions[c.SubjectID] += c.Sessions
		}
		for i := range out {
			out[i].Sessions = sessions[out[i].SubjectID]
		}
		return out
	default: // "class"
		return rollup(rows, "subject",
			func(r model.AttendanceRecapRow) string { return r.SubjectID + "|" + r.ClassroomID },
			func(r model.AttendanceRecapRow) model.AttendanceRecapRow {
				return model.AttendanceRecapRow{SubjectID: r.SubjectID, SubjectName: r.SubjectName, SchoolID: r.SchoolID, ClassroomID: r.ClassroomID, ClassroomName: r.ClassroomName}
			})
	}
}

// RankClasses orders class rows by kehadiran % (desc, ties by name), sets
// Rank 1..n and the delta against the overall kehadiran % of all rows
// together (FR-43-002 class comparison).
func RankClasses(rows []model.AttendanceRecapRow) []model.AttendanceRecapRow {
	out := append([]model.AttendanceRecapRow(nil), rows...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Percent.Kehadiran != out[j].Percent.Kehadiran {
			return out[i].Percent.Kehadiran > out[j].Percent.Kehadiran
		}
		return out[i].ClassroomName < out[j].ClassroomName
	})
	avg := SumCounts(out, "class").Percent.Kehadiran
	for i := range out {
		out[i].Rank = i + 1
		d := math.Round((out[i].Percent.Kehadiran-avg)*10) / 10
		out[i].DeltaVsAvg = &d
	}
	return out
}

// SortRecapRows orders rows for display: unit by school name, class by
// classroom name, student/subject by classroom → student/subject name.
func SortRecapRows(rows []model.AttendanceRecapRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		keys := func(r model.AttendanceRecapRow) []string {
			return []string{strings.ToLower(r.SchoolName), strings.ToLower(r.SubjectName), strings.ToLower(r.ClassroomName), strings.ToLower(r.StudentName), r.StudentID}
		}
		ka, kb := keys(a), keys(b)
		for x := range ka {
			if ka[x] != kb[x] {
				return ka[x] < kb[x]
			}
		}
		return false
	})
}
