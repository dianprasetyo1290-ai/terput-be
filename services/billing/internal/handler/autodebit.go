package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/gateway"
	"skci/billing/internal/model"
)

// paymentAudit records a T14 mutation in billing's audit log (when the
// change belongs to a school) and in core's central audit (module payment).
func (a *App) paymentAudit(ctx context.Context, c *common.Claims, schoolID, entityType, entityID, action, reason string, before, after any) {
	actorID, actorRole := "system", "system"
	if c != nil {
		actorID, actorRole = c.UserID, c.ActiveRole
	}
	if schoolID != "" {
		a.audit(ctx, schoolID, entityType, entityID, action, actorRole, actorID, "", reason, before, after)
	}
	if a.Core != nil {
		_ = a.Core.Audit(ctx, client.AuditEvent{
			SchoolID: schoolID, Service: "billing", Module: "payment", EntityType: entityType, EntityID: entityID,
			Action: action, ActorUserID: actorID, ActorRole: actorRole, Before: before, After: after, Reason: reason,
		})
	}
}

// ---- settings -------------------------------------------------------------

func (a *App) loadAutodebitSettings(ctx context.Context, schoolID string) (gateway.Settings, error) {
	s := gateway.Settings{MaxRetries: 2, RunHour: -1}
	var interval, hour sql.NullInt64
	err := a.DB.QueryRowContext(ctx,
		`SELECT max_retries, retry_interval_days, run_hour FROM autodebit_settings WHERE school_id = $1`, schoolID).
		Scan(&s.MaxRetries, &interval, &hour)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if interval.Valid {
		s.RetryIntervalDays = int(interval.Int64)
	}
	if hour.Valid {
		s.RunHour = int(hour.Int64)
	}
	return s, nil
}

type autodebitSettingsDTO struct {
	MaxRetries        int  `json:"max_retries"`
	RetryIntervalDays *int `json:"retry_interval_days"`
	RunHour           *int `json:"run_hour"`
	Ready             bool `json:"ready"`
}

func settingsDTO(s gateway.Settings) autodebitSettingsDTO {
	d := autodebitSettingsDTO{MaxRetries: s.MaxRetries, Ready: s.Ready()}
	if s.RetryIntervalDays > 0 {
		v := s.RetryIntervalDays
		d.RetryIntervalDays = &v
	}
	if s.RunHour >= 0 {
		v := s.RunHour
		d.RunHour = &v
	}
	return d
}

// AdminGetAutodebitSettings: GET /admin/autodebit/settings (FR-151-002).
func (a *App) AdminGetAutodebitSettings(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentAutodebit) {
		return
	}
	s, err := a.loadAutodebitSettings(r.Context(), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, settingsDTO(s), nil)
}

// AdminPutAutodebitSettings: PUT /admin/autodebit/settings.
func (a *App) AdminPutAutodebitSettings(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentAutodebit) {
		return
	}
	var in autodebitSettingsDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.MaxRetries < 0 || in.RetryIntervalDays == nil || *in.RetryIntervalDays <= 0 || in.RunHour == nil || *in.RunHour < 0 || *in.RunHour > 23 {
		common.WriteError(w, http.StatusBadRequest, "jumlah retry ≥ 0, interval retry (hari) > 0, dan jam jalan 0–23 wajib diisi")
		return
	}
	before, _ := a.loadAutodebitSettings(r.Context(), schoolID)
	_, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO autodebit_settings (school_id, max_retries, retry_interval_days, run_hour, updated_by)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (school_id) DO UPDATE SET max_retries = $2, retry_interval_days = $3, run_hour = $4, updated_by = $5, updated_at = now()`,
		schoolID, in.MaxRetries, *in.RetryIntervalDays, *in.RunHour, claims.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	after, _ := a.loadAutodebitSettings(r.Context(), schoolID)
	a.paymentAudit(r.Context(), claims, schoolID, "autodebit_settings", schoolID, "update", "", settingsDTO(before), settingsDTO(after))
	common.WriteSuccess(w, settingsDTO(after), nil)
}

// ---- mandates -------------------------------------------------------------

type mandateDTO struct {
	ID            string     `json:"id"`
	StudentUserID string     `json:"student_user_id"`
	GuardianID    string     `json:"guardian_user_id"`
	Provider      string     `json:"provider"`
	AccountMasked *string    `json:"account_masked,omitempty"`
	Status        string     `json:"status"`
	MaxAmountIDR  *int64     `json:"max_amount_idr,omitempty"`
	RedirectURL   *string    `json:"redirect_url,omitempty"`
	SchemeIDs     []string   `json:"scheme_ids"`
	SchemeNames   []string   `json:"scheme_names"`
	ActivatedAt   *time.Time `json:"activated_at,omitempty"`
	CancelledAt   *time.Time `json:"cancelled_at,omitempty"`
	CancelReason  *string    `json:"cancel_reason,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

