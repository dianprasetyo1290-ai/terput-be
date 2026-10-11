package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// resolvePeriodID picks a period for the config forms that do not send one:
// an explicit query param wins, otherwise the most recent period is used.
func (a *App) resolvePeriodID(r *http.Request, schoolID, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if q := r.URL.Query().Get("period_id"); q != "" {
		return q, nil
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id FROM admission_periods WHERE school_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`, schoolID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", &httpError{http.StatusUnprocessableEntity, "no period configured yet"}
	}
	return id, err
}

// ---------------------------------------------------------------------------
// Fee schemes
// ---------------------------------------------------------------------------

type feeSchemeInput struct {
	Name              string `json:"name"`
	PeriodID          string `json:"period_id"`
	TrackID           string `json:"track_id"`
	Status            string `json:"status"`
	RegistrationFee   int64  `json:"registration_fee"`
	ReregistrationFee int64  `json:"reregistration_fee"`
	InstallmentCount  int    `json:"installment_count"`
}

// ListFeeSchemes is GET /admin/finance/fee-schemes.
func (a *App) ListFeeSchemes(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,name,period_id,COALESCE(track_id::text,''),status,revision,registration_fee_idr,reregistration_fee_idr,installment_count
		 FROM fee_schemes WHERE school_id=$1 ORDER BY created_at DESC`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, periodID, trackID, status string
			revision                            int
			regFee, reregFee                    int64
			installmentCount                    int
		)
		if err := rows.Scan(&id, &name, &periodID, &trackID, &status, &revision, &regFee, &reregFee, &installmentCount); err != nil {
			writeErr(w, err)
			return
		}
		items, err := a.feeItems(r, schoolID, id)
		if err != nil {
			writeErr(w, err)
			return
		}
		installments, err := a.installments(r.Context(), schoolID, id)
		if err != nil {
			writeErr(w, err)
			return
		}
		scheme := map[string]any{
			"id":                 id,
			"name":               name,
			"period_id":          periodID,
			"status":             status,
			"revision":           revision,
			"registration_fee":   regFee,
			"reregistration_fee": reregFee,
			"items":              items,
			"installments":       installments,
		}
		if trackID != "" {
			scheme["track_id"] = trackID
		}
		out = append(out, scheme)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (a *App) feeItems(r *http.Request, schoolID, schemeID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,name,amount_idr FROM fee_items WHERE fee_scheme_id=$1 AND school_id=$2 ORDER BY code`, schemeID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name string
			amount   int64
		)
		if err := rows.Scan(&id, &name, &amount); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "amount": amount, "discount": 0, "net_amount": amount})
	}
	return out, rows.Err()
}

// CreateFeeScheme is POST /admin/finance/fee-schemes.
func (a *App) CreateFeeScheme(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in feeSchemeInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	in.Status = normalizeSchemeStatus(in.Status)

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var revision int
	if err = tx.QueryRowContext(r.Context(),
		`SELECT COALESCE(max(revision),0)+1 FROM fee_schemes WHERE school_id=$1 AND period_id=$2 AND track_id IS NULL`,
		schoolID, periodID).Scan(&revision); err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO fee_schemes(school_id,period_id,track_id,name,revision,status,registration_fee_idr,reregistration_fee_idr,installment_count)
		 VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9) RETURNING id`,
		schoolID, periodID, in.TrackID, in.Name, revision, in.Status, in.RegistrationFee, in.ReregistrationFee, maxInt(in.InstallmentCount, 1)).Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = writeFeeSchemeChildren(r, tx, schoolID, id, in); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "revision": revision, "status": in.Status}, nil)
}

// UpdateFeeScheme is PUT /admin/finance/fee-schemes/{id}.
func (a *App) UpdateFeeScheme(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in feeSchemeInput
	if !common.Decode(w, r, &in) {
		return
	}
	id := param(r, "id")
	in.Status = normalizeSchemeStatus(in.Status)

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(r.Context(),
		`UPDATE fee_schemes SET name=COALESCE(NULLIF($1,''),name),status=$2,registration_fee_idr=$3,reregistration_fee_idr=$4,installment_count=$5
		 WHERE id=$6 AND school_id=$7`,
		in.Name, in.Status, in.RegistrationFee, in.ReregistrationFee, maxInt(in.InstallmentCount, 1), id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "fee scheme not found")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM fee_items WHERE fee_scheme_id=$1`, id); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM installment_plans WHERE fee_scheme_id=$1`, id); err != nil {
		writeErr(w, err)
		return
	}
	if err = writeFeeSchemeChildren(r, tx, schoolID, id, in); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": in.Status}, nil)
}

