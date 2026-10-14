package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// Account mapping administration for the auto-journals (BR-ACC-05) and the
// journal outbox monitor (IF-ACC-02). Finance and accounting roles.

func (a *App) journalAdmin(w http.ResponseWriter, r *http.Request) (*common.Claims, string) {
	c := common.RequireFinanceOrAccounting(w, r)
	if c == nil {
		return nil, ""
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return nil, ""
	}
	return c, schoolID
}

type knownComponent struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// GetJournalAccounts is GET /admin/journal-accounts: unit settings, gateway
// accounts, component mappings and the component codes in use.
func (a *App) GetJournalAccounts(w http.ResponseWriter, r *http.Request) {
	_, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	ctx := r.Context()
	set, err := a.loadJournalSettings(ctx, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var gw *model.GatewayAccounts
	var g model.GatewayAccounts
	err = a.DB.QueryRowContext(ctx, `SELECT clearing_acc, fee_expense_acc, receivable_acc, deposit_acc FROM gateway_accounts WHERE school_id = $1`, schoolID).
		Scan(&g.Clearing, &g.FeeExpense, &g.Receivable, &g.Deposit)
	switch {
	case err == nil:
		gw = &g
	case err != sql.ErrNoRows:
		writeErr(w, err)
		return
	}
	comps, err := a.listComponentAccounts(r, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT code, min(name) FROM (
		   SELECT c.code, c.name FROM billing_scheme_components c JOIN billing_schemes s ON s.id = c.scheme_id
		    WHERE c.school_id = $1 AND s.status <> 'archived'
		   UNION ALL
		   SELECT DISTINCT source_module, source_module FROM student_bills WHERE school_id = $1 AND source_module IS NOT NULL
		 ) k GROUP BY code ORDER BY code`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	known := []knownComponent{}
	for rows.Next() {
		var k knownComponent
		if err := rows.Scan(&k.Code, &k.Name); err != nil {
			writeErr(w, err)
			return
		}
		known = append(known, k)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"settings": set, "gateway_accounts": gw, "components": comps, "known_components": known,
	}, nil)
}

func (a *App) listComponentAccounts(r *http.Request, schoolID string) ([]model.ComponentAccount, error) {
	m, err := a.loadComponentAccounts(r.Context(), schoolID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ComponentAccount, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}

func trimAcc(v string) any {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

// PutJournalSettings is PUT /admin/journal-accounts/settings.
func (a *App) PutJournalSettings(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	var in model.JournalSettings
	if !common.Decode(w, r, &in) {
		return
	}
	before, err := a.loadJournalSettings(r.Context(), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO billing_journal_settings (school_id, unit_div_code, receivable_acc, cash_acc, bank_acc, discount_acc, scholarship_acc, refund_acc, deposit_acc, updated_by, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, '')::uuid, now())
		 ON CONFLICT (school_id) DO UPDATE SET unit_div_code = EXCLUDED.unit_div_code, receivable_acc = EXCLUDED.receivable_acc,
		   cash_acc = EXCLUDED.cash_acc, bank_acc = EXCLUDED.bank_acc, discount_acc = EXCLUDED.discount_acc,
		   scholarship_acc = EXCLUDED.scholarship_acc, refund_acc = EXCLUDED.refund_acc, deposit_acc = EXCLUDED.deposit_acc,
		   updated_by = EXCLUDED.updated_by, updated_at = now()`,
		schoolID, trimAcc(in.UnitDivCode), trimAcc(in.ReceivableAcc), trimAcc(in.CashAcc), trimAcc(in.BankAcc),
		trimAcc(in.DiscountAcc), trimAcc(in.ScholarshipAcc), trimAcc(in.RefundAcc), trimAcc(in.DepositAcc), uuidOrEmpty(c.UserID)); err != nil {
		writeErr(w, err)
		return
	}
	after, err := a.loadJournalSettings(r.Context(), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "journal_settings", schoolID, "journal_settings_updated", c.ActiveRole, c.UserID, "", "", before, after)
	common.WriteJSON(w, http.StatusOK, after, nil)
}

func uuidOrEmpty(v string) string {
	if len(v) == 36 && strings.Count(v, "-") == 4 {
		return v
	}
	return ""
}

// PutGatewayAccounts is PUT /admin/journal-accounts/gateway (all four
// accounts are required by the daily gateway journal).
func (a *App) PutGatewayAccounts(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	var in model.GatewayAccounts
	if !common.Decode(w, r, &in) {
		return
	}
	in.Clearing, in.FeeExpense = strings.TrimSpace(in.Clearing), strings.TrimSpace(in.FeeExpense)
	in.Receivable, in.Deposit = strings.TrimSpace(in.Receivable), strings.TrimSpace(in.Deposit)
	if in.Clearing == "" || in.FeeExpense == "" || in.Receivable == "" || in.Deposit == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "keempat akun payment gateway wajib diisi")
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO gateway_accounts (school_id, clearing_acc, fee_expense_acc, receivable_acc, deposit_acc)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (school_id) DO UPDATE SET clearing_acc = EXCLUDED.clearing_acc, fee_expense_acc = EXCLUDED.fee_expense_acc,
		   receivable_acc = EXCLUDED.receivable_acc, deposit_acc = EXCLUDED.deposit_acc, updated_at = now()`,
		schoolID, in.Clearing, in.FeeExpense, in.Receivable, in.Deposit); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "gateway_accounts", schoolID, "gateway_accounts_updated", c.ActiveRole, c.UserID, "", "", nil, in)
	common.WriteJSON(w, http.StatusOK, in, nil)
}

// PutComponentAccount is PUT /admin/journal-accounts/components/{code}.
func (a *App) PutComponentAccount(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	code := strings.TrimSpace(param(r, "code"))
	var in model.ComponentAccount
	if !common.Decode(w, r, &in) {
		return
	}
	in.Code, in.Name = code, strings.TrimSpace(in.Name)
	in.RevenueAcc, in.ReceivableAcc = strings.TrimSpace(in.RevenueAcc), strings.TrimSpace(in.ReceivableAcc)
	if code == "" || in.RevenueAcc == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "kode komponen dan akun pendapatan wajib diisi")
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc, receivable_acc, updated_by)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid)
		 ON CONFLICT (school_id, component_code) DO UPDATE SET name = EXCLUDED.name, revenue_acc = EXCLUDED.revenue_acc,
		   receivable_acc = EXCLUDED.receivable_acc, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		schoolID, code, in.Name, in.RevenueAcc, trimAcc(in.ReceivableAcc), uuidOrEmpty(c.UserID)); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "fee_component_account", code, "component_account_saved", c.ActiveRole, c.UserID, "", "", nil, in)
	common.WriteJSON(w, http.StatusOK, in, nil)
}

// DeleteComponentAccount is DELETE /admin/journal-accounts/components/{code}.
func (a *App) DeleteComponentAccount(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	code := param(r, "code")
	// BUG-050: a mapping still used by an active scheme cannot be removed,
	// otherwise the next generation or posting would have no account.
	var schemes []string
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT DISTINCT s.name FROM billing_scheme_components c JOIN billing_schemes s ON s.id = c.scheme_id
		  WHERE c.school_id = $1 AND c.code = $2 AND s.status <> 'archived' ORDER BY 1`, schoolID, code)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		schemes = append(schemes, n)
	}
	rows.Close()
	if len(schemes) > 0 {
		common.WriteError(w, http.StatusConflict, "pemetaan masih dipakai skema aktif: "+strings.Join(schemes, ", ")+"; arsipkan skema atau ganti akunnya")
		return
	}
	res, err := a.DB.ExecContext(r.Context(), `DELETE FROM fee_component_accounts WHERE school_id = $1 AND component_code = $2`, schoolID, code)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "pemetaan komponen tidak ditemukan")
		return
	}
	a.audit(r.Context(), schoolID, "fee_component_account", code, "component_account_deleted", c.ActiveRole, c.UserID, "", "", nil, nil)
	common.WriteJSON(w, http.StatusOK, map[string]any{"component_code": code}, nil)
}

