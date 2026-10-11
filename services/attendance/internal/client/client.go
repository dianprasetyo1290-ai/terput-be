// Package client holds thin, best-effort HTTP clients for the sibling
// identity-service and academic-service. attendance-service never stores
// users/classrooms/schedules itself (see CONVENTIONS.md) — it only stores
// day-by-day attendance events against their UUIDs, and enriches responses
// with names/details by calling out over internal HTTP when needed.
//
// These calls are treated as best-effort enrichment, not a hard dependency:
// if a sibling service is down or the record isn't found, callers degrade
// gracefully (omit the name field) rather than failing the whole request.
// The one exception is schedule lookup during guru check-in, where a
// request-supplied fallback is accepted — see handler.CheckIn for the
// rationale (documented in README "Assumptions").
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Client struct {
	notificationURL string
	identityBaseURL string
	academicBaseURL string
	internalKey     string
	httpClient      *http.Client
}

func New() *Client {
	c := NewWithBaseURLs(
		getEnv("IDENTITY_SERVICE_URL", "http://localhost:8081"),
		getEnv("ACADEMIC_SERVICE_URL", "http://localhost:8082"),
		getEnv("SEKOCI_INTERNAL_KEY", "change-me"),
	)
	c.notificationURL = getEnv("NOTIFICATION_SERVICE_URL", "http://notification:8089")
	return c
}

