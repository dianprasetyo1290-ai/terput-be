package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

type CashierPaymentInput struct {
	StudentUserID   string                       `json:"student_user_id"`
	Method          string                       `json:"method"` // 'cash' | 'transfer'
	AmountIDR       int64                        `json:"amount_idr"`
	Note            string                       `json:"note"`
	IdempotencyKey  string                       `json:"idempotency_key"`
	BillAllocations []model.BillAllocationResult `json:"bill_allocations,omitempty"`
}

type CashierPaymentResponse struct {
	ID              string                       `json:"id"`
	ReceiptNo       string                       `json:"receipt_no"`
	StudentUserID   string                       `json:"student_user_id"`
	Method          string                       `json:"method"`
	AmountIDR       int64                        `json:"amount_idr"`
	DepositIDR      int64                        `json:"deposit_idr"`
	Allocations     []model.BillAllocationResult `json:"allocations"`
	Status          string                       `json:"status"`
	CreatedAt       time.Time                    `json:"created_at"`
}

// CashierCreatePayment records a cash or direct transfer payment at the cashier counter (AC-29-03).
func (a *App) CashierCreatePayment(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireCashier(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	var req CashierPaymentInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.StudentUserID == "" || req.AmountIDR <= 0 {
		common.WriteError(w, http.StatusBadRequest, "student_user_id and positive amount_idr are required")
		return
	}
	if req.Method != "cash" && req.Method != "transfer" {
		req.Method = "cash"
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = idempotencyKey(r)
	}

	// Idempotency check
	var existingID, existingStatus string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id, status FROM cashier_payments WHERE school_id = $1 AND idempotency_key = $2`,
		schoolID, req.IdempotencyKey).Scan(&existingID, &existingStatus)
	if err == nil && existingID != "" {
		// Return existing receipt
		var rNo string
		_ = a.DB.QueryRowContext(r.Context(),
			`SELECT receipt_no FROM receipts WHERE school_id = $1 AND source_id = $2`,
			schoolID, existingID).Scan(&rNo)
		common.WriteSuccess(w, CashierPaymentResponse{
			ID:            existingID,
			ReceiptNo:     rNo,
			StudentUserID: req.StudentUserID,
			Method:        req.Method,
			AmountIDR:     req.AmountIDR,
			Status:        existingStatus,
		}, nil)
		return
	}

	// Read school allocation setting
	autoAlloc := "earliest_due"
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT auto_allocation FROM gateway_settings WHERE school_id = $1`, schoolID).Scan(&autoAlloc)

	// Fetch open bills for the student
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, net_idr, due_at FROM student_bills
		 WHERE school_id = $1 AND student_user_id = $2 AND status IN ('unpaid','partial')
		 ORDER BY due_at ASC NULLS LAST`, schoolID, req.StudentUserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	var openBills []model.OpenBill
	for rows.Next() {
		var bID string
		var netIDR int64
		var dueAt sql.NullTime
		if err := rows.Scan(&bID, &netIDR, &dueAt); err == nil {
			var verifiedSum, reversalSum int64
			_ = a.DB.QueryRowContext(r.Context(),
				`SELECT COALESCE(SUM(a.amount_idr), 0)
				 FROM bill_payment_allocations a
				 JOIN bill_payments p ON p.id = a.payment_id
				 WHERE p.student_bill_id = $1 AND p.status = 'verified'`, bID).Scan(&verifiedSum)
			_ = a.DB.QueryRowContext(r.Context(),
				`SELECT COALESCE(SUM(r.amount_idr), 0)
				 FROM bill_payment_reversals r
				 JOIN bill_payments p ON p.id = r.payment_id
				 WHERE p.student_bill_id = $1`, bID).Scan(&reversalSum)
			out, _ := model.Outstanding(netIDR, []int64{verifiedSum}, []int64{reversalSum})
			if out > 0 {
				var d time.Time
				if dueAt.Valid {
					d = dueAt.Time
				} else {
					d = time.Now()
				}
				openBills = append(openBills, model.OpenBill{
					BillID:         bID,
					OutstandingIDR: out,
					DueDate:        d,
				})
			}
		}
	}

	allocations := req.BillAllocations
	var depositIDR int64
	if len(allocations) == 0 {
		allocs, dep, allocErr := model.Allocate(req.AmountIDR, openBills, autoAlloc)
		if allocErr != nil {
			common.WriteError(w, http.StatusBadRequest, allocErr.Error())
			return
		}
		allocations = allocs
		depositIDR = dep
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback()

	now := time.Now()
	var cashierPaymentID string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO cashier_payments (
			school_id, student_user_id, method, amount_idr, received_by, note, status, idempotency_key, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'recorded', $7, $8, $8)
		RETURNING id`,
		schoolID, req.StudentUserID, req.Method, req.AmountIDR, claims.UserID, req.Note, req.IdempotencyKey, now).Scan(&cashierPaymentID)
	if err != nil {
		writeErr(w, err)
		return
	}

	// Create verified bill_payments records for each allocation
	for _, al := range allocations {
		var pID string
		allocIdem := fmt.Sprintf("cashier-alloc:%s:%s", cashierPaymentID, al.BillID)
		err = tx.QueryRowContext(r.Context(),
			`INSERT INTO bill_payments (
				school_id, student_bill_id, amount_idr, transfer_at, bank_name, account_name,
				reference, object_key, file_name, mime, size_bytes, sha256, status,
				idempotency_key, uploader_user_id, verified_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, 'Kasir', 'Tunai/Transfer Kasir',
				'KASIR-' || $5, 'cashier/' || $5, 'cashier_receipt', 'application/json',
				100, 'cashier_sha', 'verified', $6, $7, $4, $4, $4
			) RETURNING id`,
			schoolID, al.BillID, al.AmountIDR, now, cashierPaymentID, allocIdem, claims.UserID).Scan(&pID)
		if err != nil {
			writeErr(w, err)
			return
		}

		_, err = tx.ExecContext(r.Context(),
			`INSERT INTO bill_payment_allocations (school_id, payment_id, amount_idr, created_at)
			 VALUES ($1, $2, $3, $4)`,
			schoolID, pID, al.AmountIDR, now)
		if err != nil {
			writeErr(w, err)
			return
		}

		recomputeBillStatusInTx(r.Context(), tx, al.BillID)
	}

	// Excess to deposit (BR-FIN-04)
	if depositIDR > 0 {
		var newBal int64
		err = tx.QueryRowContext(r.Context(),
			`INSERT INTO student_deposits (school_id, student_user_id, balance_idr, updated_at)
			 VALUES ($1, $2, $3, now())
			 ON CONFLICT (school_id, student_user_id)
			 DO UPDATE SET balance_idr = student_deposits.balance_idr + $3, updated_at = now()
			 RETURNING balance_idr`,
			schoolID, req.StudentUserID, depositIDR).Scan(&newBal)
		if err == nil {
			_, _ = tx.ExecContext(r.Context(),
				`INSERT INTO deposit_movements (school_id, student_user_id, amount_idr, balance_after_idr, reason, ref_id)
				 VALUES ($1, $2, $3, $4, 'Kelebihan pembayaran kasir', $5)`,
				schoolID, req.StudentUserID, depositIDR, newBal, cashierPaymentID)
		}
	}

	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	// Generate unique receipt (AC-29-03)
	receiptNo, _ := a.atomicNextReceiptNo(r.Context(), schoolID, now.Year())
	docKey, _ := a.generateReceiptDocument(r.Context(), schoolID, ReceiptDetails{
		ReceiptNo:   receiptNo,
		SchoolName:  "Sekolah Teratai Putih",
		StudentName: "Siswa",
		PayerName:   "Orang Tua / Wali",
		AmountIDR:  req.AmountIDR,
		MethodLabel: req.Method,
		Date:        now,
		Items:       []string{fmt.Sprintf("Kasir %s", req.Method)},
	})

	_, _ = a.DB.ExecContext(r.Context(),
		`INSERT INTO receipts (school_id, receipt_no, source, source_id, object_key, sent_at, created_at)
		 VALUES ($1, $2, 'cashier', $3, $4, now(), now())`,
		schoolID, receiptNo, cashierPaymentID, docKey)

	common.WriteSuccess(w, CashierPaymentResponse{
		ID:            cashierPaymentID,
		ReceiptNo:     receiptNo,
		StudentUserID: req.StudentUserID,
		Method:        req.Method,
		AmountIDR:     req.AmountIDR,
		DepositIDR:    depositIDR,
		Allocations:   allocations,
		Status:        "recorded",
		CreatedAt:     now,
	}, nil)
}