const mandateSelect = `SELECT m.id, m.student_user_id, m.guardian_user_id, m.provider, m.account_masked, m.status, m.max_amount_idr,
	m.redirect_url, m.activated_at, m.cancelled_at, m.cancel_reason, m.created_at,
	COALESCE(array_to_string(array_agg(s.scheme_id::text ORDER BY bs.name), ','), ''),
	COALESCE(array_to_string(array_agg(bs.name ORDER BY bs.name), '|'), '')
	FROM autodebit_mandates m
	LEFT JOIN autodebit_mandate_schemes s ON s.mandate_id = m.id
	LEFT JOIN billing_schemes bs ON bs.id = s.scheme_id`

const mandateGroup = ` GROUP BY m.id ORDER BY m.created_at DESC`

func (a *App) queryMandates(ctx context.Context, where string, args ...any) ([]mandateDTO, error) {
	rows, err := a.DB.QueryContext(ctx, mandateSelect+" WHERE "+where+mandateGroup, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mandateDTO{}
	for rows.Next() {
		var m mandateDTO
		var acc, redirect, reason sql.NullString
		var maxAmt sql.NullInt64
		var activated, cancelled sql.NullTime
		var ids, names string
		if err := rows.Scan(&m.ID, &m.StudentUserID, &m.GuardianID, &m.Provider, &acc, &m.Status, &maxAmt,
			&redirect, &activated, &cancelled, &reason, &m.CreatedAt, &ids, &names); err != nil {
			return nil, err
		}
		m.AccountMasked, m.RedirectURL, m.CancelReason = optString(acc.String), optString(redirect.String), optString(reason.String)
		if maxAmt.Valid {
			v := maxAmt.Int64
			m.MaxAmountIDR = &v
		}
		if activated.Valid {
			m.ActivatedAt = &activated.Time
		}
		if cancelled.Valid {
			m.CancelledAt = &cancelled.Time
		}
		m.SchemeIDs, m.SchemeNames = splitNonEmpty(ids, ","), splitNonEmpty(names, "|")
		out = append(out, m)
	}
	return out, rows.Err()
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, sep)
}

