package payroll

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// AccountingClient posts payroll journals to the accounting service
// (POST {ACCOUNTING_URL}/api/internal/journals, X-Internal-Key). An empty
// BaseURL means the integration is not configured: journals stay pending.
type AccountingClient struct {
	BaseURL     string
	InternalKey string
	HTTP        *http.Client
}

func NewAccountingClient(baseURL, internalKey string) *AccountingClient {
	return &AccountingClient{
		BaseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		InternalKey: internalKey,
		HTTP:        &http.Client{Timeout: 15 * time.Second},
	}
}

// ErrAccountingNotConfigured: ACCOUNTING_URL is empty.
var ErrAccountingNotConfigured = errors.New("integrasi akuntansi belum dikonfigurasi (ACCOUNTING_URL kosong)")

func (c *AccountingClient) Configured() bool { return c != nil && c.BaseURL != "" }

// JournalLine is one accounting line (integer rupiah).
type JournalLine struct {
	AccCode   string `json:"acc_code"`
	DebitIDR  int64  `json:"debit_idr"`
	CreditIDR int64  `json:"credit_idr"`
	Memo      string `json:"memo,omitempty"`
}

// Post sends a journal and returns the accounting transaction number.
func (c *AccountingClient) Post(ctx context.Context, payload map[string]any) (string, error) {
	if !c.Configured() {
		return "", ErrAccountingNotConfigured
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/internal/journals", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.InternalKey != "" {
		req.Header.Set("X-Internal-Key", c.InternalKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("layanan akuntansi tidak dapat dihubungi")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var res struct {
		TransNo string `json:"trans_no"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	_ = json.Unmarshal(raw, &res)
	switch {
	case resp.StatusCode == http.StatusConflict:
		return "", errors.New("modul akuntansi tidak aktif untuk sekolah ini")
	case resp.StatusCode == http.StatusUnprocessableEntity && res.Code == "period_closed":
		return "", errors.New("periode akuntansi sudah ditutup")
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg := res.Message
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("akuntansi menolak jurnal: %s", msg)
	case res.TransNo == "":
		return "", errors.New("akuntansi tidak mengembalikan nomor transaksi")
	}
	return res.TransNo, nil
}

// AccountMapping is payroll_accounts plus the per-component accounts.
type AccountMapping struct {
	SalaryExpense, SalaryPayable, PPh21Payable, BPJSPayable, BPJSEmployerExpense string
	Component                                                                    map[string]compAccount
}

type compAccount struct{ acc, liability string }

func loadAccountMapping(ctx context.Context, q querier, schoolID string) (AccountMapping, error) {
	m := AccountMapping{
		SalaryExpense: "5101", SalaryPayable: "2101", PPh21Payable: "2102", BPJSPayable: "2103", BPJSEmployerExpense: "5102",
		Component: map[string]compAccount{},
	}
	err := q.QueryRowContext(ctx, `
		SELECT salary_expense_acc, salary_payable_acc, pph21_payable_acc, bpjs_payable_acc, bpjs_employer_expense_acc
		FROM payroll_accounts WHERE school_id = $1`, schoolID).
		Scan(&m.SalaryExpense, &m.SalaryPayable, &m.PPh21Payable, &m.BPJSPayable, &m.BPJSEmployerExpense)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return m, err
	}
	rows, err := q.QueryContext(ctx, `SELECT component_id, acc_code, COALESCE(liability_acc_code, '') FROM pay_component_accounts WHERE school_id = $1`, schoolID)
	if err != nil {
		return m, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var a compAccount
		if err := rows.Scan(&id, &a.acc, &a.liability); err != nil {
			return m, err
		}
		m.Component[id] = a
	}
	return m, rows.Err()
}

// BuildAccrualJournal (T3 §4): debit expense per earning (base salary to
// salary_expense) and the employer BPJS expense; credit salary payable
// (net), PPh 21 payable, BPJS payable (employee + employer) and each other
// deduction to its component liability account. Lines on the same account
// and side are merged; the result is balanced or an error.
func BuildAccrualJournal(lines []LineDetailsWithTotals, m AccountMapping) ([]JournalLine, error) {
	debit := map[string]int64{}
	credit := map[string]int64{}
	var missing []string
	for _, l := range lines {
		for _, it := range l.Details.Items {
			if it.Component == "" { // base salary
				debit[m.SalaryExpense] += it.AmountIDR
				continue
			}
			acc, ok := m.Component[it.Component]
			if it.Kind == "earning" {
				if !ok || acc.acc == "" {
					missing = append(missing, it.Code)
					continue
				}
				debit[acc.acc] += it.AmountIDR
			} else {
				target := acc.liability
				if target == "" {
					target = acc.acc
				}
				if target == "" {
					missing = append(missing, it.Code)
					continue
				}
				credit[target] += it.AmountIDR
			}
		}
		debit[m.BPJSEmployerExpense] += l.BPJSEmployerIDR
		credit[m.SalaryPayable] += l.NetIDR
		credit[m.PPh21Payable] += l.PPh21IDR
		credit[m.BPJSPayable] += l.BPJSEmployeeIDR + l.BPJSEmployerIDR
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("komponen tanpa mapping akun: %s", strings.Join(uniq(missing), ", "))
	}
	var out []JournalLine
	var td, tc int64
	for _, acc := range sortedKeys(debit) {
		if v := debit[acc]; v > 0 {
			out = append(out, JournalLine{AccCode: acc, DebitIDR: v})
			td += v
		}
	}
	for _, acc := range sortedKeys(credit) {
		if v := credit[acc]; v > 0 {
			out = append(out, JournalLine{AccCode: acc, CreditIDR: v})
			tc += v
		}
	}
	if td != tc {
		return nil, fmt.Errorf("jurnal payroll tidak seimbang: debit %d kredit %d", td, tc)
	}
	return out, nil
}

// LineDetailsWithTotals is one payroll line as the journal needs it.
type LineDetailsWithTotals struct {
	NetIDR, PPh21IDR, BPJSEmployeeIDR, BPJSEmployerIDR int64
	Details                                            LineDetails
}

func sortedKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// PostJournal sends one payroll_journals row to accounting and records the
// outcome. Not configured → stays pending with that reason; failure →
// failed with the error; success → posted with trans_no. Accounting is
// idempotent on (source, reference), so a retry never double-books.
func (s *Store) PostJournal(ctx context.Context, schoolID, journalID string) (*PayrollJournal, error) {
	var runID, kind, reference, status string
	var linesRaw []byte
	var period string
	err := s.DB.QueryRowContext(ctx, `
		SELECT j.run_id, j.kind, j.reference, j.status, j.lines, r.period
		FROM payroll_journals j JOIN payroll_runs r ON r.id = j.run_id
		WHERE j.id = $1 AND j.school_id = $2`, journalID, schoolID).
		Scan(&runID, &kind, &reference, &status, &linesRaw, &period)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status == "posted" {
		return s.getJournal(ctx, schoolID, journalID)
	}
	var lines []JournalLine
	if err := json.Unmarshal(linesRaw, &lines); err != nil || len(lines) < 2 {
		_, _ = s.DB.ExecContext(ctx, `UPDATE payroll_journals SET status = 'failed', error = $2, attempts = attempts + 1, updated_at = now() WHERE id = $1`,
			journalID, "baris jurnal kosong")
		return s.getJournal(ctx, schoolID, journalID)
	}
	_, end, _ := periodBounds(period)
	payload := map[string]any{
		"school_id":   schoolID,
		"source":      "payroll",
		"reference":   reference,
		"trans_date":  end.Format("2006-01-02"),
		"description": fmt.Sprintf("Payroll %s periode %s", kind, period),
		"lines":       lines,
	}
	transNo, perr := s.Accounting.Post(ctx, payload)
	switch {
	case errors.Is(perr, ErrAccountingNotConfigured):
		_, err = s.DB.ExecContext(ctx, `UPDATE payroll_journals SET status = 'pending', error = $2, updated_at = now() WHERE id = $1`,
			journalID, perr.Error())
	case perr != nil:
		_, err = s.DB.ExecContext(ctx, `UPDATE payroll_journals SET status = 'failed', error = $2, attempts = attempts + 1, updated_at = now() WHERE id = $1`,
			journalID, perr.Error())
	default:
		_, err = s.DB.ExecContext(ctx, `UPDATE payroll_journals SET status = 'posted', trans_no = $2, error = NULL, attempts = attempts + 1, updated_at = now() WHERE id = $1`,
			journalID, transNo)
	}
	if err != nil {
		return nil, err
	}
	return s.getJournal(ctx, schoolID, journalID)
}

func (s *Store) getJournal(ctx context.Context, schoolID, id string) (*PayrollJournal, error) {
	list, err := s.listJournals(ctx, schoolID, id, "")
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNotFound
	}
	return &list[0], nil
}
