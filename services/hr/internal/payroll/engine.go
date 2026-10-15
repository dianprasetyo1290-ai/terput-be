package payroll

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"skci/hr/internal/payroll/calc"
)

// Payroll engine (T3 §3–4). Replaces the previous CalculateRun, which wrote
// a hardcoded Rp5.000.000 base salary and a flat 1% PPh 21 for every
// employee. Everything below comes from the employee's assignment, the
// school's pay components and inputs, and the statutory_* tables.

var periodRe = regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`)

// ValidPeriod reports whether p is a YYYY-MM payroll period.
func ValidPeriod(p string) bool { return periodRe.MatchString(p) }

func periodBounds(period string) (start, end time.Time, err error) {
	start, err = time.Parse("2006-01", period)
	if err != nil {
		return
	}
	end = start.AddDate(0, 1, -1)
	return
}

var (
	// ErrRunNotEditable: only draft/calculated runs can be (re)calculated.
	ErrRunNotEditable = errors.New("payroll run sudah diajukan atau final, tidak dapat dihitung ulang")
)

// LineItem is one row of the payslip breakdown stored in payroll_lines.details.
type LineItem struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // earning | deduction
	AmountIDR int64  `json:"amount_idr"`
	Taxable   bool   `json:"taxable,omitempty"`
	BPJSBase  bool   `json:"bpjs_base,omitempty"`
	Component string `json:"component_id,omitempty"`
}

// LineDetails is payroll_lines.details: everything the payslip, the tax
// reports and the journal need, so none of them recompute.
type LineDetails struct {
	Items         []LineItem         `json:"items"`
	BaseSalaryIDR int64              `json:"base_salary_idr"`
	BPJS          calc.BPJSResult    `json:"bpjs"`
	PTKP          string             `json:"ptkp"`
	TERCategory   string             `json:"ter_category,omitempty"`
	TERRateBP     int                `json:"ter_rate_bp,omitempty"`
	PensionIDR    int64              `json:"pension_idr"`
	Annual        *calc.AnnualResult `json:"annual,omitempty"`
	Supplementary *struct {
		RegularTaxableIDR int64 `json:"regular_taxable_idr"`
		RegularPPh21IDR   int64 `json:"regular_pph21_idr"`
	} `json:"supplementary,omitempty"`
}

// SkippedEmployee is an active employee the run could not pay, with why.
type SkippedEmployee struct {
	EmployeeID string `json:"employee_id"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
}

// RunTotals is payroll_runs.totals.
type RunTotals struct {
	Lines              int               `json:"lines"`
	GrossIDR           int64             `json:"gross_idr"`
	NetIDR             int64             `json:"net_idr"`
	PPh21IDR           int64             `json:"pph21_idr"`
	BPJSEmployeeIDR    int64             `json:"bpjs_employee_idr"`
	BPJSEmployerIDR    int64             `json:"bpjs_employer_idr"`
	OtherDeductionsIDR int64             `json:"other_deductions_idr"`
	Flagged            int               `json:"flagged"`
	Skipped            []SkippedEmployee `json:"skipped"`
}

type engineEmployee struct {
	id, name, ptkp string
	hire           time.Time
	exit           *time.Time
}

type engineComponent struct {
	id, code, name, kind, calc string
	taxable, bpjsBase          bool
	defaultAmt, rate           *int64
}

