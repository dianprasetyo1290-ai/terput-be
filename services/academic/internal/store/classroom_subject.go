package store

import (
	"context"
	"database/sql"

	"skci/academic/internal/model"
)

func (s *Store) AddClassroomSubject(ctx context.Context, cs model.ClassroomSubject) (*model.ClassroomSubject, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, is_active)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6, true))
		ON CONFLICT (classroom_id, subject_id) DO UPDATE SET teacher_user_id = EXCLUDED.teacher_user_id, is_active = EXCLUDED.is_active, deleted_at = NULL, updated_at = now()
		RETURNING id, school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, is_active, created_at, updated_at
	`, cs.SchoolID, cs.AcademicYearID, cs.ClassroomID, cs.SubjectID, cs.TeacherUserID, cs.IsActive)
	return scanClassroomSubject(row)
}

// ListClassroomSubjects returns the (subject x guru) mapping for a
// classroom. Also serves as the "Matriks Pelajaran" listing
// (GET /classrooms/{id}/subject-matrix) since is_active lives on this row.
func (s *Store) ListClassroomSubjects(ctx context.Context, schoolID, classroomID string, activeOnly bool) ([]model.ClassroomSubject, error) {
	q := `
		SELECT cs.id, cs.school_id, cs.academic_year_id, cs.classroom_id, cs.subject_id, cs.teacher_user_id, cs.is_active, cs.created_at, cs.updated_at, sub.name, sub.subject_group
		FROM classroom_subjects cs
		JOIN subjects sub ON sub.id = cs.subject_id
		WHERE ($1 = '' OR cs.school_id::text = $1) AND cs.classroom_id = $2 AND cs.deleted_at IS NULL
	`
	if activeOnly {
		q += " AND cs.is_active = true"
	}
	q += " ORDER BY sub.name"
	rows, err := s.DB.QueryContext(ctx, q, schoolID, classroomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ClassroomSubject{}
	for rows.Next() {
		var cs model.ClassroomSubject
		var subjectName string
		var subjectGroup *string
		if err := rows.Scan(&cs.ID, &cs.SchoolID, &cs.AcademicYearID, &cs.ClassroomID, &cs.SubjectID, &cs.TeacherUserID, &cs.IsActive, &cs.CreatedAt, &cs.UpdatedAt, &subjectName, &subjectGroup); err != nil {
			return nil, err
		}
		cs.SubjectName = &subjectName
		cs.SubjectGroup = subjectGroup
		out = append(out, cs)
	}
	return out, rows.Err()
}

func (s *Store) GetClassroomSubject(ctx context.Context, schoolID, id string) (*model.ClassroomSubject, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, is_active, created_at, updated_at
		FROM classroom_subjects WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
	`, id, schoolID)
	cs, err := scanClassroomSubject(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cs, err
}

