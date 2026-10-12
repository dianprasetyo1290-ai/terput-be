package payroll

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"skci/hr/internal/payroll/calc"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Store struct {
	DB *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db}
}

// -----------------------------------------------------------------------------
// Pay Components
// -----------------------------------------------------------------------------

func (s *Store) ListComponents(ctx context.Context, schoolID string) ([]PayComponent, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, school_id, code, name, kind, nature, taxable, bpjs_base, calc,
		       default_amount_idr, rate_per_unit_idr, max_input_idr, sort, status,
		       approval_id, created_at, updated_at
		FROM pay_components
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY sort ASC, code ASC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PayComponent
	for rows.Next() {
		var c PayComponent
		if err := rows.Scan(
			&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.Kind, &c.Nature, &c.Taxable,
			&c.BPJSBase, &c.Calc, &c.DefaultAmountIDR, &c.RatePerUnitIDR, &c.MaxInputIDR,
			&c.Sort, &c.Status, &c.ApprovalID, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

func (s *Store) CreateComponent(ctx context.Context, c PayComponent) (*PayComponent, error) {
	c.Status = "active"
	var id string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO pay_components (
			school_id, code, name, kind, nature, taxable, bpjs_base, calc,
			default_amount_idr, rate_per_unit_idr, max_input_idr, sort, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at
	`, c.SchoolID, c.Code, c.Name, c.Kind, c.Nature, c.Taxable, c.BPJSBase, c.Calc,
		c.DefaultAmountIDR, c.RatePerUnitIDR, c.MaxInputIDR, c.Sort, c.Status).Scan(&id, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.ID = id
	return &c, nil
}

// -----------------------------------------------------------------------------
// Payroll Runs
// -----------------------------------------------------------------------------

func (s *Store) ListRuns(ctx context.Context, schoolID string) ([]PayrollRun, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.id, r.school_id, r.period, r.kind, r.status, r.approval_id,
		       r.calculated_at, r.finalized_at, r.finalized_by, r.totals, r.idempotency_key,
		       COALESCE(SUM(l.gross_idr), 0) as total_gross,
		       COALESCE(SUM(l.net_idr), 0) as total_net,
		       COUNT(l.id) as total_lines,
		       r.created_at, r.updated_at
		FROM payroll_runs r
		LEFT JOIN payroll_lines l ON r.id = l.run_id
		WHERE r.school_id = $1
		GROUP BY r.id
		ORDER BY r.period DESC, r.created_at DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PayrollRun
	for rows.Next() {
		var r PayrollRun
		if err := rows.Scan(
			&r.ID, &r.SchoolID, &r.Period, &r.Kind, &r.Status, &r.ApprovalID,
			&r.CalculatedAt, &r.FinalizedAt, &r.FinalizedBy, &r.Totals, &r.IdempotencyKey,
			&r.TotalGrossIDR, &r.TotalNetIDR, &r.TotalLines,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) GetRun(ctx context.Context, schoolID, id string) (*PayrollRun, error) {
	var r PayrollRun
	err := s.DB.QueryRowContext(ctx, `
		SELECT r.id, r.school_id, r.period, r.kind, r.status, r.approval_id,
		       r.calculated_at, r.finalized_at, r.finalized_by, r.totals, r.idempotency_key,
		       COALESCE(SUM(l.gross_idr), 0) as total_gross,
		       COALESCE(SUM(l.net_idr), 0) as total_net,
		       COUNT(l.id) as total_lines,
		       r.created_at, r.updated_at
		FROM payroll_runs r
		LEFT JOIN payroll_lines l ON r.id = l.run_id
		WHERE r.id = $1 AND r.school_id = $2
		GROUP BY r.id`, id, schoolID).Scan(
		&r.ID, &r.SchoolID, &r.Period, &r.Kind, &r.Status, &r.ApprovalID,
		&r.CalculatedAt, &r.FinalizedAt, &r.FinalizedBy, &r.Totals, &r.IdempotencyKey,
		&r.TotalGrossIDR, &r.TotalNetIDR, &r.TotalLines,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) CreateRun(ctx context.Context, r PayrollRun) (*PayrollRun, error) {
	if r.IdempotencyKey == "" {
		r.IdempotencyKey = fmt.Sprintf("run-%s-%s-%d", r.SchoolID, r.Period, time.Now().UnixNano())
	}
	r.Status = "draft"
	var id string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO payroll_runs (school_id, period, kind, status, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, r.SchoolID, r.Period, r.Kind, r.Status, r.IdempotencyKey).Scan(&id, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.ID = id
	return &r, nil
}

// CalculateRun calculates payroll lines for all active employees
func (s *Store) CalculateRun(ctx context.Context, schoolID, runID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Delete previous lines for this run
	_, err = tx.ExecContext(ctx, `DELETE FROM payroll_lines WHERE run_id = $1 AND school_id = $2`, runID, schoolID)
	if err != nil {
		return err
	}

	// 2. Fetch active employees
	rows, err := tx.QueryContext(ctx, `
		SELECT id, full_name, COALESCE(nip, '') FROM employees
		WHERE school_id = $1 AND status = 'active'`, schoolID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type empInfo struct {
		id   string
		name string
		nip  string
	}
	var emps []empInfo
	for rows.Next() {
		var e empInfo
		if err := rows.Scan(&e.id, &e.name, &e.nip); err == nil {
			emps = append(emps, e)
		}
	}
	rows.Close()

	// Default BPJS rules
	bpjsRule := calc.BPJSRule{EmployerBP: 400, EmployeeBP: 100, WageCapIDR: 12_000_000}

	for _, emp := range emps {
		// Mock base salary 5.000.000 IDR
		baseSalary := int64(5_000_000)
		empBPJS, empyrBPJS := calc.CalcBPJS(baseSalary, bpjsRule)
		pph21 := calc.CalcPPh21Monthly(baseSalary, 100) // 100 bp (1%)
		net := baseSalary - empBPJS - pph21

		_, err = tx.ExecContext(ctx, `
			INSERT INTO payroll_lines (
				school_id, run_id, employee_id, gross_idr, taxable_idr, bpjs_base_idr,
				bpjs_employee_idr, bpjs_employer_idr, pph21_idr, net_idr
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, schoolID, runID, emp.id, baseSalary, baseSalary, baseSalary, empBPJS, empyrBPJS, pph21, net)
		if err != nil {
			return err
		}
	}

	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		UPDATE payroll_runs
		SET status = 'calculated', calculated_at = $3, updated_at = $3
		WHERE id = $1 AND school_id = $2`, runID, schoolID, now)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// SubmitRun moves a calculated run to pending_approval and links its core
// approval (T3: core approval payroll_run).
func (s *Store) SubmitRun(ctx context.Context, schoolID, runID, approvalID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE payroll_runs
		SET status = 'pending_approval', approval_id = $3::uuid, updated_at = NOW()
		WHERE id = $1 AND school_id = $2 AND status IN ('calculated', 'draft')`, runID, schoolID, approvalID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ApplyRunDecision applies core's approval callback once; a repeated
// callback on a run that is no longer pending is a no-op.
func (s *Store) ApplyRunDecision(ctx context.Context, runID, decision string) error {
	return applyDecisionOnce(ctx, s.DB, "payroll_runs", runID, decision)
}

func (s *Store) FinalizeRun(ctx context.Context, schoolID, runID, userID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		UPDATE payroll_runs
		SET status = 'final', finalized_at = $3, finalized_by = $4, updated_at = $3
		WHERE id = $1 AND school_id = $2 AND status = 'approved'`, runID, schoolID, now, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConflict
	}

	// Create payslips for all lines in this run
	lines, err := s.ListLines(ctx, schoolID, runID, false)
	if err != nil {
		return err
	}

	for _, line := range lines {
		key := fmt.Sprintf("payslips/%s/%s.pdf", runID, line.ID)
		hash := fmt.Sprintf("sha256-%s", line.ID)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO payslips (school_id, line_id, object_key, sha256, published_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (line_id) DO NOTHING
		`, schoolID, line.ID, key, hash, now)
		if err != nil {
			return err
		}
	}

	// Create accrual journal entry
	ref := fmt.Sprintf("payroll:%s:accrual", runID)
	_, _ = tx.ExecContext(ctx, `
		INSERT INTO payroll_journals (school_id, run_id, kind, reference, status)
		VALUES ($1, $2, 'accrual', $3, 'posted')
		ON CONFLICT (reference) DO NOTHING
	`, schoolID, runID, ref)

	return tx.Commit()
}

// -----------------------------------------------------------------------------
// Payroll Lines & Payslips
// -----------------------------------------------------------------------------

func (s *Store) ListLines(ctx context.Context, schoolID, runID string, flaggedOnly bool) ([]PayrollLine, error) {
	q := `
		SELECT l.id, l.school_id, l.run_id, l.employee_id, e.full_name as employee_name,
		       COALESCE(e.nip, '') as employee_nip, l.gross_idr, l.taxable_idr, l.bpjs_base_idr,
		       l.bpjs_employee_idr, l.bpjs_employer_idr, l.pph21_idr, l.other_deductions_idr,
		       l.net_idr, l.prev_net_idr, l.variance_bp, l.flagged, l.payment_status, l.created_at
		FROM payroll_lines l
		JOIN employees e ON l.employee_id = e.id
		WHERE l.school_id = $1 AND l.run_id = $2`
	if flaggedOnly {
		q += " AND l.flagged = true"
	}
	q += " ORDER BY e.full_name ASC"

	rows, err := s.DB.QueryContext(ctx, q, schoolID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PayrollLine
	for rows.Next() {
		var l PayrollLine
		if err := rows.Scan(
			&l.ID, &l.SchoolID, &l.RunID, &l.EmployeeID, &l.EmployeeName, &l.EmployeeNIP,
			&l.GrossIDR, &l.TaxableIDR, &l.BPJSBaseIDR, &l.BPJSEmployeeIDR, &l.BPJSEmployerIDR,
			&l.PPh21IDR, &l.OtherDeductionsIDR, &l.NetIDR, &l.PrevNetIDR, &l.VarianceBP,
			&l.Flagged, &l.PaymentStatus, &l.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, l)
	}
	return list, nil
}

func (s *Store) ListPayslips(ctx context.Context, schoolID, employeeID string) ([]Payslip, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ps.id, ps.school_id, ps.line_id, r.period, e.full_name,
		       l.gross_idr, l.net_idr, ps.object_key, ps.sha256, ps.published_at,
		       ps.reissued_count, ps.created_at
		FROM payslips ps
		JOIN payroll_lines l ON ps.line_id = l.id
		JOIN payroll_runs r ON l.run_id = r.id
		JOIN employees e ON l.employee_id = e.id
		WHERE ps.school_id = $1 AND ($2 = '' OR l.employee_id = $2::uuid)
		  AND ps.published_at IS NOT NULL
		ORDER BY r.period DESC`, schoolID, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Payslip
	for rows.Next() {
		var p Payslip
		if err := rows.Scan(
			&p.ID, &p.SchoolID, &p.LineID, &p.Period, &p.EmployeeName,
			&p.GrossIDR, &p.NetIDR, &p.ObjectKey, &p.SHA256, &p.PublishedAt,
			&p.ReissuedCount, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (s *Store) ListJournals(ctx context.Context, schoolID string) ([]PayrollJournal, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT j.id, j.school_id, j.run_id, r.period, j.kind, j.reference, j.status,
		       j.trans_no, j.attempts, j.created_at
		FROM payroll_journals j
		JOIN payroll_runs r ON j.run_id = r.id
		WHERE j.school_id = $1
		ORDER BY j.created_at DESC`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PayrollJournal
	for rows.Next() {
		var j PayrollJournal
		if err := rows.Scan(
			&j.ID, &j.SchoolID, &j.RunID, &j.Period, &j.Kind, &j.Reference,
			&j.Status, &j.TransNo, &j.Attempts, &j.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, j)
	}
	return list, nil
}

// applyDecisionOnce sets status = decision on a row still pending_approval.
// Zero rows updated means already decided (no-op) or missing (ErrNotFound).
func applyDecisionOnce(ctx context.Context, db *sql.DB, table, id, decision string) error {
	res, err := db.ExecContext(ctx, `UPDATE `+table+` SET status = $2, updated_at = NOW() WHERE id = $1 AND status = 'pending_approval'`, id, decision)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
