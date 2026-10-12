// Package client calls the other Sekoci services over X-Internal-Key:
// identity (feature switch, guardians, names), academic (classrooms),
// attendance (UKS sick absence), notification, core (approvals, audit,
// exceptions) and ops (inventory issue, T15). Copied pattern from
// services/hr/internal/client and services/billing/internal/client.
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
	"sync"
	"time"

	"skci/studentlife/internal/common"
)

// ErrNotConfigured is returned when the target base URL is empty.
var ErrNotConfigured = errors.New("downstream base URL is not configured")

// HTTPError is a non-2xx downstream response.
type HTTPError struct {
	Status int
	Body   string
	URL    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("downstream %s returned %d: %s", e.URL, e.Status, e.Body)
}

// Message is the downstream envelope's message (falls back to the raw body).
func (e *HTTPError) Message() string {
	var env struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &env) == nil && env.Message != "" {
		return env.Message
	}
	return e.Body
}

// StatusOf returns the downstream HTTP status of err, or 0.
func StatusOf(err error) int {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status
	}
	return 0
}

type URLs struct {
	Identity, Academic, Attendance, Notification, Core, Ops string
}

type Client struct {
	URLs        URLs
	InternalKey string
	HTTP        *http.Client

	featMu    sync.Mutex
	featCache map[string]cachedFeature
}

type cachedFeature struct {
	enabled   bool
	expiresAt time.Time
}

const featureTTL = 60 * time.Second

func New(u URLs, internalKey string) *Client {
	trim := func(s string) string { return strings.TrimRight(s, "/") }
	return &Client{
		URLs: URLs{Identity: trim(u.Identity), Academic: trim(u.Academic), Attendance: trim(u.Attendance),
			Notification: trim(u.Notification), Core: trim(u.Core), Ops: trim(u.Ops)},
		InternalKey: internalKey,
		HTTP:        &http.Client{Timeout: 8 * time.Second},
		featCache:   map[string]cachedFeature{},
	}
}