// CalculateRun computes every employee line of a draft/calculated run in
// one transaction and stores the parameter snapshot used (repeatable while
// the run is not submitted).
func (s *Store) CalculateRun(ctx context.Context, schoolID, runID string) (*RunTotals, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var period, kind, status string
	if err := tx.QueryRowContext(ctx, `SELECT period, kind, status FROM payroll_runs WHERE id = $1 AND school_id = $2 FOR UPDATE`,
		runID, schoolID).Scan(&period, &kind, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if status != "draft" && status != "calculated" {
		return nil, ErrRunNotEditable
	}
	start, end, err := periodBounds(period)
	if err != nil {
		return nil, fmt.Errorf("periode run tidak valid: %w", err)
	}
	statutory, params, err := LoadStatutory(ctx, tx, schoolID, end)
	if err != nil {
		return nil, err
	}

	emps, err := s.engineEmployees(ctx, tx, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	comps, err := s.engineComponents(ctx, tx, schoolID)
	if err != nil {
		return nil, err
	}
	inputs, err := s.engineInputs(ctx, tx, schoolID, period)
	if err != nil {
		return nil, err
	}
	fixed, err := s.engineEmployeeComponents(ctx, tx, schoolID, end)
	if err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM payroll_lines WHERE run_id = $1 AND school_id = $2`, runID, schoolID); err != nil {
		return nil, err
	}

	totals := RunTotals{Skipped: []SkippedEmployee{}}
	for _, emp := range emps {
		d := LineDetails{Items: []LineItem{}, PTKP: emp.ptkp}
		var gross, taxableEarn, bpjsBase, deductions int64

		if kind == "regular" {
			salary, ok, err := s.baseSalary(ctx, tx, schoolID, emp.id, start, end)
			if err != nil {
				return nil, err
			}
			if !ok {
				totals.Skipped = append(totals.Skipped, SkippedEmployee{emp.id, emp.name, "gaji pokok belum diisi pada penugasan aktif"})
				continue
			}
			d.BaseSalaryIDR = salary
			d.Items = append(d.Items, LineItem{Code: "GAPOK", Name: "Gaji Pokok", Kind: "earning", AmountIDR: salary, Taxable: true, BPJSBase: true})
			gross, taxableEarn, bpjsBase = salary, salary, salary
		}

		for _, c := range comps {
			amt, ok := componentAmount(c, inputs[emp.id+"|"+c.id], fixed[emp.id+"|"+c.id], kind)
			if !ok || amt == 0 {
				continue
			}
			d.Items = append(d.Items, LineItem{Code: c.code, Name: c.name, Kind: c.kind, AmountIDR: amt, Taxable: c.taxable, BPJSBase: c.bpjsBase, Component: c.id})
			if c.kind == "earning" {
				gross += amt
				if c.taxable {
					taxableEarn += amt
				}
				if c.bpjsBase {
					bpjsBase += amt
				}
			} else {
				deductions += amt
			}
		}
		if kind == "supplementary" && gross == 0 && deductions == 0 {
			continue // nothing to pay this employee in a susulan run
		}

		// BPJS only on the regular run; a supplementary run is extra income.
		if kind == "regular" {
			b, err := calc.CalcAllBPJS(bpjsBase, params)
			if err != nil {
				return nil, err
			}
			d.BPJS = b
			d.PensionIDR = b.Pension
		} else {
			d.BPJS = calc.BPJSResult{Employee: map[string]int64{}, Employer: map[string]int64{}}
		}

		// PPh 21 gross = taxable earnings + employer kesehatan/JKK/JKM.
		taxable := taxableEarn + d.BPJS.EmployerTaxed
		var pph21 int64
		lastMonth := start.Month() == time.December || (emp.exit != nil && !emp.exit.After(end) && !emp.exit.Before(start))
		switch {
		case kind == "supplementary":
			regTaxable, regTax, err := regularLineTax(ctx, tx, schoolID, emp.id, period, runID)
			if err != nil {
				return nil, err
			}
			tax, cat, rate, err := calc.CalcPPh21TER(regTaxable+taxable, emp.ptkp, params)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", emp.name, err)
			}
			d.TERCategory, d.TERRateBP = cat, rate
			d.Supplementary = &struct {
				RegularTaxableIDR int64 `json:"regular_taxable_idr"`
				RegularPPh21IDR   int64 `json:"regular_pph21_idr"`
			}{regTaxable, regTax}
			pph21 = max(tax-regTax, 0)
		case lastMonth:
			prev, err := yearToDate(ctx, tx, schoolID, emp.id, period, runID)
			if err != nil {
				return nil, err
			}
			months := prev.months + 1
			ar, err := calc.CalcPPh21December(calc.AnnualInput{
				PTKP: emp.ptkp, GrossYearIDR: prev.taxable + taxable, PensionYearIDR: prev.pension + d.PensionIDR,
				WithheldPrevIDR: prev.pph21, Months: months,
			}, params)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", emp.name, err)
			}
			d.Annual = &ar
			pph21 = ar.ThisMonthIDR
		default:
			tax, cat, rate, err := calc.CalcPPh21TER(taxable, emp.ptkp, params)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", emp.name, err)
			}
			d.TERCategory, d.TERRateBP = cat, rate
			pph21 = tax
		}

		net := gross - d.BPJS.EmployeeTotal - pph21 - deductions
		var prevNet *int64
		var variance *int
		flagged := false
		if kind == "regular" {
			var pn int64
			err := tx.QueryRowContext(ctx, `
				SELECT l.net_idr FROM payroll_lines l JOIN payroll_runs r ON r.id = l.run_id
				WHERE l.school_id = $1 AND l.employee_id = $2 AND r.kind = 'regular' AND r.status = 'final' AND r.period < $3
				ORDER BY r.period DESC LIMIT 1`, schoolID, emp.id, period).Scan(&pn)
			if err == nil {
				prevNet = &pn
				bp, fl := calc.Variance(pn, net, 1000)
				variance, flagged = &bp, fl
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		detailsJSON, _ := json.Marshal(d)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO payroll_lines (school_id, run_id, employee_id, gross_idr, taxable_idr, bpjs_base_idr,
				bpjs_employee_idr, bpjs_employer_idr, pph21_idr, other_deductions_idr, net_idr,
				prev_net_idr, variance_bp, flagged, details)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			schoolID, runID, emp.id, gross, taxable, bpjsBase, d.BPJS.EmployeeTotal, d.BPJS.EmployerTotal,
			pph21, deductions, net, prevNet, variance, flagged, detailsJSON); err != nil {
			return nil, err
		}
		totals.Lines++
		totals.GrossIDR += gross
		totals.NetIDR += net
		totals.PPh21IDR += pph21
		totals.BPJSEmployeeIDR += d.BPJS.EmployeeTotal
		totals.BPJSEmployerIDR += d.BPJS.EmployerTotal
		totals.OtherDeductionsIDR += deductions
		if flagged {
			totals.Flagged++
		}
	}

	totalsJSON, _ := json.Marshal(totals)
	snapshot, _ := json.Marshal(map[string]any{"statutory": statutory, "params": params})
	if _, err := tx.ExecContext(ctx, `
		UPDATE payroll_runs SET status = 'calculated', calculated_at = now(), updated_at = now(),
		       totals = $3, params_snapshot = $4
		WHERE id = $1 AND school_id = $2`, runID, schoolID, totalsJSON, snapshot); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &totals, nil
}

// componentAmount: a period input wins, then the employee's fixed amount,
// then the component default (fixed_amount only). Supplementary runs only
// pay period inputs.
func componentAmount(c engineComponent, in *periodInput, fixedAmt *int64, kind string) (int64, bool) {
	if in != nil {
		if in.amount == 0 && in.quantity > 0 && c.rate != nil {
			return int64(in.quantity * float64(*c.rate)), true
		}
		return in.amount, true
	}
	if kind == "supplementary" {
		return 0, false
	}
	if fixedAmt != nil {
		return *fixedAmt, true
	}
	if c.calc == "fixed_amount" && c.defaultAmt != nil {
		return *c.defaultAmt, true
	}
	return 0, false
}

func (s *Store) engineEmployees(ctx context.Context, q querier, schoolID string, start, end time.Time) ([]engineEmployee, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, full_name, ptkp_status, hire_date, exit_date
		FROM employees
		WHERE school_id = $1 AND deleted_at IS NULL AND status IN ('active', 'exited')
		  AND hire_date <= $3 AND (exit_date IS NULL OR exit_date >= $2)
		ORDER BY full_name`, schoolID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engineEmployee
	for rows.Next() {
		var e engineEmployee
		var exit sql.NullTime
		if err := rows.Scan(&e.id, &e.name, &e.ptkp, &e.hire, &exit); err != nil {
			return nil, err
		}
		if exit.Valid {
			e.exit = &exit.Time
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) engineComponents(ctx context.Context, q querier, schoolID string) ([]engineComponent, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, code, name, kind, calc, taxable, bpjs_base, default_amount_idr, rate_per_unit_idr
		FROM pay_components
		WHERE school_id = $1 AND status = 'active' AND deleted_at IS NULL
		ORDER BY kind DESC, sort, code`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engineComponent
	for rows.Next() {
		var c engineComponent
		if err := rows.Scan(&c.id, &c.code, &c.name, &c.kind, &c.calc, &c.taxable, &c.bpjsBase, &c.defaultAmt, &c.rate); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type periodInput struct {
	amount   int64
	quantity float64
}

func (s *Store) engineInputs(ctx context.Context, q querier, schoolID, period string) (map[string]*periodInput, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT employee_id, component_id, amount_idr, COALESCE(quantity, 0)
		FROM payroll_inputs WHERE school_id = $1 AND period = $2`, schoolID, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*periodInput{}
	for rows.Next() {
		var emp, comp string
		in := &periodInput{}
		if err := rows.Scan(&emp, &comp, &in.amount, &in.quantity); err != nil {
			return nil, err
		}
		out[emp+"|"+comp] = in
	}
	return out, rows.Err()
}

func (s *Store) engineEmployeeComponents(ctx context.Context, q querier, schoolID string, at time.Time) (map[string]*int64, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT DISTINCT ON (employee_id, component_id) employee_id, component_id, amount_idr
		FROM employee_components
		WHERE school_id = $1 AND effective_from <= $2 AND (effective_to IS NULL OR effective_to >= $2)
		ORDER BY employee_id, component_id, effective_from DESC`, schoolID, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*int64{}
	for rows.Next() {
		var emp, comp string
		var amt int64
		if err := rows.Scan(&emp, &comp, &amt); err != nil {
			return nil, err
		}
		out[emp+"|"+comp] = &amt
	}
	return out, rows.Err()
}

// baseSalary is the encrypted base salary of the assignment in force at the
// end of the period (or the last one that overlapped it, for leavers).
func (s *Store) baseSalary(ctx context.Context, q querier, schoolID, employeeID string, start, end time.Time) (int64, bool, error) {
	var enc []byte
	err := q.QueryRowContext(ctx, `
		SELECT base_salary_enc FROM employment_assignments
		WHERE school_id = $1 AND employee_id = $2 AND effective_from <= $4
		  AND (effective_to IS NULL OR effective_to > $3)
		ORDER BY effective_from DESC LIMIT 1`, schoolID, employeeID, start, end).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(enc) == 0) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if s.DecryptAmount == nil {
		return 0, false, errors.New("payroll store has no salary decryptor")
	}
	amt, err := s.DecryptAmount(enc, schoolID)
	if err != nil {
		return 0, false, fmt.Errorf("dekripsi gaji pokok: %w", err)
	}
	return amt, amt > 0, nil
}

type ytd struct {
	taxable, pension, pph21 int64
	months                  int
}

// yearToDate sums the employee's final lines earlier in the same tax year.
func yearToDate(ctx context.Context, q querier, schoolID, employeeID, period, runID string) (ytd, error) {
	var y ytd
	err := q.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.taxable_idr), 0), COALESCE(SUM((l.details->>'pension_idr')::bigint), 0),
		       COALESCE(SUM(l.pph21_idr), 0), COUNT(DISTINCT r.period)
		FROM payroll_lines l JOIN payroll_runs r ON r.id = l.run_id
		WHERE l.school_id = $1 AND l.employee_id = $2 AND r.status = 'final' AND r.id <> $4
		  AND left(r.period, 4) = left($3, 4) AND r.period <= $3`, schoolID, employeeID, period, runID).
		Scan(&y.taxable, &y.pension, &y.pph21, &y.months)
	if err != nil {
		return y, err
	}
	// The current period counts once even when a regular run already exists.
	var samePeriod bool
	_ = q.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM payroll_lines l JOIN payroll_runs r ON r.id = l.run_id
		WHERE l.school_id = $1 AND l.employee_id = $2 AND r.status = 'final' AND r.period = $3 AND r.id <> $4)`,
		schoolID, employeeID, period, runID).Scan(&samePeriod)
	if samePeriod {
		y.months--
	}
	return y, nil
}

// regularLineTax: the employee's regular line in the same period (any
// non-cancelled run), which a supplementary run tops up.
func regularLineTax(ctx context.Context, q querier, schoolID, employeeID, period, runID string) (int64, int64, error) {
	var taxable, pph int64
	err := q.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.taxable_idr), 0), COALESCE(SUM(l.pph21_idr), 0)
		FROM payroll_lines l JOIN payroll_runs r ON r.id = l.run_id
		WHERE l.school_id = $1 AND l.employee_id = $2 AND r.period = $3 AND r.id <> $4
		  AND r.status NOT IN ('rejected', 'cancelled')`, schoolID, employeeID, period, runID).Scan(&taxable, &pph)
	return taxable, pph, err
}

// sortedItems keeps earnings before deductions for the payslip.
func sortedItems(items []LineItem) []LineItem {
	out := append([]LineItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Kind == "earning" && out[j].Kind != "earning" })
	return out
}
