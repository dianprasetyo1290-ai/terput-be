package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"skci/admission/internal/model"
)

// Billing stages: the form fee (formulir) and re-registration (daftar ulang +
// uang pangkal). Each stage has its own charge revisions, installments and
// paid total, all scoped to one application.
const (
	stageRegistration   = "registration"
	stageReregistration = "reregistration"
)

// queryer is satisfied by *sql.DB and *sql.Tx.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type feeItem struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	AmountIDR    int64  `json:"amount_idr"`
	Discountable bool   `json:"discountable"`
}

// stageItemCodes lists the fee item codes billed in each stage.
func stageItemCodes(stage string) []string {
	if stage == stageReregistration {
		return []string{"reregistration", "entrance"}
	}
	return []string{"registration"}
}

func stageLabel(stage string) string {
	if stage == stageReregistration {
		return "Daftar ulang"
	}
	return "Formulir pendaftaran"
}

// discountSnapshot is what discount_applications.snapshot stores: the applied
// step plus the candidate terms, so a frozen bill can be rebuilt without
// re-reading (possibly edited or expired) promo rows (BR-PPDB-01).
type discountSnapshot struct {
	model.DiscountStep
	Origin    string             `json:"origin"`
	Kind      model.DiscountKind `json:"kind"`
	Value     int64              `json:"value"`
	Cap       int64              `json:"cap"`
	Exclusive bool               `json:"exclusive"`
	CreatedAt time.Time          `json:"created_at"`
}

type candidate struct {
	model.DiscountCandidate
	origin string
}

type chargeRow struct {
	ID         string
	Stage      string
	SchemeID   string
	Revision   int
	Gross      int64
	Discount   int64
	Net        int64
	MinPayment int64
	DueAt      sql.NullTime
	Cancelled  sql.NullTime
	Items      []byte
}