func (s *Store) UpdateClassroomSubject(ctx context.Context, schoolID, id string, teacherUserID *string, isActive *bool) (*model.ClassroomSubject, error) {
	row := s.DB.QueryRowContext(ctx, `
		UPDATE classroom_subjects SET
			teacher_user_id = COALESCE($3, teacher_user_id),
			is_active = COALESCE($4, is_active),
			updated_at = now()
		WHERE id = $1 AND school_id = $2 AND deleted_at IS NULL
		RETURNING id, school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, is_active, created_at, updated_at
	`, id, schoolID, teacherUserID, isActive)
	cs, err := scanClassroomSubject(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cs, err
}

func (s *Store) DeleteClassroomSubject(ctx context.Context, schoolID, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE classroom_subjects SET deleted_at = now() WHERE id = $1 AND school_id = $2`, id, schoolID)
	return err
}

// --- Student subject enrollments (resolves subject-group variant per student) ---

func (s *Store) SetStudentSubjectEnrollment(ctx context.Context, e model.StudentSubjectEnrollment) (*model.StudentSubjectEnrollment, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO student_subject_enrollments (school_id, academic_year_id, student_user_id, classroom_subject_id, is_enrolled)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (student_user_id, classroom_subject_id) DO UPDATE SET is_enrolled = EXCLUDED.is_enrolled, updated_at = now()
		RETURNING id, school_id, academic_year_id, student_user_id, classroom_subject_id, is_enrolled, created_at, updated_at
	`, e.SchoolID, e.AcademicYearID, e.StudentUserID, e.ClassroomSubjectID, e.IsEnrolled)
	var out model.StudentSubjectEnrollment
	if err := row.Scan(&out.ID, &out.SchoolID, &out.AcademicYearID, &out.StudentUserID, &out.ClassroomSubjectID, &out.IsEnrolled, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListEnrollmentsForClassroom returns every existing student_subject_enrollments
// row scoped to this classroom's subjects — the raw per-cell state behind
// the Matriks Pelajaran per-student grid (GET .../student-subject-matrix).
// A missing (student, classroom_subject) pair just means "not decided yet";
// the frontend renders that as unchecked.
func (s *Store) ListEnrollmentsForClassroom(ctx context.Context, schoolID, classroomID string) ([]model.StudentSubjectEnrollment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT sse.id, sse.school_id, sse.academic_year_id, sse.student_user_id, sse.classroom_subject_id, sse.is_enrolled, sse.created_at, sse.updated_at
		FROM student_subject_enrollments sse
		JOIN classroom_subjects cs ON cs.id = sse.classroom_subject_id
		WHERE ($1 = '' OR sse.school_id::text = $1) AND cs.classroom_id = $2 AND cs.deleted_at IS NULL
	`, schoolID, classroomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.StudentSubjectEnrollment{}
	for rows.Next() {
		var e model.StudentSubjectEnrollment
		if err := rows.Scan(&e.ID, &e.SchoolID, &e.AcademicYearID, &e.StudentUserID, &e.ClassroomSubjectID, &e.IsEnrolled, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// BulkSetStudentSubjectEnrollments upserts many (student, classroom_subject)
// checkbox states in one transaction — backs Ceklis Semua / Kosongkan Semua
// / Sync Matrix / Auto-Enroll Agama, which all reduce to "here is the set of
// cells that should now be checked/unchecked" once the frontend has done
// its own diffing/matching (see MatriksPelajaranView.vue).
//
// siblingsOf maps classroom_subject_id -> the OTHER classroom_subject_ids
// in the same exclusivity group (e.g. every other Agama variant), already
// resolved by the caller (see normalizeGroup in the handler — real subject
// data turned out to use per-religion group strings like "Agama Islam"
// rather than one shared "Agama", so the grouping decision has to happen
// once in Go, not be re-derived from the raw column here). When an item
// enrolls (is_enrolled=true) a student into a subject that has siblings,
// those siblings are unset for that student first — "satu siswa hanya bisa
// terdaftar ke satu mata pelajaran dalam grup agama... akan otomatis
// dipindahkan" from the spec. This is the one place all four buttons' writes
// funnel through, so the invariant holds regardless of which button (or a
// future caller) triggers the write.
func (s *Store) BulkSetStudentSubjectEnrollments(ctx context.Context, schoolID, academicYearID, classroomID string, items []model.StudentSubjectEnrollment, siblingsOf map[string][]string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, item := range items {
		if item.IsEnrolled {
			// Siblings lists are small (a handful of religion variants at
			// most) — a plain loop of scalar UPDATEs avoids pulling in an
			// array-param dependency academic-service doesn't otherwise need.
			for _, sibling := range siblingsOf[item.ClassroomSubjectID] {
				if _, err := tx.ExecContext(ctx, `
					UPDATE student_subject_enrollments SET is_enrolled = false, updated_at = now()
					WHERE student_user_id = $1 AND classroom_subject_id = $2
				`, item.StudentUserID, sibling); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO student_subject_enrollments (school_id, academic_year_id, student_user_id, classroom_subject_id, is_enrolled)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (student_user_id, classroom_subject_id) DO UPDATE SET is_enrolled = EXCLUDED.is_enrolled, updated_at = now()
		`, schoolID, academicYearID, item.StudentUserID, item.ClassroomSubjectID, item.IsEnrolled); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ActiveSubjectsForStudent resolves which subjects (and which specific
// classroom_subjects row, i.e. which elective variant) are active for a
// given student: the classroom_subjects for their classroom that are
// is_active=true AND (not in a subject_group that has student-level
// enrollment rows, OR this student is the one is_enrolled=true row for that
// group). This is what exam-service is meant to call via
// GET /students/{id}/active-subjects to decide which exam variant a student
// sees.
func (s *Store) ActiveSubjectsForStudent(ctx context.Context, schoolID, studentUserID, academicYearID string) ([]model.ClassroomSubject, error) {
	rows, err := s.DB.QueryContext(ctx, `
		WITH student_classroom AS (
			SELECT classroom_id FROM class_enrollments
			WHERE ($1 = '' OR school_id::text = $1) AND student_user_id = $2 AND academic_year_id = $3 AND status = 'active'
			LIMIT 1
		)
		SELECT cs.id, cs.school_id, cs.academic_year_id, cs.classroom_id, cs.subject_id, cs.teacher_user_id, cs.is_active, cs.created_at, cs.updated_at, sub.name
		FROM classroom_subjects cs
		JOIN subjects sub ON sub.id = cs.subject_id
		WHERE ($1 = '' OR cs.school_id::text = $1)
		  AND cs.classroom_id IN (SELECT classroom_id FROM student_classroom)
		  AND cs.is_active = true
		  AND cs.deleted_at IS NULL
		  AND (
		    sub.subject_group IS NULL
		    OR EXISTS (
		      SELECT 1 FROM student_subject_enrollments sse
		      WHERE sse.classroom_subject_id = cs.id AND sse.student_user_id = $2 AND sse.is_enrolled = true
		    )
		    OR NOT EXISTS (
		      -- no explicit enrollment rows exist yet for this subject_group's variants for this student;
		      -- fall back to showing all active variants (nothing has resolved the split yet)
		      SELECT 1 FROM student_subject_enrollments sse2
		      JOIN classroom_subjects cs2 ON cs2.id = sse2.classroom_subject_id
		      JOIN subjects sub2 ON sub2.id = cs2.subject_id
		      WHERE sse2.student_user_id = $2 AND sub2.subject_group = sub.subject_group
		    )
		  )
		ORDER BY sub.name
	`, schoolID, studentUserID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ClassroomSubject{}
	for rows.Next() {
		var cs model.ClassroomSubject
		var subjectName string
		if err := rows.Scan(&cs.ID, &cs.SchoolID, &cs.AcademicYearID, &cs.ClassroomID, &cs.SubjectID, &cs.TeacherUserID, &cs.IsActive, &cs.CreatedAt, &cs.UpdatedAt, &subjectName); err != nil {
			return nil, err
		}
		cs.SubjectName = &subjectName
		out = append(out, cs)
	}
	return out, rows.Err()
}

// TeachingAssignment is one (classroom, subject) a guru is assigned to teach
// — consumed by evaluation-service's Rapor Kinerja Guru print (Kelas yang
// Diampu / Classes Taught + Mata Pelajaran / Subject fields) and
// report-service's Penilaian Formatif (ClassroomSubjectID scopes which
// formative-grade grid a guru may edit).
type TeachingAssignment struct {
	ClassroomSubjectID string `json:"classroom_subject_id"`
	ClassroomID        string `json:"classroom_id"`
	ClassroomName      string `json:"classroom_name"`
	SubjectID          string `json:"subject_id"`
	SubjectName        string `json:"subject_name"`
}

// ListTeachingAssignments returns every active classroom+subject a teacher
// is assigned to, across all schools (internal caller passes no school_id
// scope — a teacher only ever belongs to one school in practice, and this
// mirrors GetUser's cross-school lookup pattern for internal calls).
func (s *Store) ListTeachingAssignments(ctx context.Context, teacherUserID string) ([]TeachingAssignment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT cs.id, cs.classroom_id, c.name, cs.subject_id, sub.name
		FROM classroom_subjects cs
		JOIN classrooms c ON c.id = cs.classroom_id
		JOIN subjects sub ON sub.id = cs.subject_id
		WHERE cs.teacher_user_id = $1 AND cs.is_active = true AND cs.deleted_at IS NULL AND c.deleted_at IS NULL
		ORDER BY c.name, sub.name
	`, teacherUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TeachingAssignment{}
	for rows.Next() {
		var ta TeachingAssignment
		if err := rows.Scan(&ta.ClassroomSubjectID, &ta.ClassroomID, &ta.ClassroomName, &ta.SubjectID, &ta.SubjectName); err != nil {
			return nil, err
		}
		out = append(out, ta)
	}
	return out, rows.Err()
}

func scanClassroomSubject(row rowScanner) (*model.ClassroomSubject, error) {
	var cs model.ClassroomSubject
	if err := row.Scan(&cs.ID, &cs.SchoolID, &cs.AcademicYearID, &cs.ClassroomID, &cs.SubjectID, &cs.TeacherUserID, &cs.IsActive, &cs.CreatedAt, &cs.UpdatedAt); err != nil {
		return nil, err
	}
	return &cs, nil
}