func writeFeeSchemeChildren(r *http.Request, tx *sql.Tx, schoolID, schemeID string, in feeSchemeInput) error {
	items := []struct {
		code, name string
		amount     int64
	}{
		{"registration", "Biaya Pendaftaran", in.RegistrationFee},
		{"reregistration", "Biaya Daftar Ulang", in.ReregistrationFee},
	}
	for _, item := range items {
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO fee_items(school_id,fee_scheme_id,code,name,amount_idr) VALUES($1,$2,$3,$4,$5)`,
			schoolID, schemeID, item.code, item.name, item.amount); err != nil {
			return err
		}
	}
	count := maxInt(in.InstallmentCount, 1)
	total := in.RegistrationFee + in.ReregistrationFee
	base := total / int64(count)
	remainder := total - base*int64(count)
	for i := 0; i < count; i++ {
		amount := base
		if int64(i) == int64(count)-1 {
			amount += remainder
		}
		due := time.Now().AddDate(0, i, 0)
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO installment_plans(school_id,fee_scheme_id,sequence,description,amount_idr,due_at) VALUES($1,$2,$3,$4,$5,$6)`,
			schoolID, schemeID, i+1, "Termin "+itoa(i+1), amount, due); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSchemeStatus(status string) string {
	switch strings.ToLower(status) {
	case "published", "archived", "draft":
		return strings.ToLower(status)
	default:
		return "draft"
	}
}

// ---------------------------------------------------------------------------
// Discount rules
// ---------------------------------------------------------------------------

type discountRuleInput struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Value     int64  `json:"value"`
	Priority  int    `json:"priority"`
	Cap       int64  `json:"cap"`
	Status    string `json:"status"`
	Stackable *bool  `json:"stackable"`
	PeriodID  string `json:"period_id"`
}

// ListDiscountRules is GET /admin/finance/discount-rules.
func (a *App) ListDiscountRules(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,name,kind,value,COALESCE(cap_idr,0),priority,status,stackable,period_id
		 FROM discount_rules WHERE school_id=$1 ORDER BY priority,created_at`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, kind, status, periodID string
			value, cap                       int64
			priority                         int
			stackable                        bool
		)
		if err := rows.Scan(&id, &name, &kind, &value, &cap, &priority, &status, &stackable, &periodID); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, map[string]any{
			"id":        id,
			"name":      name,
			"type":      ruleKindToAPI(kind),
			"value":     value,
			"priority":  priority,
			"cap":       cap,
			"status":    status,
			"stackable": stackable,
			"period_id": periodID,
		})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreateDiscountRule is POST /admin/finance/discount-rules.
func (a *App) CreateDiscountRule(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in discountRuleInput
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	stackable := in.Stackable == nil || *in.Stackable
	status := normalizeRuleStatus(in.Status)
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO discount_rules(school_id,period_id,name,kind,value,cap_idr,priority,active,stackable,status)
		 VALUES($1,$2,$3,$4,$5,NULLIF($6,0),$7,$8,$9,$10) RETURNING id`,
		schoolID, periodID, in.Name, kind, value, in.Cap, in.Priority, status == "active", stackable, status).Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateDiscountRule is PUT /admin/finance/discount-rules/{id}.
