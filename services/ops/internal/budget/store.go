package budget

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/ops/internal/procurement"
)

type Store struct {
	db  *sql.DB
	acc AccountingClient
}

func NewStore(db *sql.DB, acc AccountingClient) *Store {
	return &Store{db: db, acc: acc}
}

func (s *Store) GetSettings(ctx context.Context, schoolID string) (*BudgetSettings, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT school_id, fiscal_year_start_month, lock_rule, tolerance_bp, warn_threshold_bp, updated_at
		FROM budget_settings
		WHERE school_id = $1
	`, schoolID)

	var st BudgetSettings
	err := row.Scan(&st.SchoolID, &st.FiscalYearStartMonth, &st.LockRule, &st.ToleranceBP, &st.WarnThresholdBP, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		// Return default settings
		return &BudgetSettings{
			SchoolID:             schoolID,
			FiscalYearStartMonth: 7,
			LockRule:             "warn",
			ToleranceBP:          0,
			WarnThresholdBP:      8000,
			UpdatedAt:            time.Now(),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

func (s *Store) UpdateSettings(ctx context.Context, st *BudgetSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO budget_settings (school_id, fiscal_year_start_month, lock_rule, tolerance_bp, warn_threshold_bp, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (school_id) DO UPDATE SET
			fiscal_year_start_month = EXCLUDED.fiscal_year_start_month,
			lock_rule = EXCLUDED.lock_rule,
			tolerance_bp = EXCLUDED.tolerance_bp,
			warn_threshold_bp = EXCLUDED.warn_threshold_bp,
			updated_at = now()
	`, st.SchoolID, st.FiscalYearStartMonth, st.LockRule, st.ToleranceBP, st.WarnThresholdBP)
	return err
}

func (s *Store) ListBudgets(ctx context.Context, schoolID string, status string) ([]Budget, error) {
	query := `
		SELECT id, school_id, code, name, fiscal_year, period_start, period_end, status, total_planned_idr, approval_id, created_by, created_at, updated_at
		FROM budgets
		WHERE school_id = $1 AND deleted_at IS NULL
	`
	args := []any{schoolID}
	if status != "" {
		query += " AND status = $2"
		args = append(args, status)
	}
	query += " ORDER BY period_start DESC, code ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Budget
	for rows.Next() {
		var b Budget
		var pStart, pEnd time.Time
		if err := rows.Scan(&b.ID, &b.SchoolID, &b.Code, &b.Name, &b.FiscalYear, &pStart, &pEnd, &b.Status, &b.TotalPlannedIDR, &b.ApprovalID, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		b.PeriodStart = pStart.Format("2006-01-02")
		b.PeriodEnd = pEnd.Format("2006-01-02")
		list = append(list, b)
	}
	return list, nil
}

func (s *Store) GetBudget(ctx context.Context, schoolID, id string) (*Budget, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, fiscal_year, period_start, period_end, status, total_planned_idr, approval_id, created_by, created_at, updated_at
		FROM budgets
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)

	var b Budget
	var pStart, pEnd time.Time
	err := row.Scan(&b.ID, &b.SchoolID, &b.Code, &b.Name, &b.FiscalYear, &pStart, &pEnd, &b.Status, &b.TotalPlannedIDR, &b.ApprovalID, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	b.PeriodStart = pStart.Format("2006-01-02")
	b.PeriodEnd = pEnd.Format("2006-01-02")

	lines, err := s.ListBudgetLines(ctx, schoolID, id)
	if err != nil {
		return nil, err
	}
	b.Lines = lines
	return &b, nil
}

func (s *Store) CreateBudget(ctx context.Context, b *Budget) (*Budget, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO budgets (school_id, code, name, fiscal_year, period_start, period_end, status, total_planned_idr, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'draft', 0, $7)
		RETURNING id, created_at, updated_at
	`, b.SchoolID, b.Code, b.Name, b.FiscalYear, b.PeriodStart, b.PeriodEnd, b.CreatedBy)

	if err := row.Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	b.Status = "draft"
	b.TotalPlannedIDR = 0
	return b, nil
}

func (s *Store) UpdateBudget(ctx context.Context, b *Budget) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE budgets
		SET code = $3, name = $4, fiscal_year = $5, period_start = $6, period_end = $7, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status = 'draft' AND deleted_at IS NULL
	`, b.SchoolID, b.ID, b.Code, b.Name, b.FiscalYear, b.PeriodStart, b.PeriodEnd)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("budget not found or not editable")
	}
	return nil
}