func (c *Client) do(ctx context.Context, method, base, path string, body any, idempotencyKey string, out any) error {
	if base == "" {
		return ErrNotConfigured
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	full := base + path
	req, err := http.NewRequestWithContext(ctx, method, full, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Internal-Key", c.InternalKey)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
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

// FeatureEnabled reports whether featureKey is on for the school, cached 60s
// per (school_id, feature_key). It fails closed: any lookup error is returned
// and callers treat it as "off".
func (c *Client) FeatureEnabled(ctx context.Context, schoolID, featureKey string) (bool, error) {
	cacheKey := schoolID + "|" + featureKey
	c.featMu.Lock()
	cached, ok := c.featCache[cacheKey]
	c.featMu.Unlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.enabled, nil
	}
	var statuses []struct {
		FeatureKey string `json:"feature_key"`
		Enabled    bool   `json:"enabled"`
	}
	if err := c.do(ctx, http.MethodGet, c.URLs.Identity, "/schools/"+url.PathEscape(schoolID)+"/features", nil, "", &statuses); err != nil {
		return false, err
	}
	enabled := false
	for _, s := range statuses {
		if s.FeatureKey == featureKey {
			enabled = s.Enabled
		}
	}
	c.featMu.Lock()
	c.featCache[cacheKey] = cachedFeature{enabled: enabled, expiresAt: time.Now().Add(featureTTL)}
	c.featMu.Unlock()
	return enabled, nil
}

// GuardianHasAccess asks identity whether guardianUserID is a verified
// guardian of studentUserID at schoolID. Fails closed (false + error).
func (c *Client) GuardianHasAccess(ctx context.Context, guardianUserID, studentUserID, schoolID string) (bool, error) {
	if strings.TrimSpace(guardianUserID) == "" || strings.TrimSpace(studentUserID) == "" || strings.TrimSpace(schoolID) == "" {
		return false, errors.New("guardian, student and school ids are required")
	}
	var grant struct {
		Allowed bool `json:"allowed"`
	}
	path := "/internal/guardians/" + url.PathEscape(guardianUserID) + "/students/" + url.PathEscape(studentUserID) +
		"/access?school_id=" + url.QueryEscape(schoolID)
	if err := c.do(ctx, http.MethodGet, c.URLs.Identity, path, nil, "", &grant); err != nil {
		return false, err
	}
	return grant.Allowed, nil
}

// Guardian is one active guardian link (identity, kontrak T9 §4.1).
type Guardian struct {
	GuardianUserID string `json:"guardian_user_id"`
	Relationship   string `json:"relationship"`
	IsPrimary      bool   `json:"is_primary"`
}

// StudentGuardians lists the student's active guardians.
func (c *Client) StudentGuardians(ctx context.Context, studentUserID, schoolID string) ([]Guardian, error) {
	var out []Guardian
	path := "/internal/students/" + url.PathEscape(studentUserID) + "/guardians?school_id=" + url.QueryEscape(schoolID)
	err := c.do(ctx, http.MethodGet, c.URLs.Identity, path, nil, "", &out)
	return out, err
}

// GuardianChildren lists the students actively linked to a guardian.
func (c *Client) GuardianChildren(ctx context.Context, guardianUserID, schoolID string) ([]UserRef, error) {
	var rows []struct {
		StudentUserID string `json:"student_user_id"`
		Name          string `json:"name"`
	}
	path := "/internal/guardians/" + url.PathEscape(guardianUserID) + "/children?school_id=" + url.QueryEscape(schoolID)
	if err := c.do(ctx, http.MethodGet, c.URLs.Identity, path, nil, "", &rows); err != nil {
		return nil, err
	}
	out := make([]UserRef, 0, len(rows))
	for _, r := range rows {
		out = append(out, UserRef{ID: r.StudentUserID, Name: r.Name})
	}
	return out, nil
}

// UserRef is a directory entry (id + display name).
type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UserNames resolves display names for ids (identity POST /users/search).
func (c *Client) UserNames(ctx context.Context, schoolID string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var users []UserRef
	path := "/users/search?per_page=200&school_id=" + url.QueryEscape(schoolID)
	if err := c.do(ctx, http.MethodPost, c.URLs.Identity, path, map[string]string{"ids": strings.Join(ids, ",")}, "", &users); err != nil {
		return out, err
	}
	for _, u := range users {
		out[u.ID] = u.Name
	}
	return out, nil
}

// SearchStudents finds siswa accounts in the school by name/email.
func (c *Client) SearchStudents(ctx context.Context, schoolID, q string) ([]UserRef, error) {
	var users []UserRef
	v := url.Values{"school_id": {schoolID}, "q": {q}, "per_page": {"20"}}
	err := c.do(ctx, http.MethodGet, c.URLs.Identity, "/students?"+v.Encode(), nil, "", &users)
	return users, err
}

// Classroom is the academic classroom subset studentlife reads.
type Classroom struct {
	ID                    string  `json:"id"`
	Name                  string  `json:"name"`
	GradeLevel            *string `json:"grade_level"`
	HomeroomTeacherUserID *string `json:"homeroom_teacher_user_id"`
}

// StudentClassroomID returns the student's active classroom ("" if none).
func (c *Client) StudentClassroomID(ctx context.Context, studentUserID, schoolID string) (string, error) {
	var res struct {
		ClassroomID string `json:"classroom_id"`
	}
	path := "/students/" + url.PathEscape(studentUserID) + "/classroom?school_id=" + url.QueryEscape(schoolID)
	err := c.do(ctx, http.MethodGet, c.URLs.Academic, path, nil, "", &res)
	return res.ClassroomID, err
}

// GetClassroom reads one classroom.
func (c *Client) GetClassroom(ctx context.Context, classroomID, schoolID string) (Classroom, error) {
	var cl Classroom
	path := "/classrooms/" + url.PathEscape(classroomID) + "?school_id=" + url.QueryEscape(schoolID)
	err := c.do(ctx, http.MethodGet, c.URLs.Academic, path, nil, "", &cl)
	return cl, err
}

// ListClassrooms lists the school's classrooms (current year, academic default).
func (c *Client) ListClassrooms(ctx context.Context, schoolID string) ([]Classroom, error) {
	var out []Classroom
	err := c.do(ctx, http.MethodGet, c.URLs.Academic, "/classrooms?per_page=500&school_id="+url.QueryEscape(schoolID), nil, "", &out)
	return out, err
}

// ClassroomStudentIDs lists the student ids enrolled in a classroom.
func (c *Client) ClassroomStudentIDs(ctx context.Context, classroomID, schoolID string) ([]string, error) {
	var rows []struct {
		StudentUserID string `json:"student_user_id"`
	}
	path := "/classrooms/" + url.PathEscape(classroomID) + "/students?school_id=" + url.QueryEscape(schoolID)
	if err := c.do(ctx, http.MethodGet, c.URLs.Academic, path, nil, "", &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.StudentUserID)
	}
	return out, nil
}

// HealthAbsence is attendance POST /internal/student-absences/health (T10 §4.2).
type HealthAbsence struct {
	SchoolID    string    `json:"school_id"`
	StudentID   string    `json:"student_id"`
	Date        string    `json:"date"`
	SickSince   time.Time `json:"sick_since"`
	SourceRef   string    `json:"source_ref"`
	Outcome     string    `json:"outcome"`
	ActorUserID string    `json:"actor_user_id"`
	Reason      string    `json:"reason"`
}

// HealthAbsenceResult is the attendance response.
type HealthAbsenceResult struct {
	Applied bool   `json:"applied"`
	Reason  string `json:"reason"`
}

// ApplyHealthAbsence marks the student sick for the day (idempotent by source_ref).
func (c *Client) ApplyHealthAbsence(ctx context.Context, in HealthAbsence) (HealthAbsenceResult, error) {
	var res HealthAbsenceResult
	err := c.do(ctx, http.MethodPost, c.URLs.Attendance, "/internal/student-absences/health", in, "studentlife:absence:"+in.SourceRef, &res)
	return res, err
}

// Notification is notification-service's POST /internal/notifications body.
type Notification struct {
	SchoolID string         `json:"school_id"`
	UserID   string         `json:"user_id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Data     map[string]any `json:"data,omitempty"`
}

// Notify sends one notification; idempotencyKey makes retries converge.
func (c *Client) Notify(ctx context.Context, n Notification, idempotencyKey string) error {
	return c.do(ctx, http.MethodPost, c.URLs.Notification, "/internal/notifications", n, idempotencyKey, nil)
}

// ApprovalRequest is core's POST /internal/approvals body (T1 §3).
type ApprovalRequest struct {
	SchoolID       string         `json:"school_id"`
	RequestType    string         `json:"request_type"`
	SourceService  string         `json:"source_service"`
	SourceID       string         `json:"source_id"`
	Title          string         `json:"title"`
	Summary        map[string]any `json:"summary"`
	MakerUserID    string         `json:"maker_user_id"`
	SubjectUserID  string         `json:"subject_user_id,omitempty"`
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// CreateApproval opens an approval in core and returns its id.
func (c *Client) CreateApproval(ctx context.Context, req ApprovalRequest) (string, error) {
	var res struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, c.URLs.Core, "/internal/approvals", req, req.IdempotencyKey, &res)
	return res.ID, err
}

// AuditEvent is core's POST /internal/audit body.
type AuditEvent struct {
	SchoolID      string          `json:"school_id"`
	Service       string          `json:"service"`
	Module        string          `json:"module"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	Action        string          `json:"action"`
	ActorUserID   *string         `json:"actor_user_id"`
	ActorRole     *string         `json:"actor_role"`
	Before        json.RawMessage `json:"before"`
	After         json.RawMessage `json:"after"`
	Reason        *string         `json:"reason"`
	CorrelationID *string         `json:"correlation_id"`
}

// PostAudit forwards one outbox row to core.
func (c *Client) PostAudit(ctx context.Context, ev AuditEvent) error {
	return c.do(ctx, http.MethodPost, c.URLs.Core, "/internal/audit", ev, "", nil)
}

// IntegrationException is core's POST /internal/exceptions body. The payload
// summary must never carry health data.
type IntegrationException struct {
	SchoolID       string         `json:"school_id"`
	SourceService  string         `json:"source_service"`
	Kind           string         `json:"kind"`
	Severity       string         `json:"severity"`
	CorrelationID  string         `json:"correlation_id"`
	ErrorMessage   string         `json:"error_message"`
	PayloadSummary map[string]any `json:"payload_summary"`
	MaxAttempts    int            `json:"max_attempts"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// ReportException records a dead-lettered integration in core.
func (c *Client) ReportException(ctx context.Context, ex IntegrationException) error {
	return c.do(ctx, http.MethodPost, c.URLs.Core, "/internal/exceptions", ex, ex.IdempotencyKey, nil)
}

// InventoryIssue is ops POST /internal/inventory/issues (kontrak untuk T15).
type InventoryIssue struct {
	SchoolID       string  `json:"school_id"`
	ItemID         string  `json:"item_id"`
	Qty            float64 `json:"qty"`
	RefType        string  `json:"ref_type"`
	RefID          string  `json:"ref_id"`
	IdempotencyKey string  `json:"idempotency_key"`
}

// IssueInventory posts a stock issue to ops. ErrNotConfigured when OPS_URL is empty.
func (c *Client) IssueInventory(ctx context.Context, in InventoryIssue) error {
	return c.do(ctx, http.MethodPost, c.URLs.Ops, "/internal/inventory/issues", in, in.IdempotencyKey, nil)
}

// RequireFeature is middleware rejecting requests (403) while featureKey is
// off for the caller's school; lookup failures fail closed (502).
func (c *Client) RequireFeature(featureKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			school := common.EffectiveSchoolID(r)
			if school == "" {
				common.WriteError(w, http.StatusBadRequest, "school_id wajib diisi")
				return
			}
			on, err := c.FeatureEnabled(r.Context(), school, featureKey)
			if err != nil {
				common.WriteError(w, http.StatusBadGateway, "Gagal memverifikasi modul sekolah")
				return
			}
			if !on {
				common.WriteError(w, http.StatusForbidden, "Modul belum diaktifkan untuk sekolah ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
