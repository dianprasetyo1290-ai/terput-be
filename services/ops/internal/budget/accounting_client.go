package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ActualQueryItem struct {
	Key     string `json:"key"` // budget_line_id
	AccCode string `json:"acc_code"`
	DivCode string `json:"div_code,omitempty"`
}

type ActualResultItem struct {
	Key       string `json:"key"`
	ActualIDR int64  `json:"actual_idr"`
}

type AccountingClient interface {
	FetchActuals(ctx context.Context, schoolID string, from, to string, items []ActualQueryItem) ([]ActualResultItem, error)
}

type HTTPAccountingClient struct {
	BaseURL     string
	InternalKey string
	HTTP        *http.Client
}

func NewAccountingClient(baseURL, internalKey string) *HTTPAccountingClient {
	return &HTTPAccountingClient{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		InternalKey: internalKey,
		HTTP:        &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *HTTPAccountingClient) FetchActuals(ctx context.Context, schoolID string, from, to string, items []ActualQueryItem) ([]ActualResultItem, error) {
	if c == nil || c.BaseURL == "" {
		return nil, nil
	}

	payload := map[string]any{
		"school_id": schoolID,
		"from":      from,
		"to":        to,
		"items":     items,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/internal/ledger/actuals", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.InternalKey != "" {
		req.Header.Set("X-Internal-Key", c.InternalKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("accounting fetch actuals error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusConflict {
		return nil, fmt.Errorf("modul akuntansi tidak aktif (409)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("accounting fetch actuals returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var res struct {
		Status int                `json:"status"`
		Items  []ActualResultItem `json:"items"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return nil, fmt.Errorf("decode actuals response error: %w", err)
	}

	return res.Items, nil
}