func (s *Store) SubmitBudget(ctx context.Context, schoolID, id string, approvalID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE budgets
		SET status = 'pending_approval', approval_id = $3, updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status = 'draft' AND deleted_at IS NULL
	`, schoolID, id, approvalID)
	return err
}

func (s *Store) ApproveBudget(ctx context.Context, schoolID, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE budgets
		SET status = 'approved', updated_at = now()
		WHERE school_id = $1 AND id = $2 AND status = 'pending_approval' AND deleted_at IS NULL
	`, schoolID, id)
	return err
}

func (s *Store) ListBudgetLines(ctx context.Context, schoolID, budgetID string) ([]BudgetLine, error) {
	query := `
		SELECT bl.id, bl.school_id, bl.budget_id, bl.code, bl.name, bl.category, bl.acc_code, bl.div_code,
		       bl.amount_planned_idr, COALESCE(c.actual_idr, 0) as actual_idr,
		       bl.created_at, bl.updated_at
		FROM budget_lines bl
		LEFT JOIN budget_actuals_cache c ON c.budget_line_id = bl.id
		WHERE bl.school_id = $1 AND bl.budget_id = $2
		ORDER BY bl.code ASC
	`
	rows, err := s.db.QueryContext(ctx, query, schoolID, budgetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []BudgetLine
	for rows.Next() {
		var l BudgetLine
		if err := rows.Scan(&l.ID, &l.SchoolID, &l.BudgetID, &l.Code, &l.Name, &l.Category, &l.AccCode, &l.DivCode,
			&l.AmountPlannedIDR, &l.ActualIDR, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}

		// Calculate commitments for this line from open PRs and POs
		commPR := s.getOpenPRCommitment(ctx, l.ID)
		commPO := s.getOpenPOCommitment(ctx, l.ID)
		l.CommitmentIDR = Commitment(commPR, commPO)
		l.RemainingIDR = Remaining(l.AmountPlannedIDR, l.ActualIDR, l.CommitmentIDR)
		l.AbsorptionBP = AbsorptionBP(l.ActualIDR, l.AmountPlannedIDR)

		lines = append(lines, l)
	}
	return lines, nil
}

func (s *Store) getOpenPRCommitment(ctx context.Context, budgetLineKey string) int64 {
	var amt int64
	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_idr), 0)
		FROM pr_open_commitment_v
		WHERE budget_line_key = $1
	`, budgetLineKey).Scan(&amt)
	return amt
}

func (s *Store) getOpenPOCommitment(ctx context.Context, budgetLineKey string) int64 {
	var amt int64
	// PO commitment is ordered - received net
	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(GREATEST(0, (qty_ordered - qty_accepted_net) * unit_price_idr)), 0)
		FROM po_line_match_v
		WHERE budget_line_key = $1 AND po_status IN ('approved', 'sent', 'partially_received')
	`, budgetLineKey).Scan(&amt)
	return amt
}

func (s *Store) CreateBudgetLine(ctx context.Context, l *BudgetLine) (*BudgetLine, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
		INSERT INTO budget_lines (school_id, budget_id, code, name, category, acc_code, div_code, amount_planned_idr)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, l.SchoolID, l.BudgetID, l.Code, l.Name, l.Category, l.AccCode, l.DivCode, l.AmountPlannedIDR)

	if err := row.Scan(&l.ID, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return nil, err
	}

	// Update total_planned_idr on budgets
	if _, err := tx.ExecContext(ctx, `
		UPDATE budgets
		SET total_planned_idr = (SELECT COALESCE(SUM(amount_planned_idr), 0) FROM budget_lines WHERE budget_id = $1),
		    updated_at = now()
		WHERE id = $1
	`, l.BudgetID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return l, nil
}

