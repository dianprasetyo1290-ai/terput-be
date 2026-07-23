package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/attendance/internal/model"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{DB: db}
}

// ---- scoping helper -------------------------------------------------------

// Scope carries the tenant/academic-year filter derived from JWT claims plus
// any admin override query params, per CONVENTIONS.md's multi-tenancy and
// academic-year scoping rules.
type Scope struct {
	SchoolID       string // "" means no filter (superadmin without ?school_id=)
	AcademicYearID string // "" means no filter
}

// ---- class_sessions ---------------------------------------------------

type CheckInInput struct {
	SchoolID       string
	AcademicYearID string
	TermID         *string
	ScheduleID     string
	ClassroomID    string
	SubjectID      *string
	GuruID         string
	SessionDate    string
	SlotStartTime  *string
	SlotEndTime    *string
}

// CheckIn creates (or, if the guru re-checks-in for the same slot/day,
// returns) the class_session row. It is an upsert keyed on
// (schedule_id, session_date) so re-hitting the endpoint is idempotent for
// the same guru; a different guru hitting an already-owned session is
// rejected by the handler before this is called.
func (s *Store) CheckIn(ctx context.Context, in CheckInInput) (*model.ClassSession, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO class_sessions (
			school_id, academic_year_id, term_id, schedule_id, classroom_id,
			subject_id, guru_id, session_date, checked_in_at, slot_start_time, slot_end_time
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now(), $9, $10)
		ON CONFLICT (schedule_id, session_date) DO UPDATE SET
			checked_in_at = COALESCE(class_sessions.checked_in_at, EXCLUDED.checked_in_at),
			updated_at = now()
		RETURNING id, school_id, academic_year_id, term_id, schedule_id, classroom_id,
			subject_id, guru_id, session_date, checked_in_at, slot_start_time, slot_end_time,
			pertemuan, pembahasan, materi_pertemuan, bentuk_kegiatan, agenda_filled_at,
			created_at, updated_at
	`, in.SchoolID, in.AcademicYearID, in.TermID, in.ScheduleID, in.ClassroomID,
		in.SubjectID, in.GuruID, in.SessionDate, in.SlotStartTime, in.SlotEndTime)
	return scanClassSession(row)
}

type AgendaInput struct {
	Pertemuan       *string
	Pembahasan      *string
	MateriPertemuan *string
	BentukKegiatan  []string
}

func (s *Store) UpdateAgenda(ctx context.Context, sessionID string, in AgendaInput) (*model.ClassSession, error) {
	bk, err := json.Marshal(in.BentukKegiatan)
	if err != nil {
		return nil, err
	}
	row := s.DB.QueryRowContext(ctx, `
		UPDATE class_sessions SET
			pertemuan = $2, pembahasan = $3, materi_pertemuan = $4,
			bentuk_kegiatan = $5::jsonb, agenda_filled_at = now(), updated_at = now()
		WHERE id = $1
		RETURNING id, school_id, academic_year_id, term_id, schedule_id, classroom_id,
			subject_id, guru_id, session_date, checked_in_at, slot_start_time, slot_end_time,
			pertemuan, pembahasan, materi_pertemuan, bentuk_kegiatan, agenda_filled_at,
			created_at, updated_at
	`, sessionID, in.Pertemuan, in.Pembahasan, in.MateriPertemuan, string(bk))
	return scanClassSession(row)
}

func (s *Store) GetClassSession(ctx context.Context, id string) (*model.ClassSession, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, term_id, schedule_id, classroom_id,
			subject_id, guru_id, session_date, checked_in_at, slot_start_time, slot_end_time,
			pertemuan, pembahasan, materi_pertemuan, bentuk_kegiatan, agenda_filled_at,
			created_at, updated_at
		FROM class_sessions WHERE id = $1
	`, id)
	return scanClassSession(row)
}

type ListSessionsFilter struct {
	SchoolID       string
	AcademicYearID string
	TermID         string
	GuruID         string
	ClassroomID    string
	Date           string // exact YYYY-MM-DD
	Month          string // YYYY-MM
	Day            string // weekday name, e.g. "Monday" (matches EXTRACT(DOW) via to_char)
	Page, PerPage  int
}

