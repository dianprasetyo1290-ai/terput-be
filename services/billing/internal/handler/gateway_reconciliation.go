package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/gateway"
	"skci/billing/internal/model"
)

// AdminRunReconciliation runs daily settlement reconciliation against successful transactions (AC-31-01).
func (a *App) AdminRunReconciliation(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	var req struct {
		BatchID  string `json:"batch_id"`
		Date     string `json:"date"`
		Provider string `json:"provider"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	dateStr := r.URL.Query().Get("date")
	if req.Date != "" {
		dateStr = req.Date
	}
	if req.BatchID != "" {
		var settledOn time.Time
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT settled_on FROM settlement_batches WHERE id = $1 AND school_id = $2`, req.BatchID, schoolID).Scan(&settledOn); err == nil {
			dateStr = settledOn.Format("2006-01-02")
		}
	}
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}

	provider := r.URL.Query().Get("provider")
	if req.Provider != "" {
		provider = req.Provider
	}

	out, err := a.runReconciliation(r.Context(), schoolID, dateStr, provider, "user", claims.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

type reconProviderResult struct {
	Provider       string                     `json:"provider"`
	MatchedCount   int                        `json:"matched_count"`
	ExceptionCount int                        `json:"exception_count"`
	Exceptions     []model.ReconcileException `json:"exceptions"`
}

type reconRunResult struct {
	Date           string                     `json:"date"`
	MatchedCount   int                        `json:"matched_count"`
	ExceptionCount int                        `json:"exception_count"`
	Exceptions     []model.ReconcileException `json:"exceptions"`
	Providers      []reconProviderResult      `json:"providers"`
	// Batches is the settlement batch status set by this run.
	Batches []reconBatchStatus `json:"batches"`
}

type reconBatchStatus struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Channel  string `json:"channel"`
	Status   string `json:"status"`
}

// runReconciliation matches one day's successful gateway transactions
// against the settlement items of that day, per provider, records the
// exceptions and marks each settlement batch of the day matched or
// mismatch (AC-31-01). Used by the admin button and the daily job.
func (a *App) runReconciliation(ctx context.Context, schoolID, dateStr, provider, actorType, actorID string) (reconRunResult, error) {
	out := reconRunResult{Date: dateStr, Exceptions: []model.ReconcileException{}, Providers: []reconProviderResult{}, Batches: []reconBatchStatus{}}
	// 1. Successful transactions paid on this date, tagged with provider.
	txRows, err := a.DB.QueryContext(ctx,
		`SELECT provider, tx_no, COALESCE(provider_ref, ''), total_idr, fee_idr
		 FROM gateway_transactions
		 WHERE school_id = $1 AND status = 'success' AND (DATE(paid_at) = $2 OR DATE(created_at) = $2)
		   AND ($3 = '' OR provider = $3)`,
		schoolID, dateStr, provider)
	if err != nil {
		return out, err
	}
	var successTxs []gateway.ProviderTx
	for txRows.Next() {
		var it gateway.ProviderTx
		if err := txRows.Scan(&it.Provider, &it.TXNo, &it.ProviderRef, &it.AmountIDR, &it.FeeIDR); err != nil {
			txRows.Close()
			return out, err
		}
		successTxs = append(successTxs, it)
	}
	txRows.Close()

	// 2. Settlement items of this date, tagged with their batch's provider.
	itemRows, err := a.DB.QueryContext(ctx,
		`SELECT b.provider, i.provider_ref, i.amount_idr, i.fee_idr, b.id::text
		 FROM settlement_items i
		 JOIN settlement_batches b ON b.id = i.batch_id
		 WHERE i.school_id = $1 AND b.settled_on = $2 AND ($3 = '' OR b.provider = $3)`,
		schoolID, dateStr, provider)
	if err != nil {
		return out, err
	}
	var settleItems []gateway.ProviderItem
	batchRefs := map[string][]string{}
	for itemRows.Next() {
		var it gateway.ProviderItem
		var batchID string
		if err := itemRows.Scan(&it.Provider, &it.ProviderRef, &it.AmountIDR, &it.FeeIDR, &batchID); err != nil {
			itemRows.Close()
			return out, err
		}
		settleItems = append(settleItems, it)
		batchRefs[batchID] = append(batchRefs[batchID], it.ProviderRef)
	}
	itemRows.Close()

	// 3. Reconcile per provider (FR-150-004, AC-31-01): a transaction of
	// provider A is never matched against provider B's settlement.
	matchedRefs := map[string]bool{}
	providerExceptions := map[string]int{}
	for _, g := range gateway.GroupForReconcile(successTxs, settleItems) {
		res := model.Reconcile(g.Txs, g.Items)
		out.Providers = append(out.Providers, reconProviderResult{g.Provider, len(res.Matched), len(res.Exceptions), res.Exceptions})
		out.MatchedCount += len(res.Matched)
		out.ExceptionCount += len(res.Exceptions)
		out.Exceptions = append(out.Exceptions, res.Exceptions...)
		providerExceptions[g.Provider] += len(res.Exceptions)
		for _, m := range res.Matched {
			matchedRefs[g.Provider+"\x00"+m.ProviderRef] = true
		}
		// 4. Save exceptions per provider; a re-run refreshes the detail.
		for _, ex := range res.Exceptions {
			if _, err := a.DB.ExecContext(ctx,
				`INSERT INTO reconciliation_exceptions (school_id, provider, kind, ref, detail, status, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, 'open', now(), now())
				 ON CONFLICT (school_id, provider, kind, ref) DO UPDATE SET detail = EXCLUDED.detail, updated_at = now()`,
				schoolID, g.Provider, string(ex.Kind), ex.Ref, jsonRaw(ex.Detail)); err != nil {
				return out, err
			}
		}
	}

	// 5. Settlement batch status of the day: matched when every item of the
	// batch matched and its provider has no open difference that day.
	bRows, err := a.DB.QueryContext(ctx,
		`SELECT id::text, provider, channel, status FROM settlement_batches
		  WHERE school_id = $1 AND settled_on = $2 AND ($3 = '' OR provider = $3) ORDER BY provider, channel`,
		schoolID, dateStr, provider)
	if err != nil {
		return out, err
	}
	type batch struct{ id, provider, channel, status string }
	var batches []batch
	for bRows.Next() {
		var b batch
		if err := bRows.Scan(&b.id, &b.provider, &b.channel, &b.status); err != nil {
			bRows.Close()
			return out, err
		}
		batches = append(batches, b)
	}
	bRows.Close()
	for _, b := range batches {
		matched := make([]bool, len(batchRefs[b.id]))
		for i, ref := range batchRefs[b.id] {
			matched[i] = matchedRefs[b.provider+"\x00"+ref]
		}
		next := model.SettlementBatchStatus(matched, providerExceptions[b.provider])
		out.Batches = append(out.Batches, reconBatchStatus{b.id, b.provider, b.channel, next})
		if next == b.status {
			continue
		}
		if _, err := a.DB.ExecContext(ctx,
			`UPDATE settlement_batches SET status = $1, updated_at = now() WHERE id = $2::uuid`, next, b.id); err != nil {
			return out, err
		}
		a.audit(ctx, schoolID, "settlement_batch", b.id, "settlement_batch_reconciled", actorType, actorID, "", "",
			map[string]any{"status": b.status}, map[string]any{"status": next, "date": dateStr})
	}
	return out, nil
}