// latestCharge returns the newest charge revision of a stage, or nil.
func latestCharge(ctx context.Context, q queryer, schoolID, applicationID, stage string) (*chargeRow, error) {
	var c chargeRow
	err := q.QueryRowContext(ctx,
		`SELECT id,stage,fee_scheme_id,revision,gross_idr,discount_idr,net_idr,min_payment_idr,due_at,cancelled_at,items_snapshot
		 FROM charges WHERE school_id=$1 AND application_id=$2 AND stage=$3 ORDER BY revision DESC LIMIT 1`,
		schoolID, applicationID, stage).
		Scan(&c.ID, &c.Stage, &c.SchemeID, &c.Revision, &c.Gross, &c.Discount, &c.Net, &c.MinPayment, &c.DueAt, &c.Cancelled, &c.Items)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// stagePaid is the verified amount an application paid toward one stage.
// Reversed payments are no longer 'verified', so they drop out on their own.
func stagePaid(ctx context.Context, q queryer, schoolID, applicationID, stage string) (int64, error) {
	var paid int64
	err := q.QueryRowContext(ctx,
		`SELECT COALESCE(sum(pa.amount_idr),0)
		 FROM payment_allocations pa
		 JOIN payment_submissions ps ON ps.id=pa.payment_id
		 JOIN charges c ON c.id=ps.charge_id
		 WHERE ps.school_id=$1 AND ps.application_id=$2 AND c.stage=$3 AND ps.status='verified'`,
		schoolID, applicationID, stage).Scan(&paid)
	return paid, err
}

// pendingOnTime reports whether a proof for the stage that was transferred on
// or before the deadline is still waiting for review.
func pendingOnTime(ctx context.Context, q queryer, schoolID, applicationID, stage string, deadline time.Time) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM payment_submissions ps JOIN charges c ON c.id=ps.charge_id
		   WHERE ps.school_id=$1 AND ps.application_id=$2 AND c.stage=$3
		     AND ps.status IN('submitted','pending_review') AND COALESCE(ps.transfer_at,ps.created_at)<=$4)`,
		schoolID, applicationID, stage, deadline).Scan(&ok)
	return ok, err
}

// stageSettled reports whether the stage's minimum (full amount for the form
// fee, BR-PPDB-04 minimum for re-registration) is paid.
func stageSettled(ctx context.Context, q queryer, schoolID, applicationID, stage string) (bool, *chargeRow, int64, error) {
	charge, err := latestCharge(ctx, q, schoolID, applicationID, stage)
	if err != nil || charge == nil {
		return false, charge, 0, err
	}
	paid, err := stagePaid(ctx, q, schoolID, applicationID, stage)
	if err != nil {
		return false, charge, 0, err
	}
	return charge.Cancelled.Valid == false && paid >= charge.MinPayment, charge, paid, nil
}

type issueOptions struct {
	// Rebuild recomputes an already issued stage (voucher redeemed, special
	// discount approved). Promo rules stay frozen at their issued terms.
	Rebuild bool
}

// issueCharge issues (or, idempotently, returns) the stage charge.
func (a *App) issueCharge(ctx context.Context, schoolID, applicationID, stage string, opts issueOptions) (string, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	id, err := a.issueChargeTx(ctx, tx, schoolID, applicationID, stage, opts)
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// issueChargeTx computes and stores a charge for one stage of one application.
//
//   - BR-PPDB-06: the form stage bills the registration fee only.
//   - TC-15-010/011: the re-registration stage is only for accepted applicants
//     and is issued once (no duplicate bill).
//   - BR-PPDB-01: promos are evaluated at issuance (validity window in the
//     school timezone, quota, conditions) and frozen afterwards.
//   - BR-PPDB-02: stack/max mode and per-applicant maximum from the period.
//   - BR-PPDB-03: only approved special discounts count.
//   - BUG-003: installments are generated per charge (per applicant).
func (a *App) issueChargeTx(ctx context.Context, tx *sql.Tx, schoolID, applicationID, stage string, opts issueOptions) (string, error) {
	var (
		periodID, waveID, trackID, status, guardianID, applicantID string
		paymentEnd, regEnd, reregEnd, reregDue                     sql.NullTime
		mode                                                       string
		maxDiscount                                                int64
		waitlistDays                                               int
	)
	err := tx.QueryRowContext(ctx,
		`SELECT a.period_id,a.wave_id,a.track_id,a.status,ap.guardian_id,ap.id,w.payment_end,w.registration_end,
		        w.reregistration_end,a.reregistration_due_at,p.discount_mode,p.max_discount_idr,w.waitlist_reregistration_days
		 FROM applications a
		 JOIN applicants ap ON ap.id=a.applicant_id
		 JOIN admission_waves w ON w.id=a.wave_id
		 JOIN admission_periods p ON p.id=a.period_id
		 WHERE a.id=$1 AND a.school_id=$2 FOR UPDATE OF a`, applicationID, schoolID).
		Scan(&periodID, &waveID, &trackID, &status, &guardianID, &applicantID, &paymentEnd, &regEnd,
			&reregEnd, &reregDue, &mode, &maxDiscount, &waitlistDays)
	if err == sql.ErrNoRows {
		return "", &httpError{http.StatusNotFound, "pendaftaran tidak ditemukan"}
	}
	if err != nil {
		return "", err
	}
	switch model.ApplicationStatus(status) {
	case model.StatusWithdrawn, model.StatusRejected, model.StatusDropped:
		return "", &httpError{http.StatusUnprocessableEntity, "pendaftaran sudah tidak aktif; tagihan tidak dapat diterbitkan"}
	}
	if stage == stageReregistration {
		switch model.ApplicationStatus(status) {
		case model.StatusOffered, model.StatusReregistration, model.StatusConverted:
		default:
			return "", &httpError{http.StatusUnprocessableEntity, "tagihan daftar ulang hanya untuk pendaftar yang dinyatakan diterima"}
		}
	}

	previous, err := latestCharge(ctx, tx, schoolID, applicationID, stage)
	if err != nil {
		return "", err
	}
	if previous != nil {
		if previous.Cancelled.Valid {
			return "", &httpError{http.StatusUnprocessableEntity, "tagihan sudah dibatalkan"}
		}
		if !opts.Rebuild {
			return previous.ID, nil
		}
	}

	var (
		schemeID         string
		installmentCount int
		minPct           int
	)
	err = tx.QueryRowContext(ctx,
		`SELECT id,installment_count,min_reregistration_pct FROM fee_schemes
		 WHERE school_id=$1 AND period_id=$2 AND (track_id=$3 OR track_id IS NULL) AND (wave_id=$4 OR wave_id IS NULL)
		   AND status IN('published','draft')
		 ORDER BY (wave_id IS NOT NULL) DESC,(track_id IS NOT NULL) DESC,(status='published') DESC,revision DESC LIMIT 1`,
		schoolID, periodID, trackID, waveID).Scan(&schemeID, &installmentCount, &minPct)
	if err == sql.ErrNoRows {
		return "", &httpError{http.StatusUnprocessableEntity, "skema biaya untuk periode, gelombang, dan jalur ini belum dikonfigurasi"}
	}
	if err != nil {
		return "", err
	}
	if previous != nil {
		// A rebuilt bill keeps the scheme it was issued with.
		schemeID = previous.SchemeID
	}
	items, gross, err := loadFeeItems(ctx, tx, schoolID, schemeID, stageItemCodes(stage))
	if err != nil {
		return "", err
	}

	now := time.Now()
	var candidates []candidate
	if previous != nil {
		candidates, err = frozenRuleCandidates(ctx, tx, previous.ID)
	} else {
		var sibling bool
		if sibling, err = hasSibling(ctx, tx, schoolID, guardianID, applicantID); err == nil {
			candidates, err = ruleCandidates(ctx, tx, schoolID, periodID, applicationID, stage, now,
				model.ApplicantFacts{TrackID: trackID, WaveID: waveID, HasSibling: sibling})
		}
	}
	if err != nil {
		return "", err
	}
	more, err := voucherAndManualCandidates(ctx, tx, schoolID, applicationID, stage)
	if err != nil {
		return "", err
	}
	candidates = append(candidates, more...)

	plain := make([]model.DiscountCandidate, len(candidates))
	byID := map[string]candidate{}
	for i, c := range candidates {
		plain[i] = c.DiscountCandidate
		byID[c.ID] = c
	}
	net, steps, err := model.ApplyDiscountPolicy(gross, plain, mode, maxDiscount)
	if err != nil {
		return "", &httpError{http.StatusUnprocessableEntity, err.Error()}
	}

	var (
		minPayment int64
		firstDue   time.Time
		count      = 1
		pct        = 100
	)
	if stage == stageReregistration {
		count, pct = installmentCount, minPct
		minPayment = model.MinimumPayment(net, pct)
		switch {
		case reregDue.Valid:
			firstDue = reregDue.Time
		case reregEnd.Valid:
			firstDue = reregEnd.Time
		default:
			firstDue = now.AddDate(0, 0, waitlistDays)
		}
	} else {
		minPayment = net
		switch {
		case paymentEnd.Valid:
			firstDue = paymentEnd.Time
		case regEnd.Valid:
			firstDue = regEnd.Time
		default:
			firstDue = now
		}
	}

	snapshots := make([]discountSnapshot, len(steps))
	for i, step := range steps {
		c := byID[step.ID]
		snapshots[i] = discountSnapshot{DiscountStep: step, Origin: c.origin, Kind: c.Kind, Value: c.Value,
			Cap: c.Cap, Exclusive: c.Exclusive, CreatedAt: c.CreatedAt}
	}
	itemJSON, _ := json.Marshal(items)
	stepJSON, _ := json.Marshal(snapshots)

	var revision int
	if err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(max(revision),0)+1 FROM charges WHERE application_id=$1`, applicationID).Scan(&revision); err != nil {
		return "", err
	}
	var chargeID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO charges(school_id,application_id,revision,fee_scheme_id,gross_idr,discount_idr,net_idr,items_snapshot,
		                     discount_snapshot,stage,min_payment_idr,due_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
		schoolID, applicationID, revision, schemeID, gross, gross-net, net, itemJSON, stepJSON, stage, minPayment, firstDue).Scan(&chargeID)
	if err != nil {
		return "", err
	}
	for i, snap := range snapshots {
		raw, _ := json.Marshal(snap)
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO discount_applications(school_id,charge_id,sequence,source,source_id,base_before_idr,requested_idr,applied_idr,base_after_idr,snapshot,origin)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			schoolID, chargeID, i+1, snap.Label, snap.ID, snap.BaseBefore, snap.Requested, snap.Applied, snap.BaseAfter, raw, snap.Origin); err != nil {
			return "", err
		}
	}
	for _, line := range model.PlanInstallments(net, count, pct, firstDue) {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO charge_installments(school_id,charge_id,application_id,sequence,description,amount_idr,due_at)
			 VALUES($1,$2,$3,$4,$5,$6,$7)`,
			schoolID, chargeID, applicationID, line.Sequence, line.Description, line.Amount, line.DueAt); err != nil {
			return "", err
		}
	}
	action := "charge_issued"
	if previous != nil {
		action = "charge_rebuilt"
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'application',$2,$3,'system','admission','',$4)`,
		schoolID, applicationID, action, jsonRaw(map[string]any{
			"charge_id": chargeID, "stage": stage, "gross": gross, "discount": gross - net, "net": net, "min_payment": minPayment,
		})); err != nil {
		return "", err
	}
	return chargeID, nil
}

