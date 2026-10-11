package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Shared bill services (used by the admin, internal and parent handlers)
// ---------------------------------------------------------------------------

// generateRequest is the shared payload for both
// POST /admin/bills/generate and POST /internal/bills/generate.
type generateRequest struct {
	SchemeID       string   `json:"scheme_id"`
	PeriodYear     int      `json:"period_year"`
	PeriodMonth    int      `json:"period_month"`
	StudentUserIDs []string `json:"student_user_ids"`
	// StudentUserID is an optional single-student convenience for the
	// scheduler-driven internal endpoint.
	StudentUserID string `json:"student_user_id"`
}

// generateResult reports what a generation run did. Running the same period
// twice yields generated=0 the second time.
type generateResult struct {
	Generated int      `json:"generated"`
	Skipped   int      `json:"skipped"`
	BillIDs   []string `json:"bill_ids"`
}

// queryer is satisfied by both *sql.DB and *sql.Tx.
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// billTotalsQ returns the snapshot net, verified allocation total, reversal
// total and outstanding balance for one bill, computed from the ledger.
func billTotalsQ(ctx context.Context, q queryer, schoolID, billID string) (net, verified, reversed, outstanding int64, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT net_idr FROM student_bills WHERE id=$1 AND school_id=$2`, billID, schoolID).Scan(&net)
	if err == sql.ErrNoRows {
		return 0, 0, 0, 0, &httpError{http.StatusNotFound, "bill not found"}
	}
	if err != nil {
		return
	}
	if err = q.QueryRowContext(ctx,
		`SELECT COALESCE(sum(a.amount_idr),0)
		 FROM bill_payment_allocations a JOIN bill_payments p ON p.id=a.payment_id
		 WHERE p.student_bill_id=$1 AND p.school_id=$2 AND p.status='verified'`,
		billID, schoolID).Scan(&verified); err != nil {
		return
	}
	if err = q.QueryRowContext(ctx,
		`SELECT COALESCE(sum(r.amount_idr),0)
		 FROM bill_payment_reversals r JOIN bill_payments p ON p.id=r.payment_id
		 WHERE p.student_bill_id=$1 AND p.school_id=$2`,
		billID, schoolID).Scan(&reversed); err != nil {
		return
	}
	outstanding, err = model.Outstanding(net, []int64{verified}, []int64{reversed})
	return
}

// recomputeBillStatusQ re-derives a bill's status from its ledger and writes
// it back. Void bills are never touched.
func recomputeBillStatusQ(ctx context.Context, q queryer, schoolID, billID string) error {
	net, verified, reversed, _, err := billTotalsQ(ctx, q, schoolID, billID)
	if err != nil {
		return err
	}
	status := model.DeriveBillStatus(net, verified-reversed)
	_, err = q.ExecContext(ctx,
		`UPDATE student_bills SET status=$1,updated_at=now() WHERE id=$2 AND school_id=$3 AND status<>'void'`,
		status, billID, schoolID)
	return err
}

// discountRule is a stored bill_discount plus its scope metadata.
type discountRule struct {
	candidate     model.DiscountCandidate
	scope         string
	schemeID      string
	gradeLevel    string
	studentUserID string
}

// rulesForStudent filters school-wide, scheme, grade and student discounts
// down to the ones that apply to a single student's bill.
func rulesForStudent(rules []discountRule, schemeID, gradeLevel, studentUserID string) []model.DiscountCandidate {
	out := make([]model.DiscountCandidate, 0, len(rules))
	for _, r := range rules {
		switch r.scope {
		case "school":
		case "scheme":
			if r.schemeID != schemeID {
				continue
			}
		case "grade":
			if r.gradeLevel == "" || r.gradeLevel != gradeLevel {
				continue
			}
		case "student":
			if r.studentUserID != studentUserID {
				continue
			}
		default:
			continue
		}
		out = append(out, r.candidate)
	}
	return out
}

// loadDiscountRules loads every active discount for the school. The list is
// small (per school) so it is filtered in Go per student.
func (a *App) loadDiscountRules(ctx context.Context, schoolID string) ([]discountRule, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id,name,kind,value,COALESCE(cap_idr,0),priority,scope,
		        COALESCE(scheme_id::text,''),COALESCE(grade_level,''),COALESCE(student_user_id::text,''),created_at
		 FROM bill_discounts WHERE school_id=$1 AND active
		 ORDER BY priority,created_at,id`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []discountRule
	for rows.Next() {
		var (
			r          discountRule
			name, kind string
			value, cap int64
			priority   int
		)
		if err := rows.Scan(&r.candidate.ID, &name, &kind, &value, &cap, &priority, &r.scope,
			&r.schemeID, &r.gradeLevel, &r.studentUserID, &r.candidate.CreatedAt); err != nil {
			return nil, err
		}
		r.candidate.Source = "bill_discount"
		r.candidate.Label = name
		r.candidate.Kind = model.DiscountKind(kind)
		r.candidate.Value = value
		r.candidate.Cap = cap
		r.candidate.Priority = priority
		out = append(out, r)
	}
	return out, rows.Err()
}