type journalPostingView struct {
	ID          string              `json:"id"`
	Reference   string              `json:"reference"`
	Kind        string              `json:"kind"`
	Source      string              `json:"source"`
	TransDate   string              `json:"trans_date"`
	Description string              `json:"description"`
	TotalIDR    int64               `json:"total_idr"`
	Status      string              `json:"status"`
	Attempts    int                 `json:"attempts"`
	TransNo     *string             `json:"trans_no"`
	LastError   *string             `json:"last_error"`
	NextAttempt time.Time           `json:"next_attempt_at"`
	PostedAt    *time.Time          `json:"posted_at"`
	CreatedAt   time.Time           `json:"created_at"`
	Lines       []model.JournalLine `json:"lines,omitempty"`
	SourceRefs  map[string][]string `json:"source_refs,omitempty"`
}

const postingCols = `id::text, reference, kind, source, trans_date, description, lines, status, attempts, trans_no, last_error,
	next_attempt_at, posted_at, created_at, source_refs`

func scanPosting(sc interface{ Scan(...any) error }, detail bool) (journalPostingView, error) {
	var v journalPostingView
	var day time.Time
	var lines, refs []byte
	err := sc.Scan(&v.ID, &v.Reference, &v.Kind, &v.Source, &day, &v.Description, &lines, &v.Status, &v.Attempts,
		&v.TransNo, &v.LastError, &v.NextAttempt, &v.PostedAt, &v.CreatedAt, &refs)
	if err != nil {
		return v, err
	}
	v.TransDate = day.Format("2006-01-02")
	var ls []model.JournalLine
	_ = json.Unmarshal(lines, &ls)
	for _, l := range ls {
		v.TotalIDR += l.DebitIDR
	}
	if detail {
		v.Lines = ls
		_ = json.Unmarshal(refs, &v.SourceRefs)
	}
	return v, nil
}

