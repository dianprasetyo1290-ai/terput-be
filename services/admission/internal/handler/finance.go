package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// ---------------------------------------------------------------------------
// Charge computation
// ---------------------------------------------------------------------------

type feeItem struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	AmountIDR    int64  `json:"amount_idr"`
	Discountable bool   `json:"discountable"`
}

// GetCharge is GET /guardian/applications/{id}/charge. The charge is computed
// lazily from the fee scheme + active rules + manual discounts + any voucher
// already redeemed for the application.
func (a *App) GetCharge(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	chargeID, err := a.ensureCharge(r.Context(), c.SchoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := a.chargeView(r.Context(), c.SchoolID, applicationID, chargeID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, view, nil)
}

// ensureCharge returns the newest charge, creating one if the application has
// none yet.
func (a *App) ensureCharge(ctx context.Context, schoolID, applicationID string) (string, error) {
	var id string
	err := a.DB.QueryRowContext(ctx,
		`SELECT id FROM charges WHERE application_id=$1 AND school_id=$2 ORDER BY revision DESC LIMIT 1`,
		applicationID, schoolID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	return a.buildCharge(ctx, schoolID, applicationID, "")
}

// buildCharge recomputes and persists a new charge revision. Passing voucherID
// applies a freshly validated voucher; a voucher already redeemed for the
// application is always re-applied so the snapshot stays consistent.
func (a *App) buildCharge(ctx context.Context, schoolID, applicationID, voucherID string) (string, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var periodID, trackID string
	err = tx.QueryRowContext(ctx,
		`SELECT period_id,track_id FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).
		Scan(&periodID, &trackID)
	if err == sql.ErrNoRows {
		return "", &httpError{http.StatusNotFound, "application not found"}
	}
	if err != nil {
		return "", err
	}

	var schemeID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM fee_schemes
		 WHERE school_id=$1 AND period_id=$2 AND (track_id=$3 OR track_id IS NULL) AND status IN('published','draft')
		 ORDER BY (track_id=$3) DESC, revision DESC LIMIT 1`, schoolID, periodID, trackID).Scan(&schemeID)
	if err == sql.ErrNoRows {
		return "", &httpError{http.StatusUnprocessableEntity, "no fee scheme configured for this period/track"}
	}
	if err != nil {
		return "", err
	}

	items, gross, err := a.loadFeeItems(ctx, tx, schoolID, schemeID)
	if err != nil {
		return "", err
	}

	existingVoucher, err := a.redeemedVoucher(ctx, tx, schoolID, applicationID)
	if err != nil {
		return "", err
	}
	if voucherID != "" && existingVoucher != "" && voucherID != existingVoucher {
		return "", &httpError{http.StatusConflict, "a voucher is already applied to this application"}
	}
	useVoucher := voucherID
	if useVoucher == "" {
		useVoucher = existingVoucher
	}

	candidates, err := a.discountCandidates(ctx, tx, schoolID, periodID, applicationID, useVoucher)
	if err != nil {
		return "", err
	}

	net, steps, err := model.ApplyDiscounts(gross, candidates)
	if err != nil {
		return "", &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	itemJSON, _ := json.Marshal(items)
	stepJSON, _ := json.Marshal(steps)

	var revision int
	if err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(max(revision),0)+1 FROM charges WHERE application_id=$1`, applicationID).Scan(&revision); err != nil {
		return "", err
	}
	var chargeID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO charges(school_id,application_id,revision,fee_scheme_id,gross_idr,discount_idr,net_idr,items_snapshot,discount_snapshot)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
		schoolID, applicationID, revision, schemeID, gross, gross-net, net, itemJSON, stepJSON).Scan(&chargeID)
	if err != nil {
		return "", err
	}
	for i, step := range steps {
		snapshot, _ := json.Marshal(step)
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO discount_applications(school_id,charge_id,sequence,source,source_id,base_before_idr,requested_idr,applied_idr,base_after_idr,snapshot)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			schoolID, chargeID, i+1, step.Source, step.ID, step.BaseBefore, step.Requested, step.Applied, step.BaseAfter, snapshot); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return chargeID, nil
}

func (a *App) loadFeeItems(ctx context.Context, tx *sql.Tx, schoolID, schemeID string) ([]feeItem, int64, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id,code,name,amount_idr,discountable FROM fee_items WHERE fee_scheme_id=$1 AND school_id=$2 ORDER BY code`,
		schemeID, schoolID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var (
		items []feeItem
		gross int64
	)
	for rows.Next() {
		var item feeItem
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.AmountIDR, &item.Discountable); err != nil {
			return nil, 0, err
		}
		gross += item.AmountIDR
		items = append(items, item)
	}
	return items, gross, rows.Err()
}

func (a *App) redeemedVoucher(ctx context.Context, tx *sql.Tx, schoolID, applicationID string) (string, error) {
	var voucherID string
	err := tx.QueryRowContext(ctx,
		`SELECT voucher_id FROM voucher_redemptions WHERE school_id=$1 AND application_id=$2`, schoolID, applicationID).Scan(&voucherID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return voucherID, err
}

func (a *App) discountCandidates(ctx context.Context, tx *sql.Tx, schoolID, periodID, applicationID, voucherID string) ([]model.DiscountCandidate, error) {
	var out []model.DiscountCandidate

	ruleRows, err := tx.QueryContext(ctx,
		`SELECT id,name,kind,value,COALESCE(cap_idr,0),priority,created_at
		 FROM discount_rules WHERE school_id=$1 AND period_id=$2 AND active ORDER BY priority,created_at,id`,
		schoolID, periodID)
	if err != nil {
		return nil, err
	}
	defer ruleRows.Close()
	for ruleRows.Next() {
		var (
			candidate model.DiscountCandidate
			name      string
		)
		if err := ruleRows.Scan(&candidate.ID, &name, &candidate.Kind, &candidate.Value, &candidate.Cap, &candidate.Priority, &candidate.CreatedAt); err != nil {
			return nil, err
		}
		candidate.Source = name
		out = append(out, candidate)
	}
	if err := ruleRows.Err(); err != nil {
		return nil, err
	}

	if voucherID != "" {
		var (
			kind      model.DiscountKind
			value     int64
			cap       int64
			priority  int
			createdAt time.Time
			hint      sql.NullString
		)
		err := tx.QueryRowContext(ctx,
			`SELECT kind,value,COALESCE(cap_idr,0),priority,created_at,code_hint FROM discount_vouchers WHERE id=$1 AND school_id=$2`,
			voucherID, schoolID).Scan(&kind, &value, &cap, &priority, &createdAt, &hint)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if err == nil {
			label := "Voucher"
			if hint.Valid && hint.String != "" {
				label = "Voucher " + hint.String
			}
			out = append(out, model.DiscountCandidate{
				ID: voucherID, Source: label, Kind: kind, Value: value, Cap: cap, Priority: priority, CreatedAt: createdAt,
			})
		}
	}

	manualRows, err := tx.QueryContext(ctx,
		`SELECT id,kind,value,COALESCE(cap_idr,0),priority,created_at,reason
		 FROM manual_discounts WHERE school_id=$1 AND application_id=$2 ORDER BY priority,created_at,id`,
		schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	defer manualRows.Close()
	for manualRows.Next() {
		var (
			candidate model.DiscountCandidate
			reason    string
		)
		if err := manualRows.Scan(&candidate.ID, &candidate.Kind, &candidate.Value, &candidate.Cap, &candidate.Priority, &candidate.CreatedAt, &reason); err != nil {
			return nil, err
		}
		candidate.Source = "Manual: " + reason
		out = append(out, candidate)
	}
	return out, manualRows.Err()
}

// chargeView renders a charge row plus its discount steps, totals and
// installments for the frontend.
func (a *App) chargeView(ctx context.Context, schoolID, applicationID, chargeID string) (map[string]any, error) {
	var (
		id, schemeID  string
		revision      int
		gross, disc   int64
		net           int64
		itemsSnapshot []byte
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT id,revision,fee_scheme_id,gross_idr,discount_idr,net_idr,items_snapshot
		 FROM charges WHERE id=$1 AND school_id=$2`, chargeID, schoolID).
		Scan(&id, &revision, &schemeID, &gross, &disc, &net, &itemsSnapshot)
	if err != nil {
		return nil, err
	}

	var items []feeItem
	_ = json.Unmarshal(itemsSnapshot, &items)
	lines := make([]map[string]any, 0, len(items))
	for _, item := range items {
		lines = append(lines, map[string]any{
			"id":         item.ID,
			"name":       item.Name,
			"amount":     item.AmountIDR,
			"discount":   0,
			"net_amount": item.AmountIDR,
		})
	}

	discounts := []map[string]any{}
	stepRows, err := a.DB.QueryContext(ctx,
		`SELECT source,COALESCE(snapshot->>'label',source),base_before_idr,applied_idr,base_after_idr
		 FROM discount_applications WHERE charge_id=$1 ORDER BY sequence`, chargeID)
	if err != nil {
		return nil, err
	}
	for stepRows.Next() {
		var (
			source, label              string
			baseBefore, applied, after int64
		)
		if err := stepRows.Scan(&source, &label, &baseBefore, &applied, &after); err != nil {
			stepRows.Close()
			return nil, err
		}
		discounts = append(discounts, map[string]any{
			"id":             source,
			"label":          label,
			"source":         "automatic",
			"base_before":    baseBefore,
			"applied_amount": applied,
			"base_after":     after,
		})
	}
	stepRows.Close()

	paid, _, _, outstanding, err := a.paymentTotals(ctx, schoolID, applicationID, net)
	if err != nil {
		return nil, err
	}
	installments, err := a.installments(ctx, schoolID, schemeID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"id":             id,
		"application_id": applicationID,
		"revision":       revision,
		"currency":       "IDR",
		"subtotal":       gross,
		"discount_total": disc,
		"total":          net,
		"paid":           paid,
		"outstanding":    outstanding,
		"lines":          lines,
		"discounts":      discounts,
		"installments":   installments,
	}, nil
}

func (a *App) installments(ctx context.Context, schoolID, schemeID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT ip.id,ip.sequence,ip.description,ip.amount_idr,ip.due_at,
		        COALESCE((SELECT sum(pa.amount_idr) FROM payment_allocations pa
		                  JOIN payment_submissions ps ON ps.id=pa.payment_id
		                  WHERE pa.installment_id=ip.id AND ps.status='verified'),0)
		 FROM installment_plans ip WHERE ip.fee_scheme_id=$1 AND ip.school_id=$2 ORDER BY ip.sequence`,
		schemeID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, description string
			sequence        int
			amount, paid    int64
			dueAt           time.Time
		)
		if err := rows.Scan(&id, &sequence, &description, &amount, &dueAt, &paid); err != nil {
			return nil, err
		}
		status := "unpaid"
		switch {
		case paid >= amount && amount > 0:
			status = "paid"
		case paid > 0:
			status = "partial"
		case dueAt.Before(now):
			status = "overdue"
		}
		out = append(out, map[string]any{
			"id":          id,
			"label":       description,
			"due_at":      dueAt,
			"amount":      amount,
			"paid_amount": paid,
			"status":      status,
		})
	}
	return out, rows.Err()
}

// paymentTotals sums verified allocations and reversals to derive paid and
// outstanding balance for an application.
func (a *App) paymentTotals(ctx context.Context, schoolID, applicationID string, charge int64) (paid, verified, reversed, outstanding int64, err error) {
	if err = a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(pa.amount_idr),0)
		 FROM payment_allocations pa JOIN payment_submissions ps ON ps.id=pa.payment_id
		 WHERE ps.application_id=$1 AND ps.school_id=$2 AND ps.status='verified'`,
		applicationID, schoolID).Scan(&verified); err != nil {
		return
	}
	if err = a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(sum(pr.amount_idr),0)
		 FROM payment_reversals pr JOIN payment_submissions ps ON ps.id=pr.payment_id
		 WHERE ps.application_id=$1 AND ps.school_id=$2`,
		applicationID, schoolID).Scan(&reversed); err != nil {
		return
	}
	paid = verified - reversed
	outstanding = charge - paid
	if outstanding < 0 {
		outstanding = 0
	}
	return
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

	voucherID, err := a.matchVoucher(r.Context(), c.SchoolID, applicationID, in.Code)
	if err != nil {
		writeErr(w, err)
		return
	}
	chargeID, err := a.buildCharge(r.Context(), c.SchoolID, applicationID, voucherID)
	if err != nil {
		writeErr(w, err)
		return
	}
	view, err := a.chargeView(r.Context(), c.SchoolID, applicationID, chargeID)
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), c.SchoolID, "charge", chargeID, "voucher_applied", "guardian", c.GuardianID, "", "", nil,
		map[string]any{"voucher_id": voucherID})
	common.WriteJSON(w, http.StatusOK, view, nil)
}