// ParentListMandates: GET /parent/autodebit/mandates.
func (a *App) ParentListMandates(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil || !a.requirePaymentFeature(w, r, claims.SchoolID, common.FeaturePaymentAutodebit) {
		return
	}
	out, err := a.queryMandates(r.Context(), "m.school_id = $1 AND m.guardian_user_id = $2", claims.SchoolID, claims.UserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

type createMandateRequest struct {
	StudentUserID string   `json:"student_user_id"`
	SchemeIDs     []string `json:"scheme_ids"`
	Provider      string   `json:"provider"`
	MaxAmountIDR  *int64   `json:"max_amount_idr"`
}

// ParentCreateMandate: POST /parent/autodebit/mandates (FR-151-001, BR-PAY-04).
func (a *App) ParentCreateMandate(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil || !a.requirePaymentFeature(w, r, claims.SchoolID, common.FeaturePaymentAutodebit) {
		return
	}
	ctx := r.Context()
	var req createMandateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.StudentUserID == "" || len(req.SchemeIDs) == 0 {
		common.WriteError(w, http.StatusBadRequest, "anak dan minimal satu jenis tagihan wajib dipilih")
		return
	}
	if req.MaxAmountIDR != nil && *req.MaxAmountIDR <= 0 {
		common.WriteError(w, http.StatusBadRequest, "batas nominal harus lebih dari 0")
		return
	}
	if a.Identity == nil {
		common.WriteError(w, http.StatusForbidden, "access denied to student")
		return
	}
	if ok, err := a.Identity.GuardianHasAccess(ctx, claims.UserID, req.StudentUserID, claims.SchoolID); err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "access denied to student")
		return
	}
	settings, err := a.loadAutodebitSettings(ctx, claims.SchoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !settings.Ready() {
		common.WriteError(w, http.StatusConflict, "Pengaturan autodebit sekolah belum lengkap")
		return
	}
	idemKey := idempotencyKey(r)
	if existing, err := a.queryMandates(ctx, "m.school_id = $1 AND m.idempotency_key = $2", claims.SchoolID, idemKey); err == nil && len(existing) == 1 {
		common.WriteSuccess(w, existing[0], nil)
		return
	}
	var schemeCount int
	if err := a.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM billing_schemes WHERE school_id = $1 AND id = ANY($2::uuid[])`,
		claims.SchoolID, "{"+strings.Join(req.SchemeIDs, ",")+"}").Scan(&schemeCount); err != nil || schemeCount != len(req.SchemeIDs) {
		common.WriteError(w, http.StatusBadRequest, "jenis tagihan tidak valid")
		return
	}
	providers, err := a.loadProviders(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	code := req.Provider
	if code == "" {
		code = firstCapable(providers, gateway.CapAutodebit)
	}
	if pi, ok := providers[code]; !ok || !pi.Enabled || !gateway.HasCapability(pi.Capabilities, gateway.CapAutodebit) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Provider tidak mendukung autodebit")
		return
	}
	p, err := a.paymentProvider(ctx, code)
	if err != nil {
		common.WriteError(w, http.StatusServiceUnavailable, "Provider autodebit sedang tidak tersedia")
		return
	}
	var maxAmt int64
	if req.MaxAmountIDR != nil {
		maxAmt = *req.MaxAmountIDR
	}
	setup, err := p.CreateMandate(ctx, gateway.MandateRequest{RefNo: "MD-" + randomKey()[:16], BuyerName: "Orang Tua Siswa", MaxAmountIDR: maxAmt})
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Gagal mendaftarkan mandat: "+err.Error())
		return
	}
	status := gateway.MandatePending
	if setup.Active {
		status = gateway.MandateActive
	}
	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer dbTx.Rollback()
	var id string
	err = dbTx.QueryRowContext(ctx,
		`INSERT INTO autodebit_mandates (school_id, guardian_user_id, student_user_id, provider, provider_mandate_ref, account_masked,
		     status, max_amount_idr, redirect_url, activated_at, idempotency_key)
		 VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,NULLIF($9,''),CASE WHEN $7 = 'active' THEN now() END,$10)
		 RETURNING id`,
		claims.SchoolID, claims.UserID, req.StudentUserID, code, setup.ProviderMandateRef, setup.AccountMasked,
		status, req.MaxAmountIDR, setup.RedirectURL, idemKey).Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, s := range req.SchemeIDs {
		if _, err := dbTx.ExecContext(ctx,
			`INSERT INTO autodebit_mandate_schemes (mandate_id, scheme_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, s); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err := dbTx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	out, err := a.queryMandates(ctx, "m.id = $1", id)
	if err != nil || len(out) != 1 {
		writeErr(w, err)
		return
	}
	a.paymentAudit(ctx, claims, claims.SchoolID, "autodebit_mandate", id, "create", "", nil, out[0])
	common.WriteJSON(w, http.StatusCreated, out[0], nil)
}

func firstCapable(providers map[string]gateway.ProviderInfo, capability string) string {
	best := ""
	for code, p := range providers {
		if p.Enabled && gateway.HasCapability(p.Capabilities, capability) && (best == "" || code < best) {
			best = code
		}
	}
	return best
}

