package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/gateway"
)

// routingRuleDTO is one payment_routing_rules row (FR-150-002/003).
type routingRuleDTO struct {
	ID               string    `json:"id"`
	MethodKind       string    `json:"method_kind"`
	MethodCode       *string   `json:"method_code"`
	PrimaryProvider  string    `json:"primary_provider"`
	FallbackProvider *string   `json:"fallback_provider"`
	FailoverEnabled  bool      `json:"failover_enabled"`
	Active           bool      `json:"active"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type routingRuleInput struct {
	MethodKind       string  `json:"method_kind"`
	MethodCode       *string `json:"method_code"`
	PrimaryProvider  string  `json:"primary_provider"`
	FallbackProvider *string `json:"fallback_provider"`
	FailoverEnabled  bool    `json:"failover_enabled"`
}

var routableKinds = []string{gateway.CapVA, gateway.CapQRIS, gateway.CapEWallet, gateway.CapRetail}

func (a *App) queryRules(ctx context.Context, where string, args ...any) ([]routingRuleDTO, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id, method_kind, method_code, primary_provider, fallback_provider, failover_enabled, active, updated_at
		   FROM payment_routing_rules WHERE `+where+` ORDER BY method_kind, method_code NULLS FIRST`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []routingRuleDTO{}
	for rows.Next() {
		var d routingRuleDTO
		var code, fb sql.NullString
		if err := rows.Scan(&d.ID, &d.MethodKind, &code, &d.PrimaryProvider, &fb, &d.FailoverEnabled, &d.Active, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.MethodCode, d.FallbackProvider = optString(code.String), optString(fb.String)
		out = append(out, d)
	}
	return out, rows.Err()
}

// validateRule checks the kind, the method code (belongs to the school and
// the kind), and the providers (exist, can serve the kind, differ).
func (a *App) validateRule(ctx context.Context, schoolID string, in *routingRuleInput) error {
	bad := func(msg string) error { return &httpError{status: http.StatusBadRequest, msg: msg} }
	if !gateway.HasCapability(routableKinds, in.MethodKind) {
		return bad("jenis metode tidak valid (va, qris, ewallet, retail)")
	}
	if in.MethodCode != nil {
		v := strings.TrimSpace(*in.MethodCode)
		if v == "" {
			in.MethodCode = nil
		} else {
			var n int
			if err := a.DB.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM payment_methods WHERE school_id = $1 AND code = $2 AND kind = $3`, schoolID, v, in.MethodKind).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return bad("metode " + v + " tidak ada untuk jenis " + in.MethodKind)
			}
			in.MethodCode = &v
		}
	}
	if in.FallbackProvider != nil && strings.TrimSpace(*in.FallbackProvider) == "" {
		in.FallbackProvider = nil
	}
	if in.FallbackProvider != nil && *in.FallbackProvider == in.PrimaryProvider {
		return bad("provider cadangan harus berbeda dari provider utama")
	}
	if in.FailoverEnabled && in.FallbackProvider == nil {
		return bad("failover membutuhkan provider cadangan")
	}
	providers, err := a.loadProviders(ctx)
	if err != nil {
		return err
	}
	check := func(code string) error {
		p, ok := providers[code]
		if !ok {
			return bad("provider " + code + " tidak dikenal")
		}
		if !gateway.HasCapability(p.Capabilities, in.MethodKind) {
			return bad("provider " + code + " tidak mendukung " + in.MethodKind)
		}
		return nil
	}
	if err := check(in.PrimaryProvider); err != nil {
		return err
	}
	if in.FallbackProvider != nil {
		return check(*in.FallbackProvider)
	}
	return nil
}

// AdminListRoutingRules: GET /admin/payment-routing-rules.
func (a *App) AdminListRoutingRules(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentMultiProvider) {
		return
	}
	out, err := a.queryRules(r.Context(), "school_id = $1 AND active", schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

// AdminCreateRoutingRule: POST /admin/payment-routing-rules.
func (a *App) AdminCreateRoutingRule(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentMultiProvider) {
		return
	}
	var in routingRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := a.validateRule(r.Context(), schoolID, &in); err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO payment_routing_rules (school_id, method_kind, method_code, primary_provider, fallback_provider, failover_enabled, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		schoolID, in.MethodKind, in.MethodCode, in.PrimaryProvider, in.FallbackProvider, in.FailoverEnabled, claims.UserID).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "aturan untuk metode ini sudah ada")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	out, _ := a.queryRules(r.Context(), "id = $1", id)
	if len(out) == 1 {
		a.paymentAudit(r.Context(), claims, schoolID, "payment_routing_rule", id, "create", "", nil, out[0])
		common.WriteJSON(w, http.StatusCreated, out[0], nil)
		return
	}
	writeErr(w, errors.New("routing rule not found after insert"))
}

// AdminUpdateRoutingRule: PUT /admin/payment-routing-rules/{id}.
func (a *App) AdminUpdateRoutingRule(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentMultiProvider) {
		return
	}
	id := param(r, "id")
	before, err := a.queryRules(r.Context(), "id = $1 AND school_id = $2 AND active", id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(before) != 1 {
		common.WriteError(w, http.StatusNotFound, "aturan tidak ditemukan")
		return
	}
	var in routingRuleInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := a.validateRule(r.Context(), schoolID, &in); err != nil {
		writeErr(w, err)
		return
	}
	_, err = a.DB.ExecContext(r.Context(),
		`UPDATE payment_routing_rules SET method_kind = $3, method_code = $4, primary_provider = $5, fallback_provider = $6,
		        failover_enabled = $7, updated_at = now()
		  WHERE id = $1 AND school_id = $2`,
		id, schoolID, in.MethodKind, in.MethodCode, in.PrimaryProvider, in.FallbackProvider, in.FailoverEnabled)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "aturan untuk metode ini sudah ada")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	after, _ := a.queryRules(r.Context(), "id = $1", id)
	if len(after) == 1 {
		a.paymentAudit(r.Context(), claims, schoolID, "payment_routing_rule", id, "update", "", before[0], after[0])
		common.WriteSuccess(w, after[0], nil)
		return
	}
	writeErr(w, errors.New("routing rule not found after update"))
}

// AdminDeleteRoutingRule: DELETE /admin/payment-routing-rules/{id} —
// deactivates (history is kept, BR-GEN-07).
func (a *App) AdminDeleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentMultiProvider) {
		return
	}
	id := param(r, "id")
	before, err := a.queryRules(r.Context(), "id = $1 AND school_id = $2 AND active", id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(before) != 1 {
		common.WriteError(w, http.StatusNotFound, "aturan tidak ditemukan")
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		`UPDATE payment_routing_rules SET active = false, updated_at = now() WHERE id = $1 AND school_id = $2`, id, schoolID); err != nil {
		writeErr(w, err)
		return
	}
	a.paymentAudit(r.Context(), claims, schoolID, "payment_routing_rule", id, "deactivate", "", before[0], nil)
	common.WriteSuccess(w, map[string]string{"id": id, "status": "inactive"}, nil)
}

// AdminPreviewRouting: POST /admin/payment-routing-rules/preview {method_id}
// answers "method X will go to provider Y" (AC-150-01).
func (a *App) AdminPreviewRouting(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequirePaymentConfig(w, r)
	if schoolID == "" || !a.requirePaymentFeature(w, r, schoolID, common.FeaturePaymentMultiProvider) {
		return
	}
	var in struct {
		MethodID string `json:"method_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.MethodID == "" {
		common.WriteError(w, http.StatusBadRequest, "method_id wajib diisi")
		return
	}
	method, err := a.fetchMethod(r.Context(), schoolID, in.MethodID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	d, err := a.decideRoute(r.Context(), schoolID, method)
	if errors.Is(err, gateway.ErrNotRouted) {
		common.WriteSuccess(w, map[string]any{"method_code": method.Code, "routed": false, "message": gateway.ErrNotRouted.Error()}, nil)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	mappings, err := a.methodMappings(r.Context(), method.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	code, mapErr := gateway.ProviderCode(*method, d.Primary, mappings)
	resp := map[string]any{"method_code": method.Code, "routed": true, "decision": d, "provider_method_code": code}
	if mapErr != nil {
		resp["routed"] = false
		resp["message"] = mapErr.Error()
	}
	common.WriteSuccess(w, resp, nil)
}
