package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrNotConfigured is returned when a downstream base URL is empty.
var ErrNotConfigured = errors.New("downstream service is not configured")

// HTTPError carries the downstream status code.
type HTTPError struct {
	Status int
	Body   string
	URL    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("downstream %s returned %d: %s", e.URL, e.Status, e.Body)
}

// IsNotFound reports whether err is a downstream 404.
func IsNotFound(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == http.StatusNotFound
}

// Get performs a GET with X-Internal-Key and decodes the envelope's data.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, "", nil, out)
}

// PostJSON performs a POST with X-Internal-Key (and Idempotency-Key).
func (c *Client) PostJSON(ctx context.Context, path, idempotencyKey string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, path, idempotencyKey, bytes.NewReader(raw), out)
}

func (c *Client) do(ctx context.Context, method, path, idempotencyKey string, body io.Reader, out any) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	full := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, full, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.InternalKey != "" {
		req.Header.Set("X-Internal-Key", c.InternalKey)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw)), URL: full}
	}
	if out == nil {
		return nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode downstream envelope: %w", err)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// ---- identity -------------------------------------------------------------

// UserRole is one identity user_school_roles row.
type UserRole struct {
	SchoolID string `json:"school_id"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

// UserWithRoles is identity GET /users/{id}.
type UserWithRoles struct {
	UserProfile
	Roles []UserRole `json:"roles"`
}

// UserRoles returns the active roles a user holds in schoolID.
func (c *Client) UserRoles(ctx context.Context, schoolID, userID string) ([]string, error) {
	var u UserWithRoles
	if err := c.Get(ctx, "/users/"+url.PathEscape(userID), &u); err != nil {
		return nil, err
	}
	out := []string{}
	for _, r := range u.Roles {
		if r.SchoolID == schoolID && (r.Status == "" || r.Status == "active") {
			out = append(out, r.Role)
		}
	}
	return out, nil
}

// UsersByRole lists users with a role in a school (identity GET /users).
func (c *Client) UsersByRole(ctx context.Context, schoolID, role string) ([]UserProfile, error) {
	var out []UserProfile
	err := c.Get(ctx, "/users?per_page=200&role="+url.QueryEscape(role)+"&school_id="+url.QueryEscape(schoolID), &out)
	return out, err
}

// GuardianChildren returns the student user ids actively linked to a
// guardian (identity GET /internal/guardians/{id}/children).
func (c *Client) GuardianChildren(ctx context.Context, schoolID, guardianUserID string) ([]string, error) {
	var rows []struct {
		StudentUserID string `json:"student_user_id"`
	}
	path := "/internal/guardians/" + url.PathEscape(guardianUserID) + "/children?school_id=" + url.QueryEscape(schoolID)
	if err := c.Get(ctx, path, &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.StudentUserID)
	}
	return out, nil
}

// GuardianHasAccess reports whether guardianUserID is an active guardian of
// studentUserID. identity answers 404 for "no link" → (false, nil); any other
// failure is an error and callers must deny (fail closed).
func (c *Client) GuardianHasAccess(ctx context.Context, schoolID, guardianUserID, studentUserID string) (bool, error) {
	var grant struct {
		Allowed bool `json:"allowed"`
	}
	path := "/internal/guardians/" + url.PathEscape(guardianUserID) + "/students/" + url.PathEscape(studentUserID) +
		"/access?school_id=" + url.QueryEscape(schoolID)
	if err := c.Get(ctx, path, &grant); err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return grant.Allowed, nil
}

// StudentGuardian is one row of the T9 contract
// GET /internal/students/{studentUserId}/guardians?school_id= (identity).
type StudentGuardian struct {
	GuardianUserID string `json:"guardian_user_id"`
	Relationship   string `json:"relationship"`
	IsPrimary      bool   `json:"is_primary"`
}

// StudentGuardians reads a student's active guardians (owned by T9).
func (c *Client) StudentGuardians(ctx context.Context, schoolID, studentUserID string) ([]StudentGuardian, error) {
	var out []StudentGuardian
	path := "/internal/students/" + url.PathEscape(studentUserID) + "/guardians?school_id=" + url.QueryEscape(schoolID)
	err := c.Get(ctx, path, &out)
	return out, err
}

// ---- academic (client bound to the academic base URL) ----------------------

// Classroom is the subset of academic GET /classrooms the resolver needs.
type Classroom struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	GradeLevel            string  `json:"grade_level"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
}

// Classrooms lists a school's current classrooms.
func (c *Client) Classrooms(ctx context.Context, schoolID string) ([]Classroom, error) {
	var out []Classroom
	err := c.Get(ctx, "/classrooms?per_page=500&school_id="+url.QueryEscape(schoolID), &out)
	return out, err
}

// ClassroomStudents lists the student user ids of one classroom.
func (c *Client) ClassroomStudents(ctx context.Context, schoolID, classroomID string) ([]string, error) {
	var rows []struct {
		StudentUserID string `json:"student_user_id"`
	}
	if err := c.Get(ctx, "/classrooms/"+url.PathEscape(classroomID)+"/students?school_id="+url.QueryEscape(schoolID), &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.StudentUserID)
	}
	return out, nil
}

