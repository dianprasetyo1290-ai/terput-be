package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"skci/insight/internal/config"
	"skci/insight/internal/predict"
)

type Client struct {
	cfg        *config.Config
	httpClient *http.Client
}

func New(cfg *config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type MetricFactResponse struct {
	Metric      string          `json:"metric"`
	Numerator   *int64          `json:"numerator,omitempty"`
	Denominator *int64          `json:"denominator,omitempty"`
	Value       *int64          `json:"value,omitempty"`
	Unit        string          `json:"unit"`
	AsOf        time.Time       `json:"as_of"`
	Rows        []MetricFactRow `json:"rows,omitempty"`
}

type MetricFactRow struct {
	DimValue    string `json:"dim_value"`
	Numerator   *int64 `json:"numerator,omitempty"`
	Denominator *int64 `json:"denominator,omitempty"`
	Value       *int64 `json:"value,omitempty"`
}

type SchoolInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level string `json:"level"`
}

func (c *Client) getBaseURL(service string) string {
	switch service {
	case "billing":
		return c.cfg.BillingURL
	case "attendance":
		return c.cfg.AttendanceURL
	case "admission":
		return c.cfg.AdmissionURL
	case "identity":
		return c.cfg.IdentityURL
	case "hr":
		return c.cfg.HrURL
	case "studentlife":
		return c.cfg.StudentlifeURL
	case "notification":
		return c.cfg.NotificationURL
	case "core":
		return c.cfg.CoreURL
	default:
		return ""
	}
}

// FetchMetricFacts calls GET /internal/metric-facts on the owner service
func (c *Client) FetchMetricFacts(ctx context.Context, service string, metric string, schoolID string, from string, to string, groupBy string) (*MetricFactResponse, error) {
	baseURL := c.getBaseURL(service)
	if baseURL == "" {
		return nil, fmt.Errorf("unknown service: %s", service)
	}

	endpoint := fmt.Sprintf("%s/internal/metric-facts", baseURL)
	params := url.Values{}
	params.Set("metric", metric)
	params.Set("school_id", schoolID)
	if from != "" {
		params.Set("from", from)
	}
	if to != "" {
		params.Set("to", to)
	}
	if groupBy != "" {
		params.Set("group_by", groupBy)
	}

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.cfg.SekociInternalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("source %s metric-facts returned %d: %s", service, resp.StatusCode, string(body))
	}

	var env struct {
		Data MetricFactResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// FetchFoundationSchools calls identity GET /internal/schools/{id}/foundation-schools
func (c *Client) FetchFoundationSchools(ctx context.Context, schoolID string) ([]SchoolInfo, error) {
	endpoint := fmt.Sprintf("%s/internal/schools/%s/foundation-schools", c.cfg.IdentityURL, schoolID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.cfg.SekociInternalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("identity foundation-schools returned %d", resp.StatusCode)
	}

	var env struct {
		Data []SchoolInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// FetchRiskFactsArrears calls billing GET /internal/risk-facts/arrears
func (c *Client) FetchRiskFactsArrears(ctx context.Context, schoolID string, asOf string) ([]predict.StudentArrearsFacts, error) {
	endpoint := fmt.Sprintf("%s/internal/risk-facts/arrears?school_id=%s&as_of=%s", c.cfg.BillingURL, url.QueryEscape(schoolID), url.QueryEscape(asOf))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.cfg.SekociInternalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("billing risk-facts arrears returned %d", resp.StatusCode)
	}

	var env struct {
		Data []predict.StudentArrearsFacts `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// FetchRiskFactsAbsence calls attendance GET /internal/risk-facts/absence
func (c *Client) FetchRiskFactsAbsence(ctx context.Context, schoolID string, from string, to string) ([]predict.StudentAbsenceFacts, error) {
	endpoint := fmt.Sprintf("%s/internal/risk-facts/absence?school_id=%s&from=%s&to=%s", c.cfg.AttendanceURL, url.QueryEscape(schoolID), url.QueryEscape(from), url.QueryEscape(to))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.cfg.SekociInternalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("attendance risk-facts absence returned %d", resp.StatusCode)
	}

	var env struct {
		Data []predict.StudentAbsenceFacts `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// ForwardWithAuth forwards a request to a source service with user's JWT
func (c *Client) ForwardWithAuth(ctx context.Context, method string, fullURL string, authHeader string, bodyData interface{}) (int, []byte, error) {
	var bodyReader io.Reader
	if bodyData != nil {
		b, err := json.Marshal(bodyData)
		if err != nil {
			return 0, nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	return resp.StatusCode, respBody, err
}

// SendScheduledNotification dispatches scheduled report notification
func (c *Client) SendScheduledNotification(ctx context.Context, recipientUserID string, title string, body string, idempotencyKey string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/internal/notifications", c.cfg.NotificationURL)
	payload := map[string]interface{}{
		"recipient_user_id": recipientUserID,
		"type":              "scheduled_report",
		"title":             title,
		"body":              body,
		"channels":          []string{"in_app", "email"},
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.cfg.SekociInternalKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// ForwardAudit sends audit event to core
func (c *Client) ForwardAudit(ctx context.Context, schoolID, eventType, actorID, entityType, entityID string, diff map[string]interface{}) error {
	endpoint := fmt.Sprintf("%s/internal/audit", c.cfg.CoreURL)
	payload := map[string]interface{}{
		"school_id":     schoolID,
		"event_type":    eventType,
		"actor_user_id": actorID,
		"entity_type":   entityType,
		"entity_id":     entityID,
		"diff":          diff,
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.cfg.SekociInternalKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
