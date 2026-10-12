package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// AccountBalances reads posted ending balances (debit-positive, whole rupiah)
// from accounting GET /api/internal/accounts/balances (T12 §4.5). Accounts
// missing from the response are reported as an error, never as zero.
func (c *Client) AccountBalances(ctx context.Context, schoolID string, codes []string) (map[string]int64, error) {
	q := url.Values{"school_id": {schoolID}}
	for _, code := range codes {
		q.Add("acc_codes[]", code)
	}
	var rows []struct {
		AccCode   string `json:"acc_code"`
		EndingIDR string `json:"ending_idr"`
	}
	if err := c.Get(ctx, "/api/internal/accounts/balances?"+q.Encode(), &rows); err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		v, err := strconv.ParseInt(strings.TrimSpace(r.EndingIDR), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("saldo akun %s tidak valid: %q", r.AccCode, r.EndingIDR)
		}
		out[r.AccCode] = v
	}
	for _, code := range codes {
		if _, ok := out[code]; !ok {
			return nil, fmt.Errorf("akun %s tidak ditemukan di accounting", code)
		}
	}
	return out, nil
}
