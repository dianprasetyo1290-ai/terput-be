package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"skci/billing/internal/canteen"
	"skci/billing/internal/client"
	"skci/billing/internal/model"
	"skci/billing/internal/wallet"
)

// Accounting auto-journals (menu 34, FR-34-001..003, BR-ACC-05/06). Every
// journal is a daily summary per unit (school) with lines per component,
// queued once in gateway_journal_postings under a deterministic reference
// (the idempotency key accounting honors on (source, reference)) and sent
// by JobJournalOutbox. Only closed days (before today, local time) within
// the lookback window are journaled. A missing account mapping never
// guesses an account: it raises a journal_posting exception (once per day)
// whose retry_url rebuilds the journal.

const (
	sourceBilling = "billing"
	sourceGateway = "payment_gateway"
	sourceWallet  = "wallet"
	sourceCanteen = "canteen"
)

type journalDraft struct {
	lines       []model.JournalLine
	description string
	refs        map[string][]string // source table -> detail ids (BR-ACC-06 drilldown)
}

type dailyKind struct {
	kind   string
	prefix string // reference = prefix:<school>:<day>
	source string
	days   string // DISTINCT school_id, day; $1 from, $2 to, $3 tz
	build  func(a *App, ctx context.Context, schoolID string, start, end time.Time) (*journalDraft, error)
}

var dailyKinds = []dailyKind{
	{
		kind: "bill_issued", prefix: "bil:issue", source: sourceBilling,
		days: `SELECT DISTINCT bi.school_id::text, to_char(bi.created_at AT TIME ZONE $3, 'YYYY-MM-DD')
		         FROM bill_items bi JOIN student_bills b ON b.id = bi.bill_id
		        WHERE ` + journalBillLive + ` AND bi.created_at >= $1 AND bi.created_at < $2`,
		build: (*App).buildBillIssued,
	},
	{
		kind: "discount", prefix: "bil:disc", source: sourceBilling,
		days: `SELECT DISTINCT st.school_id::text, to_char(st.created_at AT TIME ZONE $3, 'YYYY-MM-DD')
		         FROM bill_discount_steps st JOIN student_bills b ON b.id = st.bill_id
		        WHERE ` + journalBillLive + ` AND st.applied_idr > 0 AND st.created_at >= $1 AND st.created_at < $2`,
		build: (*App).buildDiscount,
	},
	{
		kind: "receipt", prefix: "bil:rcpt", source: sourceBilling,
		days: `SELECT DISTINCT school_id::text, to_char(created_at AT TIME ZONE $3, 'YYYY-MM-DD') FROM cashier_payments
		        WHERE status IN ('recorded','corrected') AND created_at >= $1 AND created_at < $2
		       UNION
		       SELECT DISTINCT school_id::text, to_char(verified_at AT TIME ZONE $3, 'YYYY-MM-DD') FROM bill_payments
		        WHERE source IN ('upload','deposit') AND status = 'verified' AND verified_at >= $1 AND verified_at < $2`,
		build: (*App).buildReceipt,
	},
	{
		kind: "refund", prefix: "bil:rfnd", source: sourceBilling,
		days: `SELECT DISTINCT school_id::text, to_char(processed_at AT TIME ZONE $3, 'YYYY-MM-DD') FROM bill_refunds
		        WHERE status = 'processed' AND wallet_id IS NULL
		          AND processed_at >= $1 AND processed_at < $2`,
		build: (*App).buildRefund,
	},
	{
		kind: "wallet", prefix: "wal", source: sourceWallet,
		days: `SELECT DISTINCT school_id::text, to_char(created_at AT TIME ZONE $3, 'YYYY-MM-DD') FROM wallet_movements
		        WHERE created_at >= $1 AND created_at < $2`,
		build: (*App).buildWallet,
	},
}

// JobBillingJournals queues the daily billing, wallet and canteen journals.
func (a *App) JobBillingJournals(ctx context.Context, _ *sql.DB) error {
	return a.runBillingJournals(ctx, time.Now())
}

