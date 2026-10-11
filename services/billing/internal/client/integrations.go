package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// PostJSON sends body with X-Internal-Key (and Idempotency-Key when given)
// and decodes either the envelope's "data" or, for non-enveloped services
// (Laravel accounting), the raw body into out. It returns the HTTP status so
// callers can tell a 201 create from a 200 idempotent replay.
func (c *Client) PostJSON(ctx context.Context, path, idempotencyKey string, body, out any) (int, error) {
	if !c.Configured() {
		return 0, ErrNotConfigured
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	full := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(data)), URL: full}
	}
	if out == nil || len(data) == 0 {
		return resp.StatusCode, nil
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &env) == nil && len(env.Data) > 0 && string(env.Data) != "null" {
		return resp.StatusCode, json.Unmarshal(env.Data, out)
	}
	return resp.StatusCode, json.Unmarshal(data, out)
}

// ---- core (T1 §3) --------------------------------------------------------

// AuditEvent is the body of core POST /internal/audit.
type AuditEvent struct {
	SchoolID      string `json:"school_id"`
	Service       string `json:"service"`
	Module        string `json:"module"`
	EntityType    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	Action        string `json:"action"`
	ActorUserID   string `json:"actor_user_id,omitempty"`
	ActorRole     string `json:"actor_role,omitempty"`
	Before        any    `json:"before,omitempty"`
	After         any    `json:"after,omitempty"`
	Reason        string `json:"reason,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Audit records one event in core's central audit log.
func (c *Client) Audit(ctx context.Context, e AuditEvent) error {
	e.Service = "billing"
	_, err := c.PostJSON(ctx, "/internal/audit", "", e, nil)
	return err
}

// ExceptionEvent is the body of core POST /internal/exceptions.
type ExceptionEvent struct {
	SchoolID       string `json:"school_id"`
	SourceService  string `json:"source_service"`
	Kind           string `json:"kind"`     // webhook | journal_posting | callback | ...
	Severity       string `json:"severity"` // normal | critical
	CorrelationID  string `json:"correlation_id"`
	ErrorMessage   string `json:"error_message"`
	PayloadSummary any    `json:"payload_summary"`
	RetryURL       string `json:"retry_url,omitempty"`
	MaxAttempts    int    `json:"max_attempts"`
	IdempotencyKey string `json:"idempotency_key"`
}

// CreateException enqueues an item in core's Integration Exception Queue.
func (c *Client) CreateException(ctx context.Context, e ExceptionEvent) error {
	e.SourceService = "billing"
	_, err := c.PostJSON(ctx, "/internal/exceptions", e.IdempotencyKey, e, nil)
	return err
}

// ApprovalRequest is the body of core POST /internal/approvals.
type ApprovalRequest struct {
	SchoolID       string `json:"school_id"`
	RequestType    string `json:"request_type"`
	SourceService  string `json:"source_service"`
	SourceID       string `json:"source_id"`
	Title          string `json:"title"`
	Summary        any    `json:"summary"`
	AmountIDR      *int64 `json:"amount_idr,omitempty"`
	MakerUserID    string `json:"maker_user_id"`
	SubjectUserID  string `json:"subject_user_id,omitempty"`
	CallbackURL    string `json:"callback_url"`
	CorrelationID  string `json:"correlation_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

// ApprovalResponse is core's reply to an approval request.
type ApprovalResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	CurrentSeq *int   `json:"current_seq"`
}

// RequestApproval submits a maker-checker request. A 422 from core means no
// flow is configured for the request type; callers must surface it, never
// auto-approve.
func (c *Client) RequestApproval(ctx context.Context, r ApprovalRequest) (ApprovalResponse, error) {
	r.SourceService = "billing"
	var out ApprovalResponse
	_, err := c.PostJSON(ctx, "/internal/approvals", r.IdempotencyKey, r, &out)
	return out, err
}

// ApprovalCallback is what core POSTs to callback_url once a request is decided.
type ApprovalCallback struct {
	ApprovalID  string `json:"approval_id"`
	SourceID    string `json:"source_id"`
	RequestType string `json:"request_type"`
	Decision    string `json:"decision"` // approved | rejected
	DecidedBy   string `json:"decided_by"`
	Comment     string `json:"comment"`
	DecidedAt   string `json:"decided_at"`
}

// ---- notification --------------------------------------------------------

// Notification is the body of notification-service POST /internal/notifications.
type Notification struct {
	SchoolID string         `json:"school_id"`
	UserID   string         `json:"user_id"`
	Type     string         `json:"type"`
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Data     map[string]any `json:"data"`
	Channels []string       `json:"channels"`
}

// Notify sends one notification; the idempotency key makes retries converge.
func (c *Client) Notify(ctx context.Context, idempotencyKey string, n Notification) error {
	_, err := c.PostJSON(ctx, "/internal/notifications", idempotencyKey, n, nil)
	return err
}

// ---- accounting (T1 §7b) -------------------------------------------------

// JournalLine is one line of an automatic journal, integer rupiah.
type JournalLine struct {
	AccCode     string  `json:"acc_code"`
	DivCode     *string `json:"div_code"`
	Description string  `json:"description"`
	DebitIDR    int64   `json:"debit_idr"`
	CreditIDR   int64   `json:"credit_idr"`
}

// JournalRequest is the body of accounting POST /api/internal/journals.
type JournalRequest struct {
	SchoolID    string        `json:"school_id"`
	Source      string        `json:"source"`
	Reference   string        `json:"reference"`
	TransDate   string        `json:"trans_date"`
	Description string        `json:"description"`
	Lines       []JournalLine `json:"lines"`
}

// PostJournal posts one balanced journal. A repeated reference returns the
// original trans_no (200), so retries never double-post.
func (c *Client) PostJournal(ctx context.Context, j JournalRequest) (string, error) {
	var out struct {
		TransNo string `json:"trans_no"`
	}
	if _, err := c.PostJSON(ctx, "/api/internal/journals", j.Reference, j, &out); err != nil {
		return "", err
	}
	if out.TransNo == "" {
		return "", fmt.Errorf("accounting returned no trans_no for %s", j.Reference)
	}
	return out.TransNo, nil
}

// ---- identity reads ------------------------------------------------------

// UserProfile is the subset of identity GET /users/{id} billing needs for
// receipts and NICEPay buyer fields.
type UserProfile struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Email string  `json:"email"`
	Phone *string `json:"phone"`
}

// User reads one user from identity.
func (c *Client) User(ctx context.Context, userID string) (UserProfile, error) {
	var u UserProfile
	err := c.Get(ctx, "/users/"+url.PathEscape(userID), &u)
	return u, err
}

// SchoolName reads the school's display name from identity.
func (c *Client) SchoolName(ctx context.Context, schoolID string) (string, error) {
	var s struct {
		Name string `json:"name"`
	}
	err := c.Get(ctx, "/schools/"+url.PathEscape(schoolID), &s)
	return s.Name, err
}

// FeatureEnabled reports whether a school has featureKey switched on. It
// fails closed: any error reads as "off".
func (c *Client) FeatureEnabled(ctx context.Context, schoolID, featureKey string) (bool, error) {
	var rows []struct {
		FeatureKey string `json:"feature_key"`
		Enabled    bool   `json:"enabled"`
	}
	if err := c.Get(ctx, "/schools/"+url.PathEscape(schoolID)+"/features", &rows); err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.FeatureKey == featureKey {
			return r.Enabled, nil
		}
	}
	return false, nil
}
