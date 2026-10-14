package handler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/gateway"
)

// ---------------------------------------------------------------------------
// Background jobs: gateway transaction expiry and the daily settlement
// reconciliation (BUG-136).
// ---------------------------------------------------------------------------

// JobGatewayExpiry closes pending (non-retail) gateway transactions past
// their deadline so the payment dashboard stops counting them as pending.
func (a *App) JobGatewayExpiry(ctx context.Context, _ *sql.DB) error {
	return a.runGatewayExpiry(ctx, time.Now())
}

// runGatewayExpiry asks each overdue transaction's provider once more: a
// paid one is settled (same path as the webhook), otherwise it is expired.
// closeTransaction only acts on a still-pending row, so a rerun (or a
// concurrent webhook) never closes twice; a late paid webhook still settles
// an expired transaction. Every expiry is audited.
func (a *App) runGatewayExpiry(ctx context.Context, now time.Time) error {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id::text, school_id::text FROM gateway_transactions
		  WHERE status = 'pending' AND pay_code IS NULL AND expires_at <= $1
		  ORDER BY expires_at LIMIT 200`, now)
	if err != nil {
		return err
	}
	type due struct{ id, school string }
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.school); err != nil {
			rows.Close()
			return err
		}
		list = append(list, d)
	}
	rows.Close()
	var firstErr error
	keep := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}
	for _, d := range list {
		tx, err := a.getTransactionByID(ctx, d.school, d.id)
		if err != nil {
			keep(err)
			continue
		}
		st := gateway.StatusExpired
		if p, perr := a.paymentProvider(ctx, tx.Provider); perr == nil {
			if got, qerr := p.QueryStatus(ctx, tx.statusQuery()); qerr == nil && got == gateway.StatusPaid {
				st = gateway.StatusPaid
			}
		}
		// One failing transaction never blocks the others; it is retried
		// next tick.
		if err := a.applyProviderStatus(ctx, d.id, st, "expiry-job:"+d.id); err != nil {
			keep(fmt.Errorf("expire %s: %w", d.id, err))
			continue
		}
		var after string
		if err := a.DB.QueryRowContext(ctx, `SELECT status FROM gateway_transactions WHERE id = $1::uuid`, d.id).Scan(&after); err != nil {
			keep(err)
			continue
		}
		if after != "pending" {
			a.audit(ctx, d.school, "gateway_transaction", d.id, "gateway_tx_"+after, "system", "job:gateway_expiry", "", "",
				map[string]any{"status": "pending", "expires_at": tx.ExpiresAt}, map[string]any{"status": after})
		}
	}
	return firstErr
}

// SettlementGraceDays is how long a day's settlement report may be late
// before finance is alerted that it never arrived.
const SettlementGraceDays = 2

// settlementLookbackDays bounds the pending window the job works on.
const settlementLookbackDays = 14

// JobSettlementReconciliation reconciles every settlement batch still
// "reported" (pending) for past days, retrying each tick until it runs, and
// alerts finance (core exception queue, once per batch/day) on a mismatch or
// on a settlement report that is overdue.
func (a *App) JobSettlementReconciliation(ctx context.Context, _ *sql.DB) error {
	return a.runSettlementReconciliation(ctx, time.Now())
}

func (a *App) runSettlementReconciliation(ctx context.Context, now time.Time) error {
	loc := a.location()
	today := now.In(loc).Format("2006-01-02")
	from := now.In(loc).AddDate(0, 0, -settlementLookbackDays).Format("2006-01-02")
	rows, err := a.DB.QueryContext(ctx,
		`SELECT DISTINCT school_id::text, provider, settled_on::text FROM settlement_batches
		  WHERE status = 'reported' AND settled_on < $1::date AND settled_on >= $2::date
		  ORDER BY 3`, today, from)
	if err != nil {
		return err
	}
	type day struct{ school, provider, date string }
	var pending []day
	for rows.Next() {
		var d day
		if err := rows.Scan(&d.school, &d.provider, &d.date); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, d)
	}
	rows.Close()
	var firstErr error
	for _, d := range pending {
		res, err := a.runReconciliation(ctx, d.school, d.date, d.provider, "system", "job:settlement_reconciliation")
		if err != nil {
			// Stays "reported": retried next tick.
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, b := range res.Batches {
			if b.Status == "mismatch" {
				a.raiseReconAlert(ctx, d.school, "recon_mismatch:"+b.ID, d.date,
					fmt.Sprintf("Rekonsiliasi settlement %s %s tanggal %s tidak cocok (%d selisih)", b.Provider, b.Channel, d.date, res.ExceptionCount),
					map[string]any{"batch_id": b.ID, "provider": b.Provider, "channel": b.Channel, "date": d.date, "exceptions": res.ExceptionCount})
			}
		}
	}

	// Successful transactions whose settlement never arrived.
	cutoff := now.In(loc).AddDate(0, 0, -SettlementGraceDays).Format("2006-01-02")
	rows, err = a.DB.QueryContext(ctx,
		`SELECT t.school_id::text, t.provider, DATE(t.paid_at)::text, count(*), COALESCE(sum(t.total_idr),0)
		   FROM gateway_transactions t
		  WHERE t.status = 'success' AND t.paid_at IS NOT NULL
		    AND DATE(t.paid_at) < $1::date AND DATE(t.paid_at) >= $2::date
		    AND NOT EXISTS (SELECT 1 FROM settlement_batches b
		                     WHERE b.school_id = t.school_id AND b.provider = t.provider AND b.settled_on = DATE(t.paid_at))
		  GROUP BY 1, 2, 3`, cutoff, from)
	if err != nil {
		return err
	}
	type missing struct {
		school, provider, date string
		n                      int
		total                  int64
	}
	var late []missing
	for rows.Next() {
		var m missing
		if err := rows.Scan(&m.school, &m.provider, &m.date, &m.n, &m.total); err != nil {
			rows.Close()
			return err
		}
		late = append(late, m)
	}
	rows.Close()
	for _, m := range late {
		a.raiseReconAlert(ctx, m.school, "settlement_missing:"+m.provider+":"+m.date, m.date,
			fmt.Sprintf("Laporan settlement %s tanggal %s belum diterima (%d transaksi, %s)", m.provider, m.date, m.n, formatRupiah(m.total)),
			map[string]any{"provider": m.provider, "date": m.date, "transactions": m.n, "total_idr": m.total})
	}
	return firstErr
}

// raiseReconAlert records a reconciliation issue in core's exception queue
// once per key and day (marker in gateway_job_runs, written only after
// core accepted it so a failed send is retried next tick).
func (a *App) raiseReconAlert(ctx context.Context, schoolID, key, day, msg string, summary map[string]any) {
	var done bool
	if err := a.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM gateway_job_runs WHERE job = $1 AND run_on = $2::date)`, key, day).Scan(&done); err != nil || done {
		return
	}
	if a.Core == nil || !a.Core.Configured() {
		return
	}
	if err := a.Core.CreateException(ctx, client.ExceptionEvent{
		SchoolID: schoolID, Kind: "reconciliation", Severity: "normal",
		CorrelationID: key, ErrorMessage: msg, PayloadSummary: summary,
		MaxAttempts: 1, IdempotencyKey: key + ":" + day,
	}); err != nil {
		return
	}
	_, _ = a.DB.ExecContext(ctx, `INSERT INTO gateway_job_runs (job, run_on) VALUES ($1, $2::date) ON CONFLICT DO NOTHING`, key, day)
}
