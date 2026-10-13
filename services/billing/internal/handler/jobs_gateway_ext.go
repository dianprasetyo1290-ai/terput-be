package handler

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"skci/billing/internal/model"
)

// Jobs of T14 §4.4, registered on internal/jobs.Runner in main.go
// (interval GATEWAY_JOB_INTERVAL_SECONDS, once-per-day markers in
// gateway_job_runs). Each has the Runner handler signature.

// JobAutodebitSchedule plans and runs due autodebit attempts.
func (a *App) JobAutodebitSchedule(ctx context.Context, _ *sql.DB) error {
	return a.runAutodebitSchedule(ctx, time.Now())
}

// JobRetailExpiry expires overdue retail pay codes.
func (a *App) JobRetailExpiry(ctx context.Context, _ *sql.DB) error {
	return a.runRetailExpiry(ctx, time.Now())
}

// JobProviderHealth resets a provider's consecutive failure counter once a
// charge succeeded there after its last failure (dashboard only; providers
// are never disabled automatically).
func (a *App) JobProviderHealth(ctx context.Context, _ *sql.DB) error {
	_, err := a.DB.ExecContext(ctx,
		`UPDATE payment_providers p SET consecutive_failures = 0, updated_at = now()
		  WHERE p.consecutive_failures > 0
		    AND EXISTS (SELECT 1 FROM gateway_transactions t
		                 WHERE t.provider = p.code AND t.created_at > COALESCE(p.last_failure_at, '-infinity'::timestamptz))`)
	return err
}

// JobGatewayJournal queues the daily gateway journal per school
// (AC-IF-03). It replaces T6's per-transaction posting from the webhook,
// which reused the daily reference so accounting dropped every transaction
// after the first one of the day. One outbox row per (school, day) with
// reference pgw:<school>:<date>, bills purpose only (wallet top-ups are in
// the wallet journal). JobJournalOutbox sends it.
// Without a gateway_accounts mapping no journal is guessed: an exception
// journal_posting is raised and the day is retried on later runs.
func (a *App) JobGatewayJournal(ctx context.Context, _ *sql.DB) error {
	return a.runGatewayJournal(ctx, time.Now())
}

func (a *App) runGatewayJournal(ctx context.Context, now time.Time) error {
	loc := a.location()
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	lookback := a.Config.Gateway.JournalLookbackDays
	if lookback <= 0 {
		lookback = 1
	}
	from := today.AddDate(0, 0, -lookback)
	rows, err := a.DB.QueryContext(ctx,
		`SELECT DISTINCT school_id::text, to_char(paid_at AT TIME ZONE $3, 'YYYY-MM-DD')
		   FROM gateway_transactions
		  WHERE purpose = 'bills' AND status IN ('success','corrected') AND paid_at >= $1 AND paid_at < $2`,
		from, today, loc.String())
	if err != nil {
		return err
	}
	type schoolDay struct{ school, day string }
	var days []schoolDay
	for rows.Next() {
		var sd schoolDay
		if err := rows.Scan(&sd.school, &sd.day); err != nil {
			rows.Close()
			return err
		}
		days = append(days, sd)
	}
	rows.Close()
	for _, sd := range days {
		if err := a.queueGatewayJournal(ctx, sd.school, sd.day, loc); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) queueGatewayJournal(ctx context.Context, schoolID, day string, loc *time.Location) error {
	job := "gateway_journal:" + schoolID
	var done bool
	if err := a.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM gateway_job_runs WHERE job = $1 AND run_on = $2)`, job, day).Scan(&done); err != nil || done {
		return err
	}
	reference := fmt.Sprintf("pgw:%s:%s", schoolID, day)
	var acc model.GatewayAccounts
	err := a.DB.QueryRowContext(ctx,
		`SELECT clearing_acc, fee_expense_acc, receivable_acc, deposit_acc FROM gateway_accounts WHERE school_id = $1`, schoolID).
		Scan(&acc.Clearing, &acc.FeeExpense, &acc.Receivable, &acc.Deposit)
	if err == sql.ErrNoRows {
		a.journalException(ctx, schoolID, reference, "akun jurnal payment gateway belum dipetakan (gateway_accounts)")
		return nil
	}
	if err != nil {
		return err
	}
	start, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return err
	}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT bills_amount_idr, fee_idr, total_idr, deposit_idr, fee_bearer FROM gateway_transactions
		  WHERE school_id = $1 AND purpose = 'bills' AND status IN ('success','corrected') AND paid_at >= $2 AND paid_at < $3`,
		schoolID, start, start.AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	var txs []model.JournalTx
	for rows.Next() {
		var t model.JournalTx
		var bearer string
		if err := rows.Scan(&t.BillsIDR, &t.FeeIDR, &t.TotalIDR, &t.DepositIDR, &bearer); err != nil {
			rows.Close()
			return err
		}
		t.FeeBearer = model.FeeBearer(bearer)
		txs = append(txs, t)
	}
	rows.Close()
	lines, err := model.GatewayJournalLines(acc, txs)
	if err != nil {
		a.journalException(ctx, schoolID, reference, err.Error())
		return nil
	}
	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer dbTx.Rollback()
	ids, err := collectIDs(ctx, a.DB,
		`SELECT id::text FROM gateway_transactions
		  WHERE school_id = $1 AND purpose = 'bills' AND status IN ('success','corrected') AND paid_at >= $2 AND paid_at < $3 ORDER BY 1`,
		schoolID, start, start.AddDate(0, 0, 1))
	if err != nil {
		return err
	}
	if _, err := dbTx.ExecContext(ctx,
		`INSERT INTO gateway_journal_postings (school_id, reference, trans_date, description, lines, kind, source, source_refs)
		 VALUES ($1, $2, $3, $4, $5, 'gateway_receipt', $6, $7) ON CONFLICT (reference) DO NOTHING`,
		schoolID, reference, day, "Penerimaan payment gateway "+day, jsonRaw(lines), sourceGateway,
		jsonRaw(map[string][]string{"gateway_transactions": ids})); err != nil {
		return err
	}
	if _, err := dbTx.ExecContext(ctx,
		`INSERT INTO gateway_job_runs (job, run_on) VALUES ($1, $2) ON CONFLICT DO NOTHING`, job, day); err != nil {
		return err
	}
	return dbTx.Commit()
}

func (a *App) journalException(ctx context.Context, schoolID, reference, msg string) {
	a.raiseQueueIssue(ctx, schoolID, reference, msg, time.Now())
}