// ListJournalPostings is GET /admin/journal-postings?status=&kind=&page=.
func (a *App) ListJournalPostings(w http.ResponseWriter, r *http.Request) {
	_, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	const per = 50
	where := `school_id = $1 AND ($2 = '' OR status = $2) AND ($3 = '' OR kind = $3)`
	args := []any{schoolID, q.Get("status"), q.Get("kind")}
	var total int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM gateway_journal_postings WHERE `+where, args...).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT `+postingCols+` FROM gateway_journal_postings WHERE `+where+` ORDER BY trans_date DESC, created_at DESC LIMIT $4 OFFSET $5`,
		append(args, per, (page-1)*per)...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []journalPostingView{}
	for rows.Next() {
		v, err := scanPosting(rows, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, v)
	}
	common.WriteJSON(w, http.StatusOK, out, map[string]any{"page": page, "per_page": per, "total": total})
}

// GetJournalPosting is GET /admin/journal-postings/{id} with lines and the
// detail transactions behind the summary (BR-ACC-06).
func (a *App) GetJournalPosting(w http.ResponseWriter, r *http.Request) {
	_, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	v, err := scanPosting(a.DB.QueryRowContext(r.Context(),
		`SELECT `+postingCols+` FROM gateway_journal_postings WHERE id::text = $1 AND school_id = $2`, param(r, "id"), schoolID), true)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "posting jurnal tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, v, nil)
}

// AdminRetryJournalPosting is POST /admin/journal-postings/{id}/retry.
func (a *App) AdminRetryJournalPosting(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.journalAdmin(w, r)
	if schoolID == "" {
		return
	}
	a.audit(r.Context(), schoolID, "journal_posting", param(r, "id"), "journal_retry_requested", c.ActiveRole, c.UserID, "", "", nil, nil)
	a.retryJournalPosting(w, r, schoolID)
}