func loadFeeItems(ctx context.Context, q queryer, schoolID, schemeID string, codes []string) ([]feeItem, int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id,code,name,amount_idr,discountable FROM fee_items
		 WHERE fee_scheme_id=$1 AND school_id=$2 AND code=ANY($3) ORDER BY code`,
		schemeID, schoolID, codes)
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

// hasSibling: the guardian has another child who is accepted or already an
// active student (converted).
func hasSibling(ctx context.Context, q queryer, schoolID, guardianID, applicantID string) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
		   WHERE a.school_id=$1 AND ap.guardian_id=$2 AND ap.id<>$3 AND ap.deleted_at IS NULL
		     AND a.status IN('offered','reregistration','converted'))`,
		schoolID, guardianID, applicantID).Scan(&ok)
	return ok, err
}

// ruleCandidates selects the promo rules that apply to this applicant at
// issuance. Rule rows are locked so the quota check is serialized.
func ruleCandidates(ctx context.Context, tx *sql.Tx, schoolID, periodID, applicationID, stage string, now time.Time, facts model.ApplicantFacts) ([]candidate, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id,name,kind,value,COALESCE(cap_idr,0),priority,created_at,stackable,conditions,valid_from,valid_until,quota
		 FROM discount_rules
		 WHERE school_id=$1 AND period_id=$2 AND active AND status='active' AND applies_to=$3
		 ORDER BY priority,created_at,id FOR UPDATE`, schoolID, periodID, stage)
	if err != nil {
		return nil, err
	}
	type ruleRow struct {
		c           candidate
		conditions  []byte
		from, until sql.NullTime
		quota       int
	}
	var rules []ruleRow
	for rows.Next() {
		var (
			r         ruleRow
			stackable bool
		)
		if err := rows.Scan(&r.c.ID, &r.c.Source, &r.c.Kind, &r.c.Value, &r.c.Cap, &r.c.Priority, &r.c.CreatedAt,
			&stackable, &r.conditions, &r.from, &r.until, &r.quota); err != nil {
			rows.Close()
			return nil, err
		}
		r.c.Exclusive = !stackable
		r.c.origin = "rule"
		rules = append(rules, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []candidate
	for _, r := range rules {
		if !model.PromoActive(now, nullTimePtr(r.from), nullTimePtr(r.until)) {
			continue
		}
		cond, err := model.ParseDiscountConditions(r.conditions)
		if err != nil || !cond.Matches(facts) {
			continue
		}
		if r.quota > 0 {
			var used int
			if err := tx.QueryRowContext(ctx,
				`SELECT count(DISTINCT c.application_id) FROM discount_applications da JOIN charges c ON c.id=da.charge_id
				 WHERE da.school_id=$1 AND da.origin='rule' AND da.source_id=$2 AND c.cancelled_at IS NULL AND c.application_id<>$3`,
				schoolID, r.c.ID, applicationID).Scan(&used); err != nil {
				return nil, err
			}
			if !model.PromoQuotaAvailable(r.quota, used) {
				continue
			}
		}
		out = append(out, r.c)
	}
	return out, nil
}

// frozenRuleCandidates rebuilds the promo candidates exactly as they were
// issued on the previous revision.
func frozenRuleCandidates(ctx context.Context, tx *sql.Tx, chargeID string) ([]candidate, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT snapshot FROM discount_applications WHERE charge_id=$1 AND origin='rule' ORDER BY sequence`, chargeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []candidate
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var snap discountSnapshot
		if err := json.Unmarshal(raw, &snap); err != nil || snap.Kind == "" {
			continue
		}
		out = append(out, candidate{origin: "rule", DiscountCandidate: model.DiscountCandidate{
			ID: snap.ID, Source: snap.Label, Kind: snap.Kind, Value: snap.Value, Cap: snap.Cap,
			Priority: snap.Priority, CreatedAt: snap.CreatedAt, Exclusive: snap.Exclusive,
		}})
	}
	return out, rows.Err()
}

