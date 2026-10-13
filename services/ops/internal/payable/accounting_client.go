package payable

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

type AccountingClient interface {
	PostJournal(ctx context.Context, payload map[string]any) (transNo string, err error)
	SyncVendor(ctx context.Context, vendorID string, payload map[string]any) error
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

func (c *HTTPAccountingClient) PostJournal(ctx context.Context, payload map[string]any) (string, error) {
	if c == nil || c.BaseURL == "" {
		return "", nil
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/internal/journals", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.InternalKey != "" {
		req.Header.Set("X-Internal-Key", c.InternalKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("accounting post journal error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusConflict {
		return "", fmt.Errorf("modul akuntansi tidak aktif (409)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("accounting post journal status %d: %s", resp.StatusCode, string(respBytes))
	}

	var res struct {
		Status  int    `json:"status"`
		TransNo string `json:"trans_no"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(respBytes, &res); err != nil {
		return "", fmt.Errorf("decode post journal error: %w", err)
	}

	return res.TransNo, nil
}

func (c *HTTPAccountingClient) SyncVendor(ctx context.Context, vendorID string, payload map[string]any) error {
	if c == nil || c.BaseURL == "" {
		return nil
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/api/internal/vendors/"+vendorID, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.InternalKey != "" {
		req.Header.Set("X-Internal-Key", c.InternalKey)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("accounting sync vendor error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("accounting sync vendor status %d: %s", resp.StatusCode, string(respBytes))
	}

	return nil
}