func (a *App) UpdateDiscountRule(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in discountRuleInput
	if !common.Decode(w, r, &in) {
		return
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	stackable := in.Stackable == nil || *in.Stackable
	status := normalizeRuleStatus(in.Status)
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE discount_rules SET name=COALESCE(NULLIF($1,''),name),kind=$2,value=$3,cap_idr=NULLIF($4,0),priority=$5,active=$6,stackable=$7,status=$8
		 WHERE id=$9 AND school_id=$10`,
		in.Name, kind, value, in.Cap, in.Priority, status == "active", stackable, status, param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "discount rule not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

// ---------------------------------------------------------------------------
// Vouchers
// ---------------------------------------------------------------------------

type voucherInput struct {
	Code       string `json:"code"`
	Type       string `json:"type"`
	Value      int64  `json:"value"`
	Quota      int    `json:"quota"`
	ValidUntil string `json:"valid_until"`
	Status     string `json:"status"`
	PeriodID   string `json:"period_id"`
	Cap        int64  `json:"cap"`
	Priority   int    `json:"priority"`
}

// ListVouchers is GET /admin/finance/vouchers. Only a masked hint of the code
// is ever returned; the plaintext is not stored.
func (a *App) ListVouchers(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,COALESCE(code_hint,''),kind,value,usage_limit,used_count,valid_until,COALESCE(status,'active')
		 FROM discount_vouchers WHERE school_id=$1 ORDER BY created_at DESC`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, hint, kind, status string
			value                  int64
			limit, used            int
			validUntil             time.Time
		)
		if err := rows.Scan(&id, &hint, &kind, &value, &limit, &used, &validUntil, &status); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, map[string]any{
			"id":          id,
			"code":        hint,
			"type":        ruleKindToAPI(kind),
			"value":       value,
			"quota":       limit,
			"used":        used,
			"valid_until": validUntil,
			"status":      status,
		})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CreateVoucher is POST /admin/finance/vouchers.
func (a *App) CreateVoucher(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in voucherInput
	if !common.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Code) == "" {
		common.WriteError(w, http.StatusBadRequest, "code is required")
		return
	}
	periodID, err := a.resolvePeriodID(r, schoolID, in.PeriodID)
	if err != nil {
		writeErr(w, err)
		return
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	validUntil, err := parseTime(in.ValidUntil)
	if err != nil || validUntil.IsZero() {
		common.WriteError(w, http.StatusBadRequest, "valid_until is required")
		return
	}
	hash, err := hashVoucher(strings.TrimSpace(in.Code))
	if err != nil {
		writeErr(w, err)
		return
	}
	quota := maxInt(in.Quota, 1)
	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO discount_vouchers(school_id,period_id,code_hash,code_hint,kind,value,cap_idr,priority,usage_limit,valid_from,valid_until,status)
		 VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,0),$8,$9,now(),$10,$11) RETURNING id`,
		schoolID, periodID, hash, maskCode(in.Code), kind, value, in.Cap, in.Priority, quota, validUntil, normalizeVoucherStatus(in.Status)).
		Scan(&id)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "code": maskCode(in.Code)}, nil)
}

// UpdateVoucher is PUT /admin/finance/vouchers/{id}.
func (a *App) UpdateVoucher(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var in voucherInput
	if !common.Decode(w, r, &in) {
		return
	}
	kind, value, err := ruleKindFromAPI(in.Type, in.Value)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	validUntil, err := parseTime(in.ValidUntil)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "valid_until is invalid")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE discount_vouchers SET kind=$1,value=$2,cap_idr=NULLIF($3,0),priority=$4,usage_limit=$5,
		        valid_until=COALESCE(NULLIF($6,'')::timestamptz,valid_until),status=$7
		 WHERE id=$8 AND school_id=$9`,
		kind, value, in.Cap, in.Priority, maxInt(in.Quota, 1), validUntil.Format(time.RFC3339), normalizeVoucherStatus(in.Status), param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "voucher not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id")}, nil)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ruleKindToAPI(kind string) string {
	if kind == string(model.DiscountPercent) {
		return "percentage"
	}
	return "fixed"
}

// ruleKindFromAPI converts the frontend percent value (0-100) into the engine's
// basis-point scale.
func ruleKindFromAPI(apiType string, value int64) (model.DiscountKind, int64, error) {
	switch apiType {
	case "percentage", "percent":
		if value < 0 || value > 100 {
			return "", 0, &httpError{http.StatusUnprocessableEntity, "percentage value must be between 0 and 100"}
		}
		return model.DiscountPercent, value * 100, nil
	case "fixed", "":
		if value < 0 {
			return "", 0, &httpError{http.StatusUnprocessableEntity, "value must not be negative"}
		}
		return model.DiscountFixed, value, nil
	default:
		return "", 0, &httpError{http.StatusUnprocessableEntity, "invalid discount type"}
	}
}

func normalizeRuleStatus(status string) string {
	if strings.EqualFold(status, "inactive") {
		return "inactive"
	}
	return "active"
}

func normalizeVoucherStatus(status string) string {
	if strings.EqualFold(status, "inactive") {
		return "inactive"
	}
	return "active"
}

func maskCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) <= 4 {
		return strings.Repeat("*", len(code))
	}
	return "****" + code[len(code)-4:]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
