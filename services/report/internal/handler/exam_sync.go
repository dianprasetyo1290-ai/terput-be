package handler

import (
	"context"

	"skci/report/internal/model"
)

// Yadika13 items 4 & 5: "Setelah ujian selesai dan guru mengoreksi di Bank
// Soal, nilai hasil koreksi harus otomatis masuk ke Penilaian Formatif" and
// "Nilai Murni STS awalnya 70 ... hasil koreksi = 80, maka Nilai Murni STS
// otomatis berubah menjadi 80".
//
// Root cause: an exam-derived score is READ live from exam-service, but then
// COPIED into report_card_subjects when Generate Rapor runs. A later koreksi
// moves the exam score and nothing ever re-reads it, so every rapor screen
// keeps showing the number from the last generate. The fix is to re-read on
// the screens that display it, which is what this file does -- one batched
// pass per classroom+subject+term, writing back only what actually changed.

// examBucketScores maps bucket ("sts"/"sas") -> student id -> exam-derived
// score for one classroom+subject+term, averaged over that bucket's exams.
//
// Batched on purpose: this replaces a per-student resolveExamBucketScore
// for ONE student and was being called inside a per-student loop, which
// re-fetched the exam list, the exam types and every exam's score sheet once
// per student. Same data, same bucketing rule, fetched once.
func (h *Handler) examBucketScores(ctx context.Context, classroomID, termID, academicYearID, subjectID string) map[string]map[string]float64 {
	exams, err := h.d.Exam.ListExams(ctx, classroomID, termID)
	if err != nil {
		return nil
	}
	examTypes, err := h.d.Exam.ListExamTypes(ctx, academicYearID)
	if err != nil {
		return nil
	}
	mappingByType := map[string]string{}
	nameByType := map[string]string{}
	for _, et := range examTypes {
		mappingByType[et.ID] = et.MappingRapor
		nameByType[et.ID] = et.Name
	}

	type acc struct {
		sum   float64
		count int
	}
	totals := map[string]map[string]*acc{}
	for _, e := range exams {
		if e.SubjectID != subjectID {
			continue
		}
		m := mappingByType[e.ExamTypeID]
		if m == "" {
			continue
		}
		bucket := classifyExamType(m, nameByType[e.ExamTypeID])
		if bucket != "sts" && bucket != "sas" {
			continue
		}
		scores, err := h.d.Exam.GetScores(ctx, e.ID)
		if err != nil {
			continue
		}
		for _, sc := range scores {
			if sc.Score == nil {
				continue
			}
			if totals[bucket] == nil {
				totals[bucket] = map[string]*acc{}
			}
			a := totals[bucket][sc.StudentID]
			if a == nil {
				a = &acc{}
				totals[bucket][sc.StudentID] = a
			}
			a.sum += *sc.Score
			a.count++
		}
	}

	out := map[string]map[string]float64{}
	for bucket, byStudent := range totals {
		out[bucket] = make(map[string]float64, len(byStudent))
		for studentID, a := range byStudent {
			out[bucket][studentID] = a.sum / float64(a.count)
		}
	}
	return out
}

// refreshExamScores re-reads the exam-derived Nilai STS/SAS for one
// classroom+subject+term and writes back any report_card_subjects row whose
// stored value has gone stale -- the automatic half of items 4 & 5.
//
// What it will NOT touch: a bucket the guru has entered by hand in Penilaian
// Formatif. GenerateReportCards' rule is that a saved formative_grades row
// wins over the raw exam average (the deliberate "raise a low score so the
// student can pass" rule), and silently overwriting that with a fresh exam
// number here would undo a teacher's decision -- a worse bug than the stale
// one being fixed.
//
// Best-effort throughout: exam-service being down, or a single write
// failing, leaves the stored value in place rather than failing the page.
// Callers re-read report_card_subjects afterwards and get the fresh values.
func (h *Handler) refreshExamScores(ctx context.Context, schoolID, classroomID, termID, subjectID string, cards []model.ReportCard) {
	if len(cards) == 0 {
		return
	}
	fresh := h.examBucketScores(ctx, classroomID, termID, cards[0].AcademicYearID, subjectID)
	if len(fresh) == 0 {
		return
	}

	// classroom_subject_id is what formative_grades is keyed by; the report
	// card only carries subject_id.
	classroomSubjectID := ""
	if subs, err := h.d.Academic.ListClassroomSubjects(ctx, classroomID, termID); err == nil {
		for _, cs := range subs {
			if cs.SubjectID == subjectID {
				classroomSubjectID = cs.ID
				break
			}
		}
	}

	weights, err := h.d.Store.GetWeightSettings(ctx, schoolID)
	if err != nil {
		return
	}
	wholeScores := false
	if classroom, err := h.d.Academic.GetClassroom(ctx, classroomID); err == nil && classroom != nil {
		wholeScores = roundsToWholeScores(classroom.GradeLevel)
	}

	for _, rc := range cards {
		subjects, err := h.d.Store.ListSubjects(ctx, rc.ID)
		if err != nil {
			continue
		}
		for _, sub := range subjects {
			if sub.SubjectID != subjectID {
				continue
			}

			// A bucket the guru has entered by hand is off limits (see
			// this function's doc comment).
			buckets := map[string]bool{}
			if classroomSubjectID != "" {
				if b, err := h.d.Store.GetFormativeBucketScores(ctx, schoolID, termID, classroomSubjectID, rc.StudentID); err == nil {
					for name := range b {
						buckets[name] = true
					}
				}
			}

			sts, sas := sub.StsScore, sub.SasScore
			changed := false
			if v, ok := fresh["sts"][rc.StudentID]; ok && !buckets["sts"] {
				rounded := maybeRound(v, wholeScores)
				if !sameScore(sts, rounded) {
					sts = rounded
					changed = true
				}
			}
			if v, ok := fresh["sas"][rc.StudentID]; ok && !buckets["sas"] {
				rounded := maybeRound(v, wholeScores)
				if !sameScore(sas, rounded) {
					sas = rounded
					changed = true
				}
			}
			if !changed {
				continue
			}

			computed := computeSubjectScore("semester", weights, sub.FormatifScore, sub.PraktikScore, sts, sas)
			if wholeScores {
				computed = roundScore(computed)
			}
			_ = h.d.Store.UpsertSubjectComputed(ctx, rc.ID, schoolID, sub.SubjectID, sub.SubjectName, sub.SubjectCode,
				computed, sub.FormatifScore, sts, sas, sub.PraktikScore, sub.SourceExamCount, nowPtr(), nil)
		}
	}
}

func maybeRound(v float64, whole bool) *float64 {
	p := &v
	if whole {
		return roundScore(p)
	}
	return p
}

// sameScore compares a stored (possibly NULL) score against a fresh one,
// tolerating the float noise an average leaves behind.
func sameScore(stored, fresh *float64) bool {
	if stored == nil || fresh == nil {
		return stored == fresh
	}
	d := *stored - *fresh
	return d < 1e-9 && d > -1e-9
}
