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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type Client struct {
	identityBaseURL string
	academicBaseURL string
	internalKey     string
	httpClient      *http.Client
}

func New() *Client {
	return &Client{
		identityBaseURL: getEnv("IDENTITY_SERVICE_URL", "http://localhost:8081"),
		academicBaseURL: getEnv("ACADEMIC_SERVICE_URL", "http://localhost:8082"),
		internalKey:     getEnv("SEKOCI_INTERNAL_KEY", "change-me"),
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
func (c *Client) GetUser(ctx context.Context, id string) (*UserSummary, error) {
	var u UserSummary
	if err := c.get(ctx, fmt.Sprintf("%s/users/%s", c.identityBaseURL, id), &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// ClassroomSummary mirrors the minimal fields attendance-service needs from
// academic-service's classroom record.
type ClassroomSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
func (c *Client) GetSchedule(ctx context.Context, id string) (*ScheduleSummary, error) {
	var s ScheduleSummary
	if err := c.get(ctx, fmt.Sprintf("%s/schedules/%s", c.academicBaseURL, id), &s); err != nil {
		return nil, err
	}
	return &s, nil
}