// Notify sends an in-app notification through notification-service's
// POST /internal/notifications. idemKey makes retries converge on one row.
func (c *Client) Notify(ctx context.Context, schoolID, userID, typ, title, body, idemKey string) error {
	if c.notificationURL == "" {
		return errors.New("notification service not configured")
	}
	payload, _ := json.Marshal(map[string]string{"school_id": schoolID, "user_id": userID, "type": typ, "title": title, "body": body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.notificationURL+"/internal/notifications", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.internalKey)
	req.Header.Set("Idempotency-Key", idemKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notification-service returned %d", resp.StatusCode)
	}
	return nil
}

// NewWithBaseURLs builds a Client from explicit configuration instead of the
// environment. Used by tests, and by anything that wants to wire the URLs
// directly. An empty identityBaseURL is meaningful for guardian checks: see
// CheckGuardianAccess, which fails closed rather than allowing.
func NewWithBaseURLs(identityBaseURL, academicBaseURL, internalKey string) *Client {
	return &Client{
		identityBaseURL: identityBaseURL,
		academicBaseURL: academicBaseURL,
		internalKey:     internalKey,
		httpClient:      &http.Client{Timeout: 2 * time.Second},
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type envelope struct {
	Data json.RawMessage `json:"data"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

func (c *Client) get(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream %s returned %d", url, resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, out)
}

// UserSummary mirrors the minimal fields attendance-service needs from
// identity-service's user record.
type UserSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GetUser fetches a guru/siswa's display name from identity-service.
// Best-effort: returns an error the caller is expected to swallow/log.
// CountUsersByRole is a live count (never cached) for the admin dashboard's
// "Total Guru" stat card -- per_page=1 keeps the payload minimal, the real
// number comes from identity-service's pagination meta.total, which is
// always computed by a real COUNT(*) query, never a stale cache.
func (c *Client) CountUsersByRole(ctx context.Context, schoolID, role string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/users?role=%s&school_id=%s&per_page=1", c.identityBaseURL, role, schoolID), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return 0, err
	}
	return env.Meta.Total, nil
}

func (c *Client) GetUser(ctx context.Context, id string) (*UserSummary, error) {
	var u UserSummary
	if err := c.get(ctx, fmt.Sprintf("%s/users/%s", c.identityBaseURL, id), &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// ClassroomSummary mirrors the minimal fields attendance-service needs from
// academic-service's classroom record. HomeroomTeacherUserID is only used by
// requireOwnClassroomIfWaliKelas (handler.go) to verify a wali-kelas caller
// actually owns the classroom_id they're asking about — never trust the
// query param alone.
type ClassroomSummary struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	AcademicYearID        string  `json:"academic_year_id"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
}

// GetStudentClassroomID returns the student's active classroom in the
// school's current academic year ("" when not enrolled).
func (c *Client) GetStudentClassroomID(ctx context.Context, studentID, schoolID string) (string, error) {
	var out struct {
		ClassroomID string `json:"classroom_id"`
	}
	err := c.get(ctx, fmt.Sprintf("%s/students/%s/classroom?school_id=%s", c.academicBaseURL, url.PathEscape(studentID), url.QueryEscape(schoolID)), &out)
	return out.ClassroomID, err
}

func (c *Client) GetClassroom(ctx context.Context, id string) (*ClassroomSummary, error) {
	var cr ClassroomSummary
	if err := c.get(ctx, fmt.Sprintf("%s/classrooms/%s", c.academicBaseURL, id), &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

// RosterStudent is one entry in a classroom roster.
type RosterStudent struct {
	StudentID string `json:"student_id"`
	Name      string `json:"name"`
}

// GetRoster fetches the list of students enrolled in a classroom from
// academic-service, for the guru's "pilih semua" attendance screen.
// classEnrollment mirrors academic-service's actual ClassEnrollment response
// shape (student_user_id, no name — academic-service doesn't own student
// display names, identity-service does). Fixed 2026-07-18: the previous
// RosterStudent{student_id,name} shape here didn't match either upstream
// service's real JSON, so every field silently decoded to "" — found by the
// attendance-frontend agent (roster count correct, every row blank).
type classEnrollment struct {
	StudentUserID string `json:"student_user_id"`
}

func (c *Client) GetRoster(ctx context.Context, classroomID string) ([]RosterStudent, error) {
	var enrollments []classEnrollment
	if err := c.get(ctx, fmt.Sprintf("%s/classrooms/%s/students", c.academicBaseURL, classroomID), &enrollments); err != nil {
		return nil, err
	}
	roster := make([]RosterStudent, 0, len(enrollments))
	for _, e := range enrollments {
		name := ""
		if u, err := c.GetUser(ctx, e.StudentUserID); err == nil && u != nil {
			name = u.Name
		}
		roster = append(roster, RosterStudent{StudentID: e.StudentUserID, Name: name})
	}
	return roster, nil
}

// activeSubjects is best-effort, mirroring exam-service's
// httpclient.ActiveSubjectIDs (item 13, V4 — Agama religion-based
// filtering): academic-service's Auto-Enroll Agama has already resolved
// which single Agama variant a student belongs to, so this set naturally
// contains only that student's own religion subject, never the others. Any
// non-Agama subject is unaffected (every student's active set already
// includes every regular subject in their class). Returns nil (not an
// error) on lookup failure, treated as "couldn't determine" by the caller.
func (c *Client) activeSubjects(ctx context.Context, schoolID, studentUserID string) map[string]bool {
	// academic-service's /students/{id}/active-subjects returns full
	// ClassroomSubject objects, not a bare list of subject_id strings --
	// decoding into []string here always failed silently (get() returning
	// an error, this function falling back to nil/"fail open"), so the
	// religion filter never actually took effect. Matches exam-service's
	// httpclient.ActiveSubjectIDs, the working reference implementation.
	var items []struct {
		SubjectID string `json:"subject_id"`
	}
	url := fmt.Sprintf("%s/students/%s/active-subjects?school_id=%s", c.academicBaseURL, studentUserID, schoolID)
	if err := c.get(ctx, url, &items); err != nil {
		return nil
	}
	out := make(map[string]bool, len(items))
	for _, it := range items {
		out[it.SubjectID] = true
	}
	return out
}

// GetRosterForSubject is GetRoster narrowed to students actually active in
// subjectID — the real fix for the parallel-Agama-classes bug: guru Islam
// and guru Kristen teaching the same classroom/period must each only ever
// see/mark THEIR OWN religion's students, never the full classroom, or
// their check-in seeding and roster screens collide over the same
// (classroom_id, session_date) rows. schoolID is required for the
// active-subjects lookup; subjectID == "" (e.g. a schedule slot with no
// resolved subject) skips filtering entirely and returns the full roster.
func (c *Client) GetRosterForSubject(ctx context.Context, classroomID, subjectID, schoolID string) ([]RosterStudent, error) {
	roster, err := c.GetRoster(ctx, classroomID)
	if err != nil || subjectID == "" {
		return roster, err
	}
	out := make([]RosterStudent, 0, len(roster))
	for _, s := range roster {
		allowed := c.activeSubjects(ctx, schoolID, s.StudentID)
		if allowed == nil || allowed[subjectID] {
			// nil (lookup failed) fails open, same as exam-service's
			// convention — don't lock a whole roster out on a network blip.
			out = append(out, s)
		}
	}
	return out, nil
}

// ScheduleSummary mirrors the minimal fields attendance-service needs from
// academic-service's schedule record (a "jam pelajaran" slot).
type ScheduleSummary struct {
	ID          string `json:"id"`
	ClassroomID string `json:"classroom_id"`
	SubjectID   string `json:"subject_id"`
	GuruID      string `json:"teacher_id"`
	StartTime   string `json:"start_time"` // HH:MM:SS
	EndTime     string `json:"end_time"`
	TermID      string `json:"term_id"`
}

// GetSchedule fetches slot details (classroom, time range, assigned guru)
// from academic-service so class_sessions can be created without duplicating
// the schedule's own data entry.
type TermSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) GetTerm(ctx context.Context, id string) (*TermSummary, error) {
	var t TermSummary
	if err := c.get(ctx, fmt.Sprintf("%s/terms/%s", c.academicBaseURL, id), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// GetSchedule needs schoolID: academic-service's internal-consumable routes
// (X-Internal-Key, no JWT claims) have no other way to know which school's
// row to scope to -- an internal caller that omits it gets schoolID="",
// which matches nothing (see scopeSchoolID's own doc comment).
func (c *Client) GetSchedule(ctx context.Context, id, schoolID string) (*ScheduleSummary, error) {
	var s ScheduleSummary
	if err := c.get(ctx, fmt.Sprintf("%s/schedules/%s?school_id=%s", c.academicBaseURL, id, schoolID), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

type SubjectSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) GetSubject(ctx context.Context, id string) (*SubjectSummary, error) {
	var s SubjectSummary
	if err := c.get(ctx, fmt.Sprintf("%s/subjects/%s", c.academicBaseURL, id), &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ActiveScheduleNow is one schedule slot (across any classroom/guru in the
// school) whose day-of-week/time window covers this exact moment, per
// academic-service's GET /schedules/active-now — the real source of truth
// for "who's scheduled to teach right now", independent of whether that
// guru has actually checked in yet (attendance's own class_sessions only
// gets a row AFTER check-in, which is why the "Guru Sedang Mengajar" widget
// used to show nothing until someone checked in).
type ActiveScheduleNow struct {
	ID                 string  `json:"id"` // academic-service's schedules.id -- the real FK attendance's class_sessions.schedule_id points at
	ClassroomSubjectID string  `json:"classroom_subject_id"`
	ClassroomID        string  `json:"classroom_id"`
	ClassroomName      *string `json:"classroom_name"`
	SubjectID          *string `json:"subject_id"`
	SubjectName        *string `json:"subject_name"`
	TeacherUserID      *string `json:"teacher_user_id"`
	StartTime          *string `json:"start_time"`
	EndTime            *string `json:"end_time"`
}

func (c *Client) ListActiveSchedulesNow(ctx context.Context, schoolID string) ([]ActiveScheduleNow, error) {
	var out []ActiveScheduleNow
	url := fmt.Sprintf("%s/schedules/active-now?school_id=%s", c.academicBaseURL, schoolID)
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- guardian access (portal orang tua) --------------------------------

var (
	// ErrGuardianNoAccess means identity-service positively answered that the
	// guardian is NOT linked to the student (HTTP 404). Handlers map it to 403.
	ErrGuardianNoAccess = errors.New("guardian is not linked to this student")

	// ErrGuardianCheckUnavailable means the check could not even be attempted
	// because the identity client is not configured. Callers MUST treat this
	// as a denial (fail closed), never as "allowed" -- a missing
	// IDENTITY_SERVICE_URL must not silently open every child's presensi.
	ErrGuardianCheckUnavailable = errors.New("guardian access check is not configured")
)

// CheckGuardianAccess asks identity-service whether guardianUserID is an
// active guardian of studentUserID at schoolID -- the same guardian_links
// relation the portal's "anak saya" list is built from. 200 means linked
// (nil), 404 means not linked (ErrGuardianNoAccess). Any other status, a
// transport error, or an unconfigured client is returned as an error that
// every caller must treat as "deny".
func (c *Client) CheckGuardianAccess(ctx context.Context, guardianUserID, studentUserID, schoolID string) error {
	if c.identityBaseURL == "" {
		return ErrGuardianCheckUnavailable
	}
	endpoint := fmt.Sprintf("%s/internal/guardians/%s/students/%s/access?school_id=%s",
		c.identityBaseURL, url.PathEscape(guardianUserID), url.PathEscape(studentUserID), url.QueryEscape(schoolID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.internalKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrGuardianNoAccess
	default:
		return fmt.Errorf("guardian access check: identity-service returned %d", resp.StatusCode)
	}
}
