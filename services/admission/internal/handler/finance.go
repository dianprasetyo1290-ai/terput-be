package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// ---------------------------------------------------------------------------
// Charges (guardian)
// ---------------------------------------------------------------------------

// GetCharge is GET /guardian/applications/{id}/charge[?stage=registration|reregistration].
// Without a stage the charge the guardian must pay now is returned; the form
// charge is issued lazily, the re-registration charge only once accepted.
func (a *App) GetCharge(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	view, err := a.stageChargeView(r.Context(), c.SchoolID, applicationID, r.URL.Query().Get("stage"))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, view, nil)
}

// stageChargeView returns the requested (or current) stage charge, issuing it
// when the application is entitled to it and none exists yet.
func (a *App) stageChargeView(ctx context.Context, schoolID, applicationID, stage string) (map[string]any, error) {
	var status string
	if err := a.DB.QueryRowContext(ctx,
		`SELECT status FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return nil, &httpError{http.StatusNotFound, "application not found"}
		}
		return nil, err
	}
	if stage != stageRegistration && stage != stageReregistration {
		stage = currentStage(status)
		if stage == stageRegistration {
			// A dropped applicant who already had a re-registration bill sees it.
			if rr, err := latestCharge(ctx, a.DB, schoolID, applicationID, stageReregistration); err != nil {
				return nil, err
			} else if rr != nil {
				stage = stageReregistration
			}
		}
	}
	charge, err := latestCharge(ctx, a.DB, schoolID, applicationID, stage)
	if err != nil {
		return nil, err
	}
	chargeID := ""
	if charge != nil {
		chargeID = charge.ID
	} else {
		if chargeID, err = a.issueCharge(ctx, schoolID, applicationID, stage, issueOptions{}); err != nil {
			return nil, err
		}
	}
	view, err := a.chargeView(ctx, schoolID, applicationID, chargeID)
	if err != nil {
		return nil, err
	}
	view["application_status"] = status
	if model.ApplicationStatus(status) == model.StatusDropped {
		view["payable"] = false
		view["blocked_reason"] = "Pendaftar sudah gugur; tagihan tidak dapat dibayar."
	} else {
		view["payable"] = view["status"] != "cancelled" && view["status"] != "paid"
	}
	return view, nil
}

// ---------------------------------------------------------------------------
// Voucher
// ---------------------------------------------------------------------------

// ApplyVoucher is POST /guardian/applications/{id}/charge/voucher.
func (a *App) ApplyVoucher(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Code == "" {
		common.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}

	voucherID, chargeID, err := a.redeemVoucher(r.Context(), c.SchoolID, applicationID, in.Code)
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := a.chargeView(r.Context(), c.SchoolID, applicationID, chargeID)
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), c.SchoolID, "application", applicationID, "voucher_applied", "guardian", c.GuardianID, "", "", nil,
		map[string]any{"voucher_id": voucherID, "charge_id": chargeID})
	common.WriteJSON(w, http.StatusOK, view, nil)
}

// redeemVoucher validates a voucher code for the application's period (status,
// validity window, usage limit), reserves it for the stage the guardian is
// paying now and rebuilds that stage's charge in the same transaction.
func (a *App) redeemVoucher(ctx context.Context, schoolID, applicationID, code string) (string, string, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()

	var periodID, status string
	if err := tx.QueryRowContext(ctx,
		`SELECT period_id,status FROM applications WHERE id=$1 AND school_id=$2 FOR UPDATE`, applicationID, schoolID).
		Scan(&periodID, &status); err != nil {
		if err == sql.ErrNoRows {
			return "", "", &httpError{http.StatusNotFound, "application not found"}
		}
		return "", "", err
	}
	stage := currentStage(status)

	var existing string
	err = tx.QueryRowContext(ctx,
		`SELECT voucher_id FROM voucher_redemptions WHERE application_id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&existing)
	if err == nil && existing != "" {
		return "", "", &httpError{http.StatusConflict, "voucher sudah dipakai untuk pendaftaran ini"}
	}
	if err != nil && err != sql.ErrNoRows {
		return "", "", err
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id,code_hash,status,valid_from,valid_until,usage_limit,used_count
		 FROM discount_vouchers WHERE school_id=$1 AND period_id=$2 FOR UPDATE`, schoolID, periodID)
	if err != nil {
		return "", "", err
	}
	type voucherRow struct {
		id, hash, status   string
		validFrom, validTo time.Time
		limit, used        int
	}
	var vouchers []voucherRow
	for rows.Next() {
		var v voucherRow
		if err := rows.Scan(&v.id, &v.hash, &v.status, &v.validFrom, &v.validTo, &v.limit, &v.used); err != nil {
			rows.Close()
			return "", "", err
		}
		vouchers = append(vouchers, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", "", err
	}

	now := time.Now()
	var matched *voucherRow
	for i := range vouchers {
		v := &vouchers[i]
		if v.status != "active" || !model.PromoActive(now, &v.validFrom, &v.validTo) || !model.PromoQuotaAvailable(v.limit, v.used) {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(v.hash), []byte(code)) == nil {
			matched = v
			break
		}
	}
	if matched == nil {
		return "", "", &httpError{http.StatusUnprocessableEntity, "voucher tidak valid, kedaluwarsa, atau kuotanya habis"}
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO voucher_redemptions(school_id,application_id,voucher_id,stage) VALUES($1,$2,$3,$4)`,
		schoolID, applicationID, matched.id, stage); err != nil {
		if isUniqueViolation(err) {
			return "", "", &httpError{http.StatusConflict, "voucher sudah dipakai untuk pendaftaran ini"}
		}
		return "", "", err
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE discount_vouchers SET used_count=used_count+1 WHERE id=$1`, matched.id); err != nil {
		return "", "", err
	}
	previous, err := latestCharge(ctx, tx, schoolID, applicationID, stage)
	if err != nil {
		return "", "", err
	}
	chargeID, err := a.issueChargeTx(ctx, tx, schoolID, applicationID, stage, issueOptions{Rebuild: previous != nil})
	if err != nil {
		return "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	return matched.id, chargeID, nil
}

// ---------------------------------------------------------------------------
// Payments
// ---------------------------------------------------------------------------

// ListPayments is GET /guardian/applications/{id}/payments.
func (a *App) ListPayments(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,amount_idr,COALESCE(transfer_at,created_at),COALESCE(bank_name,''),COALESCE(account_name,''),
		        COALESCE(reference,''),COALESCE(file_name,''),status,COALESCE(review_note,''),created_at
		 FROM payment_submissions WHERE application_id=$1 AND school_id=$2 ORDER BY created_at DESC`,
		applicationID, c.SchoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, bank, account, reference, fileName, status, reviewNote string
			amount                                                     int64
			transferAt, createdAt                                      time.Time
		)
		if err := rows.Scan(&id, &amount, &transferAt, &bank, &account, &reference, &fileName, &status, &reviewNote, &createdAt); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id":             id,
			"application_id": applicationID,
			"amount":         amount,
			"transfer_at":    transferAt,
			"bank_name":      bank,
			"account_name":   account,
			"reference":      reference,
			"file_name":      fileName,
			"status":         status,
			"created_at":     createdAt,
		}
		if reviewNote != "" {
			item["review_note"] = reviewNote
		}
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// SubmitPayment is POST /guardian/applications/{id}/payments (multipart). The
// proof is attached to the latest charge of the stage being paid
// (form field "stage", default: the stage due now). Dropped or inactive
// applications cannot pay (TC-15-017).
func (a *App) SubmitPayment(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	if err := r.ParseMultipartForm(a.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	amount, ok := parsePositive(r.FormValue("amount"))
	if !ok {
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

	var status string
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT status FROM applications WHERE id=$1 AND school_id=$2`, applicationID, c.SchoolID).Scan(&status); err != nil {
		writeErr(w, err)
		return
	}
	switch model.ApplicationStatus(status) {
	case model.StatusDropped:
		common.WriteError(w, http.StatusUnprocessableEntity, "Pendaftar sudah gugur; tagihan tidak dapat dibayar.")
		return
	case model.StatusWithdrawn, model.StatusRejected:
		common.WriteError(w, http.StatusUnprocessableEntity, "Pendaftaran sudah tidak aktif; tagihan tidak dapat dibayar.")
		return
	}
	stage := r.FormValue("stage")
	if stage != stageRegistration && stage != stageReregistration {
		stage = currentStage(status)
	}
	charge, err := latestCharge(r.Context(), a.DB, c.SchoolID, applicationID, stage)
	if err != nil {
		writeErr(w, err)
		return
	}
	if charge == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "tagihan untuk tahap ini belum terbit")
		return
	}
	if charge.Cancelled.Valid {
		common.WriteError(w, http.StatusUnprocessableEntity, "tagihan sudah dibatalkan dan tidak dapat dibayar")
		return
	}

	mime, body, err := sniffUpload(file, header)
	if err != nil {
		writeErr(w, err)
		return
	}
	key := c.SchoolID + "/payments/" + randomKey()
	obj, err := a.Storage.Put(r.Context(), key, mime, body, header.Size)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "berkas bukti bayar tidak valid")
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = randomKey()
	}

	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO payment_submissions(school_id,application_id,charge_id,amount_idr,object_key,file_name,mime,size_bytes,sha256,
		        transfer_at,bank_name,account_name,reference,uploader_type,uploader_id,idempotency_key)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),'guardian',$14,$15)
		 ON CONFLICT(school_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key
		 RETURNING id`,
		c.SchoolID, applicationID, charge.ID, amount, obj.Key, header.Filename, obj.MIME, obj.Size, obj.Checksum,
		nullTime(transferAt), r.FormValue("bank_name"), r.FormValue("account_name"), r.FormValue("reference"),
		c.GuardianID, idempotencyKey).Scan(&id)
	if err != nil {
		_ = a.Storage.Delete(r.Context(), obj.Key)
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), c.SchoolID, "application", applicationID, "payment_submitted", "guardian", c.GuardianID, "", "", nil,
		map[string]any{"payment_id": id, "amount": amount, "charge_id": charge.ID, "stage": stage})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "submitted", "stage": stage}, nil)
}

