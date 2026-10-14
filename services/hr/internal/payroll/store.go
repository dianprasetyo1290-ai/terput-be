package payroll

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type Store struct {
	DB *sql.DB
	// DecryptAmount decrypts employment_assignments.base_salary_enc
	// (crypto.Service.DecryptAmount, aad = school id).
	DecryptAmount func(raw []byte, aad string) (int64, error)
	Accounting    *AccountingClient
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db}
}

// -----------------------------------------------------------------------------
// Pay Components
// -----------------------------------------------------------------------------

func (s *Store) ListComponents(ctx context.Context, schoolID string) ([]PayComponent, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.id, c.school_id, c.code, c.name, c.kind, c.nature, c.taxable, c.bpjs_base, c.calc,
		       c.default_amount_idr, c.rate_per_unit_idr, c.max_input_idr, c.sort, c.status,
		       c.approval_id, a.acc_code, a.liability_acc_code, c.created_at, c.updated_at
		FROM pay_components c
		LEFT JOIN pay_component_accounts a ON a.component_id = c.id AND a.school_id = c.school_id
		WHERE c.school_id = $1 AND c.deleted_at IS NULL
		ORDER BY c.sort ASC, c.code ASC`, schoolID)
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
			&c.Sort, &c.Status, &c.ApprovalID, &c.AccCode, &c.LiabilityAccCode, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

// CreateComponent stores a new component as draft with its account
// mapping (BR-PRL-04); it is used only after core approves it.
func (s *Store) CreateComponent(ctx context.Context, c PayComponent) (*PayComponent, error) {
	c.Status = "draft"
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO pay_components (
			school_id, code, name, kind, nature, taxable, bpjs_base, calc,
			default_amount_idr, rate_per_unit_idr, max_input_idr, sort, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at, updated_at
	`, c.SchoolID, c.Code, c.Name, c.Kind, c.Nature, c.Taxable, c.BPJSBase, c.Calc,
		c.DefaultAmountIDR, c.RatePerUnitIDR, c.MaxInputIDR, c.Sort, c.Status).Scan(&id, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	c.ID = id
	if c.AccCode != nil && *c.AccCode != "" {
		if err := upsertComponentAccount(ctx, tx, c.SchoolID, id, *c.AccCode, c.LiabilityAccCode); err != nil {
			return nil, err
		}
	}
	return &c, tx.Commit()
}

// SetComponentAccounts maps a component to its expense/liability account.
func (s *Store) SetComponentAccounts(ctx context.Context, schoolID, id, acc string, liability *string) error {
	if _, err := s.GetComponent(ctx, schoolID, id); err != nil {
		return err
	}
	return upsertComponentAccount(ctx, s.DB, schoolID, id, acc, liability)
}

