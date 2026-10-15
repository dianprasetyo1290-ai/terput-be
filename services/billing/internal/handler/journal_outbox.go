package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// Journal outbox sender (IF-ACC-01..03). Pending rows of
// gateway_journal_postings are posted to accounting POST
// /api/internal/journals with the unit's division code on every line; the
// returned trans_no and status are stored. A failure is retried with
// exponential backoff (model.JournalBackoff) up to
// gateway_settings.journal_max_attempts (default 5), then the row turns
// 'failed' and a journal_posting exception with a retry_url is raised in
// core's queue (menu 80). Accounting is idempotent on (source, reference),
// so a retry never double-posts.

// JournalPoster is the accounting client surface the sender needs.
type JournalPoster interface {
	PostJournal(ctx context.Context, j client.JournalRequest) (string, error)
}

const defaultJournalMaxAttempts = 5

type outboxRow struct {
	ID          string
	SchoolID    string
	Source      string
	Reference   string
	TransDate   string
	Description string
	Lines       []model.JournalLine
	Status      string
	Attempts    int
	TransNo     string
	LastError   string
	SourceRefs  map[string][]string
}

// JobJournalOutbox sends due outbox rows.
func (a *App) JobJournalOutbox(ctx context.Context, _ *sql.DB) error {
	return a.sendJournalOutbox(ctx, time.Now())
}

func (a *App) journalPoster() JournalPoster {
	if a.JournalPoster != nil {
		return a.JournalPoster
	}
	if a.Accounting.Configured() {
		return a.Accounting
	}
	return nil
}

func (a *App) sendJournalOutbox(ctx context.Context, now time.Time) error {
	if a.journalPoster() == nil {
		return nil // accounting not configured: rows wait, attempts are not burned
	}
	ids, err := collectIDs(ctx, a.DB,
		`SELECT id::text FROM gateway_journal_postings WHERE status = 'pending' AND next_attempt_at <= $1
		  ORDER BY next_attempt_at, created_at LIMIT 100`, now)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if _, _, err := a.postOutboxRow(ctx, id, now, false); err != nil {
			errs = append(errs, err)
		}
	}
	// Rows that failed while core was unreachable still need their exception.
	failed, err := collectIDs(ctx, a.DB,
		`SELECT id::text FROM gateway_journal_postings WHERE status = 'failed' AND exception_raised_at IS NULL LIMIT 100`)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, id := range failed {
		a.raisePostingException(ctx, id)
	}
	return errors.Join(errs...)
}

func loadOutboxRow(ctx context.Context, q queryer, id, lock string) (outboxRow, error) {
	var r outboxRow
	var lines, refs []byte
	var transNo, lastErr sql.NullString
	var day time.Time
	err := q.QueryRowContext(ctx,
		`SELECT id::text, school_id::text, source, reference, trans_date, description, lines, status, attempts, trans_no, last_error,
		        COALESCE(source_refs, '{}'::jsonb)
		   FROM gateway_journal_postings WHERE id::text = $1 `+lock, id).
		Scan(&r.ID, &r.SchoolID, &r.Source, &r.Reference, &day, &r.Description, &lines, &r.Status, &r.Attempts, &transNo, &lastErr, &refs)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(refs, &r.SourceRefs); err != nil {
		return r, err
	}
	r.TransDate, r.TransNo, r.LastError = day.Format("2006-01-02"), transNo.String, lastErr.String
	if r.Source == "gateway" {
		r.Source = sourceGateway
	}
	return r, json.Unmarshal(lines, &r.Lines)
}

// journalRetryable: a 4xx other than 408/409/429 (validation, unknown
// account) will not heal by itself; the row fails at once.
func journalRetryable(err error) bool {
	var he *client.HTTPError
	if errors.As(err, &he) && he.Status >= 400 && he.Status < 500 {
		return he.Status == http.StatusRequestTimeout || he.Status == http.StatusConflict || he.Status == http.StatusTooManyRequests
	}
	return true
}