// generateBills creates the bills a request asks for, idempotently. It is the
// single implementation behind both the admin and internal generate routes so
// the two can never drift.
func (a *App) generateBills(ctx context.Context, schoolID string, in generateRequest) (generateResult, error) {
	result := generateResult{BillIDs: []string{}}

	students := normalizeStudentIDs(in.StudentUserIDs, in.StudentUserID)
	if strings.TrimSpace(in.SchemeID) == "" {
		return result, &httpError{http.StatusUnprocessableEntity, "scheme_id is required"}
	}
	if in.PeriodMonth < 1 || in.PeriodMonth > 12 {
		return result, &httpError{http.StatusUnprocessableEntity, "period_month must be between 1 and 12"}
	}
	if in.PeriodYear < 2000 || in.PeriodYear > 2100 {
		return result, &httpError{http.StatusUnprocessableEntity, "period_year is out of range"}
	}
	if len(students) == 0 {
		return result, &httpError{http.StatusUnprocessableEntity, "at least one student_user_id is required"}
	}

	// Resolve the scheme (school-scoped).
	var (
		gradeLevel  string
		dueDay      int
		schemeState string
	)
	err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(grade_level,''),due_day,status
		 FROM billing_schemes WHERE id=$1 AND school_id=$2`, in.SchemeID, schoolID).
		Scan(&gradeLevel, &dueDay, &schemeState)
	if err == sql.ErrNoRows {
		return result, &httpError{http.StatusNotFound, "scheme not found"}
	}
	if err != nil {
		return result, err
	}
	if schemeState == "archived" {
		return result, &httpError{http.StatusUnprocessableEntity, "scheme is archived"}
	}

	// Line items from the scheme's components.
	itemRows, err := a.DB.QueryContext(ctx,
		`SELECT code,name,amount_idr FROM billing_scheme_components
		 WHERE scheme_id=$1 AND school_id=$2 ORDER BY sort_order,code`, in.SchemeID, schoolID)
	if err != nil {
		return result, err
	}
	var items []model.BillItem
	for itemRows.Next() {
		var code, name string
		var amount int64
		if err := itemRows.Scan(&code, &name, &amount); err != nil {
			itemRows.Close()
			return result, err
		}
		items = append(items, model.BillItem{Name: name, ComponentCode: code, AmountIDR: amount})
	}
	itemRows.Close()
	if err := itemRows.Err(); err != nil {
		return result, err
	}
	if len(items) == 0 {
		return result, &httpError{http.StatusUnprocessableEntity, "scheme has no bill components"}
	}
	base, err := model.TotalBase(items)
	if err != nil {
		return result, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}

	rules, err := a.loadDiscountRules(ctx, schoolID)
	if err != nil {
		return result, err
	}

	// Existing bills for this period+scheme make generation idempotent.
	existing, err := a.existingBillKeys(ctx, schoolID, in.SchemeID, in.PeriodYear, in.PeriodMonth, students)
	if err != nil {
		return result, err
	}
	// Exclude exited students (FR-78-005: hentikan penerbitan tagihan berikutnya)
	exitedStudents := map[string]bool{}
	if exitRows, err := a.DB.QueryContext(ctx, `
		SELECT student_user_id::text FROM student_billing_exits
		WHERE school_id=$1 AND effective_date <= make_date($2, $3, 1)
	`, schoolID, in.PeriodYear, in.PeriodMonth); err == nil {
		defer exitRows.Close()
		for exitRows.Next() {
			var sid string
			if err := exitRows.Scan(&sid); err == nil {
				exitedStudents[sid] = true
			}
		}
	}

	candidates := make([]model.BillCandidate, 0, len(students))
	for _, s := range students {
		if exitedStudents[s] {
			continue
		}
		candidates = append(candidates, model.BillCandidate{Key: model.BillKey{
			StudentUserID: s, Year: in.PeriodYear, Month: in.PeriodMonth, SchemeID: in.SchemeID,
		}})
	}
	plan, err := model.PlanGeneration(existing, candidates)
	if err != nil {
		return result, &httpError{http.StatusUnprocessableEntity, err.Error()}
	}
	result.Skipped = len(students) - len(plan)

	dueAt := model.DueDate(in.PeriodYear, in.PeriodMonth, dueDay)

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	for _, c := range plan {
		studentID := c.Key.StudentUserID
		net, steps, err := model.ApplyDiscounts(base, rulesForStudent(rules, in.SchemeID, gradeLevel, studentID))
		if err != nil {
			return result, &httpError{http.StatusUnprocessableEntity, err.Error()}
		}

		var billID string
		err = tx.QueryRowContext(ctx,
			`INSERT INTO student_bills(school_id,student_user_id,scheme_id,period_year,period_month,base_idr,discount_idr,net_idr,due_at,discount_snapshot,status)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'unpaid')
			 ON CONFLICT(school_id,student_user_id,period_year,period_month,scheme_id) DO NOTHING
			 RETURNING id`,
			schoolID, studentID, in.SchemeID, in.PeriodYear, in.PeriodMonth, base, base-net, net, dueAt, jsonRaw(steps)).Scan(&billID)
		if err == sql.ErrNoRows {
			// Lost a race with a concurrent run: the bill already exists.
			result.Skipped++
			continue
		}
		if err != nil {
			return result, err
		}
		for _, it := range items {
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO bill_items(school_id,bill_id,name,component_code,amount_idr,discount_idr,net_idr)
				 VALUES($1,$2,$3,NULLIF($4,''),$5,0,$5)`,
				schoolID, billID, it.Name, it.ComponentCode, it.AmountIDR); err != nil {
				return result, err
			}
		}
		for i, step := range steps {
			if _, err = tx.ExecContext(ctx,
				`INSERT INTO bill_discount_steps(school_id,bill_id,discount_id,sequence,source,label,priority,base_before_idr,requested_idr,applied_idr,base_after_idr)
				 VALUES($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,$7,$8,$9,$10,$11)`,
				schoolID, billID, step.ID, i+1, step.Source, step.Label, step.Priority,
				step.BaseBefore, step.Requested, step.Applied, step.BaseAfter); err != nil {
				return result, err
			}
		}
		result.Generated++
		result.BillIDs = append(result.BillIDs, billID)
	}
	if err = tx.Commit(); err != nil {
		return result, err
	}

	a.audit(ctx, schoolID, "bill", in.SchemeID, "bills_generated", "admin", "", "",
		"", nil, map[string]any{
			"period_year": in.PeriodYear, "period_month": in.PeriodMonth,
			"generated": result.Generated, "skipped": result.Skipped,
		})
	return result, nil
}

func (a *App) existingBillKeys(ctx context.Context, schoolID, schemeID string, year, month int, students []string) ([]model.BillKey, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT student_user_id::text FROM student_bills
		 WHERE school_id=$1 AND scheme_id=$2 AND period_year=$3 AND period_month=$4
		   AND student_user_id::text = ANY($5::text[])`,
		schoolID, schemeID, year, month, students)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.BillKey
	for rows.Next() {
		var studentID string
		if err := rows.Scan(&studentID); err != nil {
			return nil, err
		}
		out = append(out, model.BillKey{StudentUserID: studentID, Year: year, Month: month, SchemeID: schemeID})
	}
	return out, rows.Err()
}

// normalizeStudentIDs merges the array and single-student forms, trims and
// de-duplicates them while preserving order.
func normalizeStudentIDs(ids []string, single string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids)+1)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, id := range ids {
		add(id)
	}
	add(single)
	return out
}
