package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// Promo rules carry their validity window, target component (form fee or
// re-registration), conditions and quota (BUG-092/097, BR-PPDB-01/02).
// Vouchers need a validity window that has not already ended.

type discountRuleInput struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Value      int64           `json:"value"`
	Priority   int             `json:"priority"`
	Cap        int64           `json:"cap"`
	Status     string          `json:"status"`
	Stackable  *bool           `json:"stackable"`
	PeriodID   string          `json:"period_id"`
	ValidFrom  string          `json:"valid_from"`
	ValidUntil string          `json:"valid_until"`
	Quota      int             `json:"quota"`
	AppliesTo  string          `json:"applies_to"`
	Conditions json.RawMessage `json:"conditions"`
}

type promoRule struct {
	kind        model.DiscountKind
	value       int64
	from, until *time.Time
	appliesTo   string
	conditions  string
}

func (in *discountRuleInput) normalize() (promoRule, error) {
	var p promoRule
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return p, &httpError{http.StatusUnprocessableEntity, "nama promo wajib diisi"}
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		return p, err
	}
	if in.Cap < 0 || in.Quota < 0 {
		return p, &httpError{http.StatusUnprocessableEntity, "batas potongan dan kuota promo tidak boleh negatif"}
	}
	from, err := model.ParseWIBDate(in.ValidFrom, false)
	if err != nil {
		return p, &httpError{http.StatusUnprocessableEntity, "tanggal mulai promo tidak valid"}
	}
	until, err := model.ParseWIBDate(in.ValidUntil, true)
	if err != nil {
		return p, &httpError{http.StatusUnprocessableEntity, "tanggal akhir promo tidak valid"}
	}
	if err := model.ValidatePromoWindow(from, until); err != nil {
		return p, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	appliesTo := in.AppliesTo
	if appliesTo == "" {
		appliesTo = stageReregistration
	}
	if appliesTo != stageRegistration && appliesTo != stageReregistration {
		return p, &httpError{http.StatusUnprocessableEntity, "komponen promo harus formulir atau daftar ulang"}
	}
	cond := "{}"
	if len(in.Conditions) > 0 && string(in.Conditions) != "null" {
		if _, err := model.ParseDiscountConditions(in.Conditions); err != nil {
			return p, &httpError{http.StatusUnprocessableEntity, err.Error()}
		}
		cond = string(in.Conditions)
	}
	return promoRule{kind, value, from, until, appliesTo, cond}, nil
}

func dateOrNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.In(wib).Format("2006-01-02")
}

// ListDiscountRules is GET /admin/finance/discount-rules.
func (a *App) ListDiscountRules(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT d.id,d.name,d.kind,d.value,COALESCE(d.cap_idr,0),d.priority,d.status,d.stackable,d.period_id,
		        d.valid_from,d.valid_until,d.quota,d.applies_to,d.conditions,
		        (SELECT count(DISTINCT c.application_id) FROM discount_applications da JOIN charges c ON c.id=da.charge_id
		          WHERE da.origin='rule' AND da.source_id=d.id::text AND c.cancelled_at IS NULL)
		 FROM discount_rules d WHERE d.school_id=$1 AND ($2='' OR d.period_id::text=$2) ORDER BY d.priority,d.created_at`,
		common.EffectiveSchoolID(r), r.URL.Query().Get("period_id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	now := time.Now()
	for rows.Next() {
		var (
			id, name, kind, status, periodID, appliesTo string
			value, cap                                  int64
			priority, quota, used                       int
			stackable                                   bool
			from, until                                 sql.NullTime
			conditions                                  []byte
		)
		if err := rows.Scan(&id, &name, &kind, &value, &cap, &priority, &status, &stackable, &periodID,
			&from, &until, &quota, &appliesTo, &conditions, &used); err != nil {
			writeErr(w, err)
			return
		}
		value = ruleValueToAPI(kind, value)
		out = append(out, map[string]any{
			"id": id, "name": name, "type": ruleKindToAPI(kind), "value": value, "priority": priority, "cap": cap,
			"status": status, "stackable": stackable, "period_id": periodID,
			"valid_from": dateOrNil(nullTimePtr(from)), "valid_until": dateOrNil(nullTimePtr(until)),
			"quota": quota, "used": used, "applies_to": appliesTo, "conditions": json.RawMessage(conditions),
			"in_window": model.PromoActive(now, nullTimePtr(from), nullTimePtr(until)),
		})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// duplicateActiveRule reports another active rule with the same name in the
// period (the seed duplicated rules caused double discounts, BUG-011).
func duplicateActiveRule(ctx context.Context, q queryer, schoolID, periodID, name, excludeID string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM discount_rules WHERE school_id=$1 AND period_id::text=$2 AND lower(name)=lower($3)
		   AND status='active' AND id::text<>$4)`, schoolID, periodID, name, excludeID).Scan(&exists)
	return exists, err
}

