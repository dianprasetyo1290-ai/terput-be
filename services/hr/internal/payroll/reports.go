package payroll

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Tax & BPJS reports (T3 §4, menu 97). Every figure comes from final runs'
// payroll_lines, never recomputed, so reports match what was withheld.

// PPh21Row is one employee in the monthly PPh 21 report.
type PPh21Row struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	EmployeeNo   string `json:"employee_no"`
	PTKP         string `json:"ptkp"`
	TERCategory  string `json:"ter_category,omitempty"`
	TERRateBP    int    `json:"ter_rate_bp,omitempty"`
	Method       string `json:"method"` // ter | annual
	TaxableIDR   int64  `json:"taxable_idr"`
	PPh21IDR     int64  `json:"pph21_idr"`
}

// PPh21Report is GET /payroll/reports/pph21?period=.
type PPh21Report struct {
	Period     string     `json:"period"`
	Rows       []PPh21Row `json:"rows"`
	TaxableIDR int64      `json:"taxable_idr"`
	PPh21IDR   int64      `json:"pph21_idr"`
}

func (s *Store) PPh21Monthly(ctx context.Context, schoolID, period string) (*PPh21Report, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT l.employee_id, e.full_name, e.employee_no, SUM(l.taxable_idr), SUM(l.pph21_idr),
		       (array_agg(l.details ORDER BY (r.kind = 'regular') DESC))[1]
		FROM payroll_lines l
		JOIN payroll_runs r ON r.id = l.run_id
		JOIN employees e ON e.id = l.employee_id
		WHERE l.school_id = $1 AND r.period = $2 AND r.status = 'final'
		GROUP BY l.employee_id, e.full_name, e.employee_no
		ORDER BY e.full_name`, schoolID, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rep := &PPh21Report{Period: period, Rows: []PPh21Row{}}
	for rows.Next() {
		var r PPh21Row
		var raw []byte
		if err := rows.Scan(&r.EmployeeID, &r.EmployeeName, &r.EmployeeNo, &r.TaxableIDR, &r.PPh21IDR, &raw); err != nil {
			return nil, err
		}
		var d LineDetails
		_ = json.Unmarshal(raw, &d)
		r.PTKP, r.TERCategory, r.TERRateBP, r.Method = d.PTKP, d.TERCategory, d.TERRateBP, "ter"
		if d.Annual != nil {
			r.Method = "annual"
		}
		rep.TaxableIDR += r.TaxableIDR
		rep.PPh21IDR += r.PPh21IDR
		rep.Rows = append(rep.Rows, r)
	}
	return rep, rows.Err()
}

// AnnualRow is one employee's year (basis for bukti potong 1721-A1).
type AnnualRow struct {
	EmployeeID     string             `json:"employee_id"`
	EmployeeName   string             `json:"employee_name"`
	EmployeeNo     string             `json:"employee_no"`
	PTKP           string             `json:"ptkp"`
	Months         int                `json:"months"`
	FirstPeriod    string             `json:"first_period"`
	LastPeriod     string             `json:"last_period"`
	GrossIDR       int64              `json:"gross_idr"`
	PensionIDR     int64              `json:"pension_idr"`
	PPh21IDR       int64              `json:"pph21_idr"`
	Reconciliation *AnnualResultBrief `json:"reconciliation,omitempty"`
}

// AnnualResultBrief is the December/exit-month reconciliation, if done.
type AnnualResultBrief struct {
	Period       string `json:"period"`
	NettoIDR     int64  `json:"netto_idr"`
	PKPIDR       int64  `json:"pkp_idr"`
	AnnualTaxIDR int64  `json:"annual_tax_idr"`
	OverpaidIDR  int64  `json:"overpaid_idr"`
}

type AnnualReport struct {
	Year     int         `json:"year"`
	Rows     []AnnualRow `json:"rows"`
	GrossIDR int64       `json:"gross_idr"`
	PPh21IDR int64       `json:"pph21_idr"`
}

func (s *Store) PPh21Annual(ctx context.Context, schoolID string, year int) (*AnnualReport, error) {
	y := fmt.Sprintf("%04d", year)
	rows, err := s.DB.QueryContext(ctx, `
		SELECT l.employee_id, e.full_name, e.employee_no, e.ptkp_status,
		       COUNT(DISTINCT r.period), MIN(r.period), MAX(r.period),
		       SUM(l.taxable_idr), SUM(COALESCE((l.details->>'pension_idr')::bigint, 0)), SUM(l.pph21_idr),
		       (array_agg(l.details->'annual' ORDER BY r.period DESC) FILTER (WHERE l.details ? 'annual'))[1],
		       (array_agg(r.period ORDER BY r.period DESC) FILTER (WHERE l.details ? 'annual'))[1]
		FROM payroll_lines l
		JOIN payroll_runs r ON r.id = l.run_id
		JOIN employees e ON e.id = l.employee_id
		WHERE l.school_id = $1 AND left(r.period, 4) = $2 AND r.status = 'final'
		GROUP BY l.employee_id, e.full_name, e.employee_no, e.ptkp_status
		ORDER BY e.full_name`, schoolID, y)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rep := &AnnualReport{Year: year, Rows: []AnnualRow{}}
	for rows.Next() {
		var r AnnualRow
		var annual []byte
		var annualPeriod *string
		if err := rows.Scan(&r.EmployeeID, &r.EmployeeName, &r.EmployeeNo, &r.PTKP, &r.Months, &r.FirstPeriod, &r.LastPeriod,
			&r.GrossIDR, &r.PensionIDR, &r.PPh21IDR, &annual, &annualPeriod); err != nil {
			return nil, err
		}
		if len(annual) > 0 && annualPeriod != nil {
			var a struct {
				NettoIDR     int64 `json:"netto_idr"`
				PKPIDR       int64 `json:"pkp_idr"`
				AnnualTaxIDR int64 `json:"annual_tax_idr"`
				OverpaidIDR  int64 `json:"overpaid_idr"`
			}
			if json.Unmarshal(annual, &a) == nil {
				r.Reconciliation = &AnnualResultBrief{Period: *annualPeriod, NettoIDR: a.NettoIDR, PKPIDR: a.PKPIDR, AnnualTaxIDR: a.AnnualTaxIDR, OverpaidIDR: a.OverpaidIDR}
			}
		}
		rep.GrossIDR += r.GrossIDR
		rep.PPh21IDR += r.PPh21IDR
		rep.Rows = append(rep.Rows, r)
	}
	return rep, rows.Err()
}

// BPJSRow is one employee's contributions per program in a period.
type BPJSRow struct {
	EmployeeID   string           `json:"employee_id"`
	EmployeeName string           `json:"employee_name"`
	EmployeeNo   string           `json:"employee_no"`
	BaseIDR      int64            `json:"base_idr"`
	Employee     map[string]int64 `json:"employee"`
	Employer     map[string]int64 `json:"employer"`
}

type BPJSReport struct {
	Period        string           `json:"period"`
	Rows          []BPJSRow        `json:"rows"`
	EmployeeTotal map[string]int64 `json:"employee_total"`
	EmployerTotal map[string]int64 `json:"employer_total"`
}

func (s *Store) BPJSMonthly(ctx context.Context, schoolID, period string) (*BPJSReport, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT l.employee_id, e.full_name, e.employee_no, l.bpjs_base_idr, l.details
		FROM payroll_lines l
		JOIN payroll_runs r ON r.id = l.run_id
		JOIN employees e ON e.id = l.employee_id
		WHERE l.school_id = $1 AND r.period = $2 AND r.status = 'final' AND r.kind = 'regular'
		ORDER BY e.full_name`, schoolID, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rep := &BPJSReport{Period: period, Rows: []BPJSRow{}, EmployeeTotal: map[string]int64{}, EmployerTotal: map[string]int64{}}
	for rows.Next() {
		var r BPJSRow
		var raw []byte
		if err := rows.Scan(&r.EmployeeID, &r.EmployeeName, &r.EmployeeNo, &r.BaseIDR, &raw); err != nil {
			return nil, err
		}
		var d LineDetails
		_ = json.Unmarshal(raw, &d)
		r.Employee, r.Employer = d.BPJS.Employee, d.BPJS.Employer
		if r.Employee == nil {
			r.Employee = map[string]int64{}
		}
		if r.Employer == nil {
			r.Employer = map[string]int64{}
		}
		for k, v := range r.Employee {
			rep.EmployeeTotal[k] += v
		}
		for k, v := range r.Employer {
			rep.EmployerTotal[k] += v
		}
		rep.Rows = append(rep.Rows, r)
	}
	return rep, rows.Err()
}