func (a *App) journalWindow(now time.Time) (from, today time.Time) {
	loc := a.location()
	local := now.In(loc)
	today = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	lookback := a.Config.Gateway.JournalLookbackDays
	if lookback <= 0 {
		lookback = 1
	}
	return today.AddDate(0, 0, -lookback), today
}

func (a *App) runBillingJournals(ctx context.Context, now time.Time) error {
	from, today := a.journalWindow(now)
	loc := a.location()
	var errs []error
	for _, k := range dailyKinds {
		days, err := a.journalDays(ctx, k.days, from, today, loc)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", k.kind, err))
			continue
		}
		for _, sd := range days {
			if _, err := a.queueDaily(ctx, k, sd[0], sd[1], now); err != nil {
				errs = append(errs, fmt.Errorf("%s %s %s: %w", k.kind, sd[0], sd[1], err))
			}
		}
	}
	if err := a.queueCanteenSettlements(ctx, now); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (a *App) journalDays(ctx context.Context, q string, from, to time.Time, loc *time.Location) ([][2]string, error) {
	rows, err := a.DB.QueryContext(ctx, q, from, to, loc.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var sd [2]string
		if err := rows.Scan(&sd[0], &sd[1]); err != nil {
			return nil, err
		}
		out = append(out, sd)
	}
	return out, rows.Err()
}

func (a *App) journalQueued(ctx context.Context, reference string) (bool, error) {
	var ok bool
	err := a.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM gateway_journal_postings WHERE reference = $1)`, reference).Scan(&ok)
	return ok, err
}

// queueDaily builds and queues one (kind, school, day) journal. It returns
// true when the journal is (now) in the outbox or there is nothing to book;
// false with a nil error when a mapping is missing (exception raised).
func (a *App) queueDaily(ctx context.Context, k dailyKind, schoolID, day string, now time.Time) (bool, error) {
	reference := fmt.Sprintf("%s:%s:%s", k.prefix, schoolID, day)
	if done, err := a.journalQueued(ctx, reference); err != nil || done {
		return done, err
	}
	start, err := time.ParseInLocation("2006-01-02", day, a.location())
	if err != nil {
		return false, err
	}
	d, err := k.build(a, ctx, schoolID, start, start.AddDate(0, 0, 1))
	if err != nil {
		if !isInfraErr(err) {
			a.raiseQueueIssue(ctx, schoolID, reference, err.Error(), now)
			return false, nil
		}
		return false, err
	}
	if d == nil || len(d.lines) == 0 {
		return true, nil
	}
	return true, a.insertOutbox(ctx, schoolID, k.kind, k.source, reference, day, d)
}

// buildError marks a journal that cannot be built from the data as it is
// (unbalanced, unknown movement, ...): a person has to look at it. Any other
// error is infrastructure (database, context) and is retried next run.
type buildError struct{ error }

func buildErr(err error) error {
	if err == nil {
		return nil
	}
	return &buildError{err}
}

func isInfraErr(err error) bool {
	var b *buildError
	var miss *model.MissingAccountsError
	return !errors.As(err, &b) && !errors.As(err, &miss)
}

func (a *App) insertOutbox(ctx context.Context, schoolID, kind, source, reference, day string, d *journalDraft) error {
	if d.refs == nil {
		d.refs = map[string][]string{}
	}
	_, err := a.DB.ExecContext(ctx,
		`INSERT INTO gateway_journal_postings (school_id, reference, trans_date, description, lines, kind, source, source_refs)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (reference) DO NOTHING`,
		schoolID, reference, day, d.description, jsonRaw(d.lines), kind, source, jsonRaw(d.refs))
	return err
}

// raiseQueueIssue reports a journal that cannot be built, at most once per
// reference per day (core counts repeats of one idempotency key as attempts).
func (a *App) raiseQueueIssue(ctx context.Context, schoolID, reference, msg string, now time.Time) {
	if a.Core == nil {
		return
	}
	day := now.In(a.location()).Format("2006-01-02")
	marker := "journal_issue:" + reference
	var done bool
	if err := a.DB.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM gateway_job_runs WHERE job = $1 AND run_on = $2)`, marker, day).Scan(&done); err != nil || done {
		return
	}
	ev := client.ExceptionEvent{
		SchoolID: schoolID, Kind: "journal_posting", Severity: "normal",
		CorrelationID: reference, ErrorMessage: msg,
		PayloadSummary: map[string]any{"reference": reference, "stage": "queue"},
		MaxAttempts:    1, IdempotencyKey: "journal-queue:" + reference,
	}
	if base := a.Config.Gateway.BillingInternalURL; base != "" {
		ev.RetryURL = base + "/internal/journal-queue/retry?reference=" + url.QueryEscape(reference)
	}
	if err := a.Core.CreateException(ctx, ev); err != nil {
		return
	}
	_, _ = a.DB.ExecContext(ctx, `INSERT INTO gateway_job_runs (job, run_on) VALUES ($1, $2) ON CONFLICT DO NOTHING`, marker, day)
}

