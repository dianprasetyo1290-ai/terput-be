package store

import (
	"context"
	"database/sql"
	"time"

	"skci/hr/internal/model"
)

func GetESSDashboard(ctx context.Context, q Q, schoolID, empID string) (*model.ESSDashboard, error) {
	now := time.Now()
	today := now.Format("2006-01-02")
	currentYear := now.Year()

	dashboard := &model.ESSDashboard{
		ServerTime: now,
	}

	// 1. Annual leave balance
	_ = q.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(lb.entitled + lb.carried - lb.used), 0)
		FROM leave_balances lb
		JOIN leave_types lt ON lt.id = lb.leave_type_id
		WHERE lb.school_id = $1 AND lb.employee_id = $2 AND lb.year = $3 AND lt.has_quota
	`, schoolID, empID, currentYear).Scan(&dashboard.LeaveBalanceAnnual)

	// 2. Pending requests count (leave + overtime + profile change)
	var (
		pendingLeave, pendingOT, pendingProfile int
	)
	_ = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM leave_requests WHERE school_id = $1 AND employee_id = $2 AND status = 'pending'`, schoolID, empID).Scan(&pendingLeave)
	_ = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM overtime_requests WHERE school_id = $1 AND employee_id = $2 AND status = 'pending'`, schoolID, empID).Scan(&pendingOT)
	_ = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_change_requests WHERE school_id = $1 AND employee_id = $2 AND status = 'pending'`, schoolID, empID).Scan(&pendingProfile)
	dashboard.PendingRequests = pendingLeave + pendingOT + pendingProfile

	// 3. Today's attendance
	dashboard.TodayAttendance, _ = GetAttendanceByEmployeeAndDate(ctx, q, schoolID, empID, today)

	return dashboard, nil
}

// --- Profile Change Requests ---

func ListProfileChangeRequests(ctx context.Context, q Q, schoolID, empID string) ([]model.ProfileChangeRequest, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, school_id, employee_id, field, status,
		approval_id::text, applied_at, created_at, updated_at
		FROM profile_change_requests
		WHERE school_id = $1 AND employee_id = $2
		ORDER BY created_at DESC`, schoolID, empID)
	return collect(rows, err, func(r *sqlRows) (model.ProfileChangeRequest, error) {
		var pcr model.ProfileChangeRequest
		err := r.Scan(&pcr.ID, &pcr.SchoolID, &pcr.EmployeeID, &pcr.Field, &pcr.Status,
			&pcr.ApprovalID, &pcr.AppliedAt, &pcr.CreatedAt, &pcr.UpdatedAt)
		return pcr, err
	})
}

func CreateProfileChangeRequest(ctx context.Context, q Q, pcr *model.ProfileChangeRequest) error {
	if pcr.Status == "" {
		pcr.Status = "pending"
	}
	return q.QueryRowContext(ctx, `INSERT INTO profile_change_requests (school_id, employee_id, field, new_value_enc, status)
		VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at, updated_at`,
		pcr.SchoolID, pcr.EmployeeID, pcr.Field, pcr.NewValueEnc, pcr.Status).Scan(&pcr.ID, &pcr.CreatedAt, &pcr.UpdatedAt)
}

func ApplyProfileChangeRequestTx(ctx context.Context, db *sql.DB, id, decision string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		schoolID, empID, field, status string
		newValueEnc                    []byte
	)
	err = tx.QueryRowContext(ctx, `SELECT school_id, employee_id, field, new_value_enc, status
		FROM profile_change_requests WHERE id = $1 FOR UPDATE`, id).
		Scan(&schoolID, &empID, &field, &newValueEnc, &status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// Idempotent: already decided
	if status == decision {
		return nil
	}

	if decision == "approved" {
		switch field {
		case "address":
			_, err = tx.ExecContext(ctx, `UPDATE employees SET address_enc = $2, updated_at = now() WHERE id = $1`, empID, newValueEnc)
		case "phone":
			_, err = tx.ExecContext(ctx, `UPDATE employees SET phone_enc = $2, updated_at = now() WHERE id = $1`, empID, newValueEnc)
		case "bank_account":
			_, err = tx.ExecContext(ctx, `UPDATE employees SET bank_account_enc = $2, updated_at = now() WHERE id = $1`, empID, newValueEnc)
		case "ptkp_status":
			// PTKP is plaintext code
			_, err = tx.ExecContext(ctx, `UPDATE employees SET ptkp_status = $2, updated_at = now() WHERE id = $1`, empID, string(newValueEnc))
		}
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `UPDATE profile_change_requests SET status = 'approved', applied_at = now(), updated_at = now() WHERE id = $1`, id)
		if err != nil {
			return err
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE profile_change_requests SET status = 'rejected', updated_at = now() WHERE id = $1`, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
