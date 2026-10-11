// Package upstream contains thin typed HTTP clients for the sibling
// services this report-service aggregates from (identity, academic,
// attendance, exam). Every call is internal service-to-service traffic:
// authenticated with X-Internal-Key (never the caller's JWT — the upstream
// services are not expected to re-validate the end-user's token on internal
// calls, see README > Expected upstream contracts) and bounded by a short
// timeout so a slow/down sibling degrades this service to partial data
// instead of hanging or crashing a request.
package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrUnavailable is returned (optionally wrapped) whenever an upstream
// service could not be reached or did not respond successfully in time.
// Handlers should treat this as "degrade to partial data", not a 500.
var ErrUnavailable = errors.New("upstream service unavailable")

// UnavailableError carries which service/call failed, for logging/response
// hints, while still satisfying errors.Is(err, ErrUnavailable).
type UnavailableError struct {
	Service string
	Op      string
	Err     error
}

func (e *UnavailableError) Error() string {
	return fmt.Sprintf("upstream %s (%s): %v", e.Service, e.Op, e.Err)
}

func (e *UnavailableError) Unwrap() error { return ErrUnavailable }

func newUnavailable(service, op string, err error) error {
	return &UnavailableError{Service: service, Op: op, Err: err}
}

// envelope mirrors the shared response.go shape used by every sekoci
// service: {"data": ..., "meta": ..., "message": "", "errors": null}.
type envelope struct {
	Data   json.RawMessage `json:"data"`
	Meta   json.RawMessage `json:"meta"`
	Errors json.RawMessage `json:"errors"`
}

// Client is a small shared HTTP helper used by every per-service client
// below. Not exported outside this package; each service gets its own
// typed wrapper (IdentityClient, AcademicClient, ...) so handler code never
// deals with raw JSON.
type client struct {
	baseURL     string
	internalKey string
	httpClient  *http.Client
	service     string // for error/logging context, e.g. "identity"
}

func newClient(baseURL, internalKey, service string, timeout time.Duration) *client {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &client{
		baseURL:     baseURL,
		internalKey: internalKey,
		service:     service,
		httpClient:  &http.Client{Timeout: timeout},
	}
}

// doGet performs the shared GET + X-Internal-Key plumbing every client call
// goes through and returns the upstream's HTTP status alongside the raw
// body. Every error here is a transport-level failure (request build,
// network, timeout, body read) and is wrapped as *UnavailableError.
func (c *client) doGet(ctx context.Context, op, path string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, nil, newUnavailable(c.service, op, err)
	}
	req.Header.Set("X-Internal-Key", c.internalKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, newUnavailable(c.service, op, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, newUnavailable(c.service, op, err)
	}
	return resp.StatusCode, body, nil
}

// getStatus performs GET path and reports the upstream's HTTP status code,
// treating every status (404 included) as a valid answer. Unlike getJSON it
// does not collapse "not found" into "empty success", so a caller that must
// tell 200 from 404 apart -- an authorization probe answering "linked" vs
// "not linked" -- can switch on the code. Transport failures still wrap
// ErrUnavailable so the caller can fail closed on a service that cannot
// answer at all.
func (c *client) getStatus(ctx context.Context, op, path string) (int, error) {
	status, _, err := c.doGet(ctx, op, path)
	return status, err
}

// getJSON performs GET path, unwraps the {"data": ...} envelope, and
// decodes .data into out. Returns an *UnavailableError (wrapping
// ErrUnavailable) for any network error, timeout, or non-2xx status other
// than 404 (404 decodes into out as-is left zero-valued and returns nil,
// since "not found upstream" is a valid, non-degraded answer for lookups).
func (c *client) getJSON(ctx context.Context, op, path string, out interface{}) error {
	status, body, err := c.doGet(ctx, op, path)
	if err != nil {
		return err
	}

	if status == http.StatusNotFound {
		return nil
	}
	if status < 200 || status >= 300 {
		return newUnavailable(c.service, op, fmt.Errorf("status %d: %s", status, string(body)))
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return newUnavailable(c.service, op, err)
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return newUnavailable(c.service, op, err)
	}
	return nil
}
