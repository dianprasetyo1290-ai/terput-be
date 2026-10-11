package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
	"skci/billing/internal/nicepay"
)

type CheckoutPreviewRequest struct {
	StudentUserID string   `json:"student_user_id"`
	BillIDs       []string `json:"bill_ids"`
	MethodID      string   `json:"method_id"`
}

type BillSummaryItem struct {
	ID             string `json:"id"`
	PeriodYear     int    `json:"period_year"`
	PeriodMonth    int    `json:"period_month"`
	NetIDR         int64  `json:"net_idr"`
	OutstandingIDR int64  `json:"outstanding_idr"`
	Status         string `json:"status"`
}

type CheckoutPreviewResponse struct {
	StudentUserID  string            `json:"student_user_id"`
	Bills          []BillSummaryItem `json:"bills"`
	Method         model.GatewayMethod `json:"method"`
	BillsAmountIDR int64             `json:"bills_amount_idr"`
	FeeIDR         int64             `json:"fee_idr"`
	TotalIDR       int64             `json:"total_idr"`
	FeeBearer      string            `json:"fee_bearer"`
}

// ParentCheckoutPreview validates bills and calculates fees before confirmation (FR-28-002).
func (a *App) ParentCheckoutPreview(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil {
		return
	}

	var req CheckoutPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.StudentUserID == "" || len(req.BillIDs) == 0 || req.MethodID == "" {
		common.WriteError(w, http.StatusBadRequest, "student_user_id, bill_ids, and method_id are required")
		return
	}

	// Verify child access
	if a.Identity != nil {
		ok, err := a.Identity.GuardianHasAccess(r.Context(), claims.UserID, req.StudentUserID, claims.SchoolID)
		if err != nil || !ok {
			common.WriteError(w, http.StatusForbidden, "access denied to student")
			return
		}
	}

	bills, totalBillsAmount, err := a.fetchOpenBills(r.Context(), claims.SchoolID, req.StudentUserID, req.BillIDs)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	method, err := a.fetchMethod(r.Context(), claims.SchoolID, req.MethodID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	totals, err := model.Totals(totalBillsAmount, *method)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := CheckoutPreviewResponse{
		StudentUserID:  req.StudentUserID,
		Bills:          bills,
		Method:         *method,
		BillsAmountIDR: totals.BillsAmountIDR,
		FeeIDR:         totals.FeeIDR,
		TotalIDR:       totals.TotalIDR,
		FeeBearer:      string(totals.FeeBearer),
	}
	common.WriteSuccess(w, resp, nil)
}

type CheckoutRequest struct {
	StudentUserID string   `json:"student_user_id"`
	BillIDs       []string `json:"bill_ids"`
	MethodID      string   `json:"method_id"`
}

type TransactionResponse struct {
	ID             string            `json:"id"`
	TXNo           string            `json:"tx_no"`
	BillsAmountIDR int64             `json:"bills_amount_idr"`
	FeeIDR         int64             `json:"fee_idr"`
	TotalIDR       int64             `json:"total_idr"`
	FeeBearer      string            `json:"fee_bearer"`
	Status         string            `json:"status"`
	MethodCode     string            `json:"method_code"`
	MethodKind     string            `json:"kind"`
	MethodLabel    string            `json:"method_label"`
	VANumber       *string           `json:"va_number,omitempty"`
	QRContent      *string           `json:"qr_content,omitempty"`
	RedirectURL    *string           `json:"redirect_url,omitempty"`
	ExpiresAt      time.Time         `json:"expires_at"`
	PaidAt         *time.Time        `json:"paid_at,omitempty"`
	Bills          []BillSummaryItem `json:"bills,omitempty"`
}

// ParentCheckout creates a single payment transaction for multiple bills (AC-27-01).
func (a *App) ParentCheckout(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil {
		return
	}

	var req CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.StudentUserID == "" || len(req.BillIDs) == 0 || req.MethodID == "" {
		common.WriteError(w, http.StatusBadRequest, "student_user_id, bill_ids, and method_id are required")
		return
	}

	if a.Identity != nil {
		ok, err := a.Identity.GuardianHasAccess(r.Context(), claims.UserID, req.StudentUserID, claims.SchoolID)
		if err != nil || !ok {
			common.WriteError(w, http.StatusForbidden, "access denied to student")
			return
		}
	}

	idemKey := idempotencyKey(r)

	// Check if already created with this idempotency key
	var existingID string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM gateway_transactions WHERE school_id = $1 AND idempotency_key = $2`,
		claims.SchoolID, idemKey).Scan(&existingID)
	if err == nil && existingID != "" {
		tx, getErr := a.getTransactionByID(r.Context(), claims.SchoolID, existingID)
		if getErr == nil {
			common.WriteSuccess(w, tx, nil)
			return
		}
	}

	// Conflict check: if any bill already has an active pending transaction
	conflictTxID, conflictTxNo, err := a.checkPendingBillsConflict(r.Context(), claims.SchoolID, req.BillIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	if conflictTxID != "" {
		common.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":          "Tagihan sedang dalam proses transaksi lain",
			"transaction_id": conflictTxID,
			"tx_no":          conflictTxNo,
		}, nil)
		return
	}

	bills, totalBillsAmount, err := a.fetchOpenBills(r.Context(), claims.SchoolID, req.StudentUserID, req.BillIDs)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	method, err := a.fetchMethod(r.Context(), claims.SchoolID, req.MethodID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	totals, err := model.Totals(totalBillsAmount, *method)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	txNo := fmt.Sprintf("TX-%s%d", time.Now().Format("060102150405"), time.Now().Nanosecond()%1000)

	// Call NICEPay (or fallback to test simulation if client not configured)
	expiresAt := time.Now().Add(time.Duration(method.ValidMinutes) * time.Minute)
	var vaNumber, qrContent, redirectURL, providerRef string

	order := nicepay.Order{
		RefNo:      txNo,
		Amount:     totals.TotalIDR,
		GoodsName:  fmt.Sprintf("Pembayaran Tagihan (%d item)", len(bills)),
		BuyerName:  "Orang Tua Siswa",
		BuyerPhone: "081200000000",
		NotifyURL:  a.Config.Gateway.NicepayCallbackURL,
		ReturnURL:  a.Config.Gateway.NicepayReturnURL,
		ClientIP:   "127.0.0.1",
		ImageURL:   a.Config.Gateway.NicepayImageURL,
	}

	if a.Nicepay != nil {
		switch method.Kind {
		case "va":
			ins, err := a.Nicepay.CreateVA(order, method.ProviderCode)
			if err != nil {
				common.WriteError(w, http.StatusBadGateway, "Gagal membuat kode VA: "+err.Error())
				return
			}
			providerRef = ins.GatewayRef
			vaNumber = ins.PayCode
			if !ins.ExpiresAt.IsZero() {
				expiresAt = ins.ExpiresAt
			}
		case "qris":
			ins, err := a.Nicepay.CreateQRIS(order, time.Duration(method.ValidMinutes)*time.Minute)
			if err != nil {
				common.WriteError(w, http.StatusBadGateway, "Gagal membuat kode QRIS: "+err.Error())
				return
			}
			providerRef = ins.GatewayRef
			qrContent = ins.PayCode
			if !ins.ExpiresAt.IsZero() {
				expiresAt = ins.ExpiresAt
			}
		case "ewallet":
			ins, err := a.Nicepay.CreateEWallet(order, method.ProviderCode, time.Duration(method.ValidMinutes)*time.Minute)
			if err != nil {
				common.WriteError(w, http.StatusBadGateway, "Gagal memproses e-wallet: "+err.Error())
				return
			}
			providerRef = ins.GatewayRef
			redirectURL = ins.PayCode
			if !ins.ExpiresAt.IsZero() {
				expiresAt = ins.ExpiresAt
			}
		}
	} else {
		// Test/dev simulation
		providerRef = "SIM-" + txNo
		switch method.Kind {
		case "va":
			vaNumber = "8808" + txNo[3:]
		case "qris":
			qrContent = "00020101021226680016ID.CO.NICEPAY.WWW0118936009990000000001520454115303360540" + fmt.Sprintf("%d", totals.TotalIDR) + "5802ID"
		case "ewallet":
			redirectURL = "https://mock-ewallet.nicepay.co.id/pay?ref=" + txNo
		}
	}

	txDB, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer txDB.Rollback()

	var txID string
	err = txDB.QueryRowContext(r.Context(),
		`INSERT INTO gateway_transactions (
			school_id, tx_no, payer_user_id, student_user_id, method_id,
			bills_amount_idr, fee_idr, total_idr, fee_bearer, status,
			provider_ref, va_number, qr_content, redirect_url, expires_at, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', $10, NULLIF($11,''), NULLIF($12,''), NULLIF($13,''), $14, $15)
		RETURNING id`,
		claims.SchoolID, txNo, claims.UserID, req.StudentUserID, method.ID,
		totals.BillsAmountIDR, totals.FeeIDR, totals.TotalIDR, totals.FeeBearer,
		providerRef, vaNumber, qrContent, redirectURL, expiresAt, idemKey).Scan(&txID)
	if err != nil {
		writeErr(w, err)
		return
	}

	for i, b := range bills {
		_, err = txDB.ExecContext(r.Context(),
			`INSERT INTO gateway_transaction_bills (transaction_id, bill_id, position, amount_idr)
			 VALUES ($1, $2, $3, $4)`,
			txID, b.ID, i+1, b.OutstandingIDR)
		if err != nil {
			writeErr(w, err)
			return
		}
	}

	if err := txDB.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	res := TransactionResponse{
		ID:             txID,
		TXNo:           txNo,
		BillsAmountIDR: totals.BillsAmountIDR,
		FeeIDR:         totals.FeeIDR,
		TotalIDR:       totals.TotalIDR,
		FeeBearer:      string(totals.FeeBearer),
		Status:         "pending",
		MethodCode:     method.Code,
		MethodKind:     method.Kind,
		MethodLabel:    method.Label,
		ExpiresAt:      expiresAt,
		Bills:          bills,
	}
	if vaNumber != "" {
		res.VANumber = &vaNumber
	}
	if qrContent != "" {
		res.QRContent = &qrContent
	}
	if redirectURL != "" {
		res.RedirectURL = &redirectURL
	}

	common.WriteSuccess(w, res, nil)
}

// ParentGetTransaction returns details of a transaction and handles expiration check (FR-27-002).
func (a *App) ParentGetTransaction(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil {
		return
	}
	id := param(r, "id")

	tx, err := a.getTransactionByID(r.Context(), claims.SchoolID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			common.WriteError(w, http.StatusNotFound, "transaksi tidak ditemukan")
			return
		}
		writeErr(w, err)
		return
	}

	// Auto expire check if pending and expired
	if tx.Status == "pending" && time.Now().After(tx.ExpiresAt) {
		// Check NICEPay status if client available
		if a.Nicepay != nil && tx.ProviderRef != nil {
			st, err := a.Nicepay.Status(nicepay.StatusQuery{
				Method:     tx.MethodKind,
				RefNo:      tx.TXNo,
				GatewayRef: *tx.ProviderRef,
				Amount:     tx.TotalIDR,
			})
			if err == nil && st == nicepay.StatusSuccess {
				// Transition to success
				_ = a.markTransactionSuccess(r.Context(), tx.ID)
				tx.Status = "success"
				now := time.Now()
				tx.PaidAt = &now
			} else {
				_, _ = a.DB.ExecContext(r.Context(),
					`UPDATE gateway_transactions SET status = 'expired', updated_at = now() WHERE id = $1`, tx.ID)
				tx.Status = "expired"
			}
		} else {
			_, _ = a.DB.ExecContext(r.Context(),
				`UPDATE gateway_transactions SET status = 'expired', updated_at = now() WHERE id = $1`, tx.ID)
			tx.Status = "expired"
		}
	}

	common.WriteSuccess(w, tx, nil)
}

// ParentRenewTransaction creates a new transaction after expiration (AC-27-02).
func (a *App) ParentRenewTransaction(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil {
		return
	}
	id := param(r, "id")

	tx, err := a.getTransactionByID(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "transaksi tidak ditemukan")
		return
	}
	if tx.Status != "expired" && tx.Status != "failed" {
		common.WriteError(w, http.StatusBadRequest, "hanya transaksi kedaluwarsa atau gagal yang dapat diperbarui")
		return
	}

	// Best-effort cancel old VA at NICEPay
	if tx.MethodKind == "va" && a.Nicepay != nil && tx.VANumber != nil && tx.ProviderRef != nil {
		_ = a.Nicepay.CancelVA(nicepay.StatusQuery{
			Method:     "va",
			RefNo:      tx.TXNo,
			GatewayRef: *tx.ProviderRef,
			PayCode:    *tx.VANumber,
			Amount:     tx.TotalIDR,
		})
	}

	var billIDs []string
	for _, b := range tx.Bills {
		billIDs = append(billIDs, b.ID)
	}

	// Find method ID
	var methodID string
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM payment_methods WHERE school_id = $1 AND code = $2`,
		claims.SchoolID, tx.MethodCode).Scan(&methodID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "metode pembayaran tidak tersedia lagi")
		return
	}

	// Forward to ParentCheckout
	checkoutReq := CheckoutRequest{
		StudentUserID: tx.StudentUserID,
		BillIDs:       billIDs,
		MethodID:      methodID,
	}
	reqBody, _ := json.Marshal(checkoutReq)
	r.Body = ioNopCloser(strings.NewReader(string(reqBody)))
	r.Header.Set("Idempotency-Key", fmt.Sprintf("renew:%s:%d", id, time.Now().UnixNano()))

	a.ParentCheckout(w, r)
}