func (s *Store) UpdateBudgetLine(ctx context.Context, l *BudgetLine) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE budget_lines
		SET code = $4, name = $5, category = $6, acc_code = $7, div_code = $8, amount_planned_idr = $9, updated_at = now()
		WHERE school_id = $1 AND budget_id = $2 AND id = $3
	`, l.SchoolID, l.BudgetID, l.ID, l.Code, l.Name, l.Category, l.AccCode, l.DivCode, l.AmountPlannedIDR)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("budget line not found")
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE budgets
		SET total_planned_idr = (SELECT COALESCE(SUM(amount_planned_idr), 0) FROM budget_lines WHERE budget_id = $1),
		    updated_at = now()
		WHERE id = $1
	`, l.BudgetID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) DeleteBudgetLine(ctx context.Context, schoolID, budgetID, lineID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		DELETE FROM budget_lines
		WHERE school_id = $1 AND budget_id = $2 AND id = $3
	`, schoolID, budgetID, lineID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("budget line not found")
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE budgets
		SET total_planned_idr = (SELECT COALESCE(SUM(amount_planned_idr), 0) FROM budget_lines WHERE budget_id = $1),
		    updated_at = now()
		WHERE id = $1
	`, budgetID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) SyncActualsFromAccounting(ctx context.Context, schoolID, budgetID string) (int, error) {
	b, err := s.GetBudget(ctx, schoolID, budgetID)
	if err != nil {
		return 0, err
	}
	if len(b.Lines) == 0 {
		return 0, nil
	}

	st, err := s.GetSettings(ctx, schoolID)
	if err != nil {
		return 0, err
	}

	var items []ActualQueryItem
	for _, l := range b.Lines {
		div := ""
		if l.DivCode != nil {
			div = *l.DivCode
		}
		items = append(items, ActualQueryItem{
			Key:     l.ID,
			AccCode: l.AccCode,
			DivCode: div,
		})
	}

	results, err := s.acc.FetchActuals(ctx, schoolID, b.PeriodStart, b.PeriodEnd, items)
	if err != nil {
		return 0, err
	}

	synced := 0
	for _, res := range results {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO budget_actuals_cache (budget_line_id, school_id, actual_idr, last_synced_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (budget_line_id) DO UPDATE SET
				actual_idr = EXCLUDED.actual_idr,
				last_synced_at = now()
		`, res.Key, schoolID, res.ActualIDR)
		if err == nil {
			synced++
		}

		// Check if warning threshold crossed
		for _, l := range b.Lines {
			if l.ID == res.Key {
				if CrossedWarn(res.ActualIDR, l.AmountPlannedIDR, st.WarnThresholdBP) {
					currBP := AbsorptionBP(res.ActualIDR, l.AmountPlannedIDR)
					msg := fmt.Sprintf("Pos anggaran %s (%s) mencapai serapan %d.%02d%%", l.Code, l.Name, currBP/100, currBP%100)
					_, _ = s.db.ExecContext(ctx, `
						INSERT INTO budget_alerts (school_id, budget_line_id, threshold_bp, current_bp, message)
						VALUES ($1, $2, $3, $4, $5)
					`, schoolID, l.ID, st.WarnThresholdBP, currBP, msg)
				}
				break
			}
		}
	}

	return synced, nil
}

// Check satisfies procurement.BudgetCheck contract
func (s *Store) Check(ctx context.Context, tx *sql.Tx, in procurement.BudgetCheckInput) (procurement.BudgetCheckResult, error) {
	res := procurement.BudgetCheckResult{
		Policy: "none",
		Lines:  make([]procurement.BudgetLineResult, 0, len(in.Lines)),
		Over:   false,
		AsOf:   time.Now(),
	}

	st, err := s.GetSettings(ctx, in.SchoolID)
	if err != nil {
		return res, err
	}

	type execQuerier interface {
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	}
	var q execQuerier = s.db
	if tx != nil {
		q = tx
	}

	worstPolicyRank := 0 // 0: none, 1: warn, 2: extra_approval, 3: reject
	rankOf := func(p string) int {
		switch p {
		case "reject":
			return 3
		case "extra_approval":
			return 2
		case "warn":
			return 1
		default:
			return 0
		}
	}

	for _, reqLine := range in.Lines {
		if reqLine.BudgetLineKey == "" {
			continue
		}

		var plannedIDR int64
		var actualIDR int64
		row := q.QueryRowContext(ctx, `
			SELECT bl.amount_planned_idr, COALESCE(c.actual_idr, 0)
			FROM budget_lines bl
			LEFT JOIN budget_actuals_cache c ON c.budget_line_id = bl.id
			WHERE bl.id = $1 AND bl.school_id = $2
		`, reqLine.BudgetLineKey, in.SchoolID)

		err := row.Scan(&plannedIDR, &actualIDR)
		if errors.Is(err, sql.ErrNoRows) {
			// Budget line does not exist
			res.Over = true
			if rankOf(st.LockRule) > worstPolicyRank {
				worstPolicyRank = rankOf(st.LockRule)
			}
			res.Lines = append(res.Lines, procurement.BudgetLineResult{
				BudgetLineKey: reqLine.BudgetLineKey,
				BudgetIDR:     0,
				ActualIDR:     0,
				CommitmentIDR: 0,
				RemainingIDR:  0,
				RequestIDR:    reqLine.AmountIDR,
				Over:          true,
			})
			continue
		} else if err != nil {
			return res, err
		}

		// Commitments:
		// Exclude source if requested (e.g. "pr:<id>" or "po:<id>")
		var commPR, commPO int64
		if in.ExcludeSource != "" && strings.HasPrefix(in.ExcludeSource, "pr:") {
			excludePRID := strings.TrimPrefix(in.ExcludeSource, "pr:")
			_ = s.db.QueryRowContext(ctx, `
				SELECT COALESCE(SUM(amount_idr), 0)
				FROM pr_open_commitment_v
				WHERE budget_line_key = $1 AND pr_id != $2
			`, reqLine.BudgetLineKey, excludePRID).Scan(&commPR)
		} else {
			commPR = s.getOpenPRCommitment(ctx, reqLine.BudgetLineKey)
		}

		if in.ExcludeSource != "" && strings.HasPrefix(in.ExcludeSource, "po:") {
			excludePOID := strings.TrimPrefix(in.ExcludeSource, "po:")
			_ = s.db.QueryRowContext(ctx, `
				SELECT COALESCE(SUM(GREATEST(0, (qty_ordered - qty_accepted_net) * unit_price_idr)), 0)
				FROM po_line_match_v
				WHERE budget_line_key = $1 AND po_id != $2 AND po_status IN ('approved', 'sent', 'partially_received')
			`, reqLine.BudgetLineKey, excludePOID).Scan(&commPO)
		} else {
			commPO = s.getOpenPOCommitment(ctx, reqLine.BudgetLineKey)
		}

		totComm := Commitment(commPR, commPO)
		rem := Remaining(plannedIDR, actualIDR, totComm)

		lineOver, linePolicy := CheckLineBudget(plannedIDR, actualIDR, totComm, reqLine.AmountIDR, st.ToleranceBP, st.LockRule)
		if lineOver {
			res.Over = true
		}
		if rankOf(linePolicy) > worstPolicyRank {
			worstPolicyRank = rankOf(linePolicy)
		}

		res.Lines = append(res.Lines, procurement.BudgetLineResult{
			BudgetLineKey: reqLine.BudgetLineKey,
			BudgetIDR:     plannedIDR,
			ActualIDR:     actualIDR,
			CommitmentIDR: totComm,
			RemainingIDR:  rem,
			RequestIDR:    reqLine.AmountIDR,
			Over:          lineOver,
		})
	}

	switch worstPolicyRank {
	case 3:
		res.Policy = "reject"
	case 2:
		res.Policy = "extra_approval"
	case 1:
		res.Policy = "warn"
	default:
		res.Policy = "none"
	}

	return res, nil
}

func (s *Store) ListAlerts(ctx context.Context, schoolID string, unackOnly bool) ([]BudgetAlert, error) {
	query := `
		SELECT ba.id, ba.school_id, ba.budget_line_id, bl.code, bl.name, ba.threshold_bp, ba.current_bp, ba.message, ba.acknowledged_at, ba.acknowledged_by, ba.created_at
		FROM budget_alerts ba
		JOIN budget_lines bl ON bl.id = ba.budget_line_id
		WHERE ba.school_id = $1
	`
	if unackOnly {
		query += " AND ba.acknowledged_at IS NULL"
	}
	query += " ORDER BY ba.created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []BudgetAlert
	for rows.Next() {
		var a BudgetAlert
		if err := rows.Scan(&a.ID, &a.SchoolID, &a.BudgetLineID, &a.BudgetLineCode, &a.BudgetLineName, &a.ThresholdBP, &a.CurrentBP, &a.Message, &a.AcknowledgedAt, &a.AcknowledgedBy, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

func (s *Store) AcknowledgeAlert(ctx context.Context, schoolID, alertID, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE budget_alerts
		SET acknowledged_at = now(), acknowledged_by = $3
		WHERE school_id = $1 AND id = $2 AND acknowledged_at IS NULL
	`, schoolID, alertID, userID)
	return err
}

func (s *Store) GetBudgetReport(ctx context.Context, schoolID, budgetID string) (*BudgetReport, error) {
	b, err := s.GetBudget(ctx, schoolID, budgetID)
	if err != nil {
		return nil, err
	}
	st, err := s.GetSettings(ctx, schoolID)
	if err != nil {
		return nil, err
	}

	rep := &BudgetReport{
		Budget:   *b,
		Settings: *st,
		Lines:    b.Lines,
	}

	for _, l := range b.Lines {
		rep.TotalPlannedIDR += l.AmountPlannedIDR
		rep.TotalActualIDR += l.ActualIDR
		rep.TotalCommitIDR += l.CommitmentIDR
		rep.TotalRemainingIDR += l.RemainingIDR
	}
	rep.TotalAbsorptionBP = AbsorptionBP(rep.TotalActualIDR, rep.TotalPlannedIDR)

	return rep, nil
}
