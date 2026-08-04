package upstream

import (
	"context"
	"fmt"
	"time"
)

// AcademicClient talks to academic-service (port 8082), which owns
// classrooms, academic years/terms, subjects, enrollments, and the subject
// matrix. See README > Expected upstream contracts.
type AcademicClient struct{ c *client }

func NewAcademicClient(baseURL, internalKey string, timeout time.Duration) *AcademicClient {
	return &AcademicClient{c: newClient(baseURL, internalKey, "academic", timeout)}
}

// GradeLevel is *string, not *int: academic-service's Classroom.GradeLevel
// is a string column (grade labels like "7" are stored/returned as text,
// see services/academic/internal/model/model.go) -- decoding it into *int
// here silently failed every GetClassroom call with a JSON unmarshal error,
// which is the actual root cause of "Kelas" (and, transitively, "Wali
// Kelas") never auto-filling on the generated report card: GetClassroom
// returned an error, classroomName/homeroomTeacherName were never resolved,
// and the whole failure was swallowed into a generic warning entry instead
// of surfacing as an obvious type-mismatch bug.
type Classroom struct {
	ID                    string  `json:"id"`
	SchoolID              string  `json:"school_id"`
	AcademicYearID        string  `json:"academic_year_id"`
	Name                  string  `json:"name"`
	GradeLevel            *string `json:"grade_level"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
}

type Term struct {
	ID             string `json:"id"`
	SchoolID       string `json:"school_id"`
	AcademicYearID string `json:"academic_year_id"`
	Name           string `json:"name"` // e.g. "Semester Ganjil 2025/2026"
	Type           string `json:"type"` // "ganjil" | "genap"
}

// StudentEnrollment mirrors academic-service's real ClassEnrollment shape
// (confirmed against academic/internal/model: id, school_id,
// academic_year_id, classroom_id, student_user_id, status, enrolled_at) —
// notably it does NOT embed the student's name/NISN, so report-service
// must always resolve those from identity-service separately (see
// GenerateReportCards: identity failure just leaves the header name null,
// there is no roster-embedded fallback).
type StudentEnrollment struct {
	ID            string `json:"id"`
	StudentUserID string `json:"student_user_id"`
	Status        string `json:"status"`
}

// Subject mirrors one row of academic-service's GET /classrooms/{id}/subjects
// response, which is really a classroom_subjects join row, not a bare
// subjects row: its "id" is the classroom_subjects.id (the specific
// classroom+subject+teacher pairing), while "subject_id" is the real
// subjects.id. These were previously conflated (ID used everywhere a real
// subjects.id was needed), which silently broke exam-score matching
// (examsBySubject is keyed by exam-service's real subject_id) and left
// report_card_subjects.subject_id pointing at the wrong table.
type Subject struct {
	ID        string `json:"id"`         // classroom_subjects.id
	SubjectID string `json:"subject_id"` // subjects.id (real subject, matches exam-service's Exam.SubjectID)
	Name      string `json:"subject_name"`
	Code      string `json:"code"`
}

// GetClassroom fetches GET /classrooms/{id}.
func (ac *AcademicClient) GetClassroom(ctx context.Context, classroomID string) (*Classroom, error) {
	var out Classroom
	if err := ac.c.getJSON(ctx, "GetClassroom", fmt.Sprintf("/classrooms/%s", classroomID), &out); err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, nil
	}
	return &out, nil
}

// GetTerm fetches GET /terms/{id}.
func (ac *AcademicClient) GetTerm(ctx context.Context, termID string) (*Term, error) {
	var out Term
	if err := ac.c.getJSON(ctx, "GetTerm", fmt.Sprintf("/terms/%s", termID), &out); err != nil {
		return nil, err
	}
	if out.ID == "" {
		return nil, nil
	}
	return &out, nil
}

// ListClassroomStudents fetches GET /classrooms/{id}/students, the enrolled
// roster for that classroom. We pass ?term_id= defensively in case
// academic-service later scopes enrollments by term, but as of this
// writing academic-service's ListClassroomStudents ignores it (enrollment
// is classroom-scoped, not term-scoped, in its current schema).
// ActiveSubjectIDs mirrors exam-service's httpclient.ActiveSubjectIDs and
// attendance-service's client.activeSubjects (item 13, V4 — Agama
// religion-based filtering): the set of subject_ids a student is actually
// enrolled in per academic-service's Auto-Enroll Agama resolution. For
// Agama specifically this is only the student's own religion's variant,
// never the others -- report-service needs this so Generate Rapor and
// E-Raport don't create a row (and show "Belum ada data") for every Agama
// subject in the classroom, only the one the student actually takes.
// Returns nil (not an error) on lookup failure -- callers fail open rather
// than blocking report generation on a network blip.
func (ac *AcademicClient) ActiveSubjectIDs(ctx context.Context, studentUserID, schoolID, academicYearID string) map[string]bool {
	// academic-service returns full ClassroomSubject objects here, not a
	// bare list of subject_id strings -- decoding into []string always
	// failed silently (getJSON returning an error, this function falling
	// back to nil/"fail open"), so the religion filter never actually took
	// effect. Matches exam-service's httpclient.ActiveSubjectIDs, the
	// working reference implementation.
	var items []struct {
		SubjectID string `json:"subject_id"`
	}
	path := fmt.Sprintf("/students/%s/active-subjects?school_id=%s&academic_year_id=%s", studentUserID, schoolID, academicYearID)
	if err := ac.c.getJSON(ctx, "ActiveSubjectIDs", path, &items); err != nil {
		return nil
	}
	out := make(map[string]bool, len(items))
	for _, it := range items {
		out[it.SubjectID] = true
	}
	return out
}

func (ac *AcademicClient) ListClassroomStudents(ctx context.Context, classroomID, termID string) ([]StudentEnrollment, error) {
	var out []StudentEnrollment
	path := fmt.Sprintf("/classrooms/%s/students?term_id=%s", classroomID, termID)
	if err := ac.c.getJSON(ctx, "ListClassroomStudents", path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListClassroomSubjects fetches GET /classrooms/{id}/subjects?term_id=, the
// subject matrix assigned to that classroom for that term.
func (ac *AcademicClient) ListClassroomSubjects(ctx context.Context, classroomID, termID string) ([]Subject, error) {
	var out []Subject
	path := fmt.Sprintf("/classrooms/%s/subjects?term_id=%s", classroomID, termID)
	if err := ac.c.getJSON(ctx, "ListClassroomSubjects", path, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// TeachingAssignment mirrors academic-service's GET /teaching-assignments
// response — the classroom+subject pairs a guru is assigned to teach.
// Powers Penilaian Formatif's "pick your class/subject" picker.
type TeachingAssignment struct {
	ClassroomSubjectID string `json:"classroom_subject_id"`
	ClassroomID        string `json:"classroom_id"`
	ClassroomName      string `json:"classroom_name"`
	SubjectID          string `json:"subject_id"`
	SubjectName        string `json:"subject_name"`
}

func (ac *AcademicClient) ListTeachingAssignments(ctx context.Context, guruID string) ([]TeachingAssignment, error) {
	var out []TeachingAssignment
	path := fmt.Sprintf("/teaching-assignments?guru_id=%s", guruID)
	if err := ac.c.getJSON(ctx, "ListTeachingAssignments", path, &out); err != nil {
		return nil, err
	}
	return out, nil
}
