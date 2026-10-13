package store

import (
	"context"
	"fmt"
)

// StudentPromotion is one admin decision: promote this student from their
// source-year classroom into a target-year classroom, or hold them back
// (ToClassroomID nil — covers both "tinggal kelas" and "lulus tidak naik";
// either way no target-year row gets created, same as how graduating
// classes already worked before per-student granularity existed).
type StudentPromotion struct {
	StudentUserID   string  `json:"student_user_id"`
	FromClassroomID string  `json:"from_classroom_id"`
	ToClassroomID   *string `json:"to_classroom_id"`
}

// PromotionResult groups the outcome by (from, to) classroom pair — ToClassroomID
// nil is the "retained / not promoted" bucket for that source classroom.
type PromotionResult struct {
	FromClassroomID string  `json:"from_classroom_id"`
	ToClassroomID   *string `json:"to_classroom_id"`
	StudentsMoved   int     `json:"students_moved"`
}

// PromoteStudents implements "kenaikan kelas" at per-student granularity:
// each entry in promotions is one student's decision, computed by the
// frontend from the roster it already loaded (defaulting every student to
// their class's mapped target, with per-student overrides to hold back —
// see KenaikanKelasView.vue). This is additive only — it never touches or
// deletes the source year's rows — and validates every promotion against
// the source year's actual active roster first, so a stale/tampered client
// payload can't move a student out of a classroom they aren't really in.
func (s *Store) PromoteStudents(ctx context.Context, schoolID, sourceAcademicYearID, targetAcademicYearID string, promotions []StudentPromotion, actorUserID string) ([]PromotionResult, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := setEnrollmentContext(ctx, tx, EnrollmentChange{Action: ActionPromotion, ActorUserID: actorUserID}); err != nil {
		return nil, err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT student_user_id, classroom_id FROM class_enrollments
		WHERE school_id = $1 AND academic_year_id = $2 AND status = 'active'
	`, schoolID, sourceAcademicYearID)
	if err != nil {
		return nil, err
	}
	actualClassroomOf := map[string]string{}
	for rows.Next() {
		var studentID, classroomID string
		if err := rows.Scan(&studentID, &classroomID); err != nil {
			rows.Close()
			return nil, err
		}
		actualClassroomOf[studentID] = classroomID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	// (from_classroom_id, to_classroom_id or "" for retained) -> count
	type resultKey struct {
		from string
		to   string // "" means retained
	}
	counts := map[resultKey]int{}
	order := []resultKey{}

	for _, p := range promotions {
		actual, ok := actualClassroomOf[p.StudentUserID]
		if !ok || actual != p.FromClassroomID {
			return nil, fmt.Errorf("student %s is not an active enrollment of classroom %s in the source academic year", p.StudentUserID, p.FromClassroomID)
		}

		key := resultKey{from: p.FromClassroomID}
		if p.ToClassroomID != nil {
			key.to = *p.ToClassroomID
			// Capacity of the target-year classroom (FR-39-001); checked
			// per student so earlier inserts in this tx count.
			if err := checkCapacity(ctx, tx, *p.ToClassroomID, p.StudentUserID); err != nil {
				return nil, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
				VALUES ($1, $2, $3, $4, 'active')
				ON CONFLICT (academic_year_id, student_user_id) DO UPDATE SET classroom_id = EXCLUDED.classroom_id, status = 'active', updated_at = now()
			`, schoolID, targetAcademicYearID, *p.ToClassroomID, p.StudentUserID); err != nil {
				return nil, err
			}
		}
		if _, seen := counts[key]; !seen {
			order = append(order, key)
		}
		counts[key]++
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	results := make([]PromotionResult, 0, len(order))
	for _, key := range order {
		r := PromotionResult{FromClassroomID: key.from, StudentsMoved: counts[key]}
		if key.to != "" {
			to := key.to
			r.ToClassroomID = &to
		}
		results = append(results, r)
	}
	return results, nil
}