// Helper query functions
type fullTxDetail struct {
	TransactionResponse
	StudentUserID string
	ProviderRef   *string
}

func (a *App) getTransactionByID(ctx context.Context, schoolID, id string) (*fullTxDetail, error) {
	var tx fullTxDetail
	var va, qr, redir, pRef sql.NullString
	var paidAt sql.NullTime

	query := `SELECT t.id, t.tx_no, t.bills_amount_idr, t.fee_idr, t.total_idr, t.fee_bearer,
	                 t.status, m.code, m.kind, m.label, t.va_number, t.qr_content, t.redirect_url,
	                 t.expires_at, t.paid_at, t.student_user_id, t.provider_ref
	          FROM gateway_transactions t
	          JOIN payment_methods m ON m.id = t.method_id
	          WHERE t.id = $1 AND t.school_id = $2`
	err := a.DB.QueryRowContext(ctx, query, id, schoolID).Scan(
		&tx.ID, &tx.TXNo, &tx.BillsAmountIDR, &tx.FeeIDR, &tx.TotalIDR, &tx.FeeBearer,
		&tx.Status, &tx.MethodCode, &tx.MethodKind, &tx.MethodLabel, &va, &qr, &redir,
		&tx.ExpiresAt, &paidAt, &tx.StudentUserID, &pRef)
	if err != nil {
		return nil, err
	}

	if va.Valid {
		tx.VANumber = &va.String
	}
	if qr.Valid {
		tx.QRContent = &qr.String
	}
	if redir.Valid {
		tx.RedirectURL = &redir.String
	}
	if paidAt.Valid {
		tx.PaidAt = &paidAt.Time
	}
	if pRef.Valid {
		tx.ProviderRef = &pRef.String
	}

	// Fetch linked bills
	bRows, err := a.DB.QueryContext(ctx,
		`SELECT b.id, b.period_year, b.period_month, b.net_idr, tb.amount_idr, b.status
		 FROM gateway_transaction_bills tb
		 JOIN student_bills b ON b.id = tb.bill_id
		 WHERE tb.transaction_id = $1`, id)
	if err == nil {
		defer bRows.Close()
		for bRows.Next() {
			var it BillSummaryItem
			if err := bRows.Scan(&it.ID, &it.PeriodYear, &it.PeriodMonth, &it.NetIDR, &it.OutstandingIDR, &it.Status); err == nil {
				tx.Bills = append(tx.Bills, it)
			}
		}
	}

	return &tx, nil
}

