package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

type StatusStat struct {
	Count     int   `json:"count"`
	AmountIDR int64 `json:"amount_idr"`
}

type MethodShareStat struct {
	MethodKind string `json:"method_kind"`
	Count      int    `json:"count"`
	ShareBP    int    `json:"share_bp"` // basis points (1 bp = 0.01%)
}

type DailyTrendItem struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	AmountIDR int64  `json:"amount_idr"`
}

type SettlementSummaryStat struct {
	TotalBatches int   `json:"total_batches"`
	GrossIDR     int64 `json:"gross_idr"`
	FeeIDR       int64 `json:"fee_idr"`
	NetIDR       int64 `json:"net_idr"`
}

type PaymentDashboardResponse struct {
	PeriodFrom        string                `json:"period_from"`
	PeriodTo          string                `json:"period_to"`
	StatusSummary     map[string]StatusStat `json:"status_summary"`
	MethodShares      []MethodShareStat     `json:"method_shares"`
	DailyTrend        []DailyTrendItem      `json:"daily_trend"`
	SettlementSummary SettlementSummaryStat `json:"settlement_summary"`
}

// AdminPaymentDashboard calculates payment statistics, method shares, and trends (FR-26-001, AC-26-01).
func (a *App) AdminPaymentDashboard(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" {
		fromStr = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}
	if toStr == "" {
		toStr = time.Now().Format("2006-01-02")
	}

	// 1. Status summary
	statusSummary := map[string]StatusStat{
		"success": {Count: 0, AmountIDR: 0},
		"pending": {Count: 0, AmountIDR: 0},
		"failed":  {Count: 0, AmountIDR: 0},
		"expired": {Count: 0, AmountIDR: 0},
	}

	sRows, err := a.DB.QueryContext(r.Context(),
		`SELECT status, COUNT(*), COALESCE(SUM(total_idr), 0)
		 FROM gateway_transactions
		 WHERE school_id = $1 AND DATE(created_at) BETWEEN $2 AND $3
		 GROUP BY status`, schoolID, fromStr, toStr)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var st string
			var count int
			var amt int64
			if err := sRows.Scan(&st, &count, &amt); err == nil {
				statusSummary[st] = StatusStat{Count: count, AmountIDR: amt}
			}
		}
	}

	// 2. Method distribution on successful transactions (AC-26-01)
	counts := make(map[string]int)
	mRows, err := a.DB.QueryContext(r.Context(),
		`SELECT m.kind, COUNT(*)
		 FROM gateway_transactions t
		 JOIN payment_methods m ON m.id = t.method_id
		 WHERE t.school_id = $1 AND t.status = 'success' AND DATE(t.created_at) BETWEEN $2 AND $3
		 GROUP BY m.kind`, schoolID, fromStr, toStr)
	if err == nil {
		defer mRows.Close()
		for mRows.Next() {
			var k string
			var c int
			if err := mRows.Scan(&k, &c); err == nil {
				counts[k] = c
			}
		}
	}

	shares := model.MethodShare(counts)
	var methodShares []MethodShareStat
	for _, k := range []string{"va", "qris", "ewallet"} {
		c := counts[k]
		bp := shares[k]
		methodShares = append(methodShares, MethodShareStat{
			MethodKind: k,
			Count:      c,
			ShareBP:    bp,
		})
	}

	// 3. Daily trend
	var dailyTrend []DailyTrendItem
	tRows, err := a.DB.QueryContext(r.Context(),
		`SELECT DATE(created_at)::text, COUNT(*), COALESCE(SUM(total_idr), 0)
		 FROM gateway_transactions
		 WHERE school_id = $1 AND status = 'success' AND DATE(created_at) BETWEEN $2 AND $3
		 GROUP BY DATE(created_at)
		 ORDER BY DATE(created_at) ASC`, schoolID, fromStr, toStr)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var it DailyTrendItem
			if err := tRows.Scan(&it.Date, &it.Count, &it.AmountIDR); err == nil {
				dailyTrend = append(dailyTrend, it)
			}
		}
	}

	// 4. Settlement summary
	var setStat SettlementSummaryStat
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*), COALESCE(SUM(gross_idr), 0), COALESCE(SUM(fee_idr), 0), COALESCE(SUM(net_idr), 0)
		 FROM settlement_batches
		 WHERE school_id = $1 AND settled_on BETWEEN $2 AND $3`, schoolID, fromStr, toStr).Scan(
		&setStat.TotalBatches, &setStat.GrossIDR, &setStat.FeeIDR, &setStat.NetIDR)

	resp := PaymentDashboardResponse{
		PeriodFrom:        fromStr,
		PeriodTo:          toStr,
		StatusSummary:     statusSummary,
		MethodShares:      methodShares,
		DailyTrend:        dailyTrend,
		SettlementSummary: setStat,
	}
	common.WriteSuccess(w, resp, nil)
}

// AdminListTransactions lists online payment transactions with filters (FR-29-001).
func (a *App) AdminListTransactions(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	status := r.URL.Query().Get("status")
	method := r.URL.Query().Get("method")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	search := r.URL.Query().Get("q")

	query := `SELECT t.id, t.tx_no, t.bills_amount_idr, t.fee_idr, t.total_idr, t.fee_bearer,
	                 t.status, m.code, m.kind, m.label, t.va_number, t.qr_content, t.redirect_url,
	                 t.expires_at, t.paid_at, t.student_user_id, t.provider_ref, t.created_at
	          FROM gateway_transactions t
	          JOIN payment_methods m ON m.id = t.method_id
	          WHERE t.school_id = $1`
	var args []any
	args = append(args, schoolID)

	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND t.status = $%d", len(args))
	}
	if method != "" {
		args = append(args, method)
		query += fmt.Sprintf(" AND m.kind = $%d", len(args))
	}
	if from != "" {
		args = append(args, from)
		query += fmt.Sprintf(" AND DATE(t.created_at) >= $%d", len(args))
	}
	if to != "" {
		args = append(args, to)
		query += fmt.Sprintf(" AND DATE(t.created_at) <= $%d", len(args))
	}
	if search != "" {
		args = append(args, "%"+search+"%")
		query += fmt.Sprintf(" AND (t.tx_no ILIKE $%d OR t.provider_ref ILIKE $%d)", len(args), len(args))
	}

	query += fmt.Sprintf(" ORDER BY t.created_at DESC LIMIT %d OFFSET %d", perPage, offset)

	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type adminTxRow struct {
		ID             string     `json:"id"`
		TXNo           string     `json:"tx_no"`
		BillsAmountIDR int64      `json:"bills_amount_idr"`
		FeeIDR         int64      `json:"fee_idr"`
		TotalIDR       int64      `json:"total_idr"`
		FeeBearer      string     `json:"fee_bearer"`
		Status         string     `json:"status"`
		MethodCode     string     `json:"method_code"`
		MethodKind     string     `json:"kind"`
		MethodLabel    string     `json:"method_label"`
		VANumber       *string    `json:"va_number,omitempty"`
		ProviderRef    *string    `json:"provider_ref,omitempty"`
		ExpiresAt      time.Time  `json:"expires_at"`
		PaidAt         *time.Time `json:"paid_at,omitempty"`
		StudentUserID  string     `json:"student_user_id"`
		CreatedAt      time.Time  `json:"created_at"`
	}

	var items []adminTxRow
	for rows.Next() {
		var it adminTxRow
		var va, qr, redir, pref sql.NullString
		var pAt sql.NullTime
		if err := rows.Scan(&it.ID, &it.TXNo, &it.BillsAmountIDR, &it.FeeIDR, &it.TotalIDR, &it.FeeBearer,
			&it.Status, &it.MethodCode, &it.MethodKind, &it.MethodLabel, &va, &qr, &redir,
			&it.ExpiresAt, &pAt, &it.StudentUserID, &pref, &it.CreatedAt); err == nil {
			if va.Valid {
				it.VANumber = &va.String
			}
			if pref.Valid {
				it.ProviderRef = &pref.String
			}
			if pAt.Valid {
				it.PaidAt = &pAt.Time
			}
			items = append(items, it)
		}
	}

	var total int
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM gateway_transactions WHERE school_id = $1`, schoolID).Scan(&total)

	common.WriteSuccess(w, items, &common.Meta{
		Page:    page,
		PerPage: perPage,
		Total:   total,
	})
}

