package store

import "context"

// EnrollmentStatusRow is one class_enrollments row read by sibling services
// (T13: notification's alumni segment, admission's referral eligibility).
type EnrollmentStatusRow struct {
	StudentUserID  string `json:"student_user_id"`
	ClassroomID    string `json:"classroom_id"`
	AcademicYearID string `json:"academic_year_id"`
	Status         string `json:"status"`
}

// ListEnrollmentsByStatus returns a school's enrollments, optionally
// filtered by status and/or one student. Read-only.
func (s *Store) ListEnrollmentsByStatus(ctx context.Context, schoolID, status, studentUserID string) ([]EnrollmentStatusRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT student_user_id, classroom_id, academic_year_id, status
		FROM class_enrollments
		WHERE school_id=$1 AND ($2='' OR status=$2) AND ($3='' OR student_user_id::text=$3)
		ORDER BY enrolled_at DESC`, schoolID, status, studentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EnrollmentStatusRow{}
	for rows.Next() {
		var e EnrollmentStatusRow
		if err := rows.Scan(&e.StudentUserID, &e.ClassroomID, &e.AcademicYearID, &e.Status); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