// matchVoucher finds a usable voucher by code for the application's period and
// reserves it for the application exactly once.
func (a *App) matchVoucher(ctx context.Context, schoolID, applicationID, code string) (string, error) {
	var periodID string
	if err := a.DB.QueryRowContext(ctx,
		`SELECT period_id FROM applications WHERE id=$1 AND school_id=$2`, applicationID, schoolID).Scan(&periodID); err != nil {
		return "", &httpError{http.StatusNotFound, "application not found"}
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var existing string
	err = tx.QueryRowContext(ctx,
		`SELECT voucher_id FROM voucher_redemptions WHERE application_id=$1 FOR UPDATE`, applicationID).Scan(&existing)
	if err == nil && existing != "" {
		return "", &httpError{http.StatusConflict, "a voucher is already applied to this application"}
	}
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id,code_hash,status,valid_from,valid_until,usage_limit,used_count
		 FROM discount_vouchers WHERE school_id=$1 AND period_id=$2`, schoolID, periodID)
	if err != nil {
		return "", err
	}
	type voucherRow struct {
		id        string
		hash      string
		status    string
		validFrom time.Time
		validTo   time.Time
		limit     int
		used      int
	}
	var vouchers []voucherRow
	for rows.Next() {
		var v voucherRow
		if err := rows.Scan(&v.id, &v.hash, &v.status, &v.validFrom, &v.validTo, &v.limit, &v.used); err != nil {
			rows.Close()
			return "", err
		}
		vouchers = append(vouchers, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", err
	}

	now := time.Now()
	var matched *voucherRow
	for i := range vouchers {
		v := &vouchers[i]
		if v.status != "active" || now.Before(v.validFrom) || now.After(v.validTo) || v.used >= v.limit {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(v.hash), []byte(code)) == nil {
			matched = v
			break
		}
	}
	if matched == nil {
		return "", &httpError{http.StatusUnprocessableEntity, "voucher is invalid, expired or fully used"}
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO voucher_redemptions(school_id,application_id,voucher_id) VALUES($1,$2,$3)`,
		schoolID, applicationID, matched.id); err != nil {
		if isUniqueViolation(err) {
			return "", &httpError{http.StatusConflict, "a voucher is already applied to this application"}
		}
		return "", err
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE discount_vouchers SET used_count=used_count+1 WHERE id=$1`, matched.id); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return matched.id, nil
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
// payment is attached to the application's newest charge.
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

	var chargeID string
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM charges WHERE application_id=$1 AND school_id=$2 ORDER BY revision DESC LIMIT 1`,
		applicationID, c.SchoolID).Scan(&chargeID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusUnprocessableEntity, "no charge issued for this application")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}

	key := c.SchoolID + "/payments/" + randomKey()
	obj, err := a.Storage.Put(r.Context(), key, header.Header.Get("Content-Type"), file, header.Size)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
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
		c.SchoolID, applicationID, chargeID, amount, obj.Key, header.Filename, obj.MIME, obj.Size, obj.Checksum,
		nullTime(transferAt), r.FormValue("bank_name"), r.FormValue("account_name"), r.FormValue("reference"),
		c.GuardianID, idempotencyKey).Scan(&id)
	if err != nil {
		_ = a.Storage.Delete(r.Context(), obj.Key)
		common.WriteError(w, http.StatusBadRequest, "failed to submit payment")
		return
	}
	a.audit(r.Context(), c.SchoolID, "payment", id, "payment_submitted", "guardian", c.GuardianID, "", "", nil,
		map[string]any{"amount": amount, "charge_id": chargeID})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "submitted"}, nil)
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
		        COALESCE(p.review_note,''),p.created_at,ap.full_name,a.registration_number
		 FROM payment_submissions p
		 JOIN applications a ON a.id=p.application_id
		 JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE p.school_id=$1 AND ($2='' OR p.status=$2)
		 ORDER BY p.created_at DESC LIMIT $3 OFFSET $4`,
		schoolID, status, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, applicationID, bank, account, reference, fileName, payStatus, reviewNote string
			amount                                                                       int64
			transferAt, createdAt                                                        time.Time
			applicantName, registrationNumber                                            string
		)
		if err := rows.Scan(&id, &applicationID, &amount, &transferAt, &bank, &account, &reference, &fileName,
			&payStatus, &reviewNote, &createdAt, &applicantName, &registrationNumber); err != nil {
			writeErr(w, err)
			return
		}
		net, err := a.latestChargeTotal(r, schoolID, applicationID)
		if err != nil {
			writeErr(w, err)
			return
		}
		_, _, _, outstanding, err := a.paymentTotals(r.Context(), schoolID, applicationID, net)
		if err != nil {
			writeErr(w, err)
			return
		}
		installments, err := a.applicationInstallments(r, schoolID, applicationID)
		if err != nil {
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
			"created_at":          createdAt,
			"outstanding":         outstanding,
			"installments":        installments,
		}
		if reviewNote != "" {
			item["review_note"] = reviewNote
		}
		items = append(items, item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

func (a *App) latestChargeTotal(r *http.Request, schoolID, applicationID string) (int64, error) {
	var net int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT net_idr FROM charges WHERE application_id=$1 AND school_id=$2 ORDER BY revision DESC LIMIT 1`,
		applicationID, schoolID).Scan(&net)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return net, err
}

