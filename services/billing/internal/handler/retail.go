package handler

import (
	"context"
	"time"

	"skci/billing/internal/gateway"
)

// runRetailExpiry is job 2 of §4.4 (FR-151-004): a pending retail pay code
// past expires_at is re-checked at its provider, then settled if it was in
// fact paid, otherwise expired.
func (a *App) runRetailExpiry(ctx context.Context, now time.Time) error {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT t.id, t.provider, t.tx_no, COALESCE(t.provider_ref,''), t.total_idr, t.expires_at
		   FROM gateway_transactions t
		  WHERE t.status = 'pending' AND t.pay_code IS NOT NULL AND t.expires_at <= $1
		  ORDER BY t.expires_at LIMIT 200`, now)
	if err != nil {
		return err
	}
	type due struct {
		id, provider, txNo, ref string
		total                   int64
		expires                 time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.provider, &d.txNo, &d.ref, &d.total, &d.expires); err != nil {
			rows.Close()
			return err
		}
		list = append(list, d)
	}
	rows.Close()
	for _, d := range list {
		if !gateway.RetailExpired(d.expires, now) {
			continue
		}
		st := gateway.StatusExpired
		if p, err := a.paymentProvider(ctx, d.provider); err == nil {
			got, qerr := p.QueryStatus(ctx, gateway.StatusQuery{Kind: gateway.CapRetail, RefNo: d.txNo, ProviderRef: d.ref, AmountIDR: d.total})
			if qerr != nil {
				continue // provider unreachable: try again next tick
			}
			if got == gateway.StatusPaid {
				st = gateway.StatusPaid
			}
		}
		if err := a.applyProviderStatus(ctx, d.id, st, "retail-expiry:"+d.id); err != nil {
			return err
		}
	}
	return nil
}
