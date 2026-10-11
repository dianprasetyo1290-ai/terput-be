package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"skci/billing/internal/common"
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
		 WHERE school_id = $1 AND enabled = true
		 ORDER BY sort ASC, created_at ASC`, claims.SchoolID)
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

	common.WriteSuccess(w, methods, nil)
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
	if req.Kind != "va" && req.Kind != "qris" && req.Kind != "ewallet" {
		common.WriteError(w, http.StatusBadRequest, "invalid kind (va, qris, ewallet)")
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