func (s *Store) ListSessions(ctx context.Context, f ListSessionsFilter) ([]model.ClassSession, int, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	add := func(cond string, val interface{}) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.SchoolID != "" {
		add("school_id = $%d", f.SchoolID)
	}
	if f.AcademicYearID != "" {
		add("academic_year_id = $%d", f.AcademicYearID)
	}
	if f.TermID != "" {
		add("term_id = $%d", f.TermID)
	}
	if f.GuruID != "" {
		add("guru_id = $%d", f.GuruID)
	}
	if f.ClassroomID != "" {
		add("classroom_id = $%d", f.ClassroomID)
	}
	if f.Date != "" {
		add("session_date = $%d", f.Date)
	}
	if f.Month != "" {
		add("to_char(session_date, 'YYYY-MM') = $%d", f.Month)
	}
	if f.Day != "" {
		add("to_char(session_date, 'FMDay') = $%d", f.Day)
	}

	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PerPage <= 0 || f.PerPage > 200 {
		f.PerPage = 20
	}
	offset := (f.Page - 1) * f.PerPage

	countQuery := "SELECT count(*) FROM class_sessions WHERE " + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, offset)
	query := fmt.Sprintf(`
		SELECT id, school_id, academic_year_id, term_id, schedule_id, classroom_id,
			subject_id, guru_id, session_date, checked_in_at, slot_start_time, slot_end_time,
			pertemuan, pembahasan, materi_pertemuan, bentuk_kegiatan, agenda_filled_at,
			created_at, updated_at
		FROM class_sessions WHERE %s
		ORDER BY session_date DESC, checked_in_at DESC NULLS LAST
		LIMIT $%d OFFSET $%d
	`, strings.Join(where, " AND "), len(args)-1, len(args))

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []model.ClassSession
	for rows.Next() {
		cs, err := scanClassSessionRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *cs)
	}
	return out, total, rows.Err()
}

// ActiveTeachers returns sessions still "in progress": checked in today, and
// the slot's end time hasn't passed yet (per the spec's "aktif" rule).
func (s *Store) ActiveTeachers(ctx context.Context, schoolID string) ([]model.ActiveTeacher, error) {
	where := []string{
		"checked_in_at IS NOT NULL",
		"session_date = CURRENT_DATE",
		"(slot_end_time IS NULL OR slot_end_time > CURRENT_TIME)",
	}
	args := []interface{}{}
	if schoolID != "" {
		args = append(args, schoolID)
		where = append(where, fmt.Sprintf("school_id = $%d", len(args)))
	}
	query := fmt.Sprintf(`
		SELECT id, guru_id, classroom_id, schedule_id, slot_start_time, slot_end_time, checked_in_at
		FROM class_sessions WHERE %s
		ORDER BY checked_in_at DESC
	`, strings.Join(where, " AND "))
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ActiveTeacher
	for rows.Next() {
		var at model.ActiveTeacher
		var start, end sql.NullString
		var checkedIn time.Time
		if err := rows.Scan(&at.SessionID, &at.GuruID, &at.ClassroomID, &at.ScheduleID, &start, &end, &checkedIn); err != nil {
			return nil, err
		}
		at.SlotStartTime = start.String
		at.SlotEndTime = end.String
		at.CheckedInAt = checkedIn.Format(time.RFC3339)
		out = append(out, at)
	}
	return out, rows.Err()
}