func (a *App) applicationInstallments(r *http.Request, schoolID, applicationID string) ([]map[string]any, error) {
	var schemeID string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT fee_scheme_id FROM charges WHERE application_id=$1 AND school_id=$2 ORDER BY revision DESC LIMIT 1`,
		applicationID, schoolID).Scan(&schemeID)
	if err == sql.ErrNoRows {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	return a.installments(r.Context(), schoolID, schemeID)
}

// ReviewPayment is POST /admin/payments/{id}/review.
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
	paymentID := param(r, "id")

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		status, uploaderType, uploaderID string
		amount                           int64
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,uploader_type,uploader_id,amount_idr FROM payment_submissions WHERE id=$1 AND school_id=$2 FOR UPDATE`,
		paymentID, schoolID).Scan(&status, &uploaderType, &uploaderID, &amount)
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
		if err = model.ValidateAllocations(amount, in.Allocations); err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		for _, alloc := range in.Allocations {
			if _, err = tx.ExecContext(r.Context(),
				`INSERT INTO payment_allocations(school_id,payment_id,installment_id,amount_idr)
				 VALUES($1,$2,NULLIF($3,'')::uuid,$4)`,
				schoolID, paymentID, alloc.InstallmentID, alloc.Amount); err != nil {
				common.WriteError(w, http.StatusUnprocessableEntity, "invalid allocation")
				return
			}
		}
	}
	if target == model.PaymentRejected && in.Note == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "a reason is required to reject a payment")
		return
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
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "payment", paymentID, "payment_reviewed", "admin", claims.UserID, "", in.Note,
		map[string]any{"status": status}, map[string]any{"status": target})
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
		status string
		amount int64
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT status,amount_idr FROM payment_submissions WHERE id=$1 AND school_id=$2 FOR UPDATE`, paymentID, schoolID).
		Scan(&status, &amount)
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
	a.audit(r.Context(), schoolID, "payment", paymentID, "payment_reversed", "admin", claims.UserID, "", in.Reason,
		map[string]any{"status": status}, map[string]any{"status": model.PaymentReversed})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": paymentID, "status": "reversed"}, nil)
}
