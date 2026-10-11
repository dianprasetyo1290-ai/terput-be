package handler

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/common"
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
		BatchID string `json:"batch_id"`
		Date    string `json:"date"`
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

	// 1. Fetch successful transactions settled or paid on this date
	txRows, err := a.DB.QueryContext(r.Context(),
		`SELECT tx_no, COALESCE(provider_ref, ''), total_idr, fee_idr
		 FROM gateway_transactions
		 WHERE school_id = $1 AND status = 'success' AND (DATE(paid_at) = $2 OR DATE(created_at) = $2)`,
		schoolID, dateStr)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer txRows.Close()

	var successTxs []model.ReconcileTx
	for txRows.Next() {
		var it model.ReconcileTx
		if err := txRows.Scan(&it.TXNo, &it.ProviderRef, &it.AmountIDR, &it.FeeIDR); err == nil {
			successTxs = append(successTxs, it)
		}
	}

	// 2. Fetch settlement items for this date
	itemRows, err := a.DB.QueryContext(r.Context(),
		`SELECT i.provider_ref, i.amount_idr, i.fee_idr
		 FROM settlement_items i
		 JOIN settlement_batches b ON b.id = i.batch_id
		 WHERE i.school_id = $1 AND b.settled_on = $2`,
		schoolID, dateStr)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer itemRows.Close()

	var settleItems []model.ReconcileItem
	for itemRows.Next() {
		var it model.ReconcileItem
		if err := itemRows.Scan(&it.ProviderRef, &it.AmountIDR, &it.FeeIDR); err == nil {
			settleItems = append(settleItems, it)
		}
	}

	// 3. Run pure Reconcile function (AC-31-01)
	reconResult := model.Reconcile(successTxs, settleItems)

	// 4. Save exceptions into database
	for _, ex := range reconResult.Exceptions {
		_, _ = a.DB.ExecContext(r.Context(),
			`INSERT INTO reconciliation_exceptions (school_id, kind, ref, detail, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'open', now(), now())`,
			schoolID, string(ex.Kind), ex.Ref, jsonRaw(ex.Detail))
	}

	common.WriteSuccess(w, map[string]any{
		"date":            dateStr,
		"matched_count":   len(reconResult.Matched),
		"exception_count": len(reconResult.Exceptions),
		"exceptions":      reconResult.Exceptions,
	}, nil)
}