func scanClassSession(row *sql.Row) (*model.ClassSession, error) {
	cs := &model.ClassSession{}
	var termID, subjectID, pertemuan, pembahasan, materi sql.NullString
	var checkedIn, agendaFilled sql.NullTime
	var slotStart, slotEnd sql.NullString
	var bk []byte
	err := row.Scan(&cs.ID, &cs.SchoolID, &cs.AcademicYearID, &termID, &cs.ScheduleID, &cs.ClassroomID,
		&subjectID, &cs.GuruID, &cs.SessionDate, &checkedIn, &slotStart, &slotEnd,
		&pertemuan, &pembahasan, &materi, &bk, &agendaFilled,
		&cs.CreatedAt, &cs.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	fillClassSession(cs, termID, subjectID, pertemuan, pembahasan, materi, checkedIn, agendaFilled, slotStart, slotEnd, bk)
	return cs, nil
}

func scanClassSessionRow(rows *sql.Rows) (*model.ClassSession, error) {
	cs := &model.ClassSession{}
	var termID, subjectID, pertemuan, pembahasan, materi sql.NullString
	var checkedIn, agendaFilled sql.NullTime
	var slotStart, slotEnd sql.NullString
	var bk []byte
	err := rows.Scan(&cs.ID, &cs.SchoolID, &cs.AcademicYearID, &termID, &cs.ScheduleID, &cs.ClassroomID,
		&subjectID, &cs.GuruID, &cs.SessionDate, &checkedIn, &slotStart, &slotEnd,
		&pertemuan, &pembahasan, &materi, &bk, &agendaFilled,
		&cs.CreatedAt, &cs.UpdatedAt)
	if err != nil {
		return nil, err
	}
	fillClassSession(cs, termID, subjectID, pertemuan, pembahasan, materi, checkedIn, agendaFilled, slotStart, slotEnd, bk)
	return cs, nil
}

func fillClassSession(cs *model.ClassSession, termID, subjectID, pertemuan, pembahasan, materi sql.NullString,
	checkedIn, agendaFilled sql.NullTime, slotStart, slotEnd sql.NullString, bk []byte) {
	if termID.Valid {
		cs.TermID = &termID.String
	}
	if subjectID.Valid {
		cs.SubjectID = &subjectID.String
	}
	if pertemuan.Valid {
		cs.Pertemuan = &pertemuan.String
	}
	if pembahasan.Valid {
		cs.Pembahasan = &pembahasan.String
	}
	if materi.Valid {
		cs.MateriPertemuan = &materi.String
	}
	if checkedIn.Valid {
		t := checkedIn.Time
		cs.CheckedInAt = &t
	}
	if agendaFilled.Valid {
		t := agendaFilled.Time
		cs.AgendaFilledAt = &t
	}
	if slotStart.Valid {
		cs.SlotStartTime = &slotStart.String
	}
	if slotEnd.Valid {
		cs.SlotEndTime = &slotEnd.String
	}
	cs.BentukKegiatan = []string{}
	if len(bk) > 0 {
		_ = json.Unmarshal(bk, &cs.BentukKegiatan)
	}
}

// ---- student_attendances -----------------------------------------------

type MarkInput struct {
	ClassSessionID string
	SchoolID       string
	AcademicYearID string
	TermID         *string
	ClassroomID    string
	StudentID      string
	Status         string
	Catatan        *string
	MarkedBy       string
}

// UpsertStudentAttendance inserts or overrides a single student's status for
// a session (used both by "pilih semua" bulk-hadir and by the per-student
// "ubah status" override).
func (s *Store) UpsertStudentAttendance(ctx context.Context, in MarkInput) (*model.StudentAttendance, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO student_attendances (
			class_session_id, school_id, academic_year_id, term_id, classroom_id,
			student_id, status, catatan, marked_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (class_session_id, student_id) DO UPDATE SET
			status = EXCLUDED.status, catatan = EXCLUDED.catatan,
			marked_by = EXCLUDED.marked_by, updated_at = now()
		RETURNING id, class_session_id, school_id, academic_year_id, term_id, classroom_id,
			student_id, status, catatan, marked_by, created_at, updated_at
	`, in.ClassSessionID, in.SchoolID, in.AcademicYearID, in.TermID, in.ClassroomID,
		in.StudentID, in.Status, in.Catatan, in.MarkedBy)
	sa := &model.StudentAttendance{}
	var termID, catatan, markedBy sql.NullString
	err := row.Scan(&sa.ID, &sa.ClassSessionID, &sa.SchoolID, &sa.AcademicYearID, &termID, &sa.ClassroomID,
		&sa.StudentID, &sa.Status, &catatan, &markedBy, &sa.CreatedAt, &sa.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if termID.Valid {
		sa.TermID = &termID.String
	}
	if catatan.Valid {
		sa.Catatan = &catatan.String
	}
	if markedBy.Valid {
		sa.MarkedBy = &markedBy.String
	}
	return sa, nil
}

func (s *Store) ListStudentAttendances(ctx context.Context, classSessionID string) ([]model.StudentAttendance, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, class_session_id, school_id, academic_year_id, term_id, classroom_id,
			student_id, status, catatan, marked_by, created_at, updated_at
		FROM student_attendances WHERE class_session_id = $1
		ORDER BY created_at ASC
	`, classSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.StudentAttendance
	for rows.Next() {
		sa := model.StudentAttendance{}
		var termID, catatan, markedBy sql.NullString
		if err := rows.Scan(&sa.ID, &sa.ClassSessionID, &sa.SchoolID, &sa.AcademicYearID, &termID, &sa.ClassroomID,
			&sa.StudentID, &sa.Status, &catatan, &markedBy, &sa.CreatedAt, &sa.UpdatedAt); err != nil {
			return nil, err
		}
		if termID.Valid {
			sa.TermID = &termID.String
		}
		if catatan.Valid {
			sa.Catatan = &catatan.String
		}
		if markedBy.Valid {
			sa.MarkedBy = &markedBy.String
		}
		out = append(out, sa)
	}
	return out, rows.Err()
}

type StudentAttendanceFilter struct {
	SchoolID       string
	AcademicYearID string
	ClassroomID    string
	Date, Month, Day string
	Page, PerPage  int
}

// ListStudentAttendanceForAdmin joins student_attendances with class_sessions
// for the "Presensi Siswa" admin view (per classroom, filterable by
// academic year/bulan/hari/tanggal, including past years).
func (s *Store) ListStudentAttendanceForAdmin(ctx context.Context, f StudentAttendanceFilter) ([]model.StudentAttendance, int, error) {
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
	if f.ClassroomID != "" {
		add("sa.classroom_id = $%d", f.ClassroomID)
	}
	if f.Date != "" {
		add("cs.session_date = $%d", f.Date)
	}
	if f.Month != "" {
		add("to_char(cs.session_date, 'YYYY-MM') = $%d", f.Month)
	}
	if f.Day != "" {
		add("to_char(cs.session_date, 'FMDay') = $%d", f.Day)
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PerPage <= 0 || f.PerPage > 500 {
		f.PerPage = 50
	}
	offset := (f.Page - 1) * f.PerPage

	countQuery := "SELECT count(*) FROM student_attendances sa JOIN class_sessions cs ON cs.id = sa.class_session_id WHERE " + strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PerPage, offset)
	query := fmt.Sprintf(`
		SELECT sa.id, sa.class_session_id, sa.school_id, sa.academic_year_id, sa.term_id, sa.classroom_id,
			sa.student_id, sa.status, sa.catatan, sa.marked_by, sa.created_at, sa.updated_at
		FROM student_attendances sa JOIN class_sessions cs ON cs.id = sa.class_session_id
		WHERE %s
		ORDER BY cs.session_date DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(where, " AND "), len(args)-1, len(args))

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []model.StudentAttendance
	for rows.Next() {
		sa := model.StudentAttendance{}
		var termID, catatan, markedBy sql.NullString
		if err := rows.Scan(&sa.ID, &sa.ClassSessionID, &sa.SchoolID, &sa.AcademicYearID, &termID, &sa.ClassroomID,
			&sa.StudentID, &sa.Status, &catatan, &markedBy, &sa.CreatedAt, &sa.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if termID.Valid {
			sa.TermID = &termID.String
		}
		if catatan.Valid {
			sa.Catatan = &catatan.String
		}
		if markedBy.Valid {
			sa.MarkedBy = &markedBy.String
		}
		out = append(out, sa)
	}
	return out, total, rows.Err()
}

// StudentAttendanceSummary aggregates counts by status for one student,
// optionally scoped to a term (used by report-service's e-raport summary
// and by the admin per-student view).
func (s *Store) StudentAttendanceSummary(ctx context.Context, studentID, termID, academicYearID, schoolID string) (*model.AttendanceSummary, error) {
	where := []string{"sa.student_id = $1"}
	args := []interface{}{studentID}
	if termID != "" {
		args = append(args, termID)
		where = append(where, fmt.Sprintf("sa.term_id = $%d", len(args)))
	}
	if academicYearID != "" {
		args = append(args, academicYearID)
		where = append(where, fmt.Sprintf("sa.academic_year_id = $%d", len(args)))
	}
	if schoolID != "" {
		args = append(args, schoolID)
		where = append(where, fmt.Sprintf("sa.school_id = $%d", len(args)))
	}
	query := fmt.Sprintf(`
		SELECT
			count(*) FILTER (WHERE sa.status = 'hadir'),
			count(*) FILTER (WHERE sa.status = 'sakit'),
			count(*) FILTER (WHERE sa.status = 'izin'),
			count(*) FILTER (WHERE sa.status = 'tanpa_keterangan')
		FROM student_attendances sa WHERE %s
	`, strings.Join(where, " AND "))
	sum := &model.AttendanceSummary{}
	err := s.DB.QueryRowContext(ctx, query, args...).Scan(&sum.Hadir, &sum.Sakit, &sum.Izin, &sum.TanpaKeterangan)
	return sum, err
}

// DashboardStudentSummary aggregates student attendance counts by month for
// a school/academic-year, for the admin dashboard chart.
func (s *Store) DashboardStudentSummary(ctx context.Context, schoolID, academicYearID string) ([]model.MonthlyStatusCount, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	if schoolID != "" {
		args = append(args, schoolID)
		where = append(where, fmt.Sprintf("sa.school_id = $%d", len(args)))
	}
	if academicYearID != "" {
		args = append(args, academicYearID)
		where = append(where, fmt.Sprintf("sa.academic_year_id = $%d", len(args)))
	}
	query := fmt.Sprintf(`
		SELECT to_char(cs.session_date, 'YYYY-MM') AS month,
			count(*) FILTER (WHERE sa.status = 'hadir'),
			count(*) FILTER (WHERE sa.status = 'sakit'),
			count(*) FILTER (WHERE sa.status = 'izin'),
			count(*) FILTER (WHERE sa.status = 'tanpa_keterangan')
		FROM student_attendances sa JOIN class_sessions cs ON cs.id = sa.class_session_id
		WHERE %s
		GROUP BY 1 ORDER BY 1
	`, strings.Join(where, " AND "))
	return scanMonthlyCounts(ctx, s.DB, query, args)
}

// DashboardTeacherSummary aggregates teacher check-in counts by month (a
// "present" bucket only, since teacher self check-in doesn't carry
// sakit/izin/tanpa_keterangan status the way student attendance does).
func (s *Store) DashboardTeacherSummary(ctx context.Context, schoolID, academicYearID string) ([]model.MonthlyStatusCount, error) {
	where := []string{"checked_in_at IS NOT NULL"}
	args := []interface{}{}
	if schoolID != "" {
		args = append(args, schoolID)
		where = append(where, fmt.Sprintf("school_id = $%d", len(args)))
	}
	if academicYearID != "" {
		args = append(args, academicYearID)
		where = append(where, fmt.Sprintf("academic_year_id = $%d", len(args)))
	}
	query := fmt.Sprintf(`
		SELECT to_char(session_date, 'YYYY-MM') AS month, count(*), 0, 0, 0
		FROM class_sessions
		WHERE %s
		GROUP BY 1 ORDER BY 1
	`, strings.Join(where, " AND "))
	return scanMonthlyCounts(ctx, s.DB, query, args)
}

func scanMonthlyCounts(ctx context.Context, db *sql.DB, query string, args []interface{}) ([]model.MonthlyStatusCount, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.MonthlyStatusCount
	for rows.Next() {
		var m model.MonthlyStatusCount
		if err := rows.Scan(&m.Month, &m.Hadir, &m.Sakit, &m.Izin, &m.TanpaKeterangan); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