// CreateDiscountRule is POST /admin/finance/discount-rules.
func (a *App) CreateDiscountRule(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in discountRuleInput
	if !common.Decode(w, r, &in) {
		return
	}
	rule, err := in.normalize()
	if err != nil {
		writeErr(w, err)
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	status := normalizeRuleStatus(in.Status)
	if status == "active" {
		dup, err := duplicateActiveRule(r.Context(), a.DB, schoolID, periodID, in.Name, "")
		if err != nil {
			writeErr(w, err)
			return
		}
		if dup {
			common.WriteError(w, http.StatusConflict, "promo aktif dengan nama yang sama sudah ada pada periode ini")
			return
		}
	}
	stackable := in.Stackable == nil || *in.Stackable
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO discount_rules(school_id,period_id,name,kind,value,cap_idr,priority,active,stackable,status,
		                            valid_from,valid_until,quota,applies_to,conditions)
		 VALUES($1,$2,$3,$4,$5,NULLIF($6,0),$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb) RETURNING id`,
		schoolID, periodID, in.Name, rule.kind, rule.value, in.Cap, in.Priority, status == "active", stackable, status,
		ptrTime(rule.from), ptrTime(rule.until), in.Quota, rule.appliesTo, rule.conditions).Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "discount_rule", id, "discount_rule_created", "admin", claims.UserID, claims.Name, "", nil, in)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateDiscountRule is PUT /admin/finance/discount-rules/{id}. Bills already
// issued keep their frozen promo snapshot (BR-PPDB-01).
func (a *App) UpdateDiscountRule(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in discountRuleInput
	if !common.Decode(w, r, &in) {
		return
	}
	rule, err := in.normalize()
	if err != nil {
		writeErr(w, err)
		return
	}
	var periodID string
	err = a.DB.QueryRowContext(r.Context(), `SELECT period_id FROM discount_rules WHERE id=$1 AND school_id=$2`, param(r, "id"), schoolID).Scan(&periodID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "promo tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	status := normalizeRuleStatus(in.Status)
	if status == "active" {
		dup, err := duplicateActiveRule(r.Context(), a.DB, schoolID, periodID, in.Name, param(r, "id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		if dup {
			common.WriteError(w, http.StatusConflict, "promo aktif dengan nama yang sama sudah ada pada periode ini")
			return
		}
	}
	stackable := in.Stackable == nil || *in.Stackable
	if _, err = a.DB.ExecContext(r.Context(),
		`UPDATE discount_rules SET name=$1,kind=$2,value=$3,cap_idr=NULLIF($4,0),priority=$5,active=$6,stackable=$7,status=$8,
		        valid_from=$9,valid_until=$10,quota=$11,applies_to=$12,conditions=$13::jsonb
		 WHERE id=$14 AND school_id=$15`,
		in.Name, rule.kind, rule.value, in.Cap, in.Priority, status == "active", stackable, status,
		ptrTime(rule.from), ptrTime(rule.until), in.Quota, rule.appliesTo, rule.conditions, param(r, "id"), schoolID); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "discount_rule", param(r, "id"), "discount_rule_updated", "admin", claims.UserID, claims.Name, "", nil, in)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

// voucherWindow parses and validates a voucher validity window (TC-09-028:
// an already expired voucher is refused).
func (in voucherInput) window(now time.Time, requireUntil bool) (*time.Time, *time.Time, error) {
	from, err := model.ParseWIBDate(in.ValidFrom, false)
	if err != nil {
		return nil, nil, &httpError{http.StatusUnprocessableEntity, "tanggal mulai voucher tidak valid"}
	}
	until, err := model.ParseWIBDate(in.ValidUntil, true)
	if err != nil {
		return nil, nil, &httpError{http.StatusUnprocessableEntity, "tanggal akhir voucher tidak valid"}
	}
	if until == nil && requireUntil {
		return nil, nil, &httpError{http.StatusUnprocessableEntity, "tanggal akhir voucher wajib diisi"}
	}
	if until != nil && until.Before(now) {
		return nil, nil, &httpError{http.StatusUnprocessableEntity, "tanggal akhir voucher sudah lewat"}
	}
	if err := model.ValidatePromoWindow(from, until); err != nil {
		return nil, nil, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	return from, until, nil
}

// CreateVoucher is POST /admin/finance/vouchers.
func (a *App) CreateVoucher(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in voucherInput
	if !common.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Code) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "kode voucher wajib diisi")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		writeErr(w, err)
		return
	}
	from, until, err := in.window(time.Now(), true)
	if err != nil {
		writeErr(w, err)
		return
	}
	if from == nil {
		now := time.Now()
		from = &now
	}
	hash, err := hashVoucher(strings.TrimSpace(in.Code))
	if err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO discount_vouchers(school_id,period_id,code_hash,code_hint,kind,value,cap_idr,priority,usage_limit,valid_from,valid_until,status)
		 VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,0),$8,$9,$10,$11,$12) RETURNING id`,
		schoolID, periodID, hash, maskCode(in.Code), kind, value, in.Cap, in.Priority, maxInt(in.Quota, 1), *from, *until,
		normalizeVoucherStatus(in.Status)).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			common.WriteError(w, http.StatusConflict, "kode voucher sudah dipakai")
			return
		}
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "voucher", id, "voucher_created", "admin", claims.UserID, claims.Name, "", nil,
		map[string]any{"code": maskCode(in.Code), "type": in.Type, "value": in.Value, "valid_until": until})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "code": maskCode(in.Code)}, nil)
}

// UpdateVoucher is PUT /admin/finance/vouchers/{id}.
func (a *App) UpdateVoucher(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in voucherInput
	if !common.Decode(w, r, &in) {
		return
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		writeErr(w, err)
		return
	}
	from, until, err := in.window(time.Now(), false)
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE discount_vouchers SET kind=$1,value=$2,cap_idr=NULLIF($3,0),priority=$4,usage_limit=$5,
		        valid_from=COALESCE($6,valid_from),valid_until=COALESCE($7,valid_until),status=$8
		 WHERE id=$9 AND school_id=$10`,
		kind, value, in.Cap, in.Priority, maxInt(in.Quota, 1), ptrTime(from), ptrTime(until), normalizeVoucherStatus(in.Status),
		param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "voucher tidak ditemukan")
		return
	}
	a.audit(r.Context(), schoolID, "voucher", param(r, "id"), "voucher_updated", "admin", claims.UserID, claims.Name, "", nil,
		map[string]any{"type": in.Type, "value": in.Value, "valid_until": until})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}
