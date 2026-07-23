package store

import "context"

type DashboardCounts struct {
	Siswa int `json:"siswa"`
	Guru  int `json:"guru"`
	Kelas int `json:"kelas"`
}

// DashboardCountsFor computes academic-service's slice of the admin
// dashboard, scoped to school_id + academic_year_id:
//   - siswa: distinct active class_enrollments rows for the year
//   - guru: distinct guru user ids across classroom_subjects.teacher_user_id
//     and classrooms.homeroom_teacher_user_id for the year (see README
//     Assumptions on why we don't ask identity-service for this instead)
//   - kelas: classrooms for the year
func (s *Store) DashboardCountsFor(ctx context.Context, schoolID, academicYearID string) (*DashboardCounts, error) {
	var c DashboardCounts
	if err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM class_enrollments WHERE school_id = $1 AND academic_year_id = $2 AND status = 'active'
	`, schoolID, academicYearID).Scan(&c.Siswa); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM classrooms WHERE school_id = $1 AND academic_year_id = $2 AND deleted_at IS NULL
	`, schoolID, academicYearID).Scan(&c.Kelas); err != nil {
		return nil, err
	}
	if err := s.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM (
			SELECT teacher_user_id AS guru_id FROM classroom_subjects
			WHERE school_id = $1 AND academic_year_id = $2 AND teacher_user_id IS NOT NULL AND deleted_at IS NULL
			UNION
			SELECT homeroom_teacher_user_id AS guru_id FROM classrooms
			WHERE school_id = $1 AND academic_year_id = $2 AND homeroom_teacher_user_id IS NOT NULL AND deleted_at IS NULL
		) g
	`, schoolID, academicYearID).Scan(&c.Guru); err != nil {
		return nil, err
	}
	return &c, nil
}