// AdminListExceptions lists reconciliation exceptions.
func (a *App) AdminListExceptions(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	status := r.URL.Query().Get("status")
	kind := r.URL.Query().Get("kind")

	query := `SELECT id, kind, ref, detail, status, COALESCE(resolution_note, ''), created_at, updated_at
	          FROM reconciliation_exceptions
	          WHERE school_id = $1`
	var args []any
	args = append(args, schoolID)

	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if kind != "" {
		args = append(args, kind)
		query += fmt.Sprintf(" AND kind = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type exRow struct {
		ID             string          `json:"id"`
		Kind           string          `json:"kind"`
		Ref            string          `json:"ref"`
		Detail         json.RawMessage `json:"detail"`
		Status         string          `json:"status"`
		ResolutionNote string          `json:"resolution_note"`
		CreatedAt      time.Time       `json:"created_at"`
		UpdatedAt      time.Time       `json:"updated_at"`
	}

	var items []exRow
	for rows.Next() {
		var it exRow
		var det []byte
		if err := rows.Scan(&it.ID, &it.Kind, &it.Ref, &det, &it.Status, &it.ResolutionNote, &it.CreatedAt, &it.UpdatedAt); err == nil {
			it.Detail = det
			items = append(items, it)
		}
	}

	common.WriteSuccess(w, items, nil)
}

// AdminResolveException marks an exception as resolved with a note.
func (a *App) AdminResolveException(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := param(r, "id")

	var req struct {
		ResolutionNote string `json:"resolution_note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.ResolutionNote == "" {
		common.WriteError(w, http.StatusBadRequest, "catatan penyelesaian (resolution_note) wajib diisi")
		return
	}

	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE reconciliation_exceptions
		 SET status = 'resolved', resolution_note = $1, updated_at = now()
		 WHERE id = $2 AND school_id = $3`, req.ResolutionNote, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		common.WriteError(w, http.StatusNotFound, "exception not found")
		return
	}

	common.WriteSuccess(w, map[string]string{"id": id, "status": "resolved"}, nil)
}

// AdminImportBankStatement imports bank mutation statement CSV lines (AC-31-02).
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

	reader := csv.NewReader(file)
	headers, err := reader.Read()
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid CSV format")
		return
	}

	headerMap := make(map[string]int)
	for i, h := range headers {
		headerMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	accIdx := headerMap["account"]
	dateIdx := headerMap["posted_on"]
	if _, ok := headerMap["posted_on"]; !ok {
		dateIdx = headerMap["date"]
	}
	amtIdx := headerMap["amount_idr"]
	if _, ok := headerMap["amount_idr"]; !ok {
		amtIdx = headerMap["amount"]
	}
	descIdx := headerMap["description"]

	importID := randomKey()
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback()

	inserted := 0
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		acc := "BANK-01"
		if accIdx < len(rec) && strings.TrimSpace(rec[accIdx]) != "" {
			acc = strings.TrimSpace(rec[accIdx])
		}
		dt := time.Now().Format("2006-01-02")
		if dateIdx < len(rec) && strings.TrimSpace(rec[dateIdx]) != "" {
			dt = strings.TrimSpace(rec[dateIdx])
		}
		var amt int64
		if amtIdx < len(rec) {
			amt, _ = strconv.ParseInt(strings.TrimSpace(rec[amtIdx]), 10, 64)
		}
		desc := ""
		if descIdx < len(rec) {
			desc = strings.TrimSpace(rec[descIdx])
		}

		if amt != 0 {
			_, err = tx.ExecContext(r.Context(),
				`INSERT INTO bank_statement_lines (school_id, account, posted_on, amount_idr, description, import_id)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				schoolID, acc, dt, amt, desc, importID)
			if err == nil {
				inserted++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	common.WriteSuccess(w, map[string]any{
		"import_id": importID,
		"inserted":  inserted,
	}, nil)
}

// AdminRunBankReconciliation matches bank statement lines against payments (AC-31-02).
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

	// 1. Fetch unmatched bank statement lines
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, account, posted_on, amount_idr, description
		 FROM bank_statement_lines
		 WHERE school_id = $1 AND matched_ref IS NULL`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	var lines []model.BankLine
	for rows.Next() {
		var l model.BankLine
		if err := rows.Scan(&l.ID, &l.Account, &l.PostedOn, &l.AmountIDR, &l.Description); err == nil {
			lines = append(lines, l)
		}
	}

	// 2. Fetch recorded verified payments in SEKOCI
	pRows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, transfer_at, amount_idr, reference
		 FROM bill_payments
		 WHERE school_id = $1 AND status = 'verified'`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer pRows.Close()

	var records []model.BankPaymentRecord
	for pRows.Next() {
		var rec model.BankPaymentRecord
		var tAt sql.NullTime
		var ref sql.NullString
		if err := pRows.Scan(&rec.ID, &tAt, &rec.AmountIDR, &ref); err == nil {
			if tAt.Valid {
				rec.Date = tAt.Time
			} else {
				rec.Date = time.Now()
			}
			if ref.Valid {
				rec.Ref = ref.String
			}
			records = append(records, rec)
		}
	}

	// 3. Match with tolerance of 2 days
	res := model.BankReconcile(lines, records, 2)

	// 4. Mark matched lines
	for _, lineID := range res.MatchedLines {
		_, _ = a.DB.ExecContext(r.Context(),
			`UPDATE bank_statement_lines SET matched_ref = 'matched' WHERE id = $1`, lineID)
	}

	// 5. Save unmatched to exceptions
	for _, ex := range res.Exceptions {
		_, _ = a.DB.ExecContext(r.Context(),
			`INSERT INTO reconciliation_exceptions (school_id, kind, ref, detail, status, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'open', now(), now())`,
			schoolID, string(ex.Kind), ex.Ref, jsonRaw(ex.Detail))
	}

	common.WriteSuccess(w, map[string]any{
		"matched_count": len(res.MatchedLines),
		"unmatched":     len(res.Exceptions),
	}, nil)
}