// TeacherStudentIDs returns every student of a classroom the teacher is wali
// kelas of or teaches in (pattern copied from billing). Fails closed.
func (c *Client) TeacherStudentIDs(ctx context.Context, schoolID, teacherUserID string) (map[string]bool, error) {
	if strings.TrimSpace(schoolID) == "" || strings.TrimSpace(teacherUserID) == "" {
		return nil, errors.New("school and teacher ids are required")
	}
	classrooms, err := c.Classrooms(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	var assignments []struct {
		ClassroomID string `json:"classroom_id"`
	}
	if err := c.Get(ctx, "/teaching-assignments?guru_id="+url.QueryEscape(teacherUserID), &assignments); err != nil {
		return nil, err
	}
	teaches := map[string]bool{}
	for _, a := range assignments {
		teaches[a.ClassroomID] = true
	}
	out := map[string]bool{}
	for _, cl := range classrooms {
		wali := cl.HomeroomTeacherUserID != nil && *cl.HomeroomTeacherUserID == teacherUserID
		if !wali && !teaches[cl.ID] {
			continue
		}
		students, err := c.ClassroomStudents(ctx, schoolID, cl.ID)
		if err != nil {
			return nil, err
		}
		for _, s := range students {
			out[s] = true
		}
	}
	return out, nil
}

// EnrollmentsByStatus reads academic GET /internal/enrollments (T13) and
// returns the distinct student user ids with that enrollment status.
func (c *Client) EnrollmentsByStatus(ctx context.Context, schoolID, status string) ([]string, error) {
	var rows []struct {
		StudentUserID string `json:"student_user_id"`
	}
	if err := c.Get(ctx, "/internal/enrollments?school_id="+url.QueryEscape(schoolID)+"&status="+url.QueryEscape(status), &rows); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if !seen[r.StudentUserID] {
			seen[r.StudentUserID] = true
			out = append(out, r.StudentUserID)
		}
	}
	return out, nil
}

// ---- admission -------------------------------------------------------------

// SegmentGuardian is one row of admission GET /internal/marketing/segment-guardians.
type SegmentGuardian struct {
	GuardianID     string `json:"guardian_id"`
	IdentityUserID string `json:"identity_user_id"`
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
}

// SegmentGuardians reads PPDB applicant guardians for a period and statuses.
func (c *Client) SegmentGuardians(ctx context.Context, schoolID, periodID string, statuses []string) ([]SegmentGuardian, error) {
	q := url.Values{}
	q.Set("school_id", schoolID)
	if periodID != "" {
		q.Set("period_id", periodID)
	}
	if len(statuses) > 0 {
		q.Set("statuses", strings.Join(statuses, ","))
	}
	var out []SegmentGuardian
	err := c.Get(ctx, "/internal/marketing/segment-guardians?"+q.Encode(), &out)
	return out, err
}

// ---- core ------------------------------------------------------------------

