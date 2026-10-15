package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"skci/ops/internal/common"
)

type AccountingClient interface {
	RegisterAsset(ctx context.Context, payload map[string]any) (transNo string, err error)
	TransferAsset(ctx context.Context, assetRef string, payload map[string]any) (transOut, transIn string, err error)
	DisposeAsset(ctx context.Context, assetRef string, payload map[string]any) (transNo string, bookVal int64, err error)
	Depreciate(ctx context.Context, schoolID, period string) (assets int, totalIDR int64, transNo string, err error)
	GetBookValues(ctx context.Context, schoolID string, refs []string) ([]BookValueDTO, error)
}

type BookValueDTO struct {
	AssetRef       string                     `json:"asset_ref"`
	CostIDR        int64                      `json:"cost_idr"`
	AccumulatedIDR int64                      `json:"accumulated_idr"`
	BookValueIDR   int64                      `json:"book_value_idr"`
	LastPeriod     string                     `json:"last_period"`
	Schedule       []DepreciationScheduleItem `json:"schedule"`
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
		HTTP:        &http.Client{Timeout: 10 * time.Second, Transport: common.CorrelationTransport{}},
	}
}

func (c *HTTPAccountingClient) do(ctx context.Context, method, path string, body any, idempotencyKey string, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}

	fullURL := c.BaseURL + path
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
		return fmt.Errorf("accounting returned status %d: %s", resp.StatusCode, string(raw))
	}

	if out == nil {
		return nil
	}

	// Try unwrapping if returned in envelope
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Data) > 0 && string(env.Data) != "null" {
		return json.Unmarshal(env.Data, out)
	}

	return json.Unmarshal(raw, out)
}

func (c *HTTPAccountingClient) RegisterAsset(ctx context.Context, payload map[string]any) (string, error) {
	ref, _ := payload["reference"].(string)
	var res struct {
		AssetRef string `json:"asset_ref"`
		TransNo  string `json:"trans_no"`
	}
	err := c.do(ctx, http.MethodPost, "/api/internal/assets", payload, ref, &res)
	return res.TransNo, err
}

func (c *HTTPAccountingClient) TransferAsset(ctx context.Context, assetRef string, payload map[string]any) (string, string, error) {
	ref, _ := payload["reference"].(string)
	var res struct {
		TransNoOut string `json:"trans_no_out"`
		TransNoIn  string `json:"trans_no_in"`
	}
	err := c.do(ctx, http.MethodPost, "/api/internal/assets/"+url.PathEscape(assetRef)+"/transfer", payload, ref, &res)
	return res.TransNoOut, res.TransNoIn, err
}

func (c *HTTPAccountingClient) DisposeAsset(ctx context.Context, assetRef string, payload map[string]any) (string, int64, error) {
	ref, _ := payload["reference"].(string)
	var res struct {
		TransNo      string `json:"trans_no"`
		BookValueIDR int64  `json:"book_value_idr"`
	}
	err := c.do(ctx, http.MethodPost, "/api/internal/assets/"+url.PathEscape(assetRef)+"/dispose", payload, ref, &res)
	return res.TransNo, res.BookValueIDR, err
}

func (c *HTTPAccountingClient) Depreciate(ctx context.Context, schoolID, period string) (int, int64, string, error) {
	payload := map[string]any{
		"school_id": schoolID,
		"period":    period,
	}
	ref := fmt.Sprintf("asset:dep:%s:%s", schoolID, period)
	var res struct {
		Assets   int    `json:"assets"`
		TotalIDR int64  `json:"total_idr"`
		TransNo  string `json:"trans_no"`
	}
	err := c.do(ctx, http.MethodPost, "/api/internal/assets/depreciate", payload, ref, &res)
	return res.Assets, res.TotalIDR, res.TransNo, err
}

func (c *HTTPAccountingClient) GetBookValues(ctx context.Context, schoolID string, refs []string) ([]BookValueDTO, error) {
	q := url.Values{
		"school_id": {schoolID},
		"refs":      {strings.Join(refs, ",")},
	}
	var res []BookValueDTO
	err := c.do(ctx, http.MethodGet, "/api/internal/assets?"+q.Encode(), nil, "", &res)
	return res, err
}
