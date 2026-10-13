// Package client calls the other Sekoci services over X-Internal-Key:
// identity (feature switches, users by role), core (approvals, audit,
// exceptions), notification, and arbitrary callback URLs (T11 tickets).
// Copied from the hr/billing pattern, not imported.
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
)

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

type Client struct {
	IdentityURL string
	CoreURL     string
	NotifURL    string
	InternalKey string
	FeatureTTL  time.Duration
	HTTP        *http.Client

	featMu    sync.Mutex
	featCache map[string]cachedFeatures
}

type cachedFeatures struct {
	enabled   map[string]bool
	expiresAt time.Time
}

func New(identityURL, coreURL, notifURL, internalKey string, featureTTL time.Duration) *Client {
	return &Client{
		IdentityURL: strings.TrimRight(identityURL, "/"),
		CoreURL:     strings.TrimRight(coreURL, "/"),
		NotifURL:    strings.TrimRight(notifURL, "/"),
		InternalKey: internalKey,
		FeatureTTL:  featureTTL,
		HTTP:        &http.Client{Timeout: 8 * time.Second},
		featCache:   map[string]cachedFeatures{},
	}
}

func (c *Client) do(ctx context.Context, method, fullURL string, body any, idempotencyKey string, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
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
		return &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw)), URL: fullURL}
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

// FeatureEnabled reports whether the school's feature key is on, caching the
// school's whole feature list for FeatureTTL. It fails closed: a lookup error
// is returned and callers treat it as "off".
func (c *Client) FeatureEnabled(ctx context.Context, schoolID, key string) (bool, error) {
	c.featMu.Lock()
	cached, ok := c.featCache[schoolID]
	c.featMu.Unlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.enabled[key], nil
	}
	var statuses []struct {
		FeatureKey string `json:"feature_key"`
		Enabled    bool   `json:"enabled"`
	}
	if err := c.do(ctx, http.MethodGet, c.IdentityURL+"/schools/"+url.PathEscape(schoolID)+"/features", nil, "", &statuses); err != nil {
		return false, err
	}
	enabled := map[string]bool{}
	for _, s := range statuses {
		enabled[s.FeatureKey] = s.Enabled
	}
	c.featMu.Lock()
	c.featCache[schoolID] = cachedFeatures{enabled: enabled, expiresAt: time.Now().Add(c.FeatureTTL)}
	c.featMu.Unlock()
	return enabled[key], nil
}

// User is the minimal identity account projection ops needs.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UsersWithRole lists the accounts holding role in the school.
func (c *Client) UsersWithRole(ctx context.Context, schoolID, role string) ([]User, error) {
	var users []User
	q := url.Values{"role": {role}, "school_id": {schoolID}, "per_page": {"200"}}
	if err := c.do(ctx, http.MethodGet, c.IdentityURL+"/users?"+q.Encode(), nil, "", &users); err != nil {
		return nil, err
	}
	return users, nil
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
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id,omitempty"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// CreateApproval opens an approval in core and returns its id.
func (c *Client) CreateApproval(ctx context.Context, req ApprovalRequest) (string, error) {
	var res struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, c.CoreURL+"/internal/approvals", req, req.IdempotencyKey, &res)
	return res.ID, err
}

// CancelApproval withdraws a pending approval in core.
func (c *Client) CancelApproval(ctx context.Context, approvalID, actorUserID, reason string) error {
	return c.do(ctx, http.MethodPost, c.CoreURL+"/internal/approvals/"+url.PathEscape(approvalID)+"/cancel",
		map[string]string{"actor_user_id": actorUserID, "reason": reason}, "", nil)
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
	return c.do(ctx, http.MethodPost, c.CoreURL+"/internal/audit", ev, "", nil)
}

// Exception is core's POST /internal/exceptions body.
type Exception struct {
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

// PostException records a failed integration in core's exception queue.
func (c *Client) PostException(ctx context.Context, ex Exception) error {
	return c.do(ctx, http.MethodPost, c.CoreURL+"/internal/exceptions", ex, ex.IdempotencyKey, nil)
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
	return c.do(ctx, http.MethodPost, c.NotifURL+"/internal/notifications", n, idempotencyKey, nil)
}

// PostCallback POSTs body to a sibling-provided callback URL (T11 ticket
// conversion) with X-Internal-Key and the given Idempotency-Key.
func (c *Client) PostCallback(ctx context.Context, callbackURL string, body any, idempotencyKey string) error {
	return c.do(ctx, http.MethodPost, callbackURL, body, idempotencyKey, nil)
}

// GuardianLink is identity's GET /internal/students/{id}/guardians entry.
type GuardianLink struct {
	GuardianUserID string `json:"guardian_user_id"`
	Relationship   string `json:"relationship"`
	IsPrimary      bool   `json:"is_primary"`
}

func (c *Client) GuardiansForStudent(ctx context.Context, schoolID, studentUserID string) ([]GuardianLink, error) {
	var list []GuardianLink
	q := url.Values{"school_id": {schoolID}}
	err := c.do(ctx, http.MethodGet, c.IdentityURL+"/internal/students/"+url.PathEscape(studentUserID)+"/guardians?"+q.Encode(), nil, "", &list)
	return list, err
}

func (c *Client) CheckGuardianStudentAccess(ctx context.Context, guardianUserID, studentUserID string) (bool, error) {
	var res struct {
		HasAccess bool `json:"has_access"`
	}
	err := c.do(ctx, http.MethodGet, c.IdentityURL+"/internal/guardians/"+url.PathEscape(guardianUserID)+"/students/"+url.PathEscape(studentUserID)+"/access", nil, "", &res)
	if err != nil {
		return false, err
	}
	return res.HasAccess, nil
}

type EmployeeOption struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	FullName     string `json:"full_name"`
	PositionName string `json:"position_name"`
}

func (c *Client) Employees(ctx context.Context, hrURL, schoolID, query string) ([]EmployeeOption, error) {
	if hrURL == "" {
		return nil, nil
	}
	var list []EmployeeOption
	q := url.Values{"school_id": {schoolID}, "q": {query}, "status": {"active"}}
	err := c.do(ctx, http.MethodGet, hrURL+"/internal/employees?"+q.Encode(), nil, "", &list)
	return list, err
}

func (c *Client) ResolveStudentCard(ctx context.Context, billingURL, schoolID, code string) (string, error) {
	if billingURL == "" {
		return "", errors.New("billing URL not configured")
	}
	var res struct {
		StudentUserID string `json:"student_user_id"`
		Status        string `json:"status"` // 'active' | 'revoked'
	}
	body := map[string]string{"school_id": schoolID, "code": code}
	err := c.do(ctx, http.MethodPost, billingURL+"/internal/student-cards/resolve", body, "", &res)
	if err != nil {
		return "", err
	}
	if res.Status == "revoked" {
		return "", errors.New("Kartu tidak aktif")
	}
	return res.StudentUserID, nil
}