// AuditEvent is the body of core POST /internal/audit (T1 §3).
type AuditEvent struct {
	SchoolID    string `json:"school_id"`
	Service     string `json:"service"`
	Module      string `json:"module"`
	EntityType  string `json:"entity_type"`
	EntityID    string `json:"entity_id"`
	Action      string `json:"action"`
	ActorUserID string `json:"actor_user_id,omitempty"`
	ActorRole   string `json:"actor_role,omitempty"`
	Before      any    `json:"before,omitempty"`
	After       any    `json:"after,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// Audit records one event in core's central audit log (module communication).
func (c *Client) Audit(ctx context.Context, e AuditEvent) error {
	e.Service = "notification"
	if e.Module == "" {
		e.Module = "communication"
	}
	return c.PostJSON(ctx, "/internal/audit", "", e, nil)
}

// StudentTeacher is one teacher of a student's current classroom.
type StudentTeacher struct {
	UserID string `json:"user_id"`
	Label  string `json:"label"` // "Wali kelas" or the subject name
}

// StudentTeachers lists the wali kelas and subject teachers of a student's
// current classroom (academic).
func (c *Client) StudentTeachers(ctx context.Context, schoolID, studentUserID string) ([]StudentTeacher, error) {
	var sc struct {
		ClassroomID string `json:"classroom_id"`
	}
	if err := c.Get(ctx, "/students/"+url.PathEscape(studentUserID)+"/classroom?school_id="+url.QueryEscape(schoolID), &sc); err != nil {
		return nil, err
	}
	if sc.ClassroomID == "" {
		return []StudentTeacher{}, nil
	}
	var cl Classroom
	if err := c.Get(ctx, "/classrooms/"+url.PathEscape(sc.ClassroomID)+"?school_id="+url.QueryEscape(schoolID), &cl); err != nil {
		return nil, err
	}
	var subjects []struct {
		TeacherUserID *string `json:"teacher_user_id"`
		SubjectName   *string `json:"subject_name"`
		IsActive      bool    `json:"is_active"`
	}
	if err := c.Get(ctx, "/classrooms/"+url.PathEscape(sc.ClassroomID)+"/subjects?school_id="+url.QueryEscape(schoolID), &subjects); err != nil {
		return nil, err
	}
	labels := map[string][]string{}
	var order []string
	add := func(id, label string) {
		if _, ok := labels[id]; !ok {
			order = append(order, id)
		}
		labels[id] = append(labels[id], label)
	}
	if cl.HomeroomTeacherUserID != nil && *cl.HomeroomTeacherUserID != "" {
		add(*cl.HomeroomTeacherUserID, "Wali kelas")
	}
	for _, s := range subjects {
		if s.TeacherUserID == nil || *s.TeacherUserID == "" || !s.IsActive {
			continue
		}
		name := "Guru mata pelajaran"
		if s.SubjectName != nil {
			name = *s.SubjectName
		}
		add(*s.TeacherUserID, name)
	}
	out := make([]StudentTeacher, 0, len(order))
	for _, id := range order {
		out = append(out, StudentTeacher{UserID: id, Label: strings.Join(labels[id], ", ")})
	}
	return out, nil
}

// Relations answers the BR-COM-02 relation questions from identity and
// academic. Every method fails closed: callers deny on error.
type Relations struct {
	Identity *Client
	Academic *Client
}

// UserRoles implements handler.Relations.
func (r Relations) UserRoles(ctx context.Context, schoolID, userID string) ([]string, error) {
	return r.Identity.UserRoles(ctx, schoolID, userID)
}

// UserName returns a user's display name ("" when unknown).
func (r Relations) UserName(ctx context.Context, userID string) string {
	u, err := r.Identity.GetUser(ctx, userID)
	if err != nil || u == nil {
		return ""
	}
	return u.Name
}

// TeacherStudentIDs implements handler.Relations.
func (r Relations) TeacherStudentIDs(ctx context.Context, schoolID, teacherID string) (map[string]bool, error) {
	return r.Academic.TeacherStudentIDs(ctx, schoolID, teacherID)
}

// GuardianHasAccess implements handler.Relations.
func (r Relations) GuardianHasAccess(ctx context.Context, schoolID, guardianID, studentID string) (bool, error) {
	return r.Identity.GuardianHasAccess(ctx, schoolID, guardianID, studentID)
}

// GuardianChildren implements handler.Relations.
func (r Relations) GuardianChildren(ctx context.Context, schoolID, guardianID string) ([]string, error) {
	return r.Identity.GuardianChildren(ctx, schoolID, guardianID)
}

// StudentGuardians implements handler.Relations.
func (r Relations) StudentGuardians(ctx context.Context, schoolID, studentID string) ([]StudentGuardian, error) {
	return r.Identity.StudentGuardians(ctx, schoolID, studentID)
}

// StudentTeachers implements handler.Relations.
func (r Relations) StudentTeachers(ctx context.Context, schoolID, studentID string) ([]StudentTeacher, error) {
	return r.Academic.StudentTeachers(ctx, schoolID, studentID)
}

// UsersByRole implements handler.Relations.
func (r Relations) UsersByRole(ctx context.Context, schoolID, role string) ([]UserProfile, error) {
	return r.Identity.UsersByRole(ctx, schoolID, role)
}
