package store

import (
	"context"
	"database/sql"

	"skci/academic/internal/model"
)

// --- Schedule periods (jam pelajaran) ---

func (s *Store) CreateSchedulePeriod(ctx context.Context, p model.SchedulePeriod) (*model.SchedulePeriod, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO schedule_periods (school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (classroom_id, day_of_week, period_no) DO UPDATE SET start_time = EXCLUDED.start_time, end_time = EXCLUDED.end_time, updated_at = now()
		RETURNING id, school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time, created_at, updated_at
	`, p.SchoolID, p.AcademicYearID, p.ClassroomID, p.DayOfWeek, p.PeriodNo, p.StartTime, p.EndTime)
	return scanSchedulePeriod(row)
}

func (s *Store) ListSchedulePeriods(ctx context.Context, schoolID, classroomID string, day *int) ([]model.SchedulePeriod, error) {
	q := `
		SELECT id, school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time, created_at, updated_at
		FROM schedule_periods WHERE school_id = $1 AND classroom_id = $2
	`
	args := []interface{}{schoolID, classroomID}
	if day != nil {
		q += " AND day_of_week = $3"
		args = append(args, *day)
	}
	q += " ORDER BY day_of_week, period_no"
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.SchedulePeriod{}
	for rows.Next() {
		p, err := scanSchedulePeriodRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) GetSchedulePeriod(ctx context.Context, schoolID, id string) (*model.SchedulePeriod, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time, created_at, updated_at
		FROM schedule_periods WHERE id = $1 AND school_id = $2
	`, id, schoolID)
	p, err := scanSchedulePeriod(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Store) UpdateSchedulePeriod(ctx context.Context, schoolID, id string, startTime, endTime *string) (*model.SchedulePeriod, error) {
	row := s.DB.QueryRowContext(ctx, `
		UPDATE schedule_periods SET
			start_time = COALESCE($3, start_time),
			end_time = COALESCE($4, end_time),
			updated_at = now()
		WHERE id = $1 AND school_id = $2
		RETURNING id, school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time, created_at, updated_at
	`, id, schoolID, startTime, endTime)
	p, err := scanSchedulePeriod(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (s *Store) DeleteSchedulePeriod(ctx context.Context, schoolID, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM schedule_periods WHERE id = $1 AND school_id = $2`, id, schoolID)
	return err
}

// --- Schedules (jadwal slot: classroom_subject assigned into a period) ---

// CreateSchedule inserts a (period, classroom_subject) pairing. Conflict
// target is (schedule_period_id, classroom_subject_id) — not
// schedule_period_id alone — so a period slot can hold more than one row
// (Agama variants running concurrently); re-posting the SAME subject into
// the same slot is still just a no-op idempotent touch, not a second row.
// Whether a genuinely DIFFERENT subject may share the slot is a business
// rule (Agama-only) checked in the handler before this is ever called, not
// here — see checkScheduleSlotConflict in internal/handler/schedule.go.
func (s *Store) CreateSchedule(ctx context.Context, sc model.Schedule) (*model.Schedule, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO schedules (school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (schedule_period_id, classroom_subject_id) DO UPDATE SET updated_at = now()
		RETURNING id, school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id, created_at, updated_at
	`, sc.SchoolID, sc.AcademicYearID, sc.ClassroomID, sc.SchedulePeriodID, sc.ClassroomSubjectID)
	return scanSchedule(row)
}

// ListSchedulesByPeriod returns every schedule row already occupying one
// schedule_period_id — used by the handler's slot-conflict check to decide
// whether a new/changed subject may join an already-occupied slot (only
// when every occupant, existing and new, is an Agama variant).
func (s *Store) ListSchedulesByPeriod(ctx context.Context, schoolID, periodID string) ([]model.Schedule, error) {
	q := `
		SELECT s.id, s.school_id, s.academic_year_id, s.classroom_id, s.schedule_period_id, s.classroom_subject_id, s.created_at, s.updated_at,
		       sub.name, c.name, sp.day_of_week, sp.period_no, sp.start_time, sp.end_time, cs.teacher_user_id
		FROM schedules s
		JOIN schedule_periods sp ON sp.id = s.schedule_period_id
		JOIN classroom_subjects cs ON cs.id = s.classroom_subject_id
		JOIN subjects sub ON sub.id = cs.subject_id
		JOIN classrooms c ON c.id = s.classroom_id
		WHERE s.school_id = $1 AND s.schedule_period_id = $2
	`
	return s.scanScheduleRows(ctx, q, schoolID, periodID)
}

func (s *Store) ListSchedulesByClassroom(ctx context.Context, schoolID, classroomID string, day *int) ([]model.Schedule, error) {
	q := `
		SELECT s.id, s.school_id, s.academic_year_id, s.classroom_id, s.schedule_period_id, s.classroom_subject_id, s.created_at, s.updated_at,
		       sub.name, c.name, sp.day_of_week, sp.period_no, sp.start_time, sp.end_time, cs.teacher_user_id
		FROM schedules s
		JOIN schedule_periods sp ON sp.id = s.schedule_period_id
		JOIN classroom_subjects cs ON cs.id = s.classroom_subject_id
		JOIN subjects sub ON sub.id = cs.subject_id
		JOIN classrooms c ON c.id = s.classroom_id
		WHERE s.school_id = $1 AND s.classroom_id = $2
	`
	args := []interface{}{schoolID, classroomID}
	if day != nil {
		q += " AND sp.day_of_week = $3"
		args = append(args, *day)
	}
	q += " ORDER BY sp.day_of_week, sp.period_no"
	return s.scanScheduleRows(ctx, q, args...)
}

// ListSchedulesForTeacher backs GET /me/schedule?day= — a guru's own
// teaching schedule, filtered to classroom_subjects.teacher_user_id = caller.
func (s *Store) ListSchedulesForTeacher(ctx context.Context, schoolID, teacherUserID string, day *int) ([]model.Schedule, error) {
	q := `
		SELECT s.id, s.school_id, s.academic_year_id, s.classroom_id, s.schedule_period_id, s.classroom_subject_id, s.created_at, s.updated_at,
		       sub.name, c.name, sp.day_of_week, sp.period_no, sp.start_time, sp.end_time, cs.teacher_user_id
		FROM schedules s
		JOIN schedule_periods sp ON sp.id = s.schedule_period_id
		JOIN classroom_subjects cs ON cs.id = s.classroom_subject_id
		JOIN subjects sub ON sub.id = cs.subject_id
		JOIN classrooms c ON c.id = s.classroom_id
		WHERE s.school_id = $1 AND cs.teacher_user_id = $2
	`
	args := []interface{}{schoolID, teacherUserID}
	if day != nil {
		q += " AND sp.day_of_week = $3"
		args = append(args, *day)
	}
	q += " ORDER BY sp.day_of_week, sp.period_no"
	return s.scanScheduleRows(ctx, q, args...)
}

// ListSchedulesActiveNow backs GET /schedules/active-now — every schedule
// slot, across ALL classrooms in the school, whose day_of_week/start_time/
// end_time window covers the current server moment. day_of_week uses the
// same 1=Senin..7=Minggu convention as schedule_periods everywhere else in
// this codebase (frontend's DAY_NAMES/dayNames arrays), which is exactly
// Postgres ISODOW's convention (1=Monday..7=Sunday) -- no remapping needed.
// Used by attendance-service's "Guru Sedang Mengajar" dashboard widget,
// which otherwise has no way to know a guru is SCHEDULED right now before
// they've actually checked in (attendance only ever sees a class_sessions
// row once check-in happens).
func (s *Store) ListSchedulesActiveNow(ctx context.Context, schoolID string) ([]model.Schedule, error) {
	q := `
		SELECT s.id, s.school_id, s.academic_year_id, s.classroom_id, s.schedule_period_id, s.classroom_subject_id, s.created_at, s.updated_at,
		       sub.name, c.name, sp.day_of_week, sp.period_no, sp.start_time, sp.end_time, cs.teacher_user_id, sub.id
		FROM schedules s
		JOIN schedule_periods sp ON sp.id = s.schedule_period_id
		JOIN classroom_subjects cs ON cs.id = s.classroom_subject_id
		JOIN subjects sub ON sub.id = cs.subject_id
		JOIN classrooms c ON c.id = s.classroom_id
		WHERE s.school_id = $1
		  AND sp.day_of_week = EXTRACT(ISODOW FROM now())::int
		  AND sp.start_time::time <= CURRENT_TIME AND sp.end_time::time > CURRENT_TIME
		ORDER BY sp.period_no
	`
	rows, err := s.DB.QueryContext(ctx, q, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Schedule{}
	for rows.Next() {
		var sc model.Schedule
		var subjectName, classroomName, startTime, endTime, subjectID string
		var dayOfWeek, periodNo int
		var teacherUserID sql.NullString
		if err := rows.Scan(&sc.ID, &sc.SchoolID, &sc.AcademicYearID, &sc.ClassroomID, &sc.SchedulePeriodID, &sc.ClassroomSubjectID, &sc.CreatedAt, &sc.UpdatedAt,
			&subjectName, &classroomName, &dayOfWeek, &periodNo, &startTime, &endTime, &teacherUserID, &subjectID); err != nil {
			return nil, err
		}
		sc.SubjectName = &subjectName
		sc.ClassroomName = &classroomName
		sc.DayOfWeek = &dayOfWeek
		sc.PeriodNo = &periodNo
		sc.StartTime = &startTime
		sc.EndTime = &endTime
		sc.SubjectID = &subjectID
		if teacherUserID.Valid {
			sc.TeacherUserID = &teacherUserID.String
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// GetScheduleEnriched is GetSchedule (bare FKs) plus the same
// subject/classroom/time-window enrichment as ListSchedulesByClassroom /
// ListSchedulesForTeacher -- backs GET /schedules/{id}, which previously
// didn't exist at all (attendance-service's GetSchedule client call always
// 404'd, silently degrading every check-in to whatever partial fields the
// guru's own browser happened to send, term_id and subject_id included).
func (s *Store) GetScheduleEnriched(ctx context.Context, schoolID, id string) (*model.Schedule, error) {
	q := `
		SELECT s.id, s.school_id, s.academic_year_id, s.classroom_id, s.schedule_period_id, s.classroom_subject_id, s.created_at, s.updated_at,
		       sub.name, c.name, sp.day_of_week, sp.period_no, sp.start_time, sp.end_time, cs.teacher_user_id, sub.id
		FROM schedules s
		JOIN schedule_periods sp ON sp.id = s.schedule_period_id
		JOIN classroom_subjects cs ON cs.id = s.classroom_subject_id
		JOIN subjects sub ON sub.id = cs.subject_id
		JOIN classrooms c ON c.id = s.classroom_id
		WHERE s.id = $1 AND s.school_id = $2
	`
	rows, err := s.DB.QueryContext(ctx, q, id, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var sc model.Schedule
	var subjectName, classroomName, startTime, endTime, subjectID string
	var dayOfWeek, periodNo int
	var teacherUserID sql.NullString
	if err := rows.Scan(&sc.ID, &sc.SchoolID, &sc.AcademicYearID, &sc.ClassroomID, &sc.SchedulePeriodID, &sc.ClassroomSubjectID, &sc.CreatedAt, &sc.UpdatedAt,
		&subjectName, &classroomName, &dayOfWeek, &periodNo, &startTime, &endTime, &teacherUserID, &subjectID); err != nil {
		return nil, err
	}
	sc.SubjectName = &subjectName
	sc.ClassroomName = &classroomName
	sc.DayOfWeek = &dayOfWeek
	sc.PeriodNo = &periodNo
	sc.StartTime = &startTime
	sc.EndTime = &endTime
	sc.SubjectID = &subjectID
	if teacherUserID.Valid {
		sc.TeacherUserID = &teacherUserID.String
	}
	return &sc, nil
}

// CurrentTermID resolves which term of academicYearID covers today's date
// (start_date <= today <= end_date). Returns "" (not an error) if no term
// has date ranges set or none matches -- callers treat that as "unknown",
// not a hard failure.
func (s *Store) CurrentTermID(ctx context.Context, schoolID, academicYearID string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id FROM terms
		WHERE school_id = $1 AND academic_year_id = $2
		  AND start_date IS NOT NULL AND end_date IS NOT NULL
		  AND CURRENT_DATE BETWEEN start_date AND end_date
		ORDER BY "order" LIMIT 1
	`, schoolID, academicYearID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (s *Store) scanScheduleRows(ctx context.Context, q string, args ...interface{}) ([]model.Schedule, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Schedule{}
	for rows.Next() {
		var sc model.Schedule
		var subjectName, classroomName, startTime, endTime string
		var dayOfWeek, periodNo int
		var teacherUserID sql.NullString
		if err := rows.Scan(&sc.ID, &sc.SchoolID, &sc.AcademicYearID, &sc.ClassroomID, &sc.SchedulePeriodID, &sc.ClassroomSubjectID, &sc.CreatedAt, &sc.UpdatedAt,
			&subjectName, &classroomName, &dayOfWeek, &periodNo, &startTime, &endTime, &teacherUserID); err != nil {
			return nil, err
		}
		sc.SubjectName = &subjectName
		sc.ClassroomName = &classroomName
		sc.DayOfWeek = &dayOfWeek
		sc.PeriodNo = &periodNo
		sc.StartTime = &startTime
		sc.EndTime = &endTime
		if teacherUserID.Valid {
			sc.TeacherUserID = &teacherUserID.String
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *Store) GetSchedule(ctx context.Context, schoolID, id string) (*model.Schedule, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id, created_at, updated_at
		FROM schedules WHERE id = $1 AND school_id = $2
	`, id, schoolID)
	sc, err := scanSchedule(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sc, err
}

func (s *Store) UpdateSchedule(ctx context.Context, schoolID, id string, classroomSubjectID *string) (*model.Schedule, error) {
	row := s.DB.QueryRowContext(ctx, `
		UPDATE schedules SET classroom_subject_id = COALESCE($3, classroom_subject_id), updated_at = now()
		WHERE id = $1 AND school_id = $2
		RETURNING id, school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id, created_at, updated_at
	`, id, schoolID, classroomSubjectID)
	sc, err := scanSchedule(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return sc, err
}

func (s *Store) DeleteSchedule(ctx context.Context, schoolID, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM schedules WHERE id = $1 AND school_id = $2`, id, schoolID)
	return err
}

func scanSchedulePeriod(row rowScanner) (*model.SchedulePeriod, error) {
	var p model.SchedulePeriod
	if err := row.Scan(&p.ID, &p.SchoolID, &p.AcademicYearID, &p.ClassroomID, &p.DayOfWeek, &p.PeriodNo, &p.StartTime, &p.EndTime, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func scanSchedulePeriodRow(rows *sql.Rows) (*model.SchedulePeriod, error) {
	return scanSchedulePeriod(rows)
}

func scanSchedule(row rowScanner) (*model.Schedule, error) {
	var sc model.Schedule
	if err := row.Scan(&sc.ID, &sc.SchoolID, &sc.AcademicYearID, &sc.ClassroomID, &sc.SchedulePeriodID, &sc.ClassroomSubjectID, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		return nil, err
	}
	return &sc, nil
}
