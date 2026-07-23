package upstream

import (
	"context"
	"fmt"
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
	var out []Exam
	path := fmt.Sprintf("/exams?classroom_id=%s&term_id=%s", classroomID, termID)
	if err := ec.c.getJSON(ctx, "ListExams", path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListExamTypes fetches GET /exam-types?academic_year_id=, used to resolve
// each Exam.ExamTypeID to its mapping_rapor bucket. exam-service requires
// academic_year_id (it does not resolve "current year" itself, per its own
// README).
func (ec *ExamClient) ListExamTypes(ctx context.Context, academicYearID string) ([]ExamType, error) {
	var out []ExamType
	path := fmt.Sprintf("/exam-types?academic_year_id=%s", academicYearID)
	if err := ec.c.getJSON(ctx, "ListExamTypes", path, &out); err != nil {
		return nil, err
	}
	return out, nil
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