func upsertComponentAccount(ctx context.Context, q querier, schoolID, componentID, acc string, liability *string) error {
	if liability != nil && *liability == "" {
		liability = nil
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO pay_component_accounts (school_id, component_id, acc_code, liability_acc_code)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (school_id, component_id) DO UPDATE SET acc_code = EXCLUDED.acc_code, liability_acc_code = EXCLUDED.liability_acc_code`,
		schoolID, componentID, acc, liability)
	return err
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

func constraintName(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.ConstraintName
	}
	return ""
}

func (s *Store) GetComponent(ctx context.Context, schoolID, id string) (*PayComponent, error) {
	var c PayComponent
	err := s.DB.QueryRowContext(ctx, `
		SELECT c.id, c.school_id, c.code, c.name, c.kind, c.nature, c.taxable, c.bpjs_base, c.calc,
		       c.default_amount_idr, c.rate_per_unit_idr, c.max_input_idr, c.sort, c.status,
		       c.approval_id, a.acc_code, a.liability_acc_code, c.created_at, c.updated_at
		FROM pay_components c
		LEFT JOIN pay_component_accounts a ON a.component_id = c.id AND a.school_id = c.school_id
		WHERE c.school_id = $1 AND c.id = $2 AND c.deleted_at IS NULL
	`, schoolID, id).Scan(
		&c.ID, &c.SchoolID, &c.Code, &c.Name, &c.Kind, &c.Nature, &c.Taxable,
		&c.BPJSBase, &c.Calc, &c.DefaultAmountIDR, &c.RatePerUnitIDR, &c.MaxInputIDR,
		&c.Sort, &c.Status, &c.ApprovalID, &c.AccCode, &c.LiabilityAccCode, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (s *Store) SubmitComponentApproval(ctx context.Context, schoolID, id, approvalID string) error {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE pay_components
		SET status = 'pending_approval', approval_id = $3, updated_at = now()
		WHERE id = $1 AND school_id = $2
	`, id, schoolID, approvalID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ApplyComponentDecision(ctx context.Context, sourceID, decision string) error {
	var targetStatus string
	switch decision {
	case "approved":
		targetStatus = "active"
	case "rejected":
		targetStatus = "retired"
	default:
		return fmt.Errorf("keputusan tidak dikenal: %s", decision)
	}

	res, err := s.DB.ExecContext(ctx, `
		UPDATE pay_components
		SET status = $2, updated_at = now()
		WHERE id = $1 AND status = 'pending_approval'
	`, sourceID, targetStatus)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		var exists bool
		_ = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pay_components WHERE id = $1)`, sourceID).Scan(&exists)
		if !exists {
			return ErrNotFound
		}
		// Already final -> idempotent no-op
		return nil
	}
	return nil
}

func (s *Store) ListInputs(ctx context.Context, schoolID, period string) ([]PayrollInputView, error) {
	query := `
		SELECT i.id, i.school_id, i.period, i.employee_id, coalesce(e.full_name, ''), e.employee_no,
		       i.component_id, coalesce(c.name, ''), i.amount_idr, i.source, i.locked,
		       EXISTS(SELECT 1 FROM payroll_input_changes pic WHERE pic.input_id = i.id AND pic.status = 'pending_approval'),
		       i.created_at
		FROM payroll_inputs i
		JOIN pay_components c ON c.id = i.component_id
		JOIN employees e ON e.id = i.employee_id
		WHERE i.school_id = $1 AND i.period = $2
		ORDER BY e.full_name ASC, c.sort ASC
	`
	rows, err := s.DB.QueryContext(ctx, query, schoolID, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PayrollInputView
	for rows.Next() {
		var v PayrollInputView
		if err := rows.Scan(
			&v.ID, &v.SchoolID, &v.Period, &v.EmployeeID, &v.EmployeeName, &v.EmployeeNIP,
			&v.ComponentID, &v.ComponentName, &v.AmountIDR, &v.Source, &v.Locked,
			&v.PendingChange, &v.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}

func (s *Store) SaveInput(ctx context.Context, schoolID, period, employeeID, componentID string, amountIDR int64, reason string) (needsApproval bool, inputID string, changeID string, err error) {
	// Check calendar cutoff_date
	var cutoffDate time.Time
	err = s.DB.QueryRowContext(ctx, `
		SELECT cutoff_date FROM payroll_calendars WHERE school_id = $1 AND period = $2
	`, schoolID, period).Scan(&cutoffDate)

	isAfterCutoff := false
	if err == nil {
		today := time.Now()
		todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
		cutoffOnly := time.Date(cutoffDate.Year(), cutoffDate.Month(), cutoffDate.Day(), 0, 0, 0, 0, cutoffDate.Location())
		if todayDate.After(cutoffOnly) {
			isAfterCutoff = true
		}
	}

	if isAfterCutoff {
		// Cutoff passed -> record change in payroll_input_changes, requiring Core approval (FR-94-004)
		var existingID string
		var beforeAmount int64
		qErr := s.DB.QueryRowContext(ctx, `
			SELECT id, amount_idr FROM payroll_inputs
			WHERE school_id = $1 AND period = $2 AND employee_id = $3 AND component_id = $4
		`, schoolID, period, employeeID, componentID).Scan(&existingID, &beforeAmount)

		var inpID *string
		if qErr == nil {
			inpID = &existingID
		}

		if reason == "" {
			reason = "Penyesuaian input payroll setelah cutoff"
		}

		var chgID string
		err = s.DB.QueryRowContext(ctx, `
			INSERT INTO payroll_input_changes (
				school_id, input_id, period, employee_id, component_id,
				before_amount_idr, after_amount_idr, reason, status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending_approval', now(), now())
			RETURNING id
		`, schoolID, inpID, period, employeeID, componentID, beforeAmount, amountIDR, reason).Scan(&chgID)
		if err != nil {
			return false, "", "", err
		}
		return true, existingID, chgID, nil
	}

	// Normal input before/on cutoff: upsert directly
	var id string
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO payroll_inputs (
			school_id, period, employee_id, component_id, amount_idr, source, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'manual', now(), now())
		ON CONFLICT (school_id, period, employee_id, component_id)
		DO UPDATE SET amount_idr = $5, updated_at = now()
		RETURNING id
	`, schoolID, period, employeeID, componentID, amountIDR).Scan(&id)
	if err != nil {
		return false, "", "", err
	}
	return false, id, "", nil
}

func (s *Store) UpdateInputChangeApproval(ctx context.Context, changeID, approvalID string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE payroll_input_changes
		SET approval_id = $2, updated_at = now()
		WHERE id = $1
	`, changeID, approvalID)
	return err
}

func (s *Store) ApplyInputChangeDecision(ctx context.Context, sourceID, decision string) error {
	var targetStatus string
	switch decision {
	case "approved":
		targetStatus = "approved"
	case "rejected":
		targetStatus = "rejected"
	default:
		return fmt.Errorf("keputusan tidak dikenal: %s", decision)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var chg struct {
		SchoolID       string
		Period         string
		EmployeeID     string
		ComponentID    string
		AfterAmountIDR int64
		Status         string
	}
	err = tx.QueryRowContext(ctx, `
		SELECT school_id, period, employee_id, component_id, after_amount_idr, status
		FROM payroll_input_changes WHERE id = $1
	`, sourceID).Scan(
		&chg.SchoolID, &chg.Period, &chg.EmployeeID, &chg.ComponentID,
		&chg.AfterAmountIDR, &chg.Status,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	if chg.Status != "pending_approval" {
		// Already decided (idempotent)
		return nil
	}

	if targetStatus == "approved" {
		// Apply to payroll_inputs
		_, err = tx.ExecContext(ctx, `
			INSERT INTO payroll_inputs (
				school_id, period, employee_id, component_id, amount_idr, source, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, 'manual', now(), now())
			ON CONFLICT (school_id, period, employee_id, component_id)
			DO UPDATE SET amount_idr = $5, updated_at = now()
		`, chg.SchoolID, chg.Period, chg.EmployeeID, chg.ComponentID, chg.AfterAmountIDR)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE payroll_input_changes
		SET status = $2, updated_at = now()
		WHERE id = $1
	`, sourceID, targetStatus)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ErrAttendanceNotSent: FR-94-001, the HR attendance recap of the period is
// not sent/locked yet.
var ErrAttendanceNotSent = errors.New("rekap absensi belum dikirim HR")

// PullResult reports what pull-attendance wrote.
type PullResult struct {
	Overtime      int      `json:"overtime"`
	Absence       int      `json:"absence"`
	Skipped       int      `json:"skipped_locked"`
	Missing       []string `json:"missing_components"`
	InsertedCount int      `json:"inserted_count"`
}

// PullAttendance (FR-94-001): approved overtime minutes of the period go to
// the active component with calc 'overtime' (hours × rate), alpa days to the
// active deduction component coded ALPA (days × rate) when configured.
// Manual inputs and locked rows are never overwritten.
func (s *Store) PullAttendance(ctx context.Context, schoolID, period string) (*PullResult, error) {
	start, end, err := periodBounds(period)
	if err != nil {
		return nil, err
	}
	var status string
	err = s.DB.QueryRowContext(ctx, `SELECT status FROM attendance_periods WHERE school_id = $1 AND period = $2`, schoolID, period).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && status != "sent" && status != "locked") {
		return nil, ErrAttendanceNotSent
	}
	if err != nil {
		return nil, err
	}
	res := &PullResult{Missing: []string{}}

	type comp struct {
		id   string
		rate *int64
	}
	find := func(where string) (*comp, error) {
		var c comp
		err := s.DB.QueryRowContext(ctx, `SELECT id, rate_per_unit_idr FROM pay_components
			WHERE school_id = $1 AND status = 'active' AND deleted_at IS NULL AND `+where+` ORDER BY sort, code LIMIT 1`, schoolID).
			Scan(&c.id, &c.rate)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return &c, err
	}
	upsert := func(empID string, c *comp, qty float64) (bool, error) {
		var amount int64
		if c.rate != nil {
			amount = int64(qty * float64(*c.rate))
		}
		r, err := s.DB.ExecContext(ctx, `
			INSERT INTO payroll_inputs (school_id, period, employee_id, component_id, quantity, amount_idr, source)
			VALUES ($1, $2, $3, $4, $5, $6, 'attendance')
			ON CONFLICT (school_id, period, employee_id, component_id) DO UPDATE
			SET quantity = EXCLUDED.quantity, amount_idr = EXCLUDED.amount_idr, updated_at = now()
			WHERE payroll_inputs.source = 'attendance' AND NOT payroll_inputs.locked`,
			schoolID, period, empID, c.id, qty, amount)
		if err != nil {
			return false, err
		}
		n, _ := r.RowsAffected()
		return n > 0, nil
	}

	ot, err := find(`calc = 'overtime' AND kind = 'earning'`)
	if err != nil {
		return nil, err
	}
	if ot == nil {
		res.Missing = append(res.Missing, "komponen lembur (calc overtime)")
	} else {
		rows, err := s.DB.QueryContext(ctx, `
			SELECT employee_id, SUM(minutes) FROM overtime_requests
			WHERE school_id = $1 AND status = 'approved' AND work_date BETWEEN $2 AND $3
			GROUP BY employee_id`, schoolID, start, end)
		if err != nil {
			return nil, err
		}
		type agg struct {
			emp string
			min int64
		}
		var list []agg
		for rows.Next() {
			var a agg
			if err := rows.Scan(&a.emp, &a.min); err != nil {
				rows.Close()
				return nil, err
			}
			list = append(list, a)
		}
		rows.Close()
		for _, a := range list {
			ok, err := upsert(a.emp, ot, float64(a.min)/60)
			if err != nil {
				return nil, err
			}
			if ok {
				res.Overtime++
			} else {
				res.Skipped++
			}
		}
	}

	alpa, err := find(`upper(code) = 'ALPA' AND kind = 'deduction'`)
	if err != nil {
		return nil, err
	}
	if alpa == nil {
		res.Missing = append(res.Missing, "komponen potongan alpa (kode ALPA)")
	} else {
		rows, err := s.DB.QueryContext(ctx, `
			SELECT employee_id, COUNT(*) FROM employee_attendance
			WHERE school_id = $1 AND status = 'alpa' AND work_date BETWEEN $2 AND $3
			GROUP BY employee_id`, schoolID, start, end)
		if err != nil {
			return nil, err
		}
		type agg struct {
			emp  string
			days int64
		}
		var list []agg
		for rows.Next() {
			var a agg
			if err := rows.Scan(&a.emp, &a.days); err != nil {
				rows.Close()
				return nil, err
			}
			list = append(list, a)
		}
		rows.Close()
		for _, a := range list {
			ok, err := upsert(a.emp, alpa, float64(a.days))
			if err != nil {
				return nil, err
			}
			if ok {
				res.Absence++
			} else {
				res.Skipped++
			}
		}
	}
	res.InsertedCount = res.Overtime + res.Absence
	return res, nil
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

// ErrDuplicateRegular: AC-95-04.
var ErrDuplicateRegular = errors.New("Payroll reguler periode ini sudah ada, gunakan run susulan")

// UnmappedComponentsError: BR-PRL-04 / AC-93-02.
type UnmappedComponentsError struct{ Codes []string }

func (e *UnmappedComponentsError) Error() string {
	return "komponen gaji belum dipetakan ke akun: " + strings.Join(e.Codes, ", ")
}

// CreateRun validates the period and kind, checks that every active
// component has an account (BR-PRL-04) and refuses a second regular run.
func (s *Store) CreateRun(ctx context.Context, r PayrollRun) (*PayrollRun, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.code FROM pay_components c
		LEFT JOIN pay_component_accounts a ON a.component_id = c.id AND a.school_id = c.school_id
		WHERE c.school_id = $1 AND c.status = 'active' AND c.deleted_at IS NULL AND a.id IS NULL
		ORDER BY c.code`, r.SchoolID)
	if err != nil {
		return nil, err
	}
	var unmapped []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			rows.Close()
			return nil, err
		}
		unmapped = append(unmapped, code)
	}
	rows.Close()
	if len(unmapped) > 0 {
		return nil, &UnmappedComponentsError{Codes: unmapped}
	}
	if r.Kind == "supplementary" {
		var hasRegular bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM payroll_runs WHERE school_id = $1 AND period = $2 AND kind = 'regular' AND status NOT IN ('rejected','cancelled'))`,
			r.SchoolID, r.Period).Scan(&hasRegular); err != nil {
			return nil, err
		}
		if !hasRegular {
			return nil, errors.New("run susulan memerlukan payroll reguler pada periode yang sama")
		}
	}
	if r.IdempotencyKey == "" {
		r.IdempotencyKey = fmt.Sprintf("run-%s-%s-%d", r.SchoolID, r.Period, time.Now().UnixNano())
	}
	r.Status = "draft"
	err = s.DB.QueryRowContext(ctx, `
		INSERT INTO payroll_runs (school_id, period, kind, status, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, r.SchoolID, r.Period, r.Kind, r.Status, r.IdempotencyKey).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			if constraintName(err) == "one_regular_run" {
				return nil, ErrDuplicateRegular
			}
			return nil, ErrConflict
		}
		return nil, err
	}
	return &r, nil
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

// FinalizeRun makes an approved run final (lines become immutable through
// the 0031 trigger), issues one payslip per line whose sha256 is the hash
// of the rendered PDF, and queues the accrual journal, which is posted to
// accounting after commit. The returned journal tells the caller whether
// posting succeeded, failed or is not configured.
func (s *Store) FinalizeRun(ctx context.Context, schoolID, runID, userID string) (*PayrollJournal, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var finalizedBy any
	if userID != "" {
		finalizedBy = userID
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE payroll_runs
		SET status = 'final', finalized_at = now(), finalized_by = $3, updated_at = now()
		WHERE id = $1 AND school_id = $2 AND status = 'approved'`, runID, schoolID, finalizedBy)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrConflict
	}
	var period, kind string
	if err := tx.QueryRowContext(ctx, `SELECT period, kind FROM payroll_runs WHERE id = $1`, runID).Scan(&period, &kind); err != nil {
		return nil, err
	}

	docs, err := s.payslipDocs(ctx, tx, schoolID, `l.run_id = $2`, runID)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, errors.New("payroll run tidak memiliki baris; hitung ulang sebelum finalisasi")
	}
	var journalLines []LineDetailsWithTotals
	for _, d := range docs {
		_, sum := RenderPayslipPDF(d)
		key := fmt.Sprintf("payslips/%s/%s.pdf", runID, d.Line.ID)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO payslips (school_id, line_id, object_key, sha256, published_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (line_id) DO NOTHING`, schoolID, d.Line.ID, key, sum); err != nil {
			return nil, err
		}
		journalLines = append(journalLines, LineDetailsWithTotals{
			NetIDR: d.Line.NetIDR, PPh21IDR: d.Line.PPh21IDR,
			BPJSEmployeeIDR: d.Line.BPJSEmployeeIDR, BPJSEmployerIDR: d.Line.BPJSEmployerIDR,
			Details: d.Details,
		})
	}

	mapping, err := loadAccountMapping(ctx, tx, schoolID)
	if err != nil {
		return nil, err
	}
	jl, err := BuildAccrualJournal(journalLines, mapping)
	if err != nil {
		return nil, &UnmappedComponentsError{Codes: []string{err.Error()}}
	}
	linesJSON, _ := json.Marshal(jl)
	var journalID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO payroll_journals (school_id, run_id, kind, reference, status, lines)
		VALUES ($1, $2, 'accrual', $3, 'pending', $4)
		ON CONFLICT (reference) DO UPDATE SET updated_at = now()
		RETURNING id`, schoolID, runID, fmt.Sprintf("payroll:%s:accrual", runID), linesJSON).Scan(&journalID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.PostJournal(ctx, schoolID, journalID)
}

// -----------------------------------------------------------------------------
// Payroll Lines & Payslips
// -----------------------------------------------------------------------------

func (s *Store) ListLines(ctx context.Context, schoolID, runID string, flaggedOnly bool) ([]PayrollLine, error) {
	q := `
		SELECT l.id, l.school_id, l.run_id, l.employee_id, e.full_name as employee_name,
		       e.employee_no AS employee_nip, l.gross_idr, l.taxable_idr, l.bpjs_base_idr,
		       l.bpjs_employee_idr, l.bpjs_employer_idr, l.pph21_idr, l.other_deductions_idr,
		       l.net_idr, l.prev_net_idr, l.variance_bp, l.flagged, l.payment_status, l.details, l.created_at
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

	list := []PayrollLine{}
	for rows.Next() {
		var l PayrollLine
		if err := rows.Scan(
			&l.ID, &l.SchoolID, &l.RunID, &l.EmployeeID, &l.EmployeeName, &l.EmployeeNIP,
			&l.GrossIDR, &l.TaxableIDR, &l.BPJSBaseIDR, &l.BPJSEmployeeIDR, &l.BPJSEmployerIDR,
			&l.PPh21IDR, &l.OtherDeductionsIDR, &l.NetIDR, &l.PrevNetIDR, &l.VarianceBP,
			&l.Flagged, &l.PaymentStatus, &l.Details, &l.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

func (s *Store) ListJournals(ctx context.Context, schoolID, period string) ([]PayrollJournal, error) {
	return s.listJournals(ctx, schoolID, "", period)
}

func (s *Store) listJournals(ctx context.Context, schoolID, id, period string) ([]PayrollJournal, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT j.id, j.school_id, j.run_id, r.period, j.kind, j.reference, j.status,
		       j.trans_no, j.error, j.lines, j.attempts, j.created_at
		FROM payroll_journals j
		JOIN payroll_runs r ON j.run_id = r.id
		WHERE j.school_id = $1 AND ($2 = '' OR j.id::text = $2) AND ($3 = '' OR r.period = $3)
		ORDER BY j.created_at DESC`, schoolID, id, period)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []PayrollJournal{}
	for rows.Next() {
		var j PayrollJournal
		if err := rows.Scan(
			&j.ID, &j.SchoolID, &j.RunID, &j.Period, &j.Kind, &j.Reference,
			&j.Status, &j.TransNo, &j.Error, &j.Lines, &j.Attempts, &j.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, j)
	}
	return list, rows.Err()
}

