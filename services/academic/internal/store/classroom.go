package store

import (
	"context"
	"database/sql"

	"skci/academic/internal/model"
)

func (s *Store) CreateClassroom(ctx context.Context, c model.Classroom) (*model.Classroom, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO classrooms (school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, created_at, updated_at
	`, c.SchoolID, c.AcademicYearID, c.Name, c.GradeLevel, c.HomeroomTeacherUserID)
	return scanClassroom(row)
}

func (s *Store) ListClassrooms(ctx context.Context, schoolID, academicYearID string, page, perPage int) ([]model.Classroom, int, error) {
	var total int
	if err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM classrooms WHERE school_id = $1 AND academic_year_id = $2 AND deleted_at IS NULL
	`, schoolID, academicYearID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, created_at, updated_at
		FROM classrooms WHERE school_id = $1 AND academic_year_id = $2 AND deleted_at IS NULL
		ORDER BY name LIMIT $3 OFFSET $4
	`, schoolID, academicYearID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.Classroom{}
	for rows.Next() {
		c, err := scanClassroomRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

func (s *Store) GetClassroom(ctx context.Context, schoolID, id string) (*model.Classroom, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, created_at, updated_at
		FROM classrooms WHERE id = $1 AND ($2 = '' OR school_id::text = $2) AND deleted_at IS NULL
	`, id, schoolID)
	c, err := scanClassroom(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *Store) UpdateClassroom(ctx context.Context, schoolID, id string, name, gradeLevel, homeroomTeacherUserID *string) (*model.Classroom, error) {
	row := s.DB.QueryRowContext(ctx, `
		UPDATE classrooms SET
			name = COALESCE($3, name),
			grade_level = COALESCE($4, grade_level),
			homeroom_teacher_user_id = COALESCE($5, homeroom_teacher_user_id),
			updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
		RETURNING id, school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, created_at, updated_at
	`, id, schoolID, name, gradeLevel, homeroomTeacherUserID)
	c, err := scanClassroom(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

func (s *Store) DeleteClassroom(ctx context.Context, schoolID, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE classrooms SET deleted_at = now() WHERE id = $1 AND school_id = $2`, id, schoolID)
	return err
}

// WaliKelas is the auto-derived Kelas <-> Guru pairing, sourced live from
// classrooms.homeroom_teacher_user_id (no separate storage).
type WaliKelas struct {
	ClassroomID           string `json:"classroom_id"`
	ClassroomName         string `json:"classroom_name"`
	HomeroomTeacherUserID string `json:"homeroom_teacher_user_id"`
	StudentCount          int    `json:"student_count"`
}

func (s *Store) ListWaliKelas(ctx context.Context, schoolID, academicYearID string) ([]WaliKelas, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.id, c.name, c.homeroom_teacher_user_id,
			(SELECT count(*) FROM class_enrollments ce WHERE ce.classroom_id = c.id AND ce.status = 'active')
		FROM classrooms c
		WHERE c.school_id = $1 AND c.academic_year_id = $2 AND c.deleted_at IS NULL AND c.homeroom_teacher_user_id IS NOT NULL
		ORDER BY c.name
	`, schoolID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WaliKelas{}
	for rows.Next() {
		var w WaliKelas
		if err := rows.Scan(&w.ClassroomID, &w.ClassroomName, &w.HomeroomTeacherUserID, &w.StudentCount); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// --- Class enrollments (siswa assigned to a classroom) ---

func (s *Store) AddClassEnrollment(ctx context.Context, e model.ClassEnrollment) (*model.ClassEnrollment, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, ''), 'active'))
		ON CONFLICT (academic_year_id, student_user_id) DO UPDATE SET classroom_id = EXCLUDED.classroom_id, status = EXCLUDED.status, updated_at = now()
		RETURNING id, school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at, created_at, updated_at
	`, e.SchoolID, e.AcademicYearID, e.ClassroomID, e.StudentUserID, e.Status)
	return scanClassEnrollment(row)
}

func (s *Store) ListClassEnrollments(ctx context.Context, schoolID, classroomID string) ([]model.ClassEnrollment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at, created_at, updated_at
		FROM class_enrollments WHERE ($1 = '' OR school_id::text = $1) AND classroom_id = $2 AND status = 'active'
		ORDER BY enrolled_at
	`, schoolID, classroomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ClassEnrollment{}
	for rows.Next() {
		e, err := scanClassEnrollmentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// ListClassEnrollmentsByYear returns every active enrollment for an academic
// year across ALL classrooms (not just one) — used by the frontend to know
// which students already have a classroom assignment this year (a student
// belongs to exactly one classroom per year, per the class_enrollments
// UNIQUE(academic_year_id, student_user_id) constraint) and to render a
// student->classroom roster view.
func (s *Store) ListClassEnrollmentsByYear(ctx context.Context, schoolID, academicYearID string) ([]model.ClassEnrollment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at, created_at, updated_at
		FROM class_enrollments WHERE ($1 = '' OR school_id::text = $1) AND academic_year_id = $2 AND status = 'active'
		ORDER BY enrolled_at
	`, schoolID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ClassEnrollment{}
	for rows.Next() {
		e, err := scanClassEnrollmentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (s *Store) RemoveClassEnrollment(ctx context.Context, schoolID, classroomID, studentUserID string) error {
	_, err := s.DB.ExecContext(ctx, `
		DELETE FROM class_enrollments WHERE school_id = $1 AND classroom_id = $2 AND student_user_id = $3
	`, schoolID, classroomID, studentUserID)
	return err
}

func scanClassroom(row rowScanner) (*model.Classroom, error) {
	var c model.Classroom
	if err := row.Scan(&c.ID, &c.SchoolID, &c.AcademicYearID, &c.Name, &c.GradeLevel, &c.HomeroomTeacherUserID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func scanClassroomRow(rows *sql.Rows) (*model.Classroom, error) {
	return scanClassroom(rows)
}

func scanClassEnrollment(row rowScanner) (*model.ClassEnrollment, error) {
	var e model.ClassEnrollment
	if err := row.Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.ClassroomID, &e.StudentUserID, &e.Status, &e.EnrolledAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	return &e, nil
}

func scanClassEnrollmentRow(rows *sql.Rows) (*model.ClassEnrollment, error) {
	return scanClassEnrollment(rows)
}