func (a *App) journalMaxAttempts(ctx context.Context, q queryer, schoolID string) int {
	n := defaultJournalMaxAttempts
	_ = q.QueryRowContext(ctx, `SELECT journal_max_attempts FROM gateway_settings WHERE school_id = $1`, schoolID).Scan(&n)
	if n <= 0 {
		n = defaultJournalMaxAttempts
	}
	return n
}

func (a *App) journalRequest(ctx context.Context, r outboxRow) (client.JournalRequest, error) {
	set, err := a.loadJournalSettings(ctx, r.SchoolID)
	if err != nil {
		return client.JournalRequest{}, err
	}
	var div *string
	if set.UnitDivCode != "" {
		v := set.UnitDivCode
		div = &v
	}
	req := client.JournalRequest{SchoolID: r.SchoolID, Source: r.Source, Reference: r.Reference,
		TransDate: r.TransDate, Description: r.Description, SourceRefs: journalSourceRefs(r.SourceRefs)}
	for _, l := range r.Lines {
		req.Lines = append(req.Lines, client.JournalLine{AccCode: l.AccCode, DivCode: div,
			Description: l.Description, DebitIDR: l.DebitIDR, CreditIDR: l.CreditIDR})
	}
	return req, nil
}

// postOutboxRow posts one row. manual=true is the exception-queue / admin
// retry: it also takes failed rows and leaves them failed when the post
// fails again. It returns the row after the attempt and the post error.
func (a *App) postOutboxRow(ctx context.Context, id string, now time.Time, manual bool) (outboxRow, error, error) {
	poster := a.journalPoster()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return outboxRow{}, nil, err
	}
	defer tx.Rollback()
	// BUG-118: a manual retry never waits behind (and then repeats) a post
	// already in flight for the same row; it is told the row is busy.
	lock := "FOR UPDATE SKIP LOCKED"
	if manual {
		lock = "FOR UPDATE NOWAIT"
	}
	r, err := loadOutboxRow(ctx, tx, id, lock)
	if err == sql.ErrNoRows && !manual {
		return r, nil, nil // taken by a concurrent sender
	}
	if manual && isLockNotAvailable(err) {
		return r, nil, errJournalBusy
	}
	if err != nil {
		return r, nil, err
	}
	if r.Status == "posted" || (!manual && r.Status != "pending") {
		return r, nil, nil
	}
	if poster == nil {
		return r, errors.New("layanan akuntansi belum dikonfigurasi"), nil
	}
	req, err := a.journalRequest(ctx, r)
	if err != nil {
		return r, nil, err
	}
	transNo, postErr := poster.PostJournal(ctx, req)
	r.Attempts++
	if postErr == nil {
		r.Status, r.TransNo, r.LastError = "posted", transNo, ""
		if _, err := tx.ExecContext(ctx,
			`UPDATE gateway_journal_postings SET status = 'posted', trans_no = $2, attempts = $3, last_error = NULL,
			        posted_at = $4, updated_at = now() WHERE id::text = $1`, id, transNo, r.Attempts, now); err != nil {
			return r, nil, err
		}
		return r, nil, tx.Commit()
	}
	r.LastError = postErr.Error()
	maxAttempts := a.journalMaxAttempts(ctx, tx, r.SchoolID)
	raise := false
	switch {
	case manual && r.Status == "failed":
	case !journalRetryable(postErr) || r.Attempts >= maxAttempts:
		r.Status, raise = "failed", true
	}
	next := now.Add(model.JournalBackoff(r.Attempts))
	if _, err := tx.ExecContext(ctx,
		`UPDATE gateway_journal_postings SET status = $2, attempts = $3, last_error = $4, next_attempt_at = $5, updated_at = now()
		  WHERE id::text = $1`, id, r.Status, r.Attempts, truncate(r.LastError, 2000), next); err != nil {
		return r, postErr, err
	}
	if err := tx.Commit(); err != nil {
		return r, postErr, err
	}
	if raise {
		a.raisePostingException(ctx, id)
	}
	return r, postErr, nil
}