// rebuildJournal re-runs the builder behind a reference (exception retry).
// It returns the build problem, if any, as a non-nil error message.
func (a *App) rebuildJournal(ctx context.Context, reference string) (string, error) {
	parts := strings.Split(reference, ":")
	loc := a.location()
	switch {
	case len(parts) == 3 && parts[0] == "pgw":
		if err := a.queueGatewayJournal(ctx, parts[1], parts[2], loc); err != nil {
			return "", err
		}
	case len(parts) == 3 && parts[0] == "cst":
		if _, err := a.queueCanteenSettlement(ctx, parts[1], parts[2], time.Now()); err != nil {
			return "", err
		}
	default:
		var kind *dailyKind
		for i := range dailyKinds {
			k := &dailyKinds[i]
			if strings.HasPrefix(reference, k.prefix+":") && len(strings.Split(strings.TrimPrefix(reference, k.prefix+":"), ":")) == 2 {
				kind = k
			}
		}
		if kind == nil {
			return "referensi jurnal tidak dikenal", nil
		}
		rest := strings.Split(strings.TrimPrefix(reference, kind.prefix+":"), ":")
		start, err := time.ParseInLocation("2006-01-02", rest[1], loc)
		if err != nil {
			return "tanggal referensi tidak valid", nil
		}
		d, err := kind.build(a, ctx, rest[0], start, start.AddDate(0, 0, 1))
		if err != nil {
			if isInfraErr(err) {
				return "", err
			}
			return err.Error(), nil
		}
		if d != nil && len(d.lines) > 0 {
			if err := a.insertOutbox(ctx, rest[0], kind.kind, kind.source, reference, rest[1], d); err != nil {
				return "", err
			}
		}
		return "", nil
	}
	if ok, err := a.journalQueued(ctx, reference); err != nil || ok {
		return "", err
	}
	return "jurnal belum dapat dibentuk, periksa pemetaan akun", nil
}

// ---- mappings ------------------------------------------------------------

func (a *App) loadJournalSettings(ctx context.Context, schoolID string) (model.JournalSettings, error) {
	var s model.JournalSettings
	var div, recv, cash, bank, disc, schol, ref, dep sql.NullString
	err := a.DB.QueryRowContext(ctx,
		`SELECT unit_div_code, receivable_acc, cash_acc, bank_acc, discount_acc, scholarship_acc, refund_acc, deposit_acc
		   FROM billing_journal_settings WHERE school_id = $1`, schoolID).
		Scan(&div, &recv, &cash, &bank, &disc, &schol, &ref, &dep)
	if err == sql.ErrNoRows {
		return s, nil
	}
	s = model.JournalSettings{UnitDivCode: div.String, ReceivableAcc: recv.String, CashAcc: cash.String, BankAcc: bank.String,
		DiscountAcc: disc.String, ScholarshipAcc: schol.String, RefundAcc: ref.String, DepositAcc: dep.String}
	return s, err
}

