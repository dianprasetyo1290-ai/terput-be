package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// AcademicClient talks to academic-service via X-Internal-Key, used only by
// Import Sekoci's "existing school" reconciliation (find/create the
// classroom named on a Data Siswa row's "Kelas" cell, then enroll the
// student) — see handler/import.go reconcileClassrooms. Mirrors ExamClient
// in this same package.
type AcademicClient struct {
	BaseURL     string
	InternalKey string
	HTTPClient  *http.Client
}

func NewAcademicClient() *AcademicClient {
	base := os.Getenv("ACADEMIC_SERVICE_URL")
	if base == "" {
		base = "http://academic:8082"
	}
	key := os.Getenv("SEKOCI_INTERNAL_KEY")
	if key == "" {
		key = "change-me"
	}
	return &AcademicClient{
		BaseURL:     base,
		InternalKey: key,
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *AcademicClient) do(method, url string, body []byte, out interface{}) error {
	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d", method, url, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type Classroom struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	GradeLevel *string `json:"grade_level"`
}

type classroomsEnvelope struct {
	Data []Classroom `json:"data"`
}

type createClassroomReq struct {
	Name                  string  `json:"name"`
	GradeLevel            *string `json:"grade_level,omitempty"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id,omitempty"`
}

type classroomEnvelope struct {
	Data Classroom `json:"data"`
}

// FindOrCreateClassroom looks up a classroom by exact name (case-insensitive)
// within one school+academic year, creating it if missing. gradeLevel/
// homeroomTeacherUserID only apply on creation — an existing classroom's
// fields are left untouched (re-importing shouldn't silently overwrite a
// wali kelas an admin already changed by hand).
func (c *AcademicClient) FindOrCreateClassroom(schoolID, academicYearID, name string, gradeLevel, homeroomTeacherUserID *string) (string, error) {
	var list classroomsEnvelope
	listURL := fmt.Sprintf("%s/classrooms?school_id=%s&academic_year_id=%s&per_page=200", c.BaseURL, schoolID, academicYearID)
	if err := c.do(http.MethodGet, listURL, nil, &list); err != nil {
		return "", err
	}
	for _, cl := range list.Data {
		if normalizeName(cl.Name) == normalizeName(name) {
			return cl.ID, nil
		}
	}
	body, _ := json.Marshal(createClassroomReq{Name: name, GradeLevel: gradeLevel, HomeroomTeacherUserID: homeroomTeacherUserID})
	createURL := fmt.Sprintf("%s/classrooms?school_id=%s&academic_year_id=%s", c.BaseURL, schoolID, academicYearID)
	var created classroomEnvelope
	if err := c.do(http.MethodPost, createURL, body, &created); err != nil {
		return "", err
	}
	return created.Data.ID, nil
}

type addStudentReq struct {
	StudentUserID string `json:"student_user_id"`
}

// EnrollStudent adds studentUserID to classroomID (idempotent — see
// AddClassEnrollment's ON CONFLICT upsert in academic-service).
func (c *AcademicClient) EnrollStudent(schoolID, classroomID, studentUserID string) error {
	body, _ := json.Marshal(addStudentReq{StudentUserID: studentUserID})
	url := fmt.Sprintf("%s/classrooms/%s/students?school_id=%s", c.BaseURL, classroomID, schoolID)
	return c.do(http.MethodPost, url, body, nil)
}

// RemoveStudentEnrollments tells academic-service to drop every
// class_enrollments row for this student in this school (all
// classrooms/years) — called from DeleteUser when the deleted account holds
// a siswa role, so a deleted student stops inflating DashboardCountsFor's
// siswa count (see Report sekoci V3.docx). Deliberately fail-open like
// EnrollStudent's sibling calls: academic-service being briefly unreachable
// shouldn't block deleting the student's account, and failing here leaves
// the same (already-existing) drift rather than a new, worse one.
func (c *AcademicClient) RemoveStudentEnrollments(schoolID, studentUserID string) error {
	url := fmt.Sprintf("%s/students/%s/enrollments?school_id=%s", c.BaseURL, studentUserID, schoolID)
	return c.do(http.MethodDelete, url, nil, nil)
}

func normalizeName(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}