// ListPayslips lists published payslips, optionally of one employee and/or
// one run, newest first. limit <= 0 means no limit.
func (s *Store) ListPayslips(ctx context.Context, schoolID, employeeID, runID string, limit int) ([]Payslip, error) {
	q := `
		SELECT ps.id, ps.school_id, ps.line_id, r.period, e.full_name, l.employee_id, l.run_id,
		       l.gross_idr, l.net_idr, ps.object_key, ps.sha256, ps.published_at,
		       ps.reissued_count, ps.created_at
		FROM payslips ps
		JOIN payroll_lines l ON ps.line_id = l.id
		JOIN payroll_runs r ON l.run_id = r.id
		JOIN employees e ON l.employee_id = e.id
		WHERE ps.school_id = $1 AND ($2 = '' OR l.employee_id::text = $2) AND ($3 = '' OR l.run_id::text = $3)
		  AND ps.published_at IS NOT NULL
		ORDER BY r.period DESC, e.full_name`
	args := []any{schoolID, employeeID, runID}
	if limit > 0 {
		q += ` LIMIT $4`
		args = append(args, limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []Payslip{}
	for rows.Next() {
		var p Payslip
		if err := rows.Scan(
			&p.ID, &p.SchoolID, &p.LineID, &p.Period, &p.EmployeeName, &p.EmployeeID, &p.RunID,
			&p.GrossIDR, &p.NetIDR, &p.ObjectKey, &p.SHA256, &p.PublishedAt,
			&p.ReissuedCount, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
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
