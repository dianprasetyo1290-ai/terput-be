package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"skci/hr/internal/model"
)

// --- Leave Types ---

func ListLeaveTypes(ctx context.Context, q Q, schoolID string) ([]model.LeaveType, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, school_id, code, name, has_quota, annual_quota_days,
		accrual, carry_over_max_days, requires_document, created_at, updated_at
		FROM leave_types WHERE school_id = $1 AND deleted_at IS NULL ORDER BY code`, schoolID)
	return collect(rows, err, func(r *sqlRows) (model.LeaveType, error) {
		var lt model.LeaveType
		err := r.Scan(&lt.ID, &lt.SchoolID, &lt.Code, &lt.Name, &lt.HasQuota, &lt.AnnualQuotaDays,
			&lt.Accrual, &lt.CarryOverMaxDays, &lt.RequiresDocument, &lt.CreatedAt, &lt.UpdatedAt)
		return lt, err
	})
}

func GetLeaveType(ctx context.Context, q Q, schoolID, id string) (*model.LeaveType, error) {
	var lt model.LeaveType
	err := q.QueryRowContext(ctx, `SELECT id, school_id, code, name, has_quota, annual_quota_days,
		accrual, carry_over_max_days, requires_document, created_at, updated_at
		FROM leave_types WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL`, schoolID, id).
		Scan(&lt.ID, &lt.SchoolID, &lt.Code, &lt.Name, &lt.HasQuota, &lt.AnnualQuotaDays,
			&lt.Accrual, &lt.CarryOverMaxDays, &lt.RequiresDocument, &lt.CreatedAt, &lt.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &lt, nil
}

func CreateLeaveType(ctx context.Context, q Q, lt *model.LeaveType) error {
	return q.QueryRowContext(ctx, `INSERT INTO leave_types (school_id, code, name, has_quota,
		annual_quota_days, accrual, carry_over_max_days, requires_document)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at, updated_at`,
		lt.SchoolID, lt.Code, lt.Name, lt.HasQuota, lt.AnnualQuotaDays, lt.Accrual,
		lt.CarryOverMaxDays, lt.RequiresDocument).Scan(&lt.ID, &lt.CreatedAt, &lt.UpdatedAt)
}

func UpdateLeaveType(ctx context.Context, q Q, lt *model.LeaveType) error {
	return affected(q.ExecContext(ctx, `UPDATE leave_types SET code=$3, name=$4, has_quota=$5,
		annual_quota_days=$6, accrual=$7, carry_over_max_days=$8, requires_document=$9, updated_at=now()
		WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`,
		lt.SchoolID, lt.ID, lt.Code, lt.Name, lt.HasQuota, lt.AnnualQuotaDays, lt.Accrual,
		lt.CarryOverMaxDays, lt.RequiresDocument))
}

func DeleteLeaveType(ctx context.Context, q Q, schoolID, id string) error {
	return affected(q.ExecContext(ctx, `UPDATE leave_types SET deleted_at = now() WHERE school_id=$1 AND id=$2 AND deleted_at IS NULL`, schoolID, id))
}

// --- Leave Balances ---

func ListLeaveBalances(ctx context.Context, q Q, schoolID, empID string, year int) ([]model.LeaveBalance, error) {
	if year == 0 {
		year = time.Now().Year()
	}
	where := " WHERE lb.school_id = $1 AND lb.year = $2"
	args := []any{schoolID, year}

	if empID != "" {
		args = append(args, empID)
		where += fmt.Sprintf(" AND lb.employee_id = $%d::uuid", len(args))
	}

	rows, err := q.QueryContext(ctx, `SELECT lb.id, lb.school_id, lb.employee_id, lb.leave_type_id,
		lt.name, lt.code, lb.year, lb.entitled, lb.carried, lb.used, (lb.entitled + lb.carried - lb.used) AS remaining,
		lb.created_at, lb.updated_at
		FROM leave_balances lb
		JOIN leave_types lt ON lt.id = lb.leave_type_id`+where+` ORDER BY lt.code`, args...)
	return collect(rows, err, func(r *sqlRows) (model.LeaveBalance, error) {
		var b model.LeaveBalance
		err := r.Scan(&b.ID, &b.SchoolID, &b.EmployeeID, &b.LeaveTypeID, &b.TypeName, &b.TypeCode,
			&b.Year, &b.Entitled, &b.Carried, &b.Used, &b.Remaining, &b.CreatedAt, &b.UpdatedAt)
		return b, err
	})
}

func GetLeaveBalance(ctx context.Context, q Q, schoolID, empID, leaveTypeID string, year int) (*model.LeaveBalance, error) {
	if year == 0 {
		year = time.Now().Year()
	}
	var b model.LeaveBalance
	err := q.QueryRowContext(ctx, `SELECT lb.id, lb.school_id, lb.employee_id, lb.leave_type_id,
		lt.name, lt.code, lb.year, lb.entitled, lb.carried, lb.used, (lb.entitled + lb.carried - lb.used) AS remaining,
		lb.created_at, lb.updated_at
		FROM leave_balances lb
		JOIN leave_types lt ON lt.id = lb.leave_type_id
		WHERE lb.school_id = $1 AND lb.employee_id = $2 AND lb.leave_type_id = $3 AND lb.year = $4`,
		schoolID, empID, leaveTypeID, year).
		Scan(&b.ID, &b.SchoolID, &b.EmployeeID, &b.LeaveTypeID, &b.TypeName, &b.TypeCode,
			&b.Year, &b.Entitled, &b.Carried, &b.Used, &b.Remaining, &b.CreatedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func EnsureLeaveBalance(ctx context.Context, q Q, schoolID, empID, leaveTypeID string, year int, entitled float64) error {
	_, err := q.ExecContext(ctx, `INSERT INTO leave_balances (school_id, employee_id, leave_type_id, year, entitled, carried, used)
		VALUES ($1,$2,$3,$4,$5,0,0)
		ON CONFLICT (employee_id, leave_type_id, year) DO UPDATE SET entitled = EXCLUDED.entitled, updated_at = now()`,
		schoolID, empID, leaveTypeID, year, entitled)
	return err
}

// --- Leave Requests ---

func ListLeaveRequests(ctx context.Context, q Q, schoolID, empID, status string, page, perPage int) ([]model.LeaveRequest, int, error) {
	where := " WHERE lr.school_id = $1"
	args := []any{schoolID}

	if empID != "" {
		args = append(args, empID)
		where += fmt.Sprintf(" AND lr.employee_id = $%d::uuid", len(args))
	}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND lr.status = $%d", len(args))
	}

	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM leave_requests lr`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sqlText := `SELECT lr.id, lr.school_id, lr.employee_id, e.full_name, lr.leave_type_id, lt.name,
		lr.start_date::text, lr.end_date::text, lr.days, lr.reason, lr.attachment_key, lr.status,
		lr.approval_id::text, lr.idempotency_key, lr.created_at, lr.updated_at
		FROM leave_requests lr
		JOIN employees e ON e.id = lr.employee_id
		JOIN leave_types lt ON lt.id = lr.leave_type_id` + where + ` ORDER BY lr.created_at DESC`

	if perPage > 0 {
		sqlText += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, (page-1)*perPage)
	}

	rows, err := q.QueryContext(ctx, sqlText, args...)
	out, err := collect(rows, err, func(r *sqlRows) (model.LeaveRequest, error) {
		var lr model.LeaveRequest
		err := r.Scan(&lr.ID, &lr.SchoolID, &lr.EmployeeID, &lr.EmployeeName, &lr.LeaveTypeID, &lr.LeaveTypeName,
			&lr.StartDate, &lr.EndDate, &lr.Days, &lr.Reason, &lr.AttachmentKey, &lr.Status,
			&lr.ApprovalID, &lr.IdempotencyKey, &lr.CreatedAt, &lr.UpdatedAt)
		return lr, err
	})
	return out, total, err
}

