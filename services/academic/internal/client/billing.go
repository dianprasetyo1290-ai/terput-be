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

	"skci/academic/internal/model"
)

type BillingClient struct {
	BaseURL     string
	InternalKey string
	HTTPClient  *http.Client
}

func NewBillingClient() *BillingClient {
	base := os.Getenv("BILLING_BASE_URL")
	if base == "" {
		base = "http://billing:8090"
	}
	key := os.Getenv("SEKOCI_INTERNAL_KEY")
	if key == "" {
		key = "change-me"
	}
	return &BillingClient{
		BaseURL:     strings.TrimRight(base, "/"),
		InternalKey: key,
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	}
}

// GetStudentFinancialSummary calls GET /internal/students/{id}/financial-summary?school_id=
func (c *BillingClient) GetStudentFinancialSummary(ctx context.Context, schoolID, studentUserID string) (*model.FinanceSnapshot, error) {
	url := fmt.Sprintf("%s/internal/students/%s/financial-summary?school_id=%s", c.BaseURL, studentUserID, schoolID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("billing returned status %d", resp.StatusCode)
	}

	var env struct {
		Data model.FinanceSnapshot `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// ExitStudent calls POST /internal/students/{id}/exit
func (c *BillingClient) ExitStudent(ctx context.Context, schoolID, studentUserID, effectiveDate, policy string) error {
	url := fmt.Sprintf("%s/internal/students/%s/exit", c.BaseURL, studentUserID)
	body, _ := json.Marshal(map[string]any{
		"school_id":      schoolID,
		"effective_date": effectiveDate,
		"policy":         policy,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("billing exit returned status %d", resp.StatusCode)
	}
	return nil
}

// RefundStudent calls POST /internal/students/{id}/refund
func (c *BillingClient) RefundStudent(ctx context.Context, schoolID, studentUserID string, amountIDR int64, reason, idempotencyKey string) error {
	url := fmt.Sprintf("%s/internal/students/%s/refund", c.BaseURL, studentUserID)
	body, _ := json.Marshal(map[string]any{
		"school_id":       schoolID,
		"amount_idr":      amountIDR,
		"reason":          reason,
		"idempotency_key": idempotencyKey,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("billing refund returned status %d", resp.StatusCode)
	}
	return nil
}
