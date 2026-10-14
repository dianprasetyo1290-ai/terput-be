package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// scanPaymentRows scans the shared payment column list used by the parent and
// admin list endpoints. Nullable columns are omitted from the output rather
// than surfaced as empty strings.
func scanPaymentRows(rows *sql.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, status                                     string
			amount                                         int64
			transferAt, verifiedAt                         sql.NullTime
			bank, account, reference, fileName, reviewNote sql.NullString
			createdAt                                      time.Time
		)
		if err := rows.Scan(&id, &amount, &status, &transferAt, &bank, &account, &reference,
			&fileName, &reviewNote, &createdAt, &verifiedAt); err != nil {
			return nil, err
		}
		m := map[string]any{"id": id, "amount_idr": amount, "status": status, "created_at": createdAt}
		if transferAt.Valid {
			m["transfer_at"] = transferAt.Time
		}
		if bank.Valid {
			m["bank_name"] = bank.String
		}
		if account.Valid {
			m["account_name"] = account.String
		}
		if reference.Valid {
			m["reference"] = reference.String
		}
		if fileName.Valid {
			m["file_name"] = fileName.String
		}
		if reviewNote.Valid && reviewNote.String != "" {
			m["review_note"] = reviewNote.String
		}
		if verifiedAt.Valid {
			m["verified_at"] = verifiedAt.Time
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// POST /parent/children/{studentUserId}/bills/{billId}/payments (multipart)
// ---------------------------------------------------------------------------

// SubmitPayment is POST /parent/children/{studentUserId}/bills/{billId}/payments.
// The proof file is stored in the private object store; only its metadata ever
// reaches the database (never a base64 blob).
func (a *App) SubmitPayment(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireChild(w, r)
	if !ok {
		return
	}
	billID := param(r, "billId")

	// Bound the whole multipart body, not just the in-memory buffer.
	r.Body = http.MaxBytesReader(w, r.Body, a.Config.MaxUploadBytes+1<<20)
	if err := r.ParseMultipartForm(a.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	amount, okAmount := parsePositive(r.FormValue("amount"))
	if !okAmount {
		common.WriteError(w, http.StatusBadRequest, "a positive amount is required")
		return
	}
	transferAt, err := parseTime(r.FormValue("transfer_at"))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "transfer_at is invalid")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	// The bill must exist, belong to the school and this exact student.
	var billStatus string
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT status FROM student_bills WHERE id::text=$1 AND school_id=$2 AND student_user_id::text=$3`,
		billID, c.SchoolID, studentID).Scan(&billStatus)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "bill not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if billStatus == "void" {
		common.WriteError(w, http.StatusUnprocessableEntity, "bill is void")
		return
	}

	key := c.SchoolID + "/transfer-proofs/" + studentID + "/" + randomKey()
	obj, err := a.Storage.Put(r.Context(), key, header.Header.Get("Content-Type"), file, header.Size)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	idem := idempotencyKey(r)

	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO bill_payments(school_id,student_bill_id,amount_idr,transfer_at,bank_name,account_name,reference,
		        object_key,file_name,mime,size_bytes,sha256,status,idempotency_key,uploader_user_id)
		 VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,'pending',$13,$14)
		 ON CONFLICT(school_id,idempotency_key) DO NOTHING
		 RETURNING id`,
		c.SchoolID, billID, amount, nullTime(transferAt), r.FormValue("bank_name"), r.FormValue("account_name"),
		r.FormValue("reference"), obj.Key, header.Filename, obj.MIME, obj.Size, obj.Checksum, idem, c.UserID).Scan(&id)
	if err == sql.ErrNoRows {
		// Idempotent replay: the object we just stored is an orphan.
		_ = a.Storage.Delete(r.Context(), obj.Key)
		var existingStatus string
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT id,status FROM bill_payments WHERE school_id=$1 AND idempotency_key=$2`,
			c.SchoolID, idem).Scan(&id, &existingStatus); err != nil {
			writeErr(w, err)
			return
		}
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": existingStatus}, nil)
		return
	}
	if err != nil {
		_ = a.Storage.Delete(r.Context(), obj.Key)
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), c.SchoolID, "payment", id, "payment_submitted", "guardian", c.UserID, "", "",
		nil, map[string]any{"bill_id": billID, "amount_idr": amount})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending"}, nil)
}

// ---------------------------------------------------------------------------
// Admin verification queue
// ---------------------------------------------------------------------------

// PaymentQueue is GET /admin/payments.
func (a *App) PaymentQueue(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	studentID := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	year := common.IntParam(r, "period_year")
	month := common.IntParam(r, "period_month")
	page, perPage := common.Page(r)

	// Shared filter for the count, the per-status summary and the page query:
	// $1 school, $2 status, $3 student, $4 year, $5 month.
	const where = `p.school_id=$1 AND ($2='' OR p.status=$2) AND ($3='' OR b.student_user_id::text=$3)
		   AND ($4=0 OR b.period_year=$4) AND ($5=0 OR b.period_month=$5)`
	filterArgs := []any{schoolID, status, studentID, year, month}

	// Per-status totals over the whole filtered set (not just this page), so
	// summary cards never sum a single page.
	total := 0
	summary := map[string]any{}
	sumRows, err := a.DB.QueryContext(r.Context(),
		`SELECT p.status,count(*),COALESCE(sum(p.amount_idr),0)
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE `+where+` GROUP BY p.status`, filterArgs...)
	if err != nil {
		writeErr(w, err)
		return
	}
	for sumRows.Next() {
		var st string
		var n int
		var amt int64
		if err := sumRows.Scan(&st, &n, &amt); err != nil {
			sumRows.Close()
			writeErr(w, err)
			return
		}
		total += n
		summary[st] = map[string]any{"count": n, "amount_idr": amt}
	}
	sumRows.Close()
	if err := sumRows.Err(); err != nil {
		writeErr(w, err)
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT p.id,p.student_bill_id::text,b.student_user_id::text,b.period_year,b.period_month,
		        p.amount_idr,p.status,p.transfer_at,p.bank_name,p.account_name,p.reference,p.file_name,
		        p.review_note,p.created_at,p.verified_at
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE `+where+`
		 ORDER BY p.created_at DESC LIMIT $6 OFFSET $7`,
		append(filterArgs, perPage, common.Offset(page, perPage))...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var (
			id, billID, studentID, payStatus               string
			year, month                                    int
			amount                                         int64
			transferAt, verifiedAt                         sql.NullTime
			bank, account, reference, fileName, reviewNote sql.NullString
			createdAt                                      time.Time
		)
		if err := rows.Scan(&id, &billID, &studentID, &year, &month, &amount, &payStatus,
			&transferAt, &bank, &account, &reference, &fileName, &reviewNote, &createdAt, &verifiedAt); err != nil {
			writeErr(w, err)
			return
		}
		m := map[string]any{
			"id": id, "student_bill_id": billID, "student_user_id": studentID,
			"period_year": year, "period_month": month, "amount_idr": amount,
			"status": payStatus, "created_at": createdAt,
		}
		if transferAt.Valid {
			m["transfer_at"] = transferAt.Time
		}
		if bank.Valid {
			m["bank_name"] = bank.String
		}
		if account.Valid {
			m["account_name"] = account.String
		}
		if reference.Valid {
			m["reference"] = reference.String
		}
		if fileName.Valid {
			m["file_name"] = fileName.String
		}
		if reviewNote.Valid && reviewNote.String != "" {
			m["review_note"] = reviewNote.String
		}
		if verifiedAt.Valid {
			m["verified_at"] = verifiedAt.Time
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, map[string]any{"page": page, "per_page": perPage, "total": total, "summary": summary})
}

// AdminPaymentProof is GET /admin/payments/{id}/proof.
func (a *App) AdminPaymentProof(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var key, mime, checksum string
	var size int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT object_key,mime,size_bytes,sha256 FROM bill_payments WHERE id::text=$1 AND school_id=$2 AND object_key IS NOT NULL`,
		param(r, "id"), schoolID).Scan(&key, &mime, &size, &checksum)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "payment not found")
		return
	}
	a.streamObject(w, r, key, mime, checksum, size)
}

// ---------------------------------------------------------------------------
// POST /admin/payments/{id}/review
// ---------------------------------------------------------------------------

// ReviewPayment is POST /admin/payments/{id}/review. Verifying writes the
// immutable allocation ledger; rejecting records a note. Neither ever edits a
// verified payment.
func (a *App) ReviewPayment(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireFinanceStaff(w, r)
	if claims == nil {
		return
	}
	var in struct {
		Action      string             `json:"action"`
		Note        string             `json:"note"`
		Allocations []model.Allocation `json:"allocations"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	target, err := model.PaymentAction(in.Action)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	paymentID := param(r, "id")

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status    string
		amount    int64
		billID    string
		studentID string
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT p.status,p.amount_idr,p.student_bill_id::text,b.student_user_id::text
		 FROM bill_payments p JOIN student_bills b ON b.id=p.student_bill_id
		 WHERE p.id::text=$1 AND p.school_id=$2
		 FOR UPDATE OF p`,
		paymentID, schoolID).Scan(&status, &amount, &billID, &studentID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = model.ValidatePaymentTransition(model.PaymentStatus(status), target); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if target == model.PaymentVerified {
		allocs := in.Allocations
		if len(allocs) == 0 {
			// The payment settles its bill as a whole by default.
			allocs = []model.Allocation{{AmountIDR: amount}}
		}
		if err = model.ValidateAllocations(amount, allocs); err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		for _, alloc := range allocs {
			if alloc.BillItemID != "" {
				var belongs bool
				if err = tx.QueryRowContext(r.Context(),
					`SELECT EXISTS(SELECT 1 FROM bill_items WHERE id::text=$1 AND bill_id::text=$2 AND school_id=$3)`,
					alloc.BillItemID, billID, schoolID).Scan(&belongs); err != nil {
					writeErr(w, err)
					return
				}
				if !belongs {
					common.WriteError(w, http.StatusUnprocessableEntity, "allocation references a foreign bill item")
					return
				}
			}
			if _, err = tx.ExecContext(r.Context(),
				`INSERT INTO bill_payment_allocations(school_id,payment_id,bill_item_id,amount_idr)
				 VALUES($1,$2,NULLIF($3,'')::uuid,$4)`,
				schoolID, paymentID, alloc.BillItemID, alloc.AmountIDR); err != nil {
				common.WriteError(w, http.StatusUnprocessableEntity, "invalid allocation")
				return
			}
		}
	}
	if target == model.PaymentRejected && strings.TrimSpace(in.Note) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "a reason is required to reject a payment")
		return
	}

	if _, err = tx.ExecContext(r.Context(),
		`UPDATE bill_payments SET status=$1,review_note=NULLIF($2,''),
		        verified_at=CASE WHEN $1='verified' THEN now() ELSE verified_at END,updated_at=now()
		 WHERE id::text=$3 AND school_id=$4`,
		target, in.Note, paymentID, schoolID); err != nil {
		writeErr(w, err)
		return
	}
	if err = recomputeBillStatusQ(r.Context(), tx, schoolID, billID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "payment", paymentID, "payment_reviewed", "admin", claims.UserID, "", in.Note,
		map[string]any{"status": status}, map[string]any{"status": target, "student_user_id": studentID})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": paymentID, "status": target}, nil)
}

// ---------------------------------------------------------------------------
// POST /admin/payments/{id}/reversal
// ---------------------------------------------------------------------------

// ReversePayment is POST /admin/payments/{id}/reversal. A verified payment is
// immutable; a correction is an append-only reversal that restores exactly the
// amount it reverses to the bill's outstanding balance.
func (a *App) ReversePayment(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireFinanceStaff(w, r)
	if claims == nil {
		return
	}
	var in struct {
		AmountIDR int64  `json:"amount_idr"`
		Reason    string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "reason is required")
		return
	}
	paymentID := param(r, "id")

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status string
		amount int64
		billID string
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,amount_idr,student_bill_id::text FROM bill_payments
		 WHERE id::text=$1 AND school_id=$2 AND source='upload' FOR UPDATE`,
		paymentID, schoolID).Scan(&status, &amount, &billID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if model.PaymentStatus(status) != model.PaymentVerified {
		common.WriteError(w, http.StatusUnprocessableEntity, "only a verified payment can be reversed")
		return
	}

	reverseAmount := in.AmountIDR
	if reverseAmount <= 0 {
		reverseAmount = amount
	}
	if reverseAmount > amount {
		common.WriteError(w, http.StatusUnprocessableEntity, "reversal exceeds the verified payment amount")
		return
	}
	var already int64
	if err = tx.QueryRowContext(r.Context(),
		`SELECT COALESCE(sum(amount_idr),0) FROM bill_payment_reversals WHERE payment_id::text=$1 AND school_id=$2`,
		paymentID, schoolID).Scan(&already); err != nil {
		writeErr(w, err)
		return
	}
	if already+reverseAmount > amount {
		common.WriteError(w, http.StatusUnprocessableEntity, "cumulative reversals exceed the verified payment amount")
		return
	}

	idem := idempotencyKey(r)
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO bill_payment_reversals(school_id,payment_id,amount_idr,reason,actor_id,idempotency_key)
		 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(school_id,idempotency_key) DO NOTHING`,
		schoolID, paymentID, reverseAmount, in.Reason, claims.UserID, idem); err != nil {
		writeErr(w, err)
		return
	}
	if err = recomputeBillStatusQ(r.Context(), tx, schoolID, billID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "payment", paymentID, "payment_reversed", "admin", claims.UserID, "", in.Reason,
		map[string]any{"status": status}, map[string]any{"reversed_idr": reverseAmount})
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"id": paymentID, "status": model.PaymentVerified, "reversed_idr": reverseAmount,
	}, nil)
}
