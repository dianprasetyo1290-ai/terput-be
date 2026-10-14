package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/gateway"
)

// providerDTO never carries credentials: only the env prefix and whether
// the credentials are installed (FR-150-001).
type providerDTO struct {
	Code                 string     `json:"code"`
	Adapter              string     `json:"adapter"`
	Label                string     `json:"label"`
	Enabled              bool       `json:"enabled"`
	CredentialRef        *string    `json:"credential_ref"`
	CredentialsInstalled bool       `json:"credentials_installed"`
	Capabilities         []string   `json:"capabilities"`
	AdapterCapabilities  []string   `json:"adapter_capabilities"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	LastFailureAt        *time.Time `json:"last_failure_at,omitempty"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type providerInput struct {
	Code          string   `json:"code"`
	Adapter       string   `json:"adapter"`
	Label         string   `json:"label"`
	Enabled       *bool    `json:"enabled"`
	CredentialRef *string  `json:"credential_ref"`
	Capabilities  []string `json:"capabilities"`
}

var (
	providerCodeRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	credentialRefRe  = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,31}$`)
	errProviderInput = errors.New("input provider tidak valid")
)

func (a *App) listProviderDTOs(ctx context.Context, where string, args ...any) ([]providerDTO, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT p.code, p.adapter, p.label, p.enabled, p.credential_ref, p.consecutive_failures, p.last_failure_at, p.updated_at,
		        COALESCE(array_to_string(array_agg(c.capability ORDER BY c.capability) FILTER (WHERE c.capability IS NOT NULL), ','), '')
		   FROM payment_providers p LEFT JOIN payment_provider_capabilities c ON c.provider_code = p.code
		  WHERE `+where+` GROUP BY p.code ORDER BY p.code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []providerDTO{}
	for rows.Next() {
		var p providerDTO
		var ref sql.NullString
		var last sql.NullTime
		var caps string
		if err := rows.Scan(&p.Code, &p.Adapter, &p.Label, &p.Enabled, &ref, &p.ConsecutiveFailures, &last, &p.UpdatedAt, &caps); err != nil {
			return nil, err
		}
		p.CredentialRef = optString(ref.String)
		p.CredentialsInstalled = a.credentialsInstalled(p.Adapter, ref.String)
		p.Capabilities = splitNonEmpty(caps, ",")
		p.AdapterCapabilities = gateway.AdapterCapabilities(p.Adapter)
		if last.Valid {
			p.LastFailureAt = &last.Time
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PlatformListProviders: GET /platform/payment-providers (superadmin).
func (a *App) PlatformListProviders(w http.ResponseWriter, r *http.Request) {
	if common.RequirePlatformAdmin(w, r) == nil {
		return
	}
	out, err := a.listProviderDTOs(r.Context(), "true")
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, out, nil)
}

func validateCapabilities(adapter string, caps []string) error {
	allowed := gateway.AdapterCapabilities(adapter)
	for _, c := range caps {
		if !gateway.HasCapability(allowed, c) {
			return &httpError{status: http.StatusBadRequest, msg: "kapabilitas " + c + " tidak didukung adapter " + adapter}
		}
	}
	return nil
}

func normalizeCredentialRef(ref *string) (any, error) {
	if ref == nil || strings.TrimSpace(*ref) == "" {
		return nil, nil
	}
	v := strings.TrimSpace(*ref)
	if !credentialRefRe.MatchString(v) {
		return nil, &httpError{status: http.StatusBadRequest, msg: "credential_ref harus berupa prefix env huruf besar, mis. NICEPAY"}
	}
	return v, nil
}

// PlatformCreateProvider: POST /platform/payment-providers.
func (a *App) PlatformCreateProvider(w http.ResponseWriter, r *http.Request) {
	claims := common.RequirePlatformAdmin(w, r)
	if claims == nil {
		return
	}
	var in providerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.Code, in.Label = strings.TrimSpace(in.Code), strings.TrimSpace(in.Label)
	if !providerCodeRe.MatchString(in.Code) || in.Label == "" || gateway.AdapterCapabilities(in.Adapter) == nil {
		common.WriteError(w, http.StatusBadRequest, "kode (huruf kecil), label, dan adapter (nicepay/sim) wajib valid")
		return
	}
	if in.Adapter == gateway.AdapterSim && !a.Config.SimulatorAllowed() {
		common.WriteError(w, http.StatusBadRequest, "adapter simulator hanya tersedia bila PAYMENT_SIMULATOR_ENABLED=true di luar production")
		return
	}
	ref, err := normalizeCredentialRef(in.CredentialRef)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := validateCapabilities(in.Adapter, in.Capabilities); err != nil {
		writeErr(w, err)
		return
	}
	enabled := in.Enabled != nil && *in.Enabled
	if err := a.saveProvider(r.Context(), true, in.Code, in.Adapter, in.Label, enabled, ref, in.Capabilities); err != nil {
		writeErr(w, err)
		return
	}
	out, _ := a.listProviderDTOs(r.Context(), "p.code = $1", in.Code)
	if len(out) == 1 {
		a.paymentAudit(r.Context(), claims, claims.SchoolID, "payment_provider", in.Code, "create", "", nil, out[0])
		common.WriteJSON(w, http.StatusCreated, out[0], nil)
		return
	}
	writeErr(w, errProviderInput)
}

// PlatformUpdateProvider: PUT /platform/payment-providers/{code}. The
// adapter and code are fixed; label, enabled, credential_ref and
// capabilities may change. No approval is required (§4.5), only audit.
func (a *App) PlatformUpdateProvider(w http.ResponseWriter, r *http.Request) {
	claims := common.RequirePlatformAdmin(w, r)
	if claims == nil {
		return
	}
	code := param(r, "code")
	before, err := a.listProviderDTOs(r.Context(), "p.code = $1", code)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(before) != 1 {
		common.WriteError(w, http.StatusNotFound, "provider tidak ditemukan")
		return
	}
	var in providerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cur := before[0]
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = cur.Label
	}
	enabled := cur.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	refIn := in.CredentialRef
	if refIn == nil {
		refIn = cur.CredentialRef
	}
	ref, err := normalizeCredentialRef(refIn)
	if err != nil {
		writeErr(w, err)
		return
	}
	caps := in.Capabilities
	if caps == nil {
		caps = cur.Capabilities
	}
	if err := validateCapabilities(cur.Adapter, caps); err != nil {
		writeErr(w, err)
		return
	}
	if err := a.saveProvider(r.Context(), false, code, cur.Adapter, label, enabled, ref, caps); err != nil {
		writeErr(w, err)
		return
	}
	a.providerCache.Delete(code)
	after, _ := a.listProviderDTOs(r.Context(), "p.code = $1", code)
	if len(after) == 1 {
		a.paymentAudit(r.Context(), claims, claims.SchoolID, "payment_provider", code, "update", "", cur, after[0])
		common.WriteSuccess(w, after[0], nil)
		return
	}
	writeErr(w, errProviderInput)
}

func (a *App) saveProvider(ctx context.Context, create bool, code, adapter, label string, enabled bool, ref any, caps []string) error {
	dbTx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer dbTx.Rollback()
	if create {
		_, err = dbTx.ExecContext(ctx,
			`INSERT INTO payment_providers (code, adapter, label, enabled, credential_ref) VALUES ($1,$2,$3,$4,$5)`,
			code, adapter, label, enabled, ref)
		if isUniqueViolation(err) {
			return &httpError{status: http.StatusConflict, msg: "kode provider sudah dipakai"}
		}
	} else {
		_, err = dbTx.ExecContext(ctx,
			`UPDATE payment_providers SET label = $2, enabled = $3, credential_ref = $4, updated_at = now() WHERE code = $1`,
			code, label, enabled, ref)
	}
	if err != nil {
		return err
	}
	if _, err := dbTx.ExecContext(ctx, `DELETE FROM payment_provider_capabilities WHERE provider_code = $1`, code); err != nil {
		return err
	}
	for _, c := range caps {
		if _, err := dbTx.ExecContext(ctx,
			`INSERT INTO payment_provider_capabilities (provider_code, capability) VALUES ($1, $2) ON CONFLICT DO NOTHING`, code, c); err != nil {
			return err
		}
	}
	return dbTx.Commit()
}

// ListPaymentProviderOptions: GET /payment-providers/options — enabled
// providers with capabilities (no credential data) for routing and mandate
// forms of admin, finance_manager and parents.
func (a *App) ListPaymentProviderOptions(w http.ResponseWriter, r *http.Request) {
	if common.FromCtx(r) == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	providers, err := a.loadProviders(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	type option struct {
		Code         string   `json:"code"`
		Label        string   `json:"label"`
		Capabilities []string `json:"capabilities"`
	}
	labels := map[string]string{}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT code, label FROM payment_providers WHERE enabled`)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var c, l string
		if rows.Scan(&c, &l) == nil {
			labels[c] = l
		}
	}
	rows.Close()
	out := []option{}
	for _, code := range sortedKeys(providers) {
		p := providers[code]
		if p.Enabled {
			out = append(out, option{Code: code, Label: labels[code], Capabilities: p.Capabilities})
		}
	}
	common.WriteSuccess(w, out, nil)
}

func sortedKeys(m map[string]gateway.ProviderInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
