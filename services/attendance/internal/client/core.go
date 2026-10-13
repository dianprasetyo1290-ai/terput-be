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

// Calls to core-service (Approval Center + central audit trail, T1 §3) and
// identity's guardian lookup, all over X-Internal-Key. Mirrors hr/ops'
// client contract so core sees the same request shapes from every source.

// ReqTypeAttendanceCorrection must match core model.KnownRequestTypes.
const ReqTypeAttendanceCorrection = "attendance_correction"

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

// SetCoreURLs configures core + the callback base (tests / explicit wiring).
func (c *Client) SetCoreURLs(coreURL, callbackURL string) {
	c.coreURL = strings.TrimRight(coreURL, "/")
	c.callbackURL = callbackURL
}

func (c *Client) doJSON(ctx context.Context, method, fullURL string, body any, idemKey string, out any) error {
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
	req.Header.Set("X-Internal-Key", c.internalKey)
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	resp, err := c.httpClient.Do(req)
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
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode downstream envelope: %w", err)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// ApprovalRequest is core's POST /internal/approvals body (T1 §3).
type ApprovalRequest struct {
	SchoolID       string         `json:"school_id"`
	RequestType    string         `json:"request_type"`
	SourceService  string         `json:"source_service"`
	SourceID       string         `json:"source_id"`
	Title          string         `json:"title"`
	Summary        map[string]any `json:"summary"`
	Flags          []string       `json:"flags,omitempty"`
	MakerUserID    string         `json:"maker_user_id"`
	SubjectUserID  string         `json:"subject_user_id,omitempty"`
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}

// CreateApproval opens an approval in core and returns its id.
func (c *Client) CreateApproval(ctx context.Context, req ApprovalRequest) (string, error) {
	if c.coreURL == "" {
		return "", errors.New("core service not configured")
	}
	req.SourceService = "attendance"
	if req.CallbackURL == "" {
		req.CallbackURL = c.callbackURL
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
	err := c.doJSON(ctx, http.MethodPost, c.coreURL+"/internal/approvals", req, req.IdempotencyKey, &res)
	if err == nil && res.ID == "" {
		err = fmt.Errorf("core tidak mengembalikan id approval")
	}
	return res.ID, err
}

// ApprovalErrorMessage turns a CreateApproval error into a user-facing
// message plus the HTTP status the caller should answer with.
func ApprovalErrorMessage(err error) (int, string) {
	var he *HTTPError
	if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 {
		return http.StatusUnprocessableEntity, "Approval Center menolak pengajuan: " + he.Message()
	}
	return http.StatusBadGateway, "Approval Center tidak dapat dihubungi, pengajuan koreksi belum tersimpan"
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
	if c.coreURL == "" {
		return errors.New("core service not configured")
	}
	ev.Service = "attendance"
	return c.doJSON(ctx, http.MethodPost, c.coreURL+"/internal/audit", ev, "", nil)
}

// StudentGuardianIDs lists the student's active guardians' user ids at the
// school (identity GET /internal/students/{id}/guardians, T9 §4.1).
func (c *Client) StudentGuardianIDs(ctx context.Context, studentID, schoolID string) ([]string, error) {
	if c.identityBaseURL == "" {
		return nil, errors.New("identity service not configured")
	}
	var rows []struct {
		GuardianUserID string `json:"guardian_user_id"`
	}
	u := fmt.Sprintf("%s/internal/students/%s/guardians?school_id=%s", c.identityBaseURL, url.PathEscape(studentID), url.QueryEscape(schoolID))
	if err := c.doJSON(ctx, http.MethodGet, u, nil, "", &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.GuardianUserID != "" {
			out = append(out, r.GuardianUserID)
		}
	}
	return out, nil
}