// Remittance is one tax_remittances row (setoran PPh 21 / BPJS).
type Remittance struct {
	ID        string    `json:"id"`
	Period    string    `json:"period"`
	Kind      string    `json:"kind"`
	AmountIDR int64     `json:"amount_idr"`
	PaidOn    string    `json:"paid_on"`
	Ref       string    `json:"ref"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListRemittances(ctx context.Context, schoolID, period string) ([]Remittance, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, period, kind, amount_idr, paid_on::text, ref, created_at
		FROM tax_remittances WHERE school_id = $1 AND ($2 = '' OR period = $2 OR left(period, 4) = $2)
		ORDER BY period DESC, kind, paid_on`, schoolID, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Remittance{}
	for rows.Next() {
		var r Remittance
		if err := rows.Scan(&r.ID, &r.Period, &r.Kind, &r.AmountIDR, &r.PaidOn, &r.Ref, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) CreateRemittance(ctx context.Context, schoolID string, r Remittance) (*Remittance, error) {
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO tax_remittances (school_id, period, kind, amount_idr, paid_on, ref)
		VALUES ($1, $2, $3, $4, $5::date, $6) RETURNING id, created_at`,
		schoolID, r.Period, r.Kind, r.AmountIDR, r.PaidOn, r.Ref).Scan(&r.ID, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ReconRow compares what payroll withheld (plus the employer share for
// BPJS) with what was remitted (AC-97-02).
type ReconRow struct {
	Kind          string `json:"kind"`
	WithheldIDR   int64  `json:"withheld_idr"`
	RemittedIDR   int64  `json:"remitted_idr"`
	DifferenceIDR int64  `json:"difference_idr"`
	Status        string `json:"status"` // match | kurang_setor | lebih_setor
}

type ReconReport struct {
	Period string     `json:"period"`
	Rows   []ReconRow `json:"rows"`
}

func (s *Store) Reconciliation(ctx context.Context, schoolID, period string) (*ReconReport, error) {
	due := map[string]int64{}
	bpjs, err := s.BPJSMonthly(ctx, schoolID, period)
	if err != nil {
		return nil, err
	}
	pph, err := s.PPh21Monthly(ctx, schoolID, period)
	if err != nil {
		return nil, err
	}
	due["pph21"] = pph.PPh21IDR
	for prog, v := range bpjs.EmployeeTotal {
		if prog == "kesehatan" {
			due["bpjs_kesehatan"] += v
		} else {
			due["bpjs_tk"] += v
		}
	}
	for prog, v := range bpjs.EmployerTotal {
		if prog == "kesehatan" {
			due["bpjs_kesehatan"] += v
		} else {
			due["bpjs_tk"] += v
		}
	}
	paid := map[string]int64{}
	rows, err := s.DB.QueryContext(ctx, `SELECT kind, SUM(amount_idr) FROM tax_remittances WHERE school_id = $1 AND period = $2 GROUP BY kind`, schoolID, period)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var v int64
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return nil, err
		}
		paid[k] = v
	}
	rows.Close()
	rep := &ReconReport{Period: period, Rows: []ReconRow{}}
	for _, k := range []string{"pph21", "bpjs_kesehatan", "bpjs_tk"} {
		r := ReconRow{Kind: k, WithheldIDR: due[k], RemittedIDR: paid[k], DifferenceIDR: due[k] - paid[k], Status: "match"}
		if r.DifferenceIDR > 0 {
			r.Status = "kurang_setor"
		} else if r.DifferenceIDR < 0 {
			r.Status = "lebih_setor"
		}
		rep.Rows = append(rep.Rows, r)
	}
	return rep, nil
}
