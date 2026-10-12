package handler

import (
	"encoding/csv"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/gateway"
)

// AdminImportSettlement imports NICEPay settlement report CSV (FR-30-001).
func (a *App) AdminImportSettlement(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
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

	// FR-150-004: every batch belongs to one provider (NICEPay = T6 format).
	provider := strings.TrimSpace(r.FormValue("provider"))
	if provider == "" {
		provider = gateway.DefaultProvider
	}
	var known bool
	if err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM payment_providers WHERE code = $1)`, provider).Scan(&known); err != nil || !known {
		common.WriteError(w, http.StatusBadRequest, "provider tidak dikenal")
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

	refIdx, hasRef := headerMap["provider_ref"]
	if !hasRef {
		refIdx = headerMap["reference"]
	}
	amtIdx, hasAmt := headerMap["amount_idr"]
	if !hasAmt {
		amtIdx = headerMap["amount"]
	}
	feeIdx := headerMap["fee_idr"]
	chanIdx, hasChan := headerMap["channel"]
	dateIdx, hasDate := headerMap["settled_on"]
	accIdx, hasAcc := headerMap["bank_account"]

	type itemRow struct {
		providerRef string
		channel     string
		settledOn   string
		amountIDR  int64
		feeIDR      int64
		bankAccount string
	}

	var items []itemRow
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		ref := ""
		if refIdx < len(rec) {
			ref = strings.TrimSpace(rec[refIdx])
		}
		if ref == "" {
			continue
		}
		var amt, fee int64
		if amtIdx < len(rec) {
			amt, _ = strconv.ParseInt(strings.TrimSpace(rec[amtIdx]), 10, 64)
		}
		if feeIdx < len(rec) {
			fee, _ = strconv.ParseInt(strings.TrimSpace(rec[feeIdx]), 10, 64)
		}
		ch := "NICEPay"
		if hasChan && chanIdx < len(rec) && strings.TrimSpace(rec[chanIdx]) != "" {
			ch = strings.TrimSpace(rec[chanIdx])
		}
		dt := time.Now().Format("2006-01-02")
		if hasDate && dateIdx < len(rec) && strings.TrimSpace(rec[dateIdx]) != "" {
			dt = strings.TrimSpace(rec[dateIdx])
		}
		acc := "1234567890"
		if hasAcc && accIdx < len(rec) && strings.TrimSpace(rec[accIdx]) != "" {
			acc = strings.TrimSpace(rec[accIdx])
		}

		items = append(items, itemRow{
			providerRef: ref,
			channel:     ch,
			settledOn:   dt,
			amountIDR:  amt,
			feeIDR:      fee,
			bankAccount: acc,
		})
	}

	if len(items) == 0 {
		common.WriteError(w, http.StatusBadRequest, "no valid rows found in CSV")
		return
	}

	// Group by channel and settledOn
	batches := make(map[string][]itemRow)
	for _, it := range items {
		key := it.channel + "|" + it.settledOn + "|" + it.bankAccount
		batches[key] = append(batches[key], it)
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback()

	importedBatches := 0
	importedItems := 0

	for _, bItems := range batches {
		first := bItems[0]
		var gross, totalFee int64
		for _, it := range bItems {
			gross += it.amountIDR
			totalFee += it.feeIDR
		}
		net := gross - totalFee

		var batchID string
		err := tx.QueryRowContext(r.Context(),
			`INSERT INTO settlement_batches (
				school_id, provider, channel, settled_on, gross_idr, fee_idr, net_idr, bank_account, status
			) VALUES ($1, $8, $2, $3, $4, $5, $6, $7, 'reported')
			ON CONFLICT (school_id, provider, channel, settled_on)
			DO UPDATE SET gross_idr = $4, fee_idr = $5, net_idr = $6, updated_at = now()
			RETURNING id`,
			schoolID, first.channel, first.settledOn, gross, totalFee, net, first.bankAccount, provider).Scan(&batchID)
		if err != nil {
			writeErr(w, err)
			return
		}
		importedBatches++

		for _, it := range bItems {
			// Find matching transaction by provider_ref
			var matchedTxID *string
			var mID string
			if err := tx.QueryRowContext(r.Context(),
				`SELECT id FROM gateway_transactions WHERE school_id = $1 AND provider = $3 AND provider_ref = $2`,
				schoolID, it.providerRef, provider).Scan(&mID); err == nil {
				matchedTxID = &mID
			}

			_, err = tx.ExecContext(r.Context(),
				`INSERT INTO settlement_items (school_id, batch_id, provider_ref, amount_idr, fee_idr, matched_transaction_id)
				 VALUES ($1, $2, $3, $4, $5, $6)`,
				schoolID, batchID, it.providerRef, it.amountIDR, it.feeIDR, matchedTxID)
			if err == nil {
				importedItems++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	common.WriteSuccess(w, map[string]any{
		"message":          "Settlement imported successfully",
		"imported_batches": importedBatches,
		"imported_items":   importedItems,
		"provider":         provider,
	}, nil)
}

// AdminListSettlements lists settlement batches.
func (a *App) AdminListSettlements(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, provider, channel, settled_on, gross_idr, fee_idr, net_idr, bank_account, status, created_at
		 FROM settlement_batches
		 WHERE school_id = $1
		 ORDER BY settled_on DESC`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type batchRow struct {
		ID          string    `json:"id"`
		Provider    string    `json:"provider"`
		Channel     string    `json:"channel"`
		SettledOn   string    `json:"settled_on"`
		GrossIDR    int64     `json:"gross_idr"`
		FeeIDR      int64     `json:"fee_idr"`
		NetIDR      int64     `json:"net_idr"`
		BankAccount string    `json:"bank_account"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"created_at"`
	}

	var items []batchRow
	for rows.Next() {
		var it batchRow
		var sOn time.Time
		if err := rows.Scan(&it.ID, &it.Provider, &it.Channel, &sOn, &it.GrossIDR, &it.FeeIDR, &it.NetIDR, &it.BankAccount, &it.Status, &it.CreatedAt); err == nil {
			it.SettledOn = sOn.Format("2006-01-02")
			items = append(items, it)
		}
	}

	common.WriteSuccess(w, items, nil)
}