// errJournalBusy: another retry or the sender is posting this row now.
var errJournalBusy = errors.New("posting jurnal ini sedang diproses, coba lagi sebentar")

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// raisePostingException reports a failed posting to core's exception queue
// with a retry_url that re-posts the same reference (IF-ACC-03).
func (a *App) raisePostingException(ctx context.Context, id string) {
	if a.Core == nil {
		return
	}
	r, err := loadOutboxRow(ctx, a.DB, id, "")
	if err != nil || r.Status != "failed" {
		return
	}
	var total int64
	for _, l := range r.Lines {
		total += l.DebitIDR
	}
	ev := client.ExceptionEvent{
		// BUG-118: the last attempt failed, so the accountant must act: critical.
		SchoolID: r.SchoolID, Kind: "journal_posting", Severity: "critical",
		CorrelationID: r.Reference, ErrorMessage: "posting jurnal gagal: " + r.LastError,
		PayloadSummary: map[string]any{"reference": r.Reference, "source": r.Source, "trans_date": r.TransDate,
			"description": r.Description, "total_idr": total, "attempts": r.Attempts},
		MaxAttempts:    a.journalMaxAttempts(ctx, a.DB, r.SchoolID),
		IdempotencyKey: "journal-post:" + r.Reference,
	}
	if base := a.Config.Gateway.BillingInternalURL; base != "" {
		ev.RetryURL = base + "/internal/journal-postings/" + r.ID + "/retry"
	}
	if err := a.Core.CreateException(ctx, ev); err != nil {
		return
	}
	_, _ = a.DB.ExecContext(ctx, `UPDATE gateway_journal_postings SET exception_raised_at = now() WHERE id::text = $1`, id)
}

// InternalRetryJournalPosting is POST /internal/journal-postings/{id}/retry,
// the retry_url core calls from the exception queue. 200 only when the
// journal is posted, so core resolves the item; 502 otherwise.
func (a *App) InternalRetryJournalPosting(w http.ResponseWriter, r *http.Request) {
	a.retryJournalPosting(w, r, "")
}

func (a *App) retryJournalPosting(w http.ResponseWriter, r *http.Request, schoolID string) {
	id := param(r, "id")
	if schoolID != "" {
		var ok bool
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM gateway_journal_postings WHERE id::text = $1 AND school_id = $2)`, id, schoolID).Scan(&ok); err != nil || !ok {
			common.WriteError(w, http.StatusNotFound, "posting jurnal tidak ditemukan")
			return
		}
	}
	row, postErr, err := a.postOutboxRow(r.Context(), id, time.Now(), true)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "posting jurnal tidak ditemukan")
		return
	}
	if errors.Is(err, errJournalBusy) {
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if postErr != nil {
		common.WriteError(w, http.StatusBadGateway, "posting jurnal gagal: "+postErr.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": row.ID, "reference": row.Reference, "status": row.Status, "trans_no": row.TransNo}, nil)
}

// InternalRetryJournalQueue is POST /internal/journal-queue/retry?reference=,
// the retry_url of a journal that could not be built (missing mapping).
// 200 once the journal is in the outbox (or there is nothing to book).
func (a *App) InternalRetryJournalQueue(w http.ResponseWriter, r *http.Request) {
	reference := r.URL.Query().Get("reference")
	if reference == "" {
		common.WriteError(w, http.StatusBadRequest, "reference wajib diisi")
		return
	}
	problem, err := a.rebuildJournal(r.Context(), reference)
	if err != nil {
		writeErr(w, err)
		return
	}
	if problem != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, problem)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"reference": reference, "status": "queued"}, nil)
}