// AdminImportBankStatement imports a bank mutation file (AC-31-02). The
// file is validated as a whole: any unreadable row rejects it with 400 and
// the supported format. Re-importing the same file is idempotent through
// line_hash (UNIQUE per school).
func (a *App) AdminImportBankStatement(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinanceOrAccounting(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file tidak terbaca")
		return
	}
	rows, err := model.ParseBankStatementCSV(data, r.FormValue("account"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	importID := randomKey()
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback()

	inserted, duplicates := 0, 0
	for _, row := range rows {
		res, err := tx.ExecContext(r.Context(),
			`INSERT INTO bank_statement_lines (school_id, account, posted_on, amount_idr, description, line_hash, import_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (school_id, line_hash) DO NOTHING`,
			schoolID, row.Account, row.PostedOn, row.AmountIDR, row.Description, row.Hash, importID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if n, _ := res.RowsAffected(); n == 1 {
			inserted++
		} else {
			duplicates++
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "bank_statement_import", importID, "bank_statement_imported", claims.ActiveRole, claims.UserID, "", "",
		nil, map[string]any{"rows": len(rows), "inserted": inserted, "duplicates": duplicates})

	common.WriteSuccess(w, map[string]any{
		"import_id":  importID,
		"rows":       len(rows),
		"inserted":   inserted,
		"duplicates": duplicates,
	}, nil)
}

// AdminRunBankReconciliation matches unmatched bank lines against the money
// SEKOCI expects in the bank (AC-31-02): verified transfer proofs, cashier
// transfers and gateway settlement batches. A matched line records which
// record it matched (matched_ref) so the record is never matched twice;
// the rest become open bank_unmatched exceptions for follow-up.
func (a *App) AdminRunBankReconciliation(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinanceOrAccounting(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}
	res, err := a.runBankReconciliation(r.Context(), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "bank_reconciliation", schoolID, "bank_reconciliation_run", claims.ActiveRole, claims.UserID, "", "",
		nil, map[string]any{"matched": len(res.Matches), "unmatched": len(res.Exceptions)})
	common.WriteSuccess(w, map[string]any{
		"matched_count": len(res.Matches),
		"unmatched":     len(res.Exceptions),
		"matches":       res.Matches,
	}, nil)
}

func (a *App) runBankReconciliation(ctx context.Context, schoolID string) (model.BankMatchResult, error) {
	var res model.BankMatchResult
	tolerance := 2
	if err := a.DB.QueryRowContext(ctx,
		`SELECT bank_match_tolerance_days FROM gateway_settings WHERE school_id = $1`, schoolID).Scan(&tolerance); err != nil && err != sql.ErrNoRows {
		return res, err
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT id::text, account, posted_on, amount_idr, description
		   FROM bank_statement_lines
		  WHERE school_id = $1 AND matched_ref IS NULL
		  ORDER BY posted_on, id
		  FOR UPDATE`, schoolID)
	if err != nil {
		return res, err
	}
	var lines []model.BankLine
	for rows.Next() {
		var l model.BankLine
		if err := rows.Scan(&l.ID, &l.Account, &l.PostedOn, &l.AmountIDR, &l.Description); err != nil {
			rows.Close()
			return res, err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	// Expected bank credits, keyed "<kind>:<id>"; keys already recorded as
	// a line's matched_ref are excluded.
	recRows, err := tx.QueryContext(ctx,
		`WITH expected AS (
		   SELECT 'bill_payment:' || p.id::text AS key, COALESCE(p.transfer_at, p.verified_at) AS at, p.amount_idr, COALESCE(p.reference, '') AS ref
		     FROM bill_payments p
		    WHERE p.school_id = $1 AND p.source = 'upload' AND p.status = 'verified'
		   UNION ALL
		   SELECT 'cashier:' || c.id::text, c.created_at, c.amount_idr, COALESCE(rc.receipt_no, '')
		     FROM cashier_payments c
		     LEFT JOIN receipts rc ON rc.school_id = c.school_id AND rc.source = 'cashier' AND rc.source_id = c.id
		    WHERE c.school_id = $1 AND c.method = 'transfer' AND c.status = 'recorded'
		   UNION ALL
		   SELECT 'settlement:' || s.id::text, s.settled_on::timestamptz, s.net_idr, ''
		     FROM settlement_batches s
		    WHERE s.school_id = $1 AND s.net_idr > 0
		 )
		 SELECT e.key, e.at, e.amount_idr, e.ref FROM expected e
		  WHERE NOT EXISTS (SELECT 1 FROM bank_statement_lines l WHERE l.school_id = $1 AND l.matched_ref = e.key)`, schoolID)
	if err != nil {
		return res, err
	}
	var records []model.BankPaymentRecord
	for recRows.Next() {
		var rec model.BankPaymentRecord
		if err := recRows.Scan(&rec.ID, &rec.Date, &rec.AmountIDR, &rec.Ref); err != nil {
			recRows.Close()
			return res, err
		}
		records = append(records, rec)
	}
	recRows.Close()
	if err := recRows.Err(); err != nil {
		return res, err
	}

	res = model.BankReconcile(lines, records, tolerance)
	for _, m := range res.Matches {
		if _, err := tx.ExecContext(ctx,
			`UPDATE bank_statement_lines SET matched_ref = $1 WHERE id = $2::uuid AND school_id = $3`,
			m.RecordID, m.LineID, schoolID); err != nil {
			return res, err
		}
		// A line that had been flagged earlier and now matches closes its exception.
		if _, err := tx.ExecContext(ctx,
			`UPDATE reconciliation_exceptions
			    SET status = 'resolved', resolution_note = 'cocok otomatis dengan ' || $1::text, resolved_by = 'system', resolved_at = now(), updated_at = now()
			  WHERE school_id = $2 AND provider = 'bank' AND kind = 'bank_unmatched' AND ref = $3 AND status = 'open'`,
			m.RecordID, schoolID, m.LineID); err != nil {
			return res, err
		}
	}
	for _, ex := range res.Exceptions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO reconciliation_exceptions (school_id, provider, kind, ref, detail, status, created_at, updated_at)
			 VALUES ($1, 'bank', $2, $3, $4, 'open', now(), now())
			 ON CONFLICT (school_id, provider, kind, ref) DO UPDATE SET detail = EXCLUDED.detail, updated_at = now()
			  WHERE reconciliation_exceptions.status = 'open'`,
			schoolID, string(ex.Kind), ex.Ref, jsonRaw(ex.Detail)); err != nil {
			return res, err
		}
	}
	return res, tx.Commit()
}