func GetLeaveRequest(ctx context.Context, q Q, schoolID, id string) (*model.LeaveRequest, error) {
	var lr model.LeaveRequest
	err := q.QueryRowContext(ctx, `SELECT lr.id, lr.school_id, lr.employee_id, e.full_name, lr.leave_type_id, lt.name,
		lr.start_date::text, lr.end_date::text, lr.days, lr.reason, lr.attachment_key, lr.status,
		lr.approval_id::text, lr.idempotency_key, lr.created_at, lr.updated_at
		FROM leave_requests lr
		JOIN employees e ON e.id = lr.employee_id
		JOIN leave_types lt ON lt.id = lr.leave_type_id
		WHERE lr.school_id = $1 AND lr.id = $2`, schoolID, id).
		Scan(&lr.ID, &lr.SchoolID, &lr.EmployeeID, &lr.EmployeeName, &lr.LeaveTypeID, &lr.LeaveTypeName,
			&lr.StartDate, &lr.EndDate, &lr.Days, &lr.Reason, &lr.AttachmentKey, &lr.Status,
			&lr.ApprovalID, &lr.IdempotencyKey, &lr.CreatedAt, &lr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &lr, nil
}

func CreateLeaveRequest(ctx context.Context, q Q, lr *model.LeaveRequest) error {
	if lr.IdempotencyKey == "" {
		lr.IdempotencyKey = fmt.Sprintf("leave:%s:%s", lr.EmployeeID, lr.StartDate)
	}
	if lr.Status == "" {
		lr.Status = "pending"
	}
	return q.QueryRowContext(ctx, `INSERT INTO leave_requests (school_id, employee_id, leave_type_id,
		start_date, end_date, days, reason, attachment_key, status, idempotency_key)
		VALUES ($1,$2,$3,$4::date,$5::date,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at`,
		lr.SchoolID, lr.EmployeeID, lr.LeaveTypeID, lr.StartDate, lr.EndDate, lr.Days,
		lr.Reason, lr.AttachmentKey, lr.Status, lr.IdempotencyKey).Scan(&lr.ID, &lr.CreatedAt, &lr.UpdatedAt)
}

func UpdateLeaveRequestApproval(ctx context.Context, q Q, id, approvalID string) error {
	_, err := q.ExecContext(ctx, `UPDATE leave_requests SET approval_id = NULLIF($2,'')::uuid, updated_at = now() WHERE id = $1`, id, approvalID)
	return err
}

// ApplyLeaveDecisionTx applies approval or rejection of leave request in a transaction (AC-86-02).
func ApplyLeaveDecisionTx(ctx context.Context, db *sql.DB, id, decision string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		schoolID, empID, leaveTypeID string
		startDate, endDate           time.Time
		days                         float64
		currentStatus                string
	)
	err = tx.QueryRowContext(ctx, `SELECT school_id, employee_id, leave_type_id, start_date, end_date, days, status
		FROM leave_requests WHERE id = $1 FOR UPDATE`, id).
		Scan(&schoolID, &empID, &leaveTypeID, &startDate, &endDate, &days, &currentStatus)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// Idempotent: a repeated callback (or a decision on a cancelled request)
	// leaves an already-final request untouched.
	if currentStatus != "pending" {
		return nil
	}

	if decision == "approved" {
		year := startDate.Year()
		_, err = tx.ExecContext(ctx, `UPDATE leave_balances SET used = used + $4, updated_at = now()
			WHERE school_id = $1 AND employee_id = $2 AND leave_type_id = $3 AND year = $5`,
			schoolID, empID, leaveTypeID, days, year)
		if err != nil {
			return err
		}

		// Insert employee_attendance for each working day in the range
		for curr := startDate; !curr.After(endDate); curr = curr.AddDate(0, 0, 1) {
			dateStr := curr.Format("2006-01-02")
			weekday := int(curr.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			// Skip weekend by default (Sat, Sun: 6, 7)
			if weekday == 6 || weekday == 7 {
				continue
			}

			note := "Cuti tahunan/khusus disetujui"
			_, err = tx.ExecContext(ctx, `INSERT INTO employee_attendance (school_id, employee_id, work_date, status, source, note)
				VALUES ($1, $2, $3::date, 'cuti', 'leave', $4)
				ON CONFLICT (employee_id, work_date) DO UPDATE SET status = 'cuti', source = 'leave', note = $4, updated_at = now()`,
				schoolID, empID, dateStr, note)
			if err != nil {
				return err
			}
		}

		_, err = tx.ExecContext(ctx, `UPDATE leave_requests SET status = 'approved', updated_at = now() WHERE id = $1`, id)
		if err != nil {
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE leave_requests SET status = 'rejected', updated_at = now() WHERE id = $1`, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func CancelLeaveRequestTx(ctx context.Context, db *sql.DB, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		schoolID, empID, leaveTypeID string
		startDate, endDate           time.Time
		days                         float64
		status                       string
	)
	err = tx.QueryRowContext(ctx, `SELECT school_id, employee_id, leave_type_id, start_date, end_date, days, status
		FROM leave_requests WHERE id = $1 FOR UPDATE`, id).
		Scan(&schoolID, &empID, &leaveTypeID, &startDate, &endDate, &days, &status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if status == "cancelled" {
		return nil
	}

	if status == "approved" {
		// Restore balance
		year := startDate.Year()
		_, err = tx.ExecContext(ctx, `UPDATE leave_balances SET used = GREATEST(0, used - $4), updated_at = now()
			WHERE school_id = $1 AND employee_id = $2 AND leave_type_id = $3 AND year = $5`,
			schoolID, empID, leaveTypeID, days, year)
		if err != nil {
			return err
		}

		// Delete attendance records created by this leave
		_, err = tx.ExecContext(ctx, `DELETE FROM employee_attendance
			WHERE employee_id = $1 AND work_date BETWEEN $2::date AND $3::date AND status = 'cuti' AND source = 'leave'`,
			empID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `UPDATE leave_requests SET status = 'cancelled', updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// --- Overtime Requests ---

func ListOvertimeRequests(ctx context.Context, q Q, schoolID, empID, status string, page, perPage int) ([]model.OvertimeRequest, int, error) {
	where := " WHERE ot.school_id = $1"
	args := []any{schoolID}

	if empID != "" {
		args = append(args, empID)
		where += fmt.Sprintf(" AND ot.employee_id = $%d::uuid", len(args))
	}
	if status != "" {
		args = append(args, status)
		where += fmt.Sprintf(" AND ot.status = $%d", len(args))
	}

	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM overtime_requests ot`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sqlText := `SELECT ot.id, ot.school_id, ot.employee_id, e.full_name, ot.work_date::text,
		ot.start_at, ot.end_at, ot.minutes, ot.reason, ot.status, ot.approval_id::text,
		ot.idempotency_key, ot.created_at, ot.updated_at
		FROM overtime_requests ot
		JOIN employees e ON e.id = ot.employee_id` + where + ` ORDER BY ot.created_at DESC`

	if perPage > 0 {
		sqlText += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, (page-1)*perPage)
	}

	rows, err := q.QueryContext(ctx, sqlText, args...)
	out, err := collect(rows, err, func(r *sqlRows) (model.OvertimeRequest, error) {
		var ot model.OvertimeRequest
		err := r.Scan(&ot.ID, &ot.SchoolID, &ot.EmployeeID, &ot.EmployeeName, &ot.WorkDate,
			&ot.StartAt, &ot.EndAt, &ot.Minutes, &ot.Reason, &ot.Status, &ot.ApprovalID,
			&ot.IdempotencyKey, &ot.CreatedAt, &ot.UpdatedAt)
		return ot, err
	})
	return out, total, err
}

func CreateOvertimeRequest(ctx context.Context, q Q, ot *model.OvertimeRequest) error {
	if ot.IdempotencyKey == "" {
		ot.IdempotencyKey = fmt.Sprintf("ot:%s:%s", ot.EmployeeID, ot.WorkDate)
	}
	if ot.Status == "" {
		ot.Status = "pending"
	}
	return q.QueryRowContext(ctx, `INSERT INTO overtime_requests (school_id, employee_id, work_date,
		start_at, end_at, minutes, reason, status, idempotency_key)
		VALUES ($1,$2,$3::date,$4,$5,$6,$7,$8,$9) RETURNING id, created_at, updated_at`,
		ot.SchoolID, ot.EmployeeID, ot.WorkDate, ot.StartAt, ot.EndAt, ot.Minutes,
		ot.Reason, ot.Status, ot.IdempotencyKey).Scan(&ot.ID, &ot.CreatedAt, &ot.UpdatedAt)
}

func UpdateOvertimeRequestApproval(ctx context.Context, q Q, id, approvalID string) error {
	_, err := q.ExecContext(ctx, `UPDATE overtime_requests SET approval_id = NULLIF($2,'')::uuid, updated_at = now() WHERE id = $1`, id, approvalID)
	return err
}

// ApplyOvertimeDecisionTx records the approval outcome once; a repeated
// callback on an already-final request is a no-op.
func ApplyOvertimeDecisionTx(ctx context.Context, db *sql.DB, id, decision string) error {
	res, err := db.ExecContext(ctx, `UPDATE overtime_requests SET status = $2, updated_at = now() WHERE id = $1 AND status = 'pending'`, id, decision)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM overtime_requests WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