func voucherAndManualCandidates(ctx context.Context, tx *sql.Tx, schoolID, applicationID, stage string) ([]candidate, error) {
	var out []candidate
	var (
		c    candidate
		hint sql.NullString
	)
	err := tx.QueryRowContext(ctx,
		`SELECT v.id,v.kind,v.value,COALESCE(v.cap_idr,0),v.priority,v.created_at,v.code_hint
		 FROM voucher_redemptions vr JOIN discount_vouchers v ON v.id=vr.voucher_id
		 WHERE vr.school_id=$1 AND vr.application_id=$2 AND vr.stage=$3`, schoolID, applicationID, stage).
		Scan(&c.ID, &c.Kind, &c.Value, &c.Cap, &c.Priority, &c.CreatedAt, &hint)
	switch {
	case err == nil:
		c.Source = "Voucher"
		if hint.Valid && hint.String != "" {
			c.Source = "Voucher " + hint.String
		}
		c.origin = "voucher"
		out = append(out, c)
	case err != sql.ErrNoRows:
		return nil, err
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id,kind,value,COALESCE(cap_idr,0),priority,created_at,reason
		 FROM manual_discounts WHERE school_id=$1 AND application_id=$2 AND stage=$3 AND status='approved'
		 ORDER BY priority,created_at,id`, schoolID, applicationID, stage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			m      candidate
			reason string
		)
		if err := rows.Scan(&m.ID, &m.Kind, &m.Value, &m.Cap, &m.Priority, &m.CreatedAt, &reason); err != nil {
			return nil, err
		}
		m.Source = "Diskon khusus: " + reason
		m.origin = "manual"
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// chargeInstallments loads a charge's own installment lines.
func chargeInstallments(ctx context.Context, q queryer, schoolID, chargeID string) ([]string, []model.InstallmentLine, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id,sequence,description,amount_idr,due_at FROM charge_installments
		 WHERE charge_id=$1 AND school_id=$2 ORDER BY sequence`, chargeID, schoolID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var (
		ids   []string
		lines []model.InstallmentLine
	)
	for rows.Next() {
		var (
			id string
			l  model.InstallmentLine
		)
		if err := rows.Scan(&id, &l.Sequence, &l.Description, &l.Amount, &l.DueAt); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		lines = append(lines, l)
	}
	return ids, lines, rows.Err()
}

// chargeView renders one charge with its discount steps, the applicant's own
// paid total for the stage and the applicant's own installments.
func (a *App) chargeView(ctx context.Context, schoolID, applicationID, chargeID string) (map[string]any, error) {
	var c chargeRow
	err := a.DB.QueryRowContext(ctx,
		`SELECT id,stage,fee_scheme_id,revision,gross_idr,discount_idr,net_idr,min_payment_idr,due_at,cancelled_at,items_snapshot
		 FROM charges WHERE id=$1 AND school_id=$2 AND application_id=$3`, chargeID, schoolID, applicationID).
		Scan(&c.ID, &c.Stage, &c.SchemeID, &c.Revision, &c.Gross, &c.Discount, &c.Net, &c.MinPayment, &c.DueAt, &c.Cancelled, &c.Items)
	if err == sql.ErrNoRows {
		return nil, &httpError{http.StatusNotFound, "tagihan tidak ditemukan"}
	}
	if err != nil {
		return nil, err
	}

	var items []feeItem
	_ = json.Unmarshal(c.Items, &items)
	lines := make([]map[string]any, 0, len(items))
	for _, item := range items {
		lines = append(lines, map[string]any{
			"id": item.ID, "code": item.Code, "name": item.Name, "amount": item.AmountIDR, "discount": 0, "net_amount": item.AmountIDR,
		})
	}

	discounts := []map[string]any{}
	stepRows, err := a.DB.QueryContext(ctx,
		`SELECT source_id,origin,COALESCE(snapshot->>'label',source),base_before_idr,applied_idr,base_after_idr
		 FROM discount_applications WHERE charge_id=$1 AND school_id=$2 ORDER BY sequence`, chargeID, schoolID)
	if err != nil {
		return nil, err
	}
	for stepRows.Next() {
		var (
			id, origin, label          string
			baseBefore, applied, after int64
		)
		if err := stepRows.Scan(&id, &origin, &label, &baseBefore, &applied, &after); err != nil {
			stepRows.Close()
			return nil, err
		}
		source := map[string]string{"rule": "automatic", "voucher": "voucher", "manual": "manual"}[origin]
		discounts = append(discounts, map[string]any{
			"id": id, "label": label, "source": source, "base_before": baseBefore, "applied_amount": applied, "base_after": after,
		})
	}
	stepRows.Close()

	paid, err := stagePaid(ctx, a.DB, schoolID, applicationID, c.Stage)
	if err != nil {
		return nil, err
	}
	ids, planned, err := chargeInstallments(ctx, a.DB, schoolID, chargeID)
	if err != nil {
		return nil, err
	}
	installments := []map[string]any{}
	for i, st := range model.SettleInstallments(planned, paid, time.Now()) {
		installments = append(installments, map[string]any{
			"id": ids[i], "label": st.Description, "sequence": st.Sequence, "due_at": st.DueAt,
			"amount": st.Amount, "paid_amount": st.Paid, "status": st.Status,
		})
	}
	outstanding := c.Net - paid
	if outstanding < 0 {
		outstanding = 0
	}
	status := "active"
	switch {
	case c.Cancelled.Valid:
		status = "cancelled"
	case paid >= c.Net:
		status = "paid"
	case paid >= c.MinPayment:
		status = "minimum_paid"
	}
	view := map[string]any{
		"id":              c.ID,
		"application_id":  applicationID,
		"revision":        c.Revision,
		"stage":           c.Stage,
		"stage_label":     stageLabel(c.Stage),
		"status":          status,
		"currency":        "IDR",
		"subtotal":        c.Gross,
		"discount_total":  c.Discount,
		"total":           c.Net,
		"paid":            paid,
		"outstanding":     outstanding,
		"min_payment":     c.MinPayment,
		"min_payment_met": paid >= c.MinPayment,
		"lines":           lines,
		"discounts":       discounts,
		"installments":    installments,
	}
	if c.DueAt.Valid {
		view["due_at"] = c.DueAt.Time
	}
	if c.Cancelled.Valid {
		view["cancelled_at"] = c.Cancelled.Time
	}
	return view, nil
}

// currentStage is the stage the guardian is expected to pay now.
func currentStage(status string) string {
	switch model.ApplicationStatus(status) {
	case model.StatusOffered, model.StatusReregistration, model.StatusConverted:
		return stageReregistration
	}
	return stageRegistration
}

// openInstallments returns the latest stage charge's installments with their
// still-open amounts (waterfall over the stage's paid total).
func openInstallments(ctx context.Context, q queryer, schoolID, applicationID, stage string) ([]model.InstallmentBalance, map[string]bool, error) {
	charge, err := latestCharge(ctx, q, schoolID, applicationID, stage)
	if err != nil || charge == nil {
		return nil, nil, err
	}
	paid, err := stagePaid(ctx, q, schoolID, applicationID, stage)
	if err != nil {
		return nil, nil, err
	}
	ids, lines, err := chargeInstallments(ctx, q, schoolID, charge.ID)
	if err != nil {
		return nil, nil, err
	}
	valid := map[string]bool{}
	out := make([]model.InstallmentBalance, 0, len(ids))
	for i, st := range model.SettleInstallments(lines, paid, time.Now()) {
		valid[ids[i]] = true
		out = append(out, model.InstallmentBalance{ID: ids[i], Remaining: st.Amount - st.Paid})
	}
	return out, valid, nil
}
