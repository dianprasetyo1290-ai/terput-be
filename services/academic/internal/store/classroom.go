package store

import (
	"context"
	"database/sql"

	"skci/academic/internal/model"
)

// classroomCols is the shared projection for every classroom read/RETURNING.
// student_count (active enrollments) is derived, not stored, so capacity
// checks and the Data Kelas list always see the live roster size.
const classroomCols = `id, school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity,
	(SELECT count(*) FROM class_enrollments ce WHERE ce.classroom_id = classrooms.id AND ce.status = 'active'),
	created_at, updated_at`

func (s *Store) CreateClassroom(ctx context.Context, c model.Classroom) (*model.Classroom, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO classrooms (school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+classroomCols, c.SchoolID, c.AcademicYearID, c.Name, c.GradeLevel, c.HomeroomTeacherUserID, c.Capacity)
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
		SELECT `+classroomCols+`
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
		SELECT `+classroomCols+`
		FROM classrooms WHERE id = $1 AND ($2 = '' OR school_id::text = $2) AND deleted_at IS NULL
	`, id, schoolID)
	c, err := scanClassroom(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// ClassroomPatch carries UpdateClassroom's optional fields. nil = unchanged.
// Capacity: nil = unchanged, *0 = clear (unlimited), >0 = set.
type ClassroomPatch struct {
	Name                  *string
	GradeLevel            *string
	HomeroomTeacherUserID *string
	Capacity              *int
}

// UpdateClassroom applies a patch. Wali kelas is a single column, so
// assigning a new guru REPLACES the previous one (FR-39-002 / AC-39-01:
// a class always has at most exactly one wali kelas). Lowering capacity
// below the current active roster is refused with a *CapacityError.
func (s *Store) UpdateClassroom(ctx context.Context, schoolID, id string, p ClassroomPatch) (*model.Classroom, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var capSet bool
	var capVal *int
	if p.Capacity != nil {
		capSet = true
		if *p.Capacity > 0 {
			v := *p.Capacity
			capVal = &v
			var name string
			var active int
			err := tx.QueryRowContext(ctx, `
				SELECT name, (SELECT count(*) FROM class_enrollments ce WHERE ce.classroom_id = classrooms.id AND ce.status = 'active')
				FROM classrooms WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL FOR UPDATE
			`, id, schoolID).Scan(&name, &active)
			if err == sql.ErrNoRows {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			if active > v {
				return nil, &CapacityError{ClassroomName: name, Capacity: v, Enrolled: active, Lowering: true}
			}
		}
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE classrooms SET
			name = COALESCE($3, name),
			grade_level = COALESCE($4, grade_level),
			homeroom_teacher_user_id = COALESCE($5, homeroom_teacher_user_id),
			capacity = CASE WHEN $6::bool THEN $7::int ELSE capacity END,
			updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
		RETURNING `+classroomCols, id, schoolID, p.Name, p.GradeLevel, p.HomeroomTeacherUserID, capSet, capVal)
	c, err := scanClassroom(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, tx.Commit()
}

// DeleteClassroom soft-deletes the classroom AND removes its class_enrollments
// rows in the same transaction. The classrooms row itself is only
// soft-deleted (deleted_at), which means its `class_enrollments.classroom_id
// REFERENCES classrooms(id) ON DELETE CASCADE` FK never actually fires — the
// row still exists. Without this explicit cleanup, DashboardCountsFor's siswa
// count (raw `class_enrollments WHERE status='active'`, not joined against
// classrooms) keeps counting students in a class that Data Kelas already
// shows as deleted (see Report sekoci V3.docx: Total Siswa stuck at 422 after
// every class was deleted).
func (s *Store) DeleteClassroom(ctx context.Context, schoolID, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setEnrollmentContext(ctx, tx, EnrollmentChange{Action: ActionClassroomDeleted}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE classrooms SET deleted_at = now() WHERE id = $1 AND school_id = $2`, id, schoolID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id = $1 AND classroom_id = $2`, schoolID, id); err != nil {
		return err
	}
	return tx.Commit()
}

// RemoveStudentEnrollments hard-deletes every class_enrollments row for a
// student across all classrooms/years in a school. Called from
// identity-service's DeleteUser (via AcademicClient) when the deleted
// account holds a siswa role — the mirror-image gap of DeleteClassroom
// above: deleting the student first, before their class, orphaned the same
// way.
func (s *Store) RemoveStudentEnrollments(ctx context.Context, schoolID, studentUserID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := setEnrollmentContext(ctx, tx, EnrollmentChange{Action: ActionStudentDeleted}); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id = $1 AND student_user_id = $2`, schoolID, studentUserID); err != nil {
		return err
	}
	return tx.Commit()
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

// AddClassEnrollment puts a student into a classroom (re-pointing their
// existing row for that academic year if they were elsewhere — that move is
// what class_enrollment_history records). Refuses with *CapacityError when
// the classroom is full (FR-39-001).
func (s *Store) AddClassEnrollment(ctx context.Context, e model.ClassEnrollment, ch EnrollmentChange) (*model.ClassEnrollment, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := checkCapacity(ctx, tx, e.ClassroomID, e.StudentUserID); err != nil {
		return nil, err
	}
	if err := setEnrollmentContext(ctx, tx, ch); err != nil {
		return nil, err
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5, ''), 'active'))
		ON CONFLICT (academic_year_id, student_user_id) DO UPDATE SET classroom_id = EXCLUDED.classroom_id, status = EXCLUDED.status, updated_at = now()
		RETURNING id, school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at, created_at, updated_at
	`, e.SchoolID, e.AcademicYearID, e.ClassroomID, e.StudentUserID, e.Status)
	out, err := scanClassEnrollment(row)
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
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

func (s *Store) RemoveClassEnrollment(ctx context.Context, schoolID, classroomID, studentUserID string, ch EnrollmentChange) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if ch.Action == "" {
		ch.Action = ActionRemove
	}
	if err := setEnrollmentContext(ctx, tx, ch); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM class_enrollments WHERE school_id = $1 AND classroom_id = $2 AND student_user_id = $3
	`, schoolID, classroomID, studentUserID); err != nil {
		return err
	}
	return tx.Commit()
}

func scanClassroom(row rowScanner) (*model.Classroom, error) {
	var c model.Classroom
	if err := row.Scan(&c.ID, &c.SchoolID, &c.AcademicYearID, &c.Name, &c.GradeLevel, &c.HomeroomTeacherUserID, &c.Capacity, &c.StudentCount, &c.CreatedAt, &c.UpdatedAt); err != nil {
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
