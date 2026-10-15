package payroll

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// payslipDocs loads lines (with run and employee) matching where, which
// may use $1 = school id and $2 = arg.
func (s *Store) payslipDocs(ctx context.Context, q querier, schoolID, where string, arg string) ([]PayslipDoc, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT l.id, l.run_id, l.employee_id, e.full_name, e.employee_no, r.period, r.kind,
		       l.gross_idr, l.taxable_idr, l.bpjs_base_idr, l.bpjs_employee_idr, l.bpjs_employer_idr,
		       l.pph21_idr, l.other_deductions_idr, l.net_idr, l.details
		FROM payroll_lines l
		JOIN payroll_runs r ON r.id = l.run_id
		JOIN employees e ON e.id = l.employee_id
		WHERE l.school_id = $1 AND `+where+`
		ORDER BY e.full_name`, schoolID, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PayslipDoc
	for rows.Next() {
		var d PayslipDoc
		var raw []byte
		l := &d.Line
		if err := rows.Scan(&l.ID, &l.RunID, &l.EmployeeID, &d.EmployeeName, &d.EmployeeNo, &d.Period, &d.RunKind,
			&l.GrossIDR, &l.TaxableIDR, &l.BPJSBaseIDR, &l.BPJSEmployeeIDR, &l.BPJSEmployerIDR,
			&l.PPh21IDR, &l.OtherDeductionsIDR, &l.NetIDR, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &d.Details)
		l.EmployeeName = d.EmployeeName
		out = append(out, d)
	}
	return out, rows.Err()
}

// PayslipPDF renders a published payslip. employeeID, when set, restricts
// it to that employee's own slip (ESS). The re-rendered PDF must hash to
// the sha256 stored at finalisation (FR-96-004).
func (s *Store) PayslipPDF(ctx context.Context, schoolID, payslipID, employeeID string) ([]byte, string, error) {
	var lineID, lineEmployee, period, stored string
	err := s.DB.QueryRowContext(ctx, `
		SELECT ps.line_id, l.employee_id, r.period, ps.sha256
		FROM payslips ps JOIN payroll_lines l ON l.id = ps.line_id JOIN payroll_runs r ON r.id = l.run_id
		WHERE ps.id = $1 AND ps.school_id = $2 AND ps.published_at IS NOT NULL`, payslipID, schoolID).
		Scan(&lineID, &lineEmployee, &period, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	if employeeID != "" && employeeID != lineEmployee {
		return nil, "", ErrForbidden
	}
	docs, err := s.payslipDocs(ctx, s.DB, schoolID, `l.id = $2`, lineID)
	if err != nil {
		return nil, "", err
	}
	if len(docs) == 0 {
		return nil, "", ErrNotFound
	}
	pdf, sum := RenderPayslipPDF(docs[0])
	if stored != sum {
		return nil, "", errors.New("slip gaji tidak cocok dengan hash saat finalisasi")
	}
	return pdf, "slip-gaji-" + period + ".pdf", nil
}

// ErrForbidden: the caller may not see this record.
var ErrForbidden = errors.New("forbidden")

// EmployeeIDByUser maps a JWT user to the employee of this school.
func (s *Store) EmployeeIDByUser(ctx context.Context, schoolID, userID string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id FROM employees WHERE school_id = $1 AND user_id::text = $2 AND deleted_at IS NULL
		ORDER BY (status = 'active') DESC, hire_date DESC LIMIT 1`, schoolID, userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}
