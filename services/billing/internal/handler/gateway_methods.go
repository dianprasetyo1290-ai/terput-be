package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"skci/billing/internal/common"
	"skci/billing/internal/gateway"
	"skci/billing/internal/model"
)

// ListPaymentMethods returns enabled payment methods for parents (AC-28-01).
func (a *App) ListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireParent(w, r)
	if claims == nil {
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort
		 FROM payment_methods
		 WHERE school_id = $1 AND enabled = true AND kind <> 'autodebit'
		   AND (kind <> 'retail' OR $2)
		 ORDER BY sort ASC, created_at ASC`, claims.SchoolID, a.paymentFeature(r.Context(), claims.SchoolID, common.FeaturePaymentAutodebit))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	var methods []model.GatewayMethod
	for rows.Next() {
		var m model.GatewayMethod
		if err := rows.Scan(&m.ID, &m.Code, &m.Kind, &m.ProviderCode, &m.Label, &m.Enabled,
			&m.FeeKind, &m.FeeFlatIDR, &m.FeeBP, &m.FeeBearer, &m.ValidMinutes, &m.Sort); err != nil {
			writeErr(w, err)
			return
		}
		methods = append(methods, m)
	}

	common.WriteSuccess(w, methods, nil)
}

// AdminListPaymentMethods returns all methods for administration.
func (a *App) AdminListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort
		 FROM payment_methods
		 WHERE school_id = $1
		 ORDER BY sort ASC, created_at ASC`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	var methods []model.GatewayMethod
	for rows.Next() {
		var m model.GatewayMethod
		if err := rows.Scan(&m.ID, &m.Code, &m.Kind, &m.ProviderCode, &m.Label, &m.Enabled,
			&m.FeeKind, &m.FeeFlatIDR, &m.FeeBP, &m.FeeBearer, &m.ValidMinutes, &m.Sort); err != nil {
			writeErr(w, err)
			return
		}
		methods = append(methods, m)
	}

	out := make([]adminMethodDTO, 0, len(methods))
	for _, m := range methods {
		mp, err := a.methodMappings(r.Context(), m.ID)
		if err != nil {
			writeErr(w, err)
			return
		}
		codes := []providerMethodCode{}
		for _, p := range sortedMapKeys(mp) {
			codes = append(codes, providerMethodCode{ProviderCode: p, ProviderMethodCode: mp[p]})
		}
		out = append(out, adminMethodDTO{GatewayMethod: m, ProviderCodes: codes})
	}
	common.WriteSuccess(w, out, nil)
}

// providerMethodCode maps a method to its code at one provider (T14).
type providerMethodCode struct {
	ProviderCode       string `json:"provider_code"`
	ProviderMethodCode string `json:"provider_method_code"`
}

type adminMethodDTO struct {
	model.GatewayMethod
	ProviderCodes []providerMethodCode `json:"provider_codes"`
}

func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// validMethodKind: retail and autodebit methods need payment_autodebit.
func (a *App) validMethodKind(ctx context.Context, schoolID, kind string) error {
	switch kind {
	case "va", "qris", "ewallet":
		return nil
	case gateway.CapRetail, kindAutodebit:
		if a.paymentFeature(ctx, schoolID, common.FeaturePaymentAutodebit) {
			return nil
		}
		return &httpError{status: http.StatusBadRequest, msg: "jenis gerai retail/autodebit membutuhkan fitur Autodebit & Gerai Retail"}
	}
	return &httpError{status: http.StatusBadRequest, msg: "invalid kind (va, qris, ewallet, retail, autodebit)"}
}

// saveMethodMappings replaces a method's provider codes when given.
func (a *App) saveMethodMappings(ctx context.Context, methodID string, codes *[]providerMethodCode) error {
	if codes == nil {
		return nil
	}
	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer dbTx.Rollback()
	if _, err := dbTx.ExecContext(ctx, `DELETE FROM payment_method_provider_codes WHERE method_id = $1`, methodID); err != nil {
		return err
	}
	for _, c := range *codes {
		pc, mc := strings.TrimSpace(c.ProviderCode), strings.TrimSpace(c.ProviderMethodCode)
		if pc == "" || mc == "" {
			continue
		}
		if _, err := dbTx.ExecContext(ctx,
			`INSERT INTO payment_method_provider_codes (method_id, provider_code, provider_method_code) VALUES ($1, $2, $3)`,
			methodID, pc, mc); err != nil {
			return &httpError{status: http.StatusBadRequest, msg: "provider " + pc + " tidak dikenal"}
		}
	}
	return dbTx.Commit()
}

