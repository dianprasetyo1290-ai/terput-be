// Package client is the thin internal HTTP client billing-service uses to
// call identity-service. Every call carries the shared X-Internal-Key and is
// bounded by a short timeout so a slow or down sibling degrades the caller to
// a safe "deny" instead of hanging.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotConfigured is returned when the identity base URL is empty.
var ErrNotConfigured = errors.New("identity service is not configured")

// HTTPError carries the downstream status code so callers can special-case a
// 404 (feature not deployed yet) without string matching.
type HTTPError struct {
	Status int
	Body   string
	URL    string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("downstream %s returned %d", e.URL, e.Status)
	}
	return fmt.Sprintf("downstream %s returned %d: %s", e.URL, e.Status, e.Body)
}

// Client is a small JSON-over-HTTP client bound to the identity base URL.
type Client struct {
	BaseURL     string
	InternalKey string
	HTTP        *http.Client
}

// New builds a client; baseURL may be empty, in which case every call fails
// with ErrNotConfigured (so the service degrades instead of panicking).
func New(baseURL, key string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		InternalKey: key,
		HTTP:        &http.Client{Timeout: 8 * time.Second},
	}
}

// Configured reports whether the client has a usable base URL.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" }

// Get performs a GET and decodes the response envelope's "data" into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	full := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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

// guardianAccess is the shape billing accepts from identity's
// GET /internal/guardians/{guardianUserId}/students/{studentUserId}/access.
// The boolean is tolerated under a few field names so a small contract
// difference between services does not silently open access.
type guardianAccess struct {
	Allowed   bool  `json:"allowed"`
	HasAccess *bool `json:"has_access"`
	Access    *bool `json:"access"`
}

func (g guardianAccess) granted() bool {
	if g.Allowed {
		return true
	}
	if g.HasAccess != nil && *g.HasAccess {
		return true
	}
	if g.Access != nil && *g.Access {
		return true
	}
	return false
}

// GuardianHasAccess asks identity whether guardianUserID is a verified
// guardian of studentUserID at schoolID. It fails closed: a missing
// configuration, a network error, or any non-2xx response returns false with
// a non-nil error, and callers must deny access in that case.
func (c *Client) GuardianHasAccess(ctx context.Context, guardianUserID, studentUserID, schoolID string) (bool, error) {
	if !c.Configured() {
		return false, ErrNotConfigured
	}
	if strings.TrimSpace(guardianUserID) == "" || strings.TrimSpace(studentUserID) == "" || strings.TrimSpace(schoolID) == "" {
		return false, errors.New("guardian, student and school ids are required")
	}
	path := "/internal/guardians/" + url.PathEscape(guardianUserID) +
		"/students/" + url.PathEscape(studentUserID) +
		"/access?school_id=" + url.QueryEscape(schoolID)
	var grant guardianAccess
	if err := c.Get(ctx, path, &grant); err != nil {
		return false, err
	}
	return grant.granted(), nil
}

// TeacherStudentIDs asks academic-service which students a teacher may see:
// every active student of a current-year classroom the teacher is wali kelas
// of or teaches a subject in. Like GuardianHasAccess it fails closed — any
// error means the caller must deny. c must be bound to the academic base URL.
// ponytail: reads one page of 200 classrooms per school; paginate if a school
// ever has more.
func (c *Client) TeacherStudentIDs(ctx context.Context, schoolID, teacherUserID string) (map[string]bool, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}
	if strings.TrimSpace(schoolID) == "" || strings.TrimSpace(teacherUserID) == "" {
		return nil, errors.New("school and teacher ids are required")
	}
	var classrooms []struct {
		ID                    string  `json:"id"`
		HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
	}
	if err := c.Get(ctx, "/classrooms?per_page=200&school_id="+url.QueryEscape(schoolID), &classrooms); err != nil {
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
		var students []struct {
			StudentUserID string `json:"student_user_id"`
		}
		if err := c.Get(ctx, "/classrooms/"+url.PathEscape(cl.ID)+"/students?school_id="+url.QueryEscape(schoolID), &students); err != nil {
			return nil, err
		}
		for _, s := range students {
			out[s.StudentUserID] = true
		}
	}
	return out, nil
}