// PaymentQueue is GET /admin/payments.
func (a *App) PaymentQueue(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT p.id,p.application_id,p.amount_idr,COALESCE(p.transfer_at,p.created_at),COALESCE(p.bank_name,''),
		        COALESCE(p.account_name,''),COALESCE(p.reference,''),COALESCE(p.file_name,''),p.status,
		        COALESCE(p.review_note,''),p.created_at,ap.full_name,a.registration_number,c.stage
		 FROM payment_submissions p
		 JOIN applications a ON a.id=p.application_id
		 JOIN applicants ap ON ap.id=a.applicant_id
		 JOIN charges c ON c.id=p.charge_id
		 WHERE p.school_id=$1 AND ($2='' OR p.status=$2)
		 ORDER BY p.created_at DESC LIMIT $3 OFFSET $4`,
		schoolID, status, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	type queued struct {
		item          map[string]any
		applicationID string
		stage         string
	}
	var list []queued
	for rows.Next() {
		var (
			id, applicationID, bank, account, reference, fileName, payStatus, reviewNote, stage string
			amount                                                                              int64
			transferAt, createdAt                                                               time.Time
			applicantName, registrationNumber                                                   string
		)
		if err := rows.Scan(&id, &applicationID, &amount, &transferAt, &bank, &account, &reference, &fileName,
			&payStatus, &reviewNote, &createdAt, &applicantName, &registrationNumber, &stage); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id":                  id,
			"application_id":      applicationID,
			"applicant_name":      applicantName,
			"registration_number": registrationNumber,
			"amount":              amount,
			"transfer_at":         transferAt,
			"bank_name":           bank,
			"account_name":        account,
			"reference":           reference,
			"file_name":           fileName,
			"status":              payStatus,
			"stage":               stage,
			"stage_label":         stageLabel(stage),
			"created_at":          createdAt,
		}
		if reviewNote != "" {
			item["review_note"] = reviewNote
		}
		list = append(list, queued{item, applicationID, stage})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	items := []map[string]any{}
	for _, q := range list {
		charge, err := latestCharge(r.Context(), a.DB, schoolID, q.applicationID, q.stage)
		if err != nil {
			writeErr(w, err)
			return
		}
		q.item["outstanding"] = int64(0)
		q.item["installments"] = []map[string]any{}
		if charge != nil {
			view, err := a.chargeView(r.Context(), schoolID, q.applicationID, charge.ID)
			if err != nil {
				writeErr(w, err)
				return
			}
			q.item["outstanding"] = view["outstanding"]
			q.item["installments"] = view["installments"]
			q.item["min_payment"] = view["min_payment"]
		}
		items = append(items, q.item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

// ReviewPayment is POST /admin/payments/{id}/review. A verified payment is
// allocated to the applicant's own installments: explicit allocations must
// reference installments of this application's stage charge, otherwise the
// amount is spread over the open installments in order.
func (a *App) ReviewPayment(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
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
	if target == model.PaymentRejected && strings.TrimSpace(in.Note) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "a reason is required to reject a payment")
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
		status, uploaderType, uploaderID, applicationID, stage string
		amount                                                 int64
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT ps.status,ps.uploader_type,ps.uploader_id,ps.amount_idr,ps.application_id,c.stage
		 FROM payment_submissions ps JOIN charges c ON c.id=ps.charge_id
		 WHERE ps.id=$1 AND ps.school_id=$2 FOR UPDATE OF ps`,
		paymentID, schoolID).Scan(&status, &uploaderType, &uploaderID, &amount, &applicationID, &stage)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	current := model.PaymentStatus(status)
	if current == model.PaymentSubmitted {
		current = model.PaymentPendingReview
	}
	if err = model.ValidatePaymentTransition(current, target); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if target == model.PaymentVerified && uploaderType == "admin" && uploaderID == claims.UserID {
		common.WriteError(w, http.StatusConflict, "the uploader cannot verify their own payment")
		return
	}
	if target == model.PaymentVerified {
		open, valid, err := openInstallments(r.Context(), tx, schoolID, applicationID, stage)
		if err != nil {
			writeErr(w, err)
			return
		}
		allocations := in.Allocations
		if len(allocations) == 0 {
			if allocations, err = model.AutoAllocate(amount, open); err != nil {
				common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
		}
		if err = model.ValidateAllocations(amount, allocations); err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		for _, alloc := range allocations {
			if !valid[alloc.InstallmentID] {
				common.WriteError(w, http.StatusUnprocessableEntity, "alokasi harus ke cicilan milik pendaftar ini")
				return
			}
			if _, err = tx.ExecContext(r.Context(),
				`INSERT INTO payment_allocations(school_id,payment_id,charge_installment_id,amount_idr) VALUES($1,$2,$3,$4)`,
				schoolID, paymentID, alloc.InstallmentID, alloc.Amount); err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE payment_submissions SET status=$1,review_note=NULLIF($2,''),updated_at=now() WHERE id=$3`,
		target, in.Note, paymentID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO payment_reviews(school_id,payment_id,action,reviewer_id,reason) VALUES($1,$2,$3,$4,NULLIF($5,''))`,
		schoolID, paymentID, target, claims.UserID, in.Note); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,before_data,after_data,reason)
		 VALUES($1,'application',$2,'payment_reviewed','admin',$3,$4,$5,$6,NULLIF($7,''))`,
		schoolID, applicationID, claims.UserID, claims.Name, jsonRaw(map[string]any{"payment_id": paymentID, "status": status}),
		jsonRaw(map[string]any{"payment_id": paymentID, "status": target, "amount": amount, "stage": stage}), in.Note); err != nil {
		writeErr(w, err)
		return
	}
	if target == model.PaymentRejected {
		if err = a.notify(r.Context(), tx, schoolID, applicationID, "payment_rejected", "Bukti pembayaran ditolak",
			"Bukti pembayaran "+stageLabel(stage)+" ditolak: "+in.Note+". Silakan unggah ulang bukti yang benar.",
			"payment_rejected:"+paymentID, map[string]any{"payment_id": paymentID}); err != nil {
			writeErr(w, err)
			return
		}
	} else {
		if err = a.notify(r.Context(), tx, schoolID, applicationID, "payment_verified", "Pembayaran terverifikasi",
			"Pembayaran "+stageLabel(stage)+" sebesar "+formatIDR(amount)+" telah diverifikasi.",
			"payment_verified:"+paymentID, map[string]any{"payment_id": paymentID}); err != nil {
			writeErr(w, err)
			return
		}
		if err = a.afterPaymentVerified(r.Context(), tx, schoolID, applicationID, stage); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": paymentID, "status": target}, nil)
}

// ReversePayment is POST /admin/payments/{id}/reversal.
func (a *App) ReversePayment(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Reason == "" {
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
		status, applicationID string
		amount                int64
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,amount_idr,application_id FROM payment_submissions WHERE id=$1 AND school_id=$2 FOR UPDATE`, paymentID, schoolID).
		Scan(&status, &amount, &applicationID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "payment not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = model.ValidatePaymentTransition(model.PaymentStatus(status), model.PaymentReversed); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = randomKey()
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO payment_reversals(school_id,payment_id,amount_idr,reason,actor_id,idempotency_key)
		 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(school_id,idempotency_key) DO NOTHING`,
		schoolID, paymentID, amount, in.Reason, claims.UserID, idempotencyKey); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE payment_submissions SET status='reversed',review_note=$1,updated_at=now() WHERE id=$2`,
		in.Reason, paymentID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "application", applicationID, "payment_reversed", "admin", claims.UserID, claims.Name, in.Reason,
		map[string]any{"payment_id": paymentID, "status": status}, map[string]any{"payment_id": paymentID, "status": model.PaymentReversed})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": paymentID, "status": "reversed"}, nil)
}