func (a *App) loadComponentAccounts(ctx context.Context, schoolID string) (map[string]model.ComponentAccount, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT component_code, name, revenue_acc, COALESCE(receivable_acc, '') FROM fee_component_accounts WHERE school_id = $1`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]model.ComponentAccount{}
	for rows.Next() {
		var c model.ComponentAccount
		if err := rows.Scan(&c.Code, &c.Name, &c.RevenueAcc, &c.ReceivableAcc); err != nil {
			return nil, err
		}
		out[c.Code] = c
	}
	return out, rows.Err()
}

func collectIDs(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// componentKey: scheme items carry component_code; ad-hoc bills are keyed
// by their source module (library, transport, ...).
// journalBillLive keeps a bill in the daily journals: live bills, and bills
// cancelled through an approved change request (BUG-053), whose original
// lines were journaled and whose cancellation lines reverse them.
const journalBillLive = `(b.status <> 'void' OR EXISTS (SELECT 1 FROM bill_adjustments xa WHERE xa.bill_id = b.id AND xa.kind = 'cancel'))`

const componentKey = `COALESCE(NULLIF(bi.component_code, ''), b.source_module, '')`

// ---- builders ------------------------------------------------------------

func (a *App) buildBillIssued(ctx context.Context, schoolID string, start, end time.Time) (*journalDraft, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT `+componentKey+`, min(bi.name), sum(bi.amount_idr)
		   FROM bill_items bi JOIN student_bills b ON b.id = bi.bill_id
		  WHERE bi.school_id = $1 AND `+journalBillLive+` AND bi.created_at >= $2 AND bi.created_at < $3
		  GROUP BY 1 ORDER BY 1`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	var items []model.ComponentAmount
	for rows.Next() {
		var c model.ComponentAmount
		if err := rows.Scan(&c.Component, &c.Name, &c.AmountIDR); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, c)
	}
	rows.Close()
	var deposit int64
	if err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(ad.deposit_idr), 0) FROM bill_adjustments ad JOIN student_bills b ON b.id = ad.bill_id
		  WHERE ad.school_id = $1 AND `+journalBillLive+` AND ad.created_at >= $2 AND ad.created_at < $3`,
		schoolID, start, end).Scan(&deposit); err != nil {
		return nil, err
	}
	set, err := a.loadJournalSettings(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	comps, err := a.loadComponentAccounts(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	lines, err := model.BillIssuedJournal(set, comps, items, deposit)
	if err != nil || lines == nil {
		return nil, buildErr(err)
	}
	ids, err := collectIDs(ctx, a.DB,
		`SELECT DISTINCT bi.bill_id::text FROM bill_items bi JOIN student_bills b ON b.id = bi.bill_id
		  WHERE bi.school_id = $1 AND `+journalBillLive+` AND bi.created_at >= $2 AND bi.created_at < $3 ORDER BY 1`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	return &journalDraft{lines: lines, description: "Tagihan terbit " + start.Format("2006-01-02"),
		refs: map[string][]string{"student_bills": ids}}, nil
}

func (a *App) buildDiscount(ctx context.Context, schoolID string, start, end time.Time) (*journalDraft, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT COALESCE(d.scholarship, st.source = 'scholarship'), st.label, sum(st.applied_idr)
		   FROM bill_discount_steps st JOIN student_bills b ON b.id = st.bill_id
		   LEFT JOIN bill_discounts d ON d.id = st.discount_id
		  WHERE st.school_id = $1 AND `+journalBillLive+` AND st.applied_idr > 0 AND st.created_at >= $2 AND st.created_at < $3
		  GROUP BY 1, 2 ORDER BY 1, 2`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	var disc, schol []model.LabelAmount
	for rows.Next() {
		var isSchol bool
		var la model.LabelAmount
		if err := rows.Scan(&isSchol, &la.Label, &la.AmountIDR); err != nil {
			rows.Close()
			return nil, err
		}
		if isSchol {
			schol = append(schol, la)
		} else {
			disc = append(disc, la)
		}
	}
	rows.Close()
	set, err := a.loadJournalSettings(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	lines, err := model.DiscountJournal(set, disc, schol)
	if err != nil || lines == nil {
		return nil, buildErr(err)
	}
	ids, err := collectIDs(ctx, a.DB,
		`SELECT DISTINCT st.bill_id::text FROM bill_discount_steps st JOIN student_bills b ON b.id = st.bill_id
		  WHERE st.school_id = $1 AND `+journalBillLive+` AND st.applied_idr > 0 AND st.created_at >= $2 AND st.created_at < $3 ORDER BY 1`,
		schoolID, start, end)
	if err != nil {
		return nil, err
	}
	return &journalDraft{lines: lines, description: "Potongan & beasiswa " + start.Format("2006-01-02"),
		refs: map[string][]string{"student_bills": ids}}, nil
}

