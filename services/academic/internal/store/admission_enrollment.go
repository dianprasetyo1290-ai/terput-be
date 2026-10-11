package store

import (
	"context"

	"skci/academic/internal/model"
)

// UpsertAdmissionEnrollment is the write behind admission-service's
// POST /internal/admission/enrollments/upsert (see
// services/admission/internal/handler/conversion.go academicAdapter.Enroll).
//
// It is idempotent on class_enrollments' (academic_year_id, student_user_id)
// unique key: replaying the conversion saga's enrollment step re-points the
// same row at the supplied classroom and forces status back to 'active'
// instead of inserting a duplicate — a student belongs to at most one
// classroom per academic year. Scoped by school_id so a caller can never
// write outside the tenant named in the request.
//
// The classroom_id / academic_year_id pair is validated against the tenant by
// the caller (internal/handler/admission_enrollment.go) before this runs,
// because this endpoint is reached with X-Internal-Key and no user JWT.
func (s *Store) UpsertAdmissionEnrollment(ctx context.Context, e model.ClassEnrollment) (*model.ClassEnrollment, error) {
	row := s.DB.QueryRowContext(ctx, `
		INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
		VALUES ($1, $2, $3, $4, 'active')
		ON CONFLICT (academic_year_id, student_user_id)
		DO UPDATE SET classroom_id = EXCLUDED.classroom_id, status = 'active', updated_at = now()
		RETURNING id, school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at, created_at, updated_at
	`, e.SchoolID, e.AcademicYearID, e.ClassroomID, e.StudentUserID)
	return scanClassEnrollment(row)
}
