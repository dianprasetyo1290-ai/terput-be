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

func (s *Store) CreateSchedule(ctx context.Context, sc model.Schedule) (*model.Schedule, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO schedules (school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (schedule_period_id) DO UPDATE SET classroom_subject_id = EXCLUDED.classroom_subject_id, updated_at = now()
		RETURNING id, school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id, created_at, updated_at
	`, sc.SchoolID, sc.AcademicYearID, sc.ClassroomID, sc.SchedulePeriodID, sc.ClassroomSubjectID)
	return scanSchedule(row)
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