func (a *App) checkPendingBillsConflict(ctx context.Context, schoolID string, billIDs []string) (string, string, error) {
	for _, bID := range billIDs {
		var txID, txNo string
		err := a.DB.QueryRowContext(ctx,
			`SELECT t.id, t.tx_no
			 FROM gateway_transaction_bills tb
			 JOIN gateway_transactions t ON t.id = tb.transaction_id
			 WHERE t.school_id = $1 AND tb.bill_id = $2 AND t.status = 'pending' AND t.expires_at > now()`,
			schoolID, bID).Scan(&txID, &txNo)
		if err == nil && txID != "" {
			return txID, txNo, nil
		}
	}
	return "", "", nil
}

func (a *App) fetchOpenBills(ctx context.Context, schoolID, studentUserID string, billIDs []string) ([]BillSummaryItem, int64, error) {
	var items []BillSummaryItem
	var total int64

	for _, bID := range billIDs {
		var it BillSummaryItem
		var netIDR int64
		err := a.DB.QueryRowContext(ctx,
			`SELECT id, period_year, period_month, net_idr, status
			 FROM student_bills
			 WHERE id = $1 AND school_id = $2 AND student_user_id = $3`,
			bID, schoolID, studentUserID).Scan(&it.ID, &it.PeriodYear, &it.PeriodMonth, &netIDR, &it.Status)
		if err != nil {
			return nil, 0, fmt.Errorf("tagihan %s tidak ditemukan", bID)
		}
		if it.Status == "paid" || it.Status == "void" {
			return nil, 0, fmt.Errorf("tagihan %s sudah lunas atau dibatalkan", bID)
		}

		// Calculate outstanding
		var verifiedSum, reversalSum int64
		_ = a.DB.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(a.amount_idr), 0)
			 FROM bill_payment_allocations a
			 JOIN bill_payments p ON p.id = a.payment_id
			 WHERE p.student_bill_id = $1 AND p.status = 'verified'`, bID).Scan(&verifiedSum)
		_ = a.DB.QueryRowContext(ctx,
			`SELECT COALESCE(SUM(r.amount_idr), 0)
			 FROM bill_payment_reversals r
			 JOIN bill_payments p ON p.id = r.payment_id
			 WHERE p.student_bill_id = $1`, bID).Scan(&reversalSum)

		out, _ := model.Outstanding(netIDR, []int64{verifiedSum}, []int64{reversalSum})
		if out <= 0 {
			return nil, 0, fmt.Errorf("tagihan %s sudah tidak memiliki sisa saldo", bID)
		}
		it.NetIDR = netIDR
		it.OutstandingIDR = out
		total += out
		items = append(items, it)
	}

	return items, total, nil
}

func (a *App) fetchMethod(ctx context.Context, schoolID, methodID string) (*model.GatewayMethod, error) {
	var m model.GatewayMethod
	err := a.DB.QueryRowContext(ctx,
		`SELECT id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort
		 FROM payment_methods
		 WHERE id = $1 AND school_id = $2 AND enabled = true`,
		methodID, schoolID).Scan(&m.ID, &m.Code, &m.Kind, &m.ProviderCode, &m.Label, &m.Enabled,
		&m.FeeKind, &m.FeeFlatIDR, &m.FeeBP, &m.FeeBearer, &m.ValidMinutes, &m.Sort)
	if err != nil {
		return nil, errors.New("metode pembayaran tidak valid atau tidak aktif")
	}
	return &m, nil
}

func ioNopCloser(r *strings.Reader) *readCloserWrapper {
	return &readCloserWrapper{Reader: r}
}

type readCloserWrapper struct {
	*strings.Reader
}

func (w *readCloserWrapper) Close() error { return nil }
