// Package httpclient makes best-effort internal HTTP calls to sibling
// services (identity, academic). Every call here is defensive: on any error
// (service down, route not yet implemented by the sibling agent building it
// in parallel, timeout) callers get a zero-value fallback rather than a hard
// failure, since exam-service's own state machine never depends on these
// calls succeeding. See README Assumptions.
package httpclient

import (
	"encoding/json"
	"net/http"
	"time"
)

type Client struct {
	IdentityURL string
	AcademicURL string
	InternalKey string
	http        *http.Client
}

func New(identityURL, academicURL, internalKey string) *Client {
	return &Client{
		IdentityURL: identityURL,
		AcademicURL: academicURL,
		InternalKey: internalKey,
		http:        &http.Client{Timeout: 2 * time.Second},
	}
}

type envelope struct {
	Data json.RawMessage `json:"data"`
}

type userDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UserName does a best-effort lookup of a student/guru's display name from
// identity-service. Returns "" (never an error) if unreachable.
func (c *Client) UserName(userID string) string {
	req, err := http.NewRequest(http.MethodGet, c.IdentityURL+"/users/"+userID, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return ""
	}
	var u userDTO
	if err := json.Unmarshal(env.Data, &u); err != nil {
		return ""
	}
	return u.Name
}

// TeachingAssignment is one (classroom, subject) a guru teaches — mirrors
// academic-service's TeachingAssignment (services/academic/internal/store/
// classroom_subject.go). Used by Buat Ujian Baru's Mata Pelajaran picker
// (item 3) so a guru only ever sees their own subject+class combos.
type TeachingAssignment struct {
	ClassroomSubjectID string `json:"classroom_subject_id"`
	ClassroomID        string `json:"classroom_id"`
	ClassroomName      string `json:"classroom_name"`
	SubjectID          string `json:"subject_id"`
	SubjectName        string `json:"subject_name"`
}

// TeachingAssignments does a best-effort lookup of every classroom+subject a
// guru teaches. Returns nil (never an error) if academic-service is
// unreachable — callers fall back to an empty picker rather than a hard
// failure, per this package's doc comment.
func (c *Client) TeachingAssignments(guruID string) []TeachingAssignment {
	req, err := http.NewRequest(http.MethodGet, c.AcademicURL+"/teaching-assignments?guru_id="+guruID, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil
	}
	var out []TeachingAssignment
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return nil
	}
	return out
}

// activeSubjectDTO mirrors the fields we need from academic-service's
// ClassroomSubject rows (services/academic/internal/model/model.go) — only
// SubjectID matters here, the rest is ignored.
type activeSubjectDTO struct {
	SubjectID string `json:"subject_id"`
}

// ActiveSubjectIDs (item 13, V4 — Agama religion-based question filtering):
// best-effort lookup of every subject a student is currently allowed to see
// in their classroom, via academic-service's GET /students/{id}/active-
// subjects. For most subjects this is just "every subject in the student's
// class". For Agama, academic-service's Auto-Enroll Agama has already
// resolved which single variant (Agama Islam / Kristen / ...) this specific
// student is enrolled in — so the returned set naturally contains ONLY that
// student's own religion's Agama subject, never the others. Callers (exam-
// service's MyExams/EnterExam) just need to check "is this exam's
// subject_id in this set" — no religion string-matching needed here at all.
//
// Returns nil (never an error) if academic-service is unreachable or the
// student has no resolved enrollment. Per this package's fail-open
// convention (exam-service's own state machine never depends on these
// calls succeeding), callers treat nil as "couldn't determine — don't
// filter" rather than blocking every student's access on a network blip.
// ponytail: fail-open on lookup failure trades a rare wrong-religion leak
// for not locking every student out of Agama exams when academic-service
// hiccups — flip to fail-closed if that tradeoff turns out to matter more.
func (c *Client) ActiveSubjectIDs(schoolID, studentUserID, academicYearID string) map[string]bool {
	url := c.AcademicURL + "/students/" + studentUserID + "/active-subjects?school_id=" + schoolID
	if academicYearID != "" {
		url += "&academic_year_id=" + academicYearID
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil
	}
	var items []activeSubjectDTO
	if err := json.Unmarshal(env.Data, &items); err != nil {
		return nil
	}
	out := make(map[string]bool, len(items))
	for _, it := range items {
		out[it.SubjectID] = true
	}
	return out
}

// StudentClassroomID does a best-effort lookup of the classroom a student is
// actively enrolled in. schoolID is required so academic-service can resolve
// the school's CURRENT academic year when academicYearID is "" — a student
// can hold an active enrollment row in more than one academic year (e.g. a
// next-year row seeded ahead of promotion), so without a school to resolve
// "current" against, academic-service cannot safely guess which enrollment
// is the live one. Returns "" (never an error) if academic-service is
// unreachable or the student has no active enrollment — see siswa dashboard
// (item 20) and Monitor (item 10), both of which treat "" as "nothing to
// show" rather than a hard failure.
func (c *Client) StudentClassroomID(userID, schoolID, academicYearID string) string {
	url := c.AcademicURL + "/students/" + userID + "/classroom?school_id=" + schoolID
	if academicYearID != "" {
		url += "&academic_year_id=" + academicYearID
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return ""
	}
	var out struct {
		ClassroomID string `json:"classroom_id"`
	}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return ""
	}
	return out.ClassroomID
}

// ClassroomStudentDTO is one enrolled student row from academic-service's
// GET /classrooms/{id}/students (ClassEnrollment) — used by Monitor (item
// 10) to show the full class roster, not just students who already have an
// exam_attempts row.
type ClassroomStudentDTO struct {
	StudentUserID string `json:"student_user_id"`
}

// ClassroomStudents does a best-effort lookup of every student actively
// enrolled in a classroom. Returns nil (never an error) if unreachable.
func (c *Client) ClassroomStudents(classroomID string) []ClassroomStudentDTO {
	req, err := http.NewRequest(http.MethodGet, c.AcademicURL+"/classrooms/"+classroomID+"/students", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil
	}
	var out []ClassroomStudentDTO
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return nil
	}
	return out
}