// AdminGetTransaction returns details of an individual transaction (FR-29-001).
func (a *App) AdminGetTransaction(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := param(r, "id")

	tx, err := a.getTransactionByID(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "transaksi tidak ditemukan")
		return
	}

	common.WriteSuccess(w, tx, nil)
}

// AdminResendReceipt resends a digital receipt notification (FR-29-003).
func (a *App) AdminResendReceipt(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := param(r, "id")

	var receiptNo string
	var payerUserID string
	var totalIDR int64
	var source string

	err := a.DB.QueryRowContext(r.Context(),
		`SELECT r.receipt_no, r.source, COALESCE(t.payer_user_id, c.student_user_id), COALESCE(t.total_idr, c.amount_idr)
		 FROM receipts r
		 LEFT JOIN gateway_transactions t ON t.id = r.source_id AND r.source = 'gateway'
		 LEFT JOIN cashier_payments c ON c.id = r.source_id AND r.source = 'cashier'
		 WHERE r.id = $1 AND r.school_id = $2`, id, schoolID).Scan(&receiptNo, &source, &payerUserID, &totalIDR)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "kuitansi tidak ditemukan")
		return
	}

	// Update resend count and sent_at
	_, _ = a.DB.ExecContext(r.Context(),
		`UPDATE receipts SET resend_count = resend_count + 1, sent_at = now() WHERE id = $1`, id)

	if a.Notification != nil {
		_ = a.Notification.Notify(r.Context(), fmt.Sprintf("receipt-resend:%s:%d", id, time.Now().Unix()), client.Notification{
			SchoolID: schoolID,
			UserID:   payerUserID,
			Type:     "receipt",
			Title:    "Kuitansi Pembayaran " + receiptNo + " (Kirim Ulang)",
			Body:     fmt.Sprintf("Salinan kuitansi pembayaran sebesar Rp %d telah dikirim ulang.", totalIDR),
			Data:     map[string]any{"receipt_no": receiptNo, "resend": true},
			Channels: []string{"in_app", "email"},
		})
	}

	common.WriteSuccess(w, map[string]string{
		"id":         id,
		"receipt_no": receiptNo,
		"status":     "sent",
		"message":    "Kuitansi berhasil dikirim ulang",
	}, nil)
}
