// Package client is the thin internal HTTP client admission uses to talk to
// its sibling services (identity, exam, academic). Every call carries the
// shared X-Internal-Key and an idempotency key so downstream upserts stay
// safe to retry.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured is returned when a downstream base URL is empty.
var ErrNotConfigured = errors.New("downstream service is not configured")

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

// NotFound reports whether an error is a downstream 404.
func NotFound(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && he.Status == http.StatusNotFound
}

// Client is a small JSON-over-HTTP client bound to one service base URL.
type Client struct {
	BaseURL     string
	InternalKey string
	HTTP        *http.Client
}

// New builds a client; url may be empty, in which case every call fails with
// ErrNotConfigured (so the service degrades instead of panicking).
func New(url, key string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(url, "/"),
		InternalKey: key,
		HTTP:        &http.Client{Timeout: 8 * time.Second},
	}
}

// Configured reports whether the client has a usable base URL.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" }

// Post sends a JSON body and decodes the envelope's "data" field into out.
// idempotencyKey is sent as the Idempotency-Key header.
func (c *Client) Post(ctx context.Context, path, idempotencyKey string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	return c.do(ctx, http.MethodPost, path, idempotencyKey, body, out)
}

// Get performs a GET and decodes the envelope's "data" field into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, "", nil, out)
}

func (c *Client) do(ctx context.Context, method, path, idempotencyKey string, body io.Reader, out any) error {
	if c.BaseURL == "" {
		return ErrNotConfigured
	}
	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(raw)), URL: url}
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

// Message extracts the envelope "message" from a downstream error body.
func (e *HTTPError) Message() string {
	var env struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &env) == nil && env.Message != "" {
		return env.Message
	}
	return e.Body
}

// Approval request types admission sends to core (core model.KnownRequestTypes).
const (
	ReqTypeDiscountSpecial = "discount_special"
	ReqTypeSelectionResult = "selection_result"
)

// ApprovalRequest is core's POST /internal/approvals body.
type ApprovalRequest struct {
	SchoolID       string         `json:"school_id"`
	RequestType    string         `json:"request_type"`
	SourceService  string         `json:"source_service"`
	SourceID       string         `json:"source_id"`
	Title          string         `json:"title"`
	Summary        map[string]any `json:"summary"`
	AmountIDR      *int64         `json:"amount_idr,omitempty"`
	MakerUserID    string         `json:"maker_user_id"`
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// CreateApproval opens a maker-checker approval in core and returns its id.
// Core never auto-approves; a missing flow is a 422 from core.
func (c *Client) CreateApproval(ctx context.Context, req ApprovalRequest) (string, error) {
	req.SourceService = "admission"
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
	if err := c.Post(ctx, "/internal/approvals", req.IdempotencyKey, req, &res); err != nil {
		return "", err
	}
	if res.ID == "" {
		return "", errors.New("core tidak mengembalikan id approval")
	}
	return res.ID, nil
}

// CancelApproval withdraws a pending approval in core.
func (c *Client) CancelApproval(ctx context.Context, approvalID, actorUserID, reason string) error {
	return c.Post(ctx, "/internal/approvals/"+approvalID+"/cancel", "",
		map[string]string{"actor_user_id": actorUserID, "reason": reason}, nil)
}

// ApprovalErrorMessage maps a CreateApproval failure to the status and
// Indonesian message the source endpoint answers with.
func ApprovalErrorMessage(err error) (int, string) {
	var he *HTTPError
	if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 {
		return http.StatusUnprocessableEntity, "Approval Center menolak pengajuan: " + he.Message()
	}
	return http.StatusBadGateway, "Approval Center tidak dapat dihubungi, pengajuan belum tersimpan"
}