// CashierListPayments returns paginated cashier payments.
func (a *App) CashierListPayments(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireCashier(w, r)
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

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT c.id, COALESCE(r.receipt_no, ''), c.student_user_id, c.method, c.amount_idr, c.status, c.note, c.created_at
		 FROM cashier_payments c
		 LEFT JOIN receipts r ON r.source = 'cashier' AND r.source_id = c.id
		 WHERE c.school_id = $1
		 ORDER BY c.created_at DESC
		 LIMIT $2 OFFSET $3`, schoolID, perPage, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type cashierRow struct {
		ID            string    `json:"id"`
		ReceiptNo     string    `json:"receipt_no"`
		StudentUserID string    `json:"student_user_id"`
		Method        string    `json:"method"`
		AmountIDR     int64     `json:"amount_idr"`
		Status        string    `json:"status"`
		Note          string    `json:"note"`
		CreatedAt     time.Time `json:"created_at"`
	}
	var items []cashierRow
	for rows.Next() {
		var it cashierRow
		if err := rows.Scan(&it.ID, &it.ReceiptNo, &it.StudentUserID, &it.Method, &it.AmountIDR, &it.Status, &it.Note, &it.CreatedAt); err == nil {
			items = append(items, it)
		}
	}

	var total int
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM cashier_payments WHERE school_id = $1`, schoolID).Scan(&total)

	common.WriteSuccess(w, items, &common.Meta{
		Page:    page,
		PerPage: perPage,
		Total:   total,
	})
}

// CashierReceiptPDF renders or streams a receipt for cashier payment.
func (a *App) CashierReceiptPDF(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireCashier(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := param(r, "id")

	var objectKey, receiptNo sql.NullString
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT object_key, receipt_no FROM receipts
		 WHERE school_id = $1 AND source = 'cashier' AND source_id = $2`,
		schoolID, id).Scan(&objectKey, &receiptNo)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "kuitansi tidak ditemukan")
		return
	}

	if objectKey.Valid && objectKey.String != "" {
		a.streamObject(w, r, objectKey.String, "text/html", "", 0)
		return
	}

	common.WriteError(w, http.StatusNotFound, "file kuitansi belum dibuat")
}
