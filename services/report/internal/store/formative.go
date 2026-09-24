package store

import (
	"context"
)

// FormativeScore is one student's saved score for one assessment type
// within a (term, classroom_subject) -- the dropdown-driven replacement
// for the old order_index spreadsheet grid (item 5).
type FormativeScore struct {
	StudentID string
	Score     float64
}

// ListFormativeScoresForType returns every student's saved score for one
// specific assessment type in a (term, classroom_subject), keyed by
// student_id -- the guru's dropdown-filtered grid.
func (s *Store) ListFormativeScoresForType(ctx context.Context, termID, classroomSubjectID, assessmentTypeID string) (map[string]float64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT student_id, score FROM formative_grades
		WHERE term_id = $1 AND classroom_subject_id = $2 AND assessment_type_id = $3`,
		termID, classroomSubjectID, assessmentTypeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var studentID string
		var score float64
		if err := rows.Scan(&studentID, &score); err != nil {
			return nil, err
		}
		out[studentID] = score
	}
	return out, rows.Err()
}

// UpsertFormativeScore saves (or overwrites) one student's score for one
// assessment type -- replaces the old UpsertFormativeGrades' whole-row
// replace-all semantics with a single-cell save matching the new
// dropdown-per-type UX: guru picks a tipe penilaian, enters/edits one
// student's score, saves. This is also how an exam-service auto-populated
// STS/SAS suggestion (see handler.examBucketScores) becomes a
// guru-owned, editable value once saved -- the deliberate "raise a low
// score" business rule from item 5.
func (s *Store) UpsertFormativeScore(ctx context.Context, schoolID, termID, classroomSubjectID, studentID, assessmentTypeID string, score float64, createdBy string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO formative_grades (school_id, term_id, classroom_subject_id, student_id, assessment_type_id, score, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (term_id, classroom_subject_id, student_id, assessment_type_id)
		DO UPDATE SET score = EXCLUDED.score, updated_at = now()`,
		schoolID, termID, classroomSubjectID, studentID, assessmentTypeID, score, createdBy)
	return err
}

// BucketScore is one (student, subject)'s pooled score for a report
// bucket (formatif/praktik/sts/sas), weighted-averaged across every
// assessment type sharing that bucket the student has an entry for.
type BucketScore struct {
	Score *float64
	Count int
}

// GetFormativeBucketScores pools a student's formative_grades entries
// (item 5's dropdown-entered scores) into their report_assessment_types
// bucket, weighted by each type's weight_percent -- e.g. "tugas" (weight
// 60) + "kuis" (weight 40) both bucket='formatif' combine into one Nilai
// Formatif value. Falls back to a plain average within a bucket if every
// present entry has weight 0 (avoids a div-by-zero from an admin
// misconfiguration rather than silently dropping the bucket).
func (s *Store) GetFormativeBucketScores(ctx context.Context, schoolID, termID, classroomSubjectID, studentID string) (map[string]BucketScore, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT rat.bucket, fg.score, rat.weight_percent
		FROM formative_grades fg
		JOIN report_assessment_types rat ON rat.id = fg.assessment_type_id
		WHERE fg.term_id = $1 AND fg.classroom_subject_id = $2 AND fg.student_id = $3 AND rat.school_id = $4`,
		termID, classroomSubjectID, studentID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type accum struct {
		weightedSum, weightSum, plainSum float64
		count                            int
	}
	acc := map[string]*accum{}
	for rows.Next() {
		var bucket string
		var score, weight float64
		if err := rows.Scan(&bucket, &score, &weight); err != nil {
			return nil, err
		}
		a, ok := acc[bucket]
		if !ok {
			a = &accum{}
			acc[bucket] = a
		}
		a.weightedSum += score * weight
		a.weightSum += weight
		a.plainSum += score
		a.count++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := map[string]BucketScore{}
	for bucket, a := range acc {
		var v float64
		if a.weightSum > 0 {
			v = a.weightedSum / a.weightSum
		} else {
			v = a.plainSum / float64(a.count)
		}
		out[bucket] = BucketScore{Score: &v, Count: a.count}
	}
	return out, nil
}
