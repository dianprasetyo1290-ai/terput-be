package upstream

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ExamClient talks to exam-service (port 8084), which owns exams and
// scores. See README > Expected upstream contracts for the exact shape
// assumed here — exam-service had no routes yet at the time this was
// written (parallel build), so this is a best-guess contract.
type ExamClient struct{ c *client }

func NewExamClient(baseURL, internalKey string, timeout time.Duration) *ExamClient {
	return &ExamClient{c: newClient(baseURL, internalKey, "exam", timeout)}
}

// Exam is one exam within a classroom+term+subject. Confirmed against
// exam-service's real model.Exam (internal/model): it carries exam_type_id
// but NOT the mapping_rapor value inline — mapping_rapor lives on the
// separate ExamType record, so report-service joins the two client-side
// (see ListExamTypes below and GenerateReportCards).
type Exam struct {
	ID          string `json:"id"`
	SubjectID   string `json:"subject_id"`
	ClassroomID string `json:"classroom_id"`
	TermID      string `json:"term_id"`
	Title       string `json:"title"`
	ExamTypeID  string `json:"exam_type_id"`
}

// ExamType carries the "Mapping Rapor" field (MappingRapor) — the
// report-card score bucket an exam's results belong in. An empty
// MappingRapor means exams of that type are NOT meant to feed into the
// report card at all (e.g. old system's "Test Kemampuan Akademik" type had
// a null grade_mapping), and report-service excludes them from aggregation.
// Field names/shape confirmed against exam-service's real
// internal/model.ExamType.
type ExamType struct {
	ID             string `json:"id"`
	SchoolID       string `json:"school_id"`
	AcademicYearID string `json:"academic_year_id"`
	Name           string `json:"name"`
	MappingRapor   string `json:"mapping_rapor"`
}

// Score is one student's result for one exam. This endpoint is a
// report-service best-guess (exam-service had no scores/results endpoint
// built yet at the time of writing) but is given verbatim by the
// report-service spec: "exam-service ... exposes GET /exams/{id}/scores".
type Score struct {
	StudentID string   `json:"user_id"`
	Score     *float64 `json:"score"`
}

// examScoresEnvelope mirrors exam-service's real GET /exams/{id}/scores
// response shape — an object wrapping the score list (also carrying
// exam_type_id/mapping_rapor, unused here), not a bare array. GetScores
// previously decoded straight into []Score and silently failed every
// call (json: cannot unmarshal object into Go value of type []Score),
// meaning computed_score has never actually been populated by a live
// Generate Rapor run — only verified now via a real classroom/term.
type examScoresEnvelope struct {
	Scores []Score `json:"scores"`
}

// ListExams fetches GET /exams?classroom_id=&term_id=, all exams scheduled
// for that classroom in that term (across all subjects).
func (ec *ExamClient) ListExams(ctx context.Context, classroomID, termID string) ([]Exam, error) {
	return getAllPages[Exam](ctx, ec.c, "ListExams",
		fmt.Sprintf("/exams?classroom_id=%s&term_id=%s", classroomID, termID))
}

// ListExamTypes fetches GET /exam-types?academic_year_id=, used to resolve
// each Exam.ExamTypeID to its mapping_rapor bucket. exam-service requires
// academic_year_id (it does not resolve "current year" itself, per its own
// README).
func (ec *ExamClient) ListExamTypes(ctx context.Context, academicYearID string) ([]ExamType, error) {
	return getAllPages[ExamType](ctx, ec.c, "ListExamTypes",
		fmt.Sprintf("/exam-types?academic_year_id=%s", academicYearID))
}

// getAllPages walks every page of a paginated exam-service list instead of
// silently taking the first one.
//
// Both endpoints above run through common.Pagination, which defaults to
// per_page=20 and orders exams created_at DESC. Asking for neither page nor
// per_page therefore returned the 20 NEWEST exams and dropped the rest with
// no error and no warning -- so at SMK Global Persada Mandiri, where two
// classrooms hold 53 and 46 exams in one term, Generate Rapor could not see
// the STS sat the week before, only the newest batch. The school read that
// as "nilai ujian minggu lalu hilang". Same trap on exam-types: a school
// with more than 20 types would lose the mapping_rapor for the extras, and
// an exam whose type has no mapping is skipped outright.
//
// Stops on the first short page; the page cap is a guard against an
// upstream that never returns one, not an expected limit.
func getAllPages[T any](ctx context.Context, c *client, op, path string) ([]T, error) {
	const perPage = 200 // exam-service caps per_page at 200
	const maxPages = 50
	sep := "&"
	if !strings.Contains(path, "?") {
		sep = "?"
	}
	var all []T
	for page := 1; page <= maxPages; page++ {
		var batch []T
		p := fmt.Sprintf("%s%sper_page=%d&page=%d", path, sep, perPage, page)
		if err := c.getJSON(ctx, op, p, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			break
		}
	}
	return all, nil
}

// GetScores fetches GET /exams/{id}/scores, all students' scores for one
// exam.
func (ec *ExamClient) GetScores(ctx context.Context, examID string) ([]Score, error) {
	var env examScoresEnvelope
	path := fmt.Sprintf("/exams/%s/scores", examID)
	if err := ec.c.getJSON(ctx, "GetScores", path, &env); err != nil {
		return nil, err
	}
	return env.Scores, nil
}