// ParentCancelMandate: POST /parent/autodebit/mandates/{id}/cancel
// (FR-151-003, AC-151-02). Effective for the next debit: scheduled attempts
// are skipped, an attempt already processing at the provider finishes.
func (a *App) ParentCancelMandate(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil || !a.requirePaymentFeature(w, r, claims.SchoolID, common.FeaturePaymentAutodebit) {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	out, err := a.cancelMandate(r.Context(), claims, claims.SchoolID, param(r, "id"), claims.UserID, strings.TrimSpace(req.Reason), time.Now())
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

func (a *App) cancelMandate(ctx context.Context, c *common.Claims, schoolID, id, guardianID, reason string, now time.Time) (*mandateDTO, error) {
	before, err := a.queryMandates(ctx, "m.id = $1 AND m.school_id = $2 AND m.guardian_user_id = $3", id, schoolID, guardianID)
	if err != nil {
		return nil, err
	}
	if len(before) != 1 {
		return nil, &httpError{status: http.StatusNotFound, msg: "mandat tidak ditemukan"}
	}
	if before[0].Status == gateway.MandateCancelled {
		return &before[0], nil
	}
	var provider, ref sql.NullString
	_ = a.DB.QueryRowContext(ctx, `SELECT provider, provider_mandate_ref FROM autodebit_mandates WHERE id = $1`, id).Scan(&provider, &ref)

	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer dbTx.Rollback()
	if _, err := dbTx.ExecContext(ctx,
		`UPDATE autodebit_mandates SET status = 'cancelled', cancelled_at = $2, cancelled_by = $3, cancel_reason = NULLIF($4,''), updated_at = now()
		  WHERE id = $1 AND status <> 'cancelled'`, id, now, guardianID, reason); err != nil {
		return nil, err
	}
	if _, err := dbTx.ExecContext(ctx,
		`UPDATE autodebit_attempts SET status = 'skipped', skip_reason = $2, updated_at = now()
		  WHERE mandate_id = $1 AND status = 'scheduled'`, id, gateway.SkipMandateCancelled); err != nil {
		return nil, err
	}
	if err := dbTx.Commit(); err != nil {
		return nil, err
	}
	// Best effort at the provider: SEKOCI stays cancelled either way.
	if ref.Valid {
		if p, perr := a.paymentProvider(ctx, provider.String); perr == nil {
			if cerr := p.CancelMandate(ctx, ref.String); cerr != nil && !errors.Is(cerr, gateway.ErrUnsupported) && a.Core != nil {
				_ = a.Core.CreateException(ctx, client.ExceptionEvent{
					SchoolID: schoolID, SourceService: "billing", Kind: "webhook", Severity: "normal", CorrelationID: id,
					ErrorMessage:   "pembatalan mandat di provider gagal: " + cerr.Error(),
					PayloadSummary: map[string]any{"event": "mandate_cancel_failed", "mandate_id": id, "provider": provider.String},
					MaxAttempts:    3, IdempotencyKey: "mandate-cancel:" + id,
				})
			}
		}
	}
	after, err := a.queryMandates(ctx, "m.id = $1", id)
	if err != nil || len(after) != 1 {
		return nil, err
	}
	a.paymentAudit(ctx, c, schoolID, "autodebit_mandate", id, "cancel", reason, before[0], after[0])
	return &after[0], nil
}

type attemptDTO struct {
	ID            string    `json:"id"`
	MandateID     string    `json:"mandate_id"`
	BillID        string    `json:"bill_id"`
	StudentUserID string    `json:"student_user_id"`
	PeriodYear    int       `json:"period_year"`
	PeriodMonth   int       `json:"period_month"`
	AttemptNo     int       `json:"attempt_no"`
	ScheduledFor  string    `json:"scheduled_for"`
	AmountIDR     int64     `json:"amount_idr"`
	Status        string    `json:"status"`
	SkipReason    *string   `json:"skip_reason,omitempty"`
	FailureReason *string   `json:"failure_reason,omitempty"`
	TransactionID *string   `json:"gateway_transaction_id,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (a *App) queryAttempts(ctx context.Context, where string, args ...any) ([]attemptDTO, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT t.id, t.mandate_id, t.bill_id, m.student_user_id, b.period_year, b.period_month, t.attempt_no, t.scheduled_for,
		        t.amount_idr, t.status, t.skip_reason, t.failure_reason, t.gateway_transaction_id::text, t.updated_at
		   FROM autodebit_attempts t
		   JOIN autodebit_mandates m ON m.id = t.mandate_id
		   JOIN student_bills b ON b.id = t.bill_id
		  WHERE `+where+` ORDER BY t.scheduled_for DESC, t.attempt_no DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []attemptDTO{}
	for rows.Next() {
		var it attemptDTO
		var sched time.Time
		var skip, fail, txID sql.NullString
		if err := rows.Scan(&it.ID, &it.MandateID, &it.BillID, &it.StudentUserID, &it.PeriodYear, &it.PeriodMonth, &it.AttemptNo, &sched,
			&it.AmountIDR, &it.Status, &skip, &fail, &txID, &it.UpdatedAt); err != nil {
			return nil, err
		}
		it.ScheduledFor = sched.Format("2006-01-02")
		it.SkipReason, it.FailureReason, it.TransactionID = optString(skip.String), optString(fail.String), optString(txID.String)
		out = append(out, it)
	}
	return out, rows.Err()
}

// ParentListAttempts: GET /parent/autodebit/attempts?mandate_id=.
func (a *App) ParentListAttempts(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil || !a.requirePaymentFeature(w, r, claims.SchoolID, common.FeaturePaymentAutodebit) {
		return
	}
	where, args := "t.school_id = $1 AND m.guardian_user_id = $2", []any{claims.SchoolID, claims.UserID}
	if mid := r.URL.Query().Get("mandate_id"); mid != "" {
		args = append(args, mid)
		where += " AND t.mandate_id = $3"
	}
	out, err := a.queryAttempts(r.Context(), where, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

// AdminListMandates: GET /admin/autodebit/mandates?status=.
func (a *App) AdminListMandates(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentAutodebit) {
		return
	}
	where, args := "m.school_id = $1", []any{schoolID}
	if s := r.URL.Query().Get("status"); s != "" {
		args = append(args, s)
		where += " AND m.status = $2"
	}
	out, err := a.queryMandates(r.Context(), where, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

// AdminListAttempts: GET /admin/autodebit/attempts?status=failed.
func (a *App) AdminListAttempts(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentAutodebit) {
		return
	}
	where, args := "t.school_id = $1", []any{schoolID}
	if s := r.URL.Query().Get("status"); s != "" {
		args = append(args, s)
		where += " AND t.status = $2"
	}
	out, err := a.queryAttempts(r.Context(), where, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

// MandateWebhook: POST /webhooks/{provider}/mandates — activation or
// cancellation reported by the provider (FR-151-001).
func (a *App) MandateWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := param(r, "provider")
	body, err := readLimited(r)
	if err != nil {
		writeSnap(w, http.StatusBadRequest, snapBadRequest)
		return
	}
	sum := sha256.Sum256(body)
	rawHash := hex.EncodeToString(sum[:])
	p, err := a.paymentProvider(ctx, code)
	if err != nil {
		_, pRef := gateway.ParseNicepayRefs(body)
		a.rejectWebhook(ctx, code, pRef, rawHash, maskJSON(body), "provider_unavailable")
		writeSnap(w, http.StatusServiceUnavailable, snapError)
		return
	}
	n, err := p.VerifyWebhook(r.Header, body)
	if err != nil {
		a.rejectWebhook(ctx, code, n.ProviderRef, rawHash, maskJSON(body), "invalid_signature")
		writeSnap(w, http.StatusUnauthorized, snapUnauthorized)
		return
	}
	if n.MandateRef == "" {
		writeSnap(w, http.StatusOK, snapOK)
		return
	}
	var id, schoolID, status string
	if err := a.DB.QueryRowContext(ctx,
		`SELECT id, school_id, status FROM autodebit_mandates WHERE provider = $1 AND provider_mandate_ref = $2`, code, n.MandateRef).
		Scan(&id, &schoolID, &status); err != nil {
		writeSnap(w, http.StatusOK, snapOK)
		return
	}
	switch {
	case n.MandateStatus == gateway.MandateActive && status == gateway.MandatePending:
		_, err = a.DB.ExecContext(ctx, `UPDATE autodebit_mandates SET status = 'active', activated_at = now(), updated_at = now() WHERE id = $1`, id)
	case n.MandateStatus == gateway.MandateFailed && status == gateway.MandatePending:
		_, err = a.DB.ExecContext(ctx, `UPDATE autodebit_mandates SET status = 'failed', updated_at = now() WHERE id = $1`, id)
	case n.MandateStatus == gateway.MandateCancelled && status != gateway.MandateCancelled:
		_, err = a.DB.ExecContext(ctx,
			`UPDATE autodebit_mandates SET status = 'cancelled', cancelled_at = now(), cancel_reason = 'dibatalkan di provider', updated_at = now() WHERE id = $1`, id)
		if err == nil {
			_, err = a.DB.ExecContext(ctx,
				`UPDATE autodebit_attempts SET status = 'skipped', skip_reason = $2, updated_at = now() WHERE mandate_id = $1 AND status = 'scheduled'`,
				id, gateway.SkipMandateCancelled)
		}
	default:
		writeSnap(w, http.StatusOK, snapOK)
		return
	}
	if err != nil {
		writeSnap(w, http.StatusInternalServerError, snapError)
		return
	}
	a.paymentAudit(ctx, nil, schoolID, "autodebit_mandate", id, "provider_"+n.MandateStatus, "", map[string]string{"status": status}, map[string]string{"status": n.MandateStatus})
	writeSnap(w, http.StatusOK, snapOK)
}

// ---- debit job ------------------------------------------------------------

// billOutstanding is net minus verified allocations plus reversals.
func (a *App) billOutstanding(ctx context.Context, billID string) (int64, error) {
	var net, verified, reversed int64
	err := a.DB.QueryRowContext(ctx,
		`SELECT b.net_idr,
		        COALESCE((SELECT SUM(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id = al.payment_id
		                   WHERE p.student_bill_id = b.id AND p.status = 'verified'), 0),
		        COALESCE((SELECT SUM(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id = rv.payment_id
		                   WHERE p.student_bill_id = b.id), 0)
		   FROM student_bills b WHERE b.id = $1 AND b.status NOT IN ('paid','void')`, billID).Scan(&net, &verified, &reversed)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return model.Outstanding(net, []int64{verified}, []int64{reversed})
}

// planAutodebit creates today's first attempts for one school (DueDebits).
func (a *App) planAutodebit(ctx context.Context, schoolID string, today time.Time, s gateway.Settings) error {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT m.id, m.student_user_id, m.status, COALESCE(m.max_amount_idr, 0), m.cancelled_at,
		        COALESCE(array_to_string(array_agg(s.scheme_id::text), ','), '')
		   FROM autodebit_mandates m JOIN autodebit_mandate_schemes s ON s.mandate_id = m.id
		  WHERE m.school_id = $1 AND m.status = 'active' GROUP BY m.id`, schoolID)
	if err != nil {
		return err
	}
	var mandates []gateway.Mandate
	students := map[string]bool{}
	for rows.Next() {
		var m gateway.Mandate
		var cancelled sql.NullTime
		var schemes string
		if err := rows.Scan(&m.ID, &m.StudentUserID, &m.Status, &m.MaxAmountIDR, &cancelled, &schemes); err != nil {
			rows.Close()
			return err
		}
		if cancelled.Valid {
			m.CancelledAt = &cancelled.Time
		}
		m.SchemeIDs = splitNonEmpty(schemes, ",")
		mandates = append(mandates, m)
		students[m.StudentUserID] = true
	}
	rows.Close()
	if len(mandates) == 0 {
		return nil
	}
	loc := a.location()
	endOfDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	// scheme_id is scanned nullable: ad-hoc bills (T12 0007) have none and
	// are never debited (K3).
	brows, err := a.DB.QueryContext(ctx,
		`SELECT b.id, b.student_user_id, b.scheme_id::text, b.due_at,
		        EXISTS (SELECT 1 FROM autodebit_attempts x WHERE x.bill_id = b.id)
		   FROM student_bills b
		  WHERE b.school_id = $1 AND b.status NOT IN ('paid','void') AND b.due_at IS NOT NULL AND b.due_at < $2
		    AND b.student_user_id = ANY($3::uuid[])`,
		schoolID, endOfDay, "{"+strings.Join(keys(students), ",")+"}")
	if err != nil {
		return err
	}
	var bills []gateway.DueBill
	existing := map[string]bool{}
	for brows.Next() {
		var b gateway.DueBill
		var scheme sql.NullString
		var due time.Time
		var has bool
		if err := brows.Scan(&b.ID, &b.StudentUserID, &scheme, &due, &has); err != nil {
			brows.Close()
			return err
		}
		b.SchemeID = scheme.String
		local := due.In(loc)
		b.DueDate = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
		existing[b.ID] = has
		bills = append(bills, b)
	}
	brows.Close()
	for i := range bills {
		if bills[i].OutstandingIDR, err = a.billOutstanding(ctx, bills[i].ID); err != nil {
			return err
		}
	}
	todayUTC := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	for _, p := range gateway.DueDebits(mandates, bills, existing, todayUTC, s) {
		var id string
		err := a.DB.QueryRowContext(ctx,
			`INSERT INTO autodebit_attempts (school_id, mandate_id, bill_id, attempt_no, scheduled_for, amount_idr, status, skip_reason, idempotency_key)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9) ON CONFLICT DO NOTHING RETURNING id`,
			schoolID, p.MandateID, p.BillID, p.AttemptNo, todayUTC, p.AmountIDR, p.Status, p.SkipReason,
			fmt.Sprintf("ad:%s:%s:%d", p.MandateID, p.BillID, p.AttemptNo)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if p.Status == gateway.AttemptSkipped {
			a.notifyGuardian(ctx, schoolID, p.MandateID, "autodebit:"+id, "Autodebit tidak dijalankan",
				fmt.Sprintf("Tagihan Rp %d melebihi batas nominal mandat autodebit. Silakan bayar manual.", p.AmountIDR))
		}
	}
	return nil
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (a *App) notifyGuardian(ctx context.Context, schoolID, mandateID, idemKey, title, body string) {
	if a.Notification == nil {
		return
	}
	var guardian string
	if err := a.DB.QueryRowContext(ctx, `SELECT guardian_user_id FROM autodebit_mandates WHERE id = $1`, mandateID).Scan(&guardian); err != nil {
		return
	}
	_ = a.Notification.Notify(ctx, idemKey, client.Notification{
		SchoolID: schoolID, UserID: guardian, Type: "autodebit", Title: title, Body: body,
		Data: map[string]any{"mandate_id": mandateID}, Channels: []string{"in_app", "email"},
	})
}

// processDueAttempts debits every scheduled attempt due by today.
func (a *App) processDueAttempts(ctx context.Context, schoolID string, today, now time.Time) error {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id FROM autodebit_attempts WHERE school_id = $1 AND status = 'scheduled' AND scheduled_for <= $2 ORDER BY scheduled_for, attempt_no`,
		schoolID, time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC))
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		a.processAttempt(ctx, id, now)
	}
	return nil
}

// processAttempt debits one attempt at the mandate's provider. A
// synchronous success is settled at once (T6 allocation); a pending debit
// completes through the webhook; a failure goes to failAttempt.
func (a *App) processAttempt(ctx context.Context, attemptID string, now time.Time) {
	var schoolID, mandateID, billID, idemKey string
	var attemptNo int
	err := a.DB.QueryRowContext(ctx,
		`UPDATE autodebit_attempts SET status = 'processing', updated_at = now()
		  WHERE id = $1 AND status = 'scheduled' RETURNING school_id, mandate_id, bill_id, attempt_no, idempotency_key`, attemptID).
		Scan(&schoolID, &mandateID, &billID, &attemptNo, &idemKey)
	if err != nil {
		return
	}
	skip := func(reason string) {
		_, _ = a.DB.ExecContext(ctx, `UPDATE autodebit_attempts SET status = 'skipped', skip_reason = $2, updated_at = now() WHERE id = $1`, attemptID, reason)
	}
	var m gateway.Mandate
	var provider, guardian, student string
	var ref sql.NullString
	var cancelled sql.NullTime
	if err := a.DB.QueryRowContext(ctx,
		`SELECT status, cancelled_at, provider, provider_mandate_ref, guardian_user_id, student_user_id FROM autodebit_mandates WHERE id = $1`, mandateID).
		Scan(&m.Status, &cancelled, &provider, &ref, &guardian, &student); err != nil {
		skip(gateway.SkipMandateCancelled)
		return
	}
	if cancelled.Valid {
		m.CancelledAt = &cancelled.Time
	}
	if !gateway.CanDebit(m, now) {
		skip(gateway.SkipMandateCancelled)
		return
	}
	outstanding, err := a.billOutstanding(ctx, billID)
	if err != nil {
		a.failAttempt(ctx, attemptID, err.Error(), now)
		return
	}
	if outstanding <= 0 {
		skip(gateway.SkipBillSettled)
		return
	}
	method, err := a.autodebitMethod(ctx, schoolID)
	if err != nil {
		a.failAttempt(ctx, attemptID, "metode autodebit sekolah belum dikonfigurasi", now)
		return
	}
	totals, err := model.Totals(outstanding, *method)
	if err != nil {
		a.failAttempt(ctx, attemptID, err.Error(), now)
		return
	}
	p, err := a.paymentProvider(ctx, provider)
	if err != nil {
		a.failAttempt(ctx, attemptID, err.Error(), now)
		return
	}
	txNo := newTxNo()
	charge, err := p.Debit(ctx, gateway.DebitRequest{RefNo: txNo, ProviderMandateRef: ref.String, AmountIDR: totals.TotalIDR, GoodsName: "Autodebit tagihan"})
	if err != nil {
		a.failAttempt(ctx, attemptID, err.Error(), now)
		return
	}
	txID, err := a.insertGatewayTx(ctx, gatewayTxRow{
		SchoolID: schoolID, TxNo: txNo, PayerUserID: guardian, StudentUserID: student, MethodID: method.ID, Totals: totals,
		Provider: provider, Purpose: PurposeBills, MandateID: mandateID, Charge: charge,
		ExpiresAt: gateway.RetailExpiry(method.ValidMinutes, now), IdemKey: idemKey,
		Bills: []BillSummaryItem{{ID: billID, OutstandingIDR: outstanding}},
	})
	if err != nil {
		a.failAttempt(ctx, attemptID, err.Error(), now)
		return
	}
	_, _ = a.DB.ExecContext(ctx, `UPDATE autodebit_attempts SET gateway_transaction_id = $2, amount_idr = $3, updated_at = now() WHERE id = $1`,
		attemptID, txID, totals.TotalIDR)
	if charge.Status == gateway.StatusPaid {
		if err := a.settleTransaction(ctx, txID, "autodebit:"+attemptID); err != nil {
			a.webhookException(ctx, schoolID, txID, txNo, "autodebit:"+attemptID, err)
		}
	}
}

// autodebitMethod is the school's enabled method carrying autodebit fees.
func (a *App) autodebitMethod(ctx context.Context, schoolID string) (*model.GatewayMethod, error) {
	var m model.GatewayMethod
	err := a.DB.QueryRowContext(ctx,
		`SELECT id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort
		   FROM payment_methods WHERE school_id = $1 AND kind = $2 AND enabled ORDER BY sort, created_at LIMIT 1`,
		schoolID, kindAutodebit).Scan(&m.ID, &m.Code, &m.Kind, &m.ProviderCode, &m.Label, &m.Enabled,
		&m.FeeKind, &m.FeeFlatIDR, &m.FeeBP, &m.FeeBearer, &m.ValidMinutes, &m.Sort)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// failAttempt marks an attempt failed, schedules the retry (NextRetry) and
// notifies the parent either way (AC-151-01).
func (a *App) failAttempt(ctx context.Context, attemptID, reason string, now time.Time) {
	var schoolID, mandateID, billID string
	var attemptNo int
	var amount int64
	err := a.DB.QueryRowContext(ctx,
		`UPDATE autodebit_attempts SET status = 'failed', failure_reason = $2, updated_at = now()
		  WHERE id = $1 AND status IN ('processing','scheduled') RETURNING school_id, mandate_id, bill_id, attempt_no, amount_idr`,
		attemptID, reason).Scan(&schoolID, &mandateID, &billID, &attemptNo, &amount)
	if err != nil {
		return
	}
	s, err := a.loadAutodebitSettings(ctx, schoolID)
	if err != nil {
		return
	}
	local := now.In(a.location())
	next, on, ok := gateway.NextRetry(attemptNo, time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC), s)
	if ok {
		_, _ = a.DB.ExecContext(ctx,
			`INSERT INTO autodebit_attempts (school_id, mandate_id, bill_id, attempt_no, scheduled_for, amount_idr, idempotency_key)
			 VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
			schoolID, mandateID, billID, next, on, amount, fmt.Sprintf("ad:%s:%s:%d", mandateID, billID, next))
		a.notifyGuardian(ctx, schoolID, mandateID, "autodebit:"+attemptID, "Autodebit gagal",
			fmt.Sprintf("Debit Rp %d gagal (%s). Akan dicoba ulang pada %s.", amount, reason, on.Format("02-01-2006")))
		return
	}
	a.notifyGuardian(ctx, schoolID, mandateID, "autodebit:"+attemptID, "Autodebit gagal, bayar manual",
		fmt.Sprintf("Debit Rp %d gagal setelah %d percobaan. Silakan bayar tagihan secara manual.", amount, attemptNo))
}

// runAutodebitSchedule is job 1 of §4.4 for every configured school whose
// run hour has passed.
func (a *App) runAutodebitSchedule(ctx context.Context, now time.Time) error {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT school_id::text FROM autodebit_settings WHERE retry_interval_days IS NOT NULL AND run_hour IS NOT NULL`)
	if err != nil {
		return err
	}
	var schools []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		schools = append(schools, s)
	}
	rows.Close()
	local := now.In(a.location())
	for _, school := range schools {
		if !a.paymentFeature(ctx, school, common.FeaturePaymentAutodebit) {
			continue
		}
		s, err := a.loadAutodebitSettings(ctx, school)
		if err != nil || !s.Ready() || local.Hour() < s.RunHour {
			continue
		}
		res, err := a.DB.ExecContext(ctx,
			`INSERT INTO gateway_job_runs (job, run_on) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			"autodebit_schedule:"+school, local.Format("2006-01-02"))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			if err := a.planAutodebit(ctx, school, local, s); err != nil {
				return err
			}
		}
		if err := a.processDueAttempts(ctx, school, local, now); err != nil {
			return err
		}
	}
	return nil
}