type PaymentMethodInput struct {
	Code         string `json:"code"`
	Kind         string `json:"kind"` // va, qris, ewallet
	ProviderCode string `json:"provider_code"`
	Label        string `json:"label"`
	Enabled      *bool  `json:"enabled"`
	FeeKind      string `json:"fee_kind"` // flat, percent
	FeeFlatIDR   int64  `json:"fee_flat_idr"`
	FeeBP        int    `json:"fee_bp"`
	FeeBearer    string `json:"fee_bearer"` // parent, school
	ValidMinutes int    `json:"valid_minutes"`
	Sort         int    `json:"sort"`
	// T14: codes per provider; nil keeps the current mapping.
	ProviderCodes *[]providerMethodCode `json:"provider_codes"`
}

// AdminCreatePaymentMethod creates a new payment method.
func (a *App) AdminCreatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id required")
		return
	}

	var req PaymentMethodInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Code = strings.TrimSpace(req.Code)
	req.Kind = strings.TrimSpace(req.Kind)
	req.ProviderCode = strings.TrimSpace(req.ProviderCode)
	req.Label = strings.TrimSpace(req.Label)
	if req.Code == "" || req.Kind == "" || req.ProviderCode == "" || req.Label == "" {
		common.WriteError(w, http.StatusBadRequest, "code, kind, provider_code, label are required")
		return
	}
	if err := a.validMethodKind(r.Context(), schoolID, req.Kind); err != nil {
		writeErr(w, err)
		return
	}
	if req.FeeKind != "flat" && req.FeeKind != "percent" {
		req.FeeKind = "flat"
	}
	if req.FeeBearer != "parent" && req.FeeBearer != "school" {
		req.FeeBearer = "parent"
	}
	if req.ValidMinutes <= 0 {
		req.ValidMinutes = 1440
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO payment_methods (school_id, code, kind, provider_code, label, enabled, fee_kind, fee_flat_idr, fee_bp, fee_bearer, valid_minutes, sort)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id`,
		schoolID, req.Code, req.Kind, req.ProviderCode, req.Label, enabled, req.FeeKind, req.FeeFlatIDR, req.FeeBP, req.FeeBearer, req.ValidMinutes, req.Sort).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "payment method code already exists")
			return
		}
		writeErr(w, err)
		return
	}
	if err := a.saveMethodMappings(r.Context(), id, req.ProviderCodes); err != nil {
		writeErr(w, err)
		return
	}

	common.WriteSuccess(w, map[string]string{"id": id}, nil)
}

// AdminUpdatePaymentMethod updates an existing payment method.
func (a *App) AdminUpdatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := param(r, "id")

	var req PaymentMethodInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var enabled bool
	if req.Enabled != nil {
		enabled = *req.Enabled
	} else {
		enabled = true
	}

	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE payment_methods
		 SET label = COALESCE(NULLIF($1, ''), label),
		     enabled = $2,
		     fee_kind = COALESCE(NULLIF($3, ''), fee_kind),
		     fee_flat_idr = $4,
		     fee_bp = $5,
		     fee_bearer = COALESCE(NULLIF($6, ''), fee_bearer),
		     valid_minutes = CASE WHEN $7 > 0 THEN $7 ELSE valid_minutes END,
		     sort = $8,
		     updated_at = now()
		 WHERE id = $9 AND school_id = $10`,
		req.Label, enabled, req.FeeKind, req.FeeFlatIDR, req.FeeBP, req.FeeBearer, req.ValidMinutes, req.Sort, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		common.WriteError(w, http.StatusNotFound, "payment method not found")
		return
	}
	if err := a.saveMethodMappings(r.Context(), id, req.ProviderCodes); err != nil {
		writeErr(w, err)
		return
	}

	common.WriteSuccess(w, map[string]string{"id": id, "status": "updated"}, nil)
}

// AdminDeletePaymentMethod deletes or disables a payment method.
func (a *App) AdminDeletePaymentMethod(w http.ResponseWriter, r *http.Request) {
	claims := common.RequireFinance(w, r)
	if claims == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := param(r, "id")

	res, err := a.DB.ExecContext(r.Context(),
		`DELETE FROM payment_methods WHERE id = $1 AND school_id = $2`, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		common.WriteError(w, http.StatusNotFound, "payment method not found")
		return
	}
	common.WriteSuccess(w, map[string]string{"id": id, "status": "deleted"}, nil)
}
