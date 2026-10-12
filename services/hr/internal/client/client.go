// Package client calls the other Sekoci services over X-Internal-Key:
// identity (feature switch, accounts), core (approvals, audit) and
// notification.
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
	// CallbackURL is where core POSTs approval decisions:
	// {HR_PUBLIC_INTERNAL_URL}/internal/approval-callbacks (T2 §approval).
	CallbackURL string
	HTTP        *http.Client

	featMu    sync.Mutex
	featCache map[string]cachedFeature
}

type cachedFeature struct {
	enabled   bool
	expiresAt time.Time
}

const featureTTL = 60 * time.Second

func New(identityURL, coreURL, notifURL, internalKey string) *Client {
	return &Client{
		IdentityURL: strings.TrimRight(identityURL, "/"),
		CoreURL:     strings.TrimRight(coreURL, "/"),
		NotifURL:    strings.TrimRight(notifURL, "/"),
		InternalKey: internalKey,
		HTTP:        &http.Client{Timeout: 8 * time.Second},
		featCache:   map[string]cachedFeature{},
	}
}

// NewClient is an alias for New.
func NewClient(identityURL, coreURL, notifURL, internalKey string) *Client {
	return New(identityURL, coreURL, notifURL, internalKey)
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

// IsHrisEnabled reports whether the school's 'hris' feature is on, cached
// 60s in memory. It fails closed: any lookup error is returned and callers
// treat it as "off".
func (c *Client) IsHrisEnabled(ctx context.Context, schoolID string) (bool, error) {
	c.featMu.Lock()
	cached, ok := c.featCache[schoolID]
	c.featMu.Unlock()
	if ok && time.Now().Before(cached.expiresAt) {
		return cached.enabled, nil
	}
	var statuses []struct {
		FeatureKey string `json:"feature_key"`
		Enabled    bool   `json:"enabled"`
	}
	if err := c.do(ctx, http.MethodGet, c.IdentityURL+"/schools/"+url.PathEscape(schoolID)+"/features", nil, "", &statuses); err != nil {
		return false, err
	}
	enabled := false
	for _, s := range statuses {
		if s.FeatureKey == "hris" {
			enabled = s.Enabled
		}
	}
	c.featMu.Lock()
	c.featCache[schoolID] = cachedFeature{enabled: enabled, expiresAt: time.Now().Add(featureTTL)}
	c.featMu.Unlock()
	return enabled, nil
}

// CreateOrLinkUser returns the identity account for an employee email.
func (c *Client) CreateOrLinkUser(ctx context.Context, schoolID, email, fullName, role string) (string, error) {
	var res struct {
		UserID string `json:"user_id"`
	}
	err := c.do(ctx, http.MethodPost, c.IdentityURL+"/internal/employees/create-or-link",
		map[string]string{"school_id": schoolID, "email": email, "full_name": fullName, "role": role}, "", &res)
	return res.UserID, err
}

// DeactivateUser blocks the account's login from effectiveAt on.
func (c *Client) DeactivateUser(ctx context.Context, userID, schoolID string, effectiveAt time.Time) error {
	return c.do(ctx, http.MethodPost, c.IdentityURL+"/internal/users/"+url.PathEscape(userID)+"/deactivate",
		map[string]any{"school_id": schoolID, "effective_at": effectiveAt}, "", nil)
}

// UserIDsWithRole lists the account ids holding role in the school (used to
// address notifications to HR).
func (c *Client) UserIDsWithRole(ctx context.Context, schoolID, role string) ([]string, error) {
	var users []struct {
		ID string `json:"id"`
	}
	q := url.Values{"role": {role}, "school_id": {schoolID}, "per_page": {"100"}}
	if err := c.do(ctx, http.MethodGet, c.IdentityURL+"/users?"+q.Encode(), nil, "", &users); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	return ids, nil
}

// ApprovalRequest is core's POST /internal/approvals body (T1 §3).
type ApprovalRequest struct {
	SchoolID       string         `json:"school_id"`
	RequestType    string         `json:"request_type"`
	SourceService  string         `json:"source_service"`
	SourceID       string         `json:"source_id"`
	Title          string         `json:"title"`
	Summary        map[string]any `json:"summary"`
	AmountIDR      *int64         `json:"amount_idr,omitempty"`
	Flags          []string       `json:"flags,omitempty"`
	MakerUserID    string         `json:"maker_user_id"`
	SubjectUserID  string         `json:"subject_user_id,omitempty"`
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// Request types hr sends to core (must match core model.KnownRequestTypes).
const (
	ReqTypeLeave         = "leave"
	ReqTypeOvertime      = "overtime"
	ReqTypeProfileChange = "profile_change"
	ReqTypeRequisition   = "requisition"
	ReqTypePayrollRun    = "payroll_run"
)

// CreateApproval opens an approval in core and returns its id. source_service,
// callback_url and the correlation/idempotency defaults are filled here so
// every hr caller sends the same contract.
func (c *Client) CreateApproval(ctx context.Context, req ApprovalRequest) (string, error) {
	req.SourceService = "hr"
	if req.CallbackURL == "" {
		req.CallbackURL = c.CallbackURL
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = req.RequestType + ":" + req.SourceID
	}
	if req.CorrelationID == "" {
		req.CorrelationID = req.IdempotencyKey
	}
	if req.Summary == nil {
		req.Summary = map[string]any{}
	}
	var res struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, c.CoreURL+"/internal/approvals", req, req.IdempotencyKey, &res)
	if err == nil && res.ID == "" {
		err = fmt.Errorf("core tidak mengembalikan id approval")
	}
	return res.ID, err
}

// CreateApprovalRequest opens an approval whose subject is the requesting
// employee (core resolves direct_supervisor from subject_user_id, T1 §4.2).
func (c *Client) CreateApprovalRequest(ctx context.Context, schoolID, makerUserID, subjectUserID, requestType, sourceID, title string) (string, error) {
	return c.CreateApproval(ctx, ApprovalRequest{
		SchoolID:      schoolID,
		RequestType:   requestType,
		SourceID:      sourceID,
		Title:         title,
		MakerUserID:   makerUserID,
		SubjectUserID: subjectUserID,
	})
}

// ApprovalErrorMessage turns a CreateApproval error into a user-facing
// message (core's 4xx message, e.g. "alur approval leave belum dikonfigurasi")
// plus the HTTP status the source should answer with.
func ApprovalErrorMessage(err error) (int, string) {
	var he *HTTPError
	if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 {
		return http.StatusUnprocessableEntity, "Approval Center menolak pengajuan: " + he.Message()
	}
	return http.StatusBadGateway, "Approval Center tidak dapat dihubungi, pengajuan belum tersimpan"
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

// Notification is notification-service's POST /internal/notifications body.
type Notification struct {
	SchoolID string         `json:"school_id"`
	UserID   string         `json:"user_id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Data     map[string]any `json:"data,omitempty"`
}

// NotificationPayload is an alias for Notification
type NotificationPayload = Notification

// Notify sends one notification; idempotencyKey makes retries converge.
func (c *Client) Notify(ctx context.Context, n Notification, idempotencyKey string) error {
	return c.do(ctx, http.MethodPost, c.NotifURL+"/internal/notifications", n, idempotencyKey, nil)
}

// SendNotification sends one notification; compatibility helper for Notify.
func (c *Client) SendNotification(ctx context.Context, n Notification, idempotencyKey string) error {
	return c.Notify(ctx, n, idempotencyKey)
}
