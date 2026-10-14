package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

type CashierPaymentInput struct {
	StudentUserID   string                       `json:"student_user_id"`
	Method          string                       `json:"method"` // 'cash' | 'transfer'
	AmountIDR       int64                        `json:"amount_idr"`
	Note            string                       `json:"note"`
	PayerName       string                       `json:"payer_name"`
	IdempotencyKey  string                       `json:"idempotency_key"`
	BillAllocations []model.BillAllocationResult `json:"bill_allocations,omitempty"`
}

type CashierPaymentResponse struct {
	ID            string                       `json:"id"`
	ReceiptNo     string                       `json:"receipt_no"`
	StudentUserID string                       `json:"student_user_id"`
	Method        string                       `json:"method"`
	AmountIDR     int64                        `json:"amount_idr"`
	DepositIDR    int64                        `json:"deposit_idr"`
	Allocations   []model.BillAllocationResult `json:"allocations"`
	Status        string                       `json:"status"`
	CreatedAt     time.Time                    `json:"created_at"`
}

// loadOpenBillsTx returns a student's non-void bills that still have an
// outstanding balance, locked FOR UPDATE so two concurrent payments cannot
// both settle the same balance. Outstanding comes from the ledger, never
// from the stored status.
func loadOpenBillsTx(ctx context.Context, tx *sql.Tx, schoolID, studentID string) ([]model.OpenBill, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id::text, due_at FROM student_bills
		  WHERE school_id = $1 AND student_user_id = $2 AND status <> 'void'
		  ORDER BY due_at ASC NULLS LAST, id
		  FOR UPDATE`, schoolID, studentID)
	if err != nil {
		return nil, err
	}
	type billRow struct {
		id  string
		due sql.NullTime
	}
	var ids []billRow
	for rows.Next() {
		var b billRow
		if err := rows.Scan(&b.id, &b.due); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var open []model.OpenBill
	for _, b := range ids {
		_, _, _, out, err := billTotalsQ(ctx, tx, schoolID, b.id)
		if err != nil {
			return nil, err
		}
		if out <= 0 {
			continue
		}
		ob := model.OpenBill{BillID: b.id, OutstandingIDR: out}
		if b.due.Valid {
			ob.DueDate = b.due.Time
		}
		open = append(open, ob)
	}
	return open, nil
}

// receiptNames resolves the school, student and payer names printed on a
// receipt from identity. payer defaults to the first linked guardian, then
// the student. A name identity cannot supply falls back to the raw id so a
// receipt never carries an invented name.
func (a *App) receiptNames(ctx context.Context, schoolID, studentID, payer string) (string, string, string) {
	school, student := schoolID, studentID
	if !a.IdentitySvc.Configured() {
		if payer == "" {
			payer = student
		}
		return school, student, payer
	}
	if n, err := a.IdentitySvc.SchoolName(ctx, schoolID); err == nil && n != "" {
		school = n
	}
	if u, err := a.IdentitySvc.User(ctx, studentID); err == nil && u.Name != "" {
		student = u.Name
	}
	if payer == "" {
		if gids, err := a.IdentitySvc.StudentGuardianIDs(ctx, schoolID, studentID); err == nil && len(gids) > 0 {
			if u, err := a.IdentitySvc.User(ctx, gids[0]); err == nil && u.Name != "" {
				payer = u.Name
			}
		}
	}
	if payer == "" {
		payer = student
	}
	return school, student, payer
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
	req.PayerName = strings.TrimSpace(req.PayerName)
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
	var existingDeposit int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id, status, deposit_idr FROM cashier_payments WHERE school_id = $1 AND idempotency_key = $2`,
		schoolID, req.IdempotencyKey).Scan(&existingID, &existingStatus, &existingDeposit)
	if err == nil && existingID != "" {
		var rNo string
		_ = a.DB.QueryRowContext(r.Context(),
			`SELECT receipt_no FROM receipts WHERE school_id = $1 AND source = 'cashier' AND source_id = $2`,
			schoolID, existingID).Scan(&rNo)
		common.WriteSuccess(w, CashierPaymentResponse{
			ID:            existingID,
			ReceiptNo:     rNo,
			StudentUserID: req.StudentUserID,
			Method:        req.Method,
			AmountIDR:     req.AmountIDR,
			DepositIDR:    existingDeposit,
			Status:        existingStatus,
		}, nil)
		return
	}

	autoAlloc := "earliest_due"
	_ = a.DB.QueryRowContext(r.Context(),
		`SELECT auto_allocation FROM gateway_settings WHERE school_id = $1`, schoolID).Scan(&autoAlloc)

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer tx.Rollback()

	// Open bills of THIS student in THIS school only: a manual allocation to
	// any other bill is rejected by ValidateManualAllocations.
	openBills, err := loadOpenBillsTx(r.Context(), tx, schoolID, req.StudentUserID)
	if err != nil {
		writeErr(w, err)
		return
	}

	allocations := req.BillAllocations
	var depositIDR int64
	if len(allocations) == 0 {
		allocs, dep, allocErr := model.Allocate(req.AmountIDR, openBills, autoAlloc)
		if allocErr != nil {
			common.WriteError(w, http.StatusBadRequest, allocErr.Error())
			return
		}
		allocations, depositIDR = allocs, dep
	} else {
		dep, vErr := model.ValidateManualAllocations(req.AmountIDR, allocations, openBills)
		if vErr != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, vErr.Error())
			return
		}
		depositIDR = dep
	}

	now := time.Now()
	var cashierPaymentID string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO cashier_payments (
			school_id, student_user_id, method, amount_idr, deposit_idr, received_by, note, status, idempotency_key, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'recorded', $8, $9, $9)
		RETURNING id`,
		schoolID, req.StudentUserID, req.Method, req.AmountIDR, depositIDR, claims.UserID, req.Note, req.IdempotencyKey, now).Scan(&cashierPaymentID)
	if err != nil {
		writeErr(w, err)
		return
	}

	// One verified ledger payment per allocated bill. source='cashier' keeps
	// it out of the transfer-proof totals (the cashier_payments row is the
	// receipt of record) and the KASIR- reference is what corrections reverse.
	for _, al := range allocations {
		var pID string
		allocIdem := fmt.Sprintf("cashier-alloc:%s:%s", cashierPaymentID, al.BillID)
		err = tx.QueryRowContext(r.Context(),
			`INSERT INTO bill_payments (
				school_id, student_bill_id, amount_idr, transfer_at, bank_name, account_name,
				reference, status, source, source_id,
				idempotency_key, uploader_user_id, verified_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, 'Kasir', $5,
				'KASIR-' || $6, 'verified', 'cashier', $6::uuid,
				$7, $8, $4, $4, $4
			) RETURNING id`,
			schoolID, al.BillID, al.AmountIDR, now, cashierAccountLabel(req.Method), cashierPaymentID, allocIdem, claims.UserID).Scan(&pID)
		if err != nil {
			writeErr(w, err)
			return
		}
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO bill_payment_allocations (school_id, payment_id, amount_idr, created_at)
			 VALUES ($1, $2, $3, $4)`,
			schoolID, pID, al.AmountIDR, now); err != nil {
			writeErr(w, err)
			return
		}
		if err = recomputeBillStatusQ(r.Context(), tx, schoolID, al.BillID); err != nil {
			writeErr(w, err)
			return
		}
	}

	// Excess to deposit (BR-FIN-04)
	if depositIDR > 0 {
		var newBal int64
		if err = tx.QueryRowContext(r.Context(),
			`INSERT INTO student_deposits (school_id, student_user_id, balance_idr, updated_at)
			 VALUES ($1, $2, $3, now())
			 ON CONFLICT (school_id, student_user_id)
			 DO UPDATE SET balance_idr = student_deposits.balance_idr + $3, updated_at = now()
			 RETURNING balance_idr`,
			schoolID, req.StudentUserID, depositIDR).Scan(&newBal); err != nil {
			writeErr(w, err)
			return
		}
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO deposit_movements (school_id, student_user_id, amount_idr, balance_after_idr, reason, ref)
			 VALUES ($1, $2, $3, $4, 'Kelebihan pembayaran kasir', $5)`,
			schoolID, req.StudentUserID, depositIDR, newBal, "cashier:"+cashierPaymentID); err != nil {
			writeErr(w, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}

	a.audit(r.Context(), schoolID, "cashier_payment", cashierPaymentID, "cashier_recorded", "cashier", claims.UserID, "", req.Note,
		nil, map[string]any{
			"student_user_id": req.StudentUserID, "method": req.Method, "amount_idr": req.AmountIDR,
			"deposit_idr": depositIDR, "allocations": allocations,
		})

	// Generate unique receipt (AC-29-03)
	receiptNo, err := a.atomicNextReceiptNo(r.Context(), schoolID, now.Year())
	if err != nil {
		common.CaptureError(err)
	}
	schoolName, studentName, payerName := a.receiptNames(r.Context(), schoolID, req.StudentUserID, req.PayerName)
	items := make([]string, 0, len(allocations)+1)
	for _, al := range allocations {
		items = append(items, fmt.Sprintf("Tagihan %s: %s", al.BillID, formatRupiah(al.AmountIDR)))
	}
	if depositIDR > 0 {
		items = append(items, "Deposit kelebihan bayar: "+formatRupiah(depositIDR))
	}
	if receiptNo != "" {
		docKey, docErr := a.generateReceiptDocument(r.Context(), schoolID, ReceiptDetails{
			ReceiptNo:   receiptNo,
			SchoolName:  schoolName,
			StudentName: studentName,
			PayerName:   payerName,
			AmountIDR:   req.AmountIDR,
			MethodLabel: cashierAccountLabel(req.Method),
			Date:        now.In(a.loc()),
			Items:       items,
		})
		if docErr != nil {
			common.CaptureError(docErr)
		}
		if _, err := a.DB.ExecContext(r.Context(),
			`INSERT INTO receipts (school_id, receipt_no, source, source_id, object_key, sent_at, created_at)
			 VALUES ($1, $2, 'cashier', $3, NULLIF($4, ''), now(), now())`,
			schoolID, receiptNo, cashierPaymentID, docKey); err != nil {
			common.CaptureError(err)
		}
	}

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

func cashierAccountLabel(method string) string {
	if method == "transfer" {
		return "Transfer (kasir)"
	}
	return "Tunai (kasir)"
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
