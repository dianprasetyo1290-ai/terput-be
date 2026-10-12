package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type CoreClient struct {
	BaseURL     string
	InternalKey string
	// CallbackBaseURL is academic's own internal URL as core reaches it
	// (ACADEMIC_INTERNAL_URL); core rejects a relative callback_url.
	CallbackBaseURL string
	HTTPClient      *http.Client
}

func NewCoreClient() *CoreClient {
	base := os.Getenv("CORE_BASE_URL")
	if base == "" {
		base = "http://core:8091"
	}
	key := os.Getenv("SEKOCI_INTERNAL_KEY")
	if key == "" {
		key = "change-me"
	}
	callbackBase := os.Getenv("ACADEMIC_INTERNAL_URL")
	if callbackBase == "" {
		callbackBase = "http://academic:8082"
	}
	return &CoreClient{
		BaseURL:         strings.TrimRight(base, "/"),
		InternalKey:     key,
		CallbackBaseURL: strings.TrimRight(callbackBase, "/"),
		HTTPClient:      &http.Client{Timeout: 5 * time.Second},
	}
}

type CoreApprovalRequest struct {
	SchoolID       string         `json:"school_id"`
	RequestType    string         `json:"request_type"`
	SourceService  string         `json:"source_service"`
	SourceID       string         `json:"source_id"`
	Title          string         `json:"title"`
	Summary        map[string]any `json:"summary"`
	AmountIDR      *int64         `json:"amount_idr,omitempty"`
	Flags          []string       `json:"flags,omitempty"`
	MakerUserID    string         `json:"maker_user_id"`
	SubjectUserID  *string        `json:"subject_user_id,omitempty"`
	CallbackURL    string         `json:"callback_url"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key"`
}

type CoreApprovalResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	CurrentSeq int    `json:"current_seq"`
}

// SubmitApproval calls core POST /internal/approvals
func (c *CoreClient) SubmitApproval(ctx context.Context, req CoreApprovalRequest) (*CoreApprovalResponse, error) {
	url := fmt.Sprintf("%s/internal/approvals", c.BaseURL)
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("X-Internal-Key", c.InternalKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("core approval returned status %d", resp.StatusCode)
	}

	var env struct {
		Data CoreApprovalResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

type CoreAuditEvent struct {
	SchoolID      string         `json:"school_id"`
	Service       string         `json:"service"`
	Module        string         `json:"module"`
	EntityType    string         `json:"entity_type"`
	EntityID      string         `json:"entity_id"`
	Action        string         `json:"action"`
	ActorUserID   string         `json:"actor_user_id"`
	ActorRole     string         `json:"actor_role"`
	Before        map[string]any `json:"before,omitempty"`
	After         map[string]any `json:"after,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
}

// Audit sends an audit event to core service POST /internal/audit
func (c *CoreClient) Audit(ctx context.Context, event CoreAuditEvent) error {
	url := fmt.Sprintf("%s/internal/audit", c.BaseURL)
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("X-Internal-Key", c.InternalKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