func (a *App) buildReceipt(ctx context.Context, schoolID string, start, end time.Time) (*journalDraft, error) {
	var d model.ReceiptDay
	if err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(amount_idr) FILTER (WHERE method = 'cash'), 0),
		        COALESCE(sum(amount_idr) FILTER (WHERE method = 'transfer'), 0),
		        COALESCE(sum(deposit_idr), 0)
		   FROM cashier_payments WHERE school_id = $1 AND status IN ('recorded','corrected') AND created_at >= $2 AND created_at < $3`,
		schoolID, start, end).Scan(&d.CashIDR, &d.BankIDR, &d.ToDepositIDR); err != nil {
		return nil, err
	}
	var upload int64
	if err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(amount_idr) FILTER (WHERE source = 'upload'), 0), COALESCE(sum(amount_idr) FILTER (WHERE source = 'deposit'), 0)
		   FROM bill_payments WHERE school_id = $1 AND source IN ('upload','deposit') AND status = 'verified' AND verified_at >= $2 AND verified_at < $3`,
		schoolID, start, end).Scan(&upload, &d.FromDepositIDR); err != nil {
		return nil, err
	}
	d.BankIDR += upload
	rows, err := a.DB.QueryContext(ctx,
		`SELECT `+componentKey+`, min(bi.name), sum(al.amount_idr)
		   FROM bill_payment_allocations al
		   JOIN bill_payments p ON p.id = al.payment_id
		   JOIN bill_items bi ON bi.id = al.bill_item_id
		   JOIN student_bills b ON b.id = bi.bill_id
		   LEFT JOIN cashier_payments cp ON p.source = 'cashier' AND cp.id = p.source_id
		  WHERE al.school_id = $1 AND (
		        (p.source = 'cashier' AND cp.status IN ('recorded','corrected') AND cp.created_at >= $2 AND cp.created_at < $3)
		     OR (p.source IN ('upload','deposit') AND p.status = 'verified' AND p.verified_at >= $2 AND p.verified_at < $3))
		  GROUP BY 1 ORDER BY 1`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c model.ComponentAmount
		if err := rows.Scan(&c.Component, &c.Name, &c.AmountIDR); err != nil {
			rows.Close()
			return nil, err
		}
		d.Allocated = append(d.Allocated, c)
	}
	rows.Close()
	set, err := a.loadJournalSettings(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	comps, err := a.loadComponentAccounts(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	lines, err := model.ReceiptJournal(set, comps, d)
	if err != nil || lines == nil {
		return nil, buildErr(err)
	}
	cashier, err := collectIDs(ctx, a.DB,
		`SELECT id::text FROM cashier_payments WHERE school_id = $1 AND status IN ('recorded','corrected') AND created_at >= $2 AND created_at < $3 ORDER BY 1`,
		schoolID, start, end)
	if err != nil {
		return nil, err
	}
	payments, err := collectIDs(ctx, a.DB,
		`SELECT id::text FROM bill_payments WHERE school_id = $1 AND source IN ('upload','deposit') AND status = 'verified' AND verified_at >= $2 AND verified_at < $3 ORDER BY 1`,
		schoolID, start, end)
	if err != nil {
		return nil, err
	}
	return &journalDraft{lines: lines, description: "Penerimaan tunai & transfer " + start.Format("2006-01-02"),
		refs: map[string][]string{"cashier_payments": cashier, "bill_payments": payments}}, nil
}

func (a *App) buildRefund(ctx context.Context, schoolID string, start, end time.Time) (*journalDraft, error) {
	// Bill refunds and student refunds (Menu 78, possibly split between
	// bill payments and the overpayment deposit). Wallet refunds stay
	// excluded, as before.
	const where = `school_id = $1 AND status = 'processed' AND wallet_id IS NULL AND processed_at >= $2 AND processed_at < $3`
	var total, deposit int64
	if err := a.DB.QueryRowContext(ctx, `SELECT COALESCE(sum(amount_idr), 0), COALESCE(sum(deposit_idr), 0) FROM bill_refunds WHERE `+where,
		schoolID, start, end).Scan(&total, &deposit); err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	set, err := a.loadJournalSettings(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	lines, err := model.RefundJournal(set, total-deposit, deposit)
	if err != nil {
		return nil, buildErr(err)
	}
	ids, err := collectIDs(ctx, a.DB, `SELECT id::text FROM bill_refunds WHERE `+where+` ORDER BY 1`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	return &journalDraft{lines: lines, description: "Refund tagihan " + start.Format("2006-01-02"),
		refs: map[string][]string{"bill_refunds": ids}}, nil
}

func (a *App) buildWallet(ctx context.Context, schoolID string, start, end time.Time) (*journalDraft, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT w.kind, m.kind, sum(m.amount_idr) FROM wallet_movements m JOIN wallets w ON w.id = m.wallet_id
		  WHERE m.school_id = $1 AND m.created_at >= $2 AND m.created_at < $3 GROUP BY 1, 2`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	agg := wallet.DailyAgg{ByKind: map[string]map[string]int64{}}
	for rows.Next() {
		var wk, mk string
		var amt int64
		if err := rows.Scan(&wk, &mk, &amt); err != nil {
			rows.Close()
			return nil, err
		}
		if agg.ByKind[wk] == nil {
			agg.ByKind[wk] = map[string]int64{}
		}
		agg.ByKind[wk][mk] = amt
	}
	rows.Close()
	if err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(fee_idr), 0) FROM gateway_transactions
		  WHERE school_id = $1 AND purpose = 'wallet_topup' AND fee_bearer = 'school' AND status IN ('success','corrected')
		    AND paid_at >= $2 AND paid_at < $3`, schoolID, start, end).Scan(&agg.GatewaySchoolFeeIDR); err != nil {
		return nil, err
	}
	acc, ok, err := loadWalletAccounts(ctx, a.DB, schoolID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &model.MissingAccountsError{Missing: []string{"akun dompet siswa (wallet_accounts)"}}
	}
	var gw model.GatewayAccounts
	err = a.DB.QueryRowContext(ctx, `SELECT clearing_acc, fee_expense_acc, receivable_acc, deposit_acc FROM gateway_accounts WHERE school_id = $1`, schoolID).
		Scan(&gw.Clearing, &gw.FeeExpense, &gw.Receivable, &gw.Deposit)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	lines, err := wallet.DailyJournal(agg, acc, gw)
	if err != nil || lines == nil {
		return nil, buildErr(err)
	}
	ids, err := collectIDs(ctx, a.DB,
		`SELECT id::text FROM wallet_movements WHERE school_id = $1 AND created_at >= $2 AND created_at < $3 ORDER BY 1`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	return &journalDraft{lines: lines, description: "Mutasi dompet siswa " + start.Format("2006-01-02"),
		refs: map[string][]string{"wallet_movements": ids}}, nil
}

// ---- canteen settlements -------------------------------------------------

// queueCanteenSettlements queues the school-share journal of approved (or
// paid) settlements and the payout journal of paid ones.
func (a *App) queueCanteenSettlements(ctx context.Context, now time.Time) error {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT s.id::text, p.part FROM canteen_settlements s
		  CROSS JOIN (VALUES ('share'), ('paid')) p(part)
		  WHERE s.status IN ('approved','paid')
		    AND ((p.part = 'share' AND s.school_share_idr > 0) OR (p.part = 'paid' AND s.status = 'paid' AND s.tenant_payout_idr > 0))
		    AND NOT EXISTS (SELECT 1 FROM gateway_journal_postings g WHERE g.reference = 'cst:' || s.id::text || ':' || p.part)`)
	if err != nil {
		return err
	}
	var todo [][2]string
	for rows.Next() {
		var r [2]string
		if err := rows.Scan(&r[0], &r[1]); err != nil {
			rows.Close()
			return err
		}
		todo = append(todo, r)
	}
	rows.Close()
	var errs []error
	for _, r := range todo {
		if _, err := a.queueCanteenSettlement(ctx, r[0], r[1], now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (a *App) queueCanteenSettlement(ctx context.Context, id, part string, now time.Time) (bool, error) {
	reference := "cst:" + id + ":" + part
	if done, err := a.journalQueued(ctx, reference); err != nil || done {
		return done, err
	}
	var schoolID, status, tenant string
	var s canteen.Settlement
	var periodFrom, periodTo time.Time
	var paidAt sql.NullTime
	err := a.DB.QueryRowContext(ctx,
		`SELECT s.school_id::text, s.status, t.name, s.scheme_kind, s.wallet_sales_idr, s.cash_sales_idr, s.school_share_idr, s.tenant_payout_idr,
		        s.period_from, s.period_to, s.paid_at
		   FROM canteen_settlements s JOIN canteen_tenants t ON t.id = s.tenant_id WHERE s.id::text = $1`, id).
		Scan(&schoolID, &status, &tenant, &s.SchemeKind, &s.WalletSalesIDR, &s.CashSalesIDR, &s.SchoolShareIDR, &s.TenantPayoutIDR,
			&periodFrom, &periodTo, &paidAt)
	if err != nil {
		return false, err
	}
	if status != "approved" && status != "paid" || part == "paid" && status != "paid" {
		return false, nil
	}
	acc, ok, err := loadWalletAccounts(ctx, a.DB, schoolID)
	if err != nil {
		return false, err
	}
	if !ok {
		a.raiseQueueIssue(ctx, schoolID, reference, "akun jurnal belum dipetakan: akun kantin (wallet_accounts)", now)
		return false, nil
	}
	share, paid, err := canteen.SettlementJournals(s, acc)
	if err != nil {
		a.raiseQueueIssue(ctx, schoolID, reference, err.Error(), now)
		return false, nil
	}
	period := periodFrom.Format("2006-01-02") + " s.d. " + periodTo.Format("2006-01-02")
	d := &journalDraft{refs: map[string][]string{"canteen_settlements": {id}}}
	day := periodTo.Format("2006-01-02")
	if part == "share" {
		d.lines, d.description = share, "Bagi hasil/sewa kantin "+tenant+" "+period
	} else {
		d.lines, d.description = paid, "Pembayaran hak tenant kantin "+tenant+" "+period
		if paidAt.Valid {
			day = paidAt.Time.In(a.location()).Format("2006-01-02")
		}
	}
	if len(d.lines) == 0 {
		return true, nil
	}
	return true, a.insertOutbox(ctx, schoolID, "canteen_"+part, sourceCanteen, reference, day, d)
}
