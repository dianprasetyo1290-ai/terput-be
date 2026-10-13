package store

import (
	"context"
	"sort"
)

// Teacher workload & unassigned detection (PRD FR-40-002, AC-40-01).
//
// Source of truth is classroom_subjects (the guru x mapel x kelas pairing)
// for one academic year, counting only active pairings in live classrooms:
//   - hours_per_week: the planned jam pelajaran per minggu set on the pairing
//   - scheduled_periods: how many Jadwal Pelajaran slots actually use it,
//     so admins can spot a plan/jadwal mismatch.
// A pairing with teacher_user_id NULL is "belum ada guru" (AC-40-01).

type WorkloadAssignment struct {
	ClassroomSubjectID string  `json:"classroom_subject_id"`
	ClassroomID        string  `json:"classroom_id"`
	ClassroomName      string  `json:"classroom_name"`
	GradeLevel         *string `json:"grade_level"`
	SubjectID          string  `json:"subject_id"`
	SubjectName        string  `json:"subject_name"`
	HoursPerWeek       int     `json:"hours_per_week"`
	ScheduledPeriods   int     `json:"scheduled_periods"`
}

type TeacherWorkload struct {
	TeacherUserID    string               `json:"teacher_user_id"`
	HoursPerWeek     int                  `json:"hours_per_week"`
	ScheduledPeriods int                  `json:"scheduled_periods"`
	ClassCount       int                  `json:"class_count"`
	SubjectCount     int                  `json:"subject_count"`
	Assignments      []WorkloadAssignment `json:"assignments"`
}

type WorkloadSummary struct {
	TotalPairs      int `json:"total_pairs"`
	AssignedPairs   int `json:"assigned_pairs"`
	UnassignedPairs int `json:"unassigned_pairs"`
	TotalHours      int `json:"total_hours"`
	UnassignedHours int `json:"unassigned_hours"`
	TeacherCount    int `json:"teacher_count"`
}

type WorkloadReport struct {
	Summary    WorkloadSummary      `json:"summary"`
	Teachers   []TeacherWorkload    `json:"teachers"`
	Unassigned []WorkloadAssignment `json:"unassigned"`
}

func (s *Store) TeachingWorkload(ctx context.Context, schoolID, academicYearID string) (*WorkloadReport, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT cs.id, cs.classroom_id, c.name, c.grade_level, cs.subject_id, sub.name,
		       cs.teacher_user_id, cs.hours_per_week,
		       (SELECT count(*) FROM schedules sc WHERE sc.classroom_subject_id = cs.id)
		FROM classroom_subjects cs
		JOIN classrooms c ON c.id = cs.classroom_id
		JOIN subjects sub ON sub.id = cs.subject_id
		WHERE cs.school_id = $1 AND c.academic_year_id = $2
		  AND cs.is_active = true AND cs.deleted_at IS NULL
		  AND c.deleted_at IS NULL AND sub.deleted_at IS NULL
		ORDER BY c.name, sub.name
	`, schoolID, academicYearID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rep := &WorkloadReport{Teachers: []TeacherWorkload{}, Unassigned: []WorkloadAssignment{}}
	byTeacher := map[string]*TeacherWorkload{}
	classesOf := map[string]map[string]bool{}
	subjectsOf := map[string]map[string]bool{}
	for rows.Next() {
		var a WorkloadAssignment
		var teacher *string
		if err := rows.Scan(&a.ClassroomSubjectID, &a.ClassroomID, &a.ClassroomName, &a.GradeLevel,
			&a.SubjectID, &a.SubjectName, &teacher, &a.HoursPerWeek, &a.ScheduledPeriods); err != nil {
			return nil, err
		}
		rep.Summary.TotalPairs++
		rep.Summary.TotalHours += a.HoursPerWeek
		if teacher == nil || *teacher == "" {
			rep.Summary.UnassignedPairs++
			rep.Summary.UnassignedHours += a.HoursPerWeek
			rep.Unassigned = append(rep.Unassigned, a)
			continue
		}
		rep.Summary.AssignedPairs++
		tw := byTeacher[*teacher]
		if tw == nil {
			tw = &TeacherWorkload{TeacherUserID: *teacher, Assignments: []WorkloadAssignment{}}
			byTeacher[*teacher] = tw
			classesOf[*teacher] = map[string]bool{}
			subjectsOf[*teacher] = map[string]bool{}
		}
		tw.HoursPerWeek += a.HoursPerWeek
		tw.ScheduledPeriods += a.ScheduledPeriods
		tw.Assignments = append(tw.Assignments, a)
		classesOf[*teacher][a.ClassroomID] = true
		subjectsOf[*teacher][a.SubjectID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for id, tw := range byTeacher {
		tw.ClassCount = len(classesOf[id])
		tw.SubjectCount = len(subjectsOf[id])
		rep.Teachers = append(rep.Teachers, *tw)
	}
	// Heaviest load first; teacher id as a stable tie-breaker.
	sort.Slice(rep.Teachers, func(i, j int) bool {
		if rep.Teachers[i].HoursPerWeek != rep.Teachers[j].HoursPerWeek {
			return rep.Teachers[i].HoursPerWeek > rep.Teachers[j].HoursPerWeek
		}
		return rep.Teachers[i].TeacherUserID < rep.Teachers[j].TeacherUserID
	})
	rep.Summary.TeacherCount = len(rep.Teachers)
	return rep, nil
}
