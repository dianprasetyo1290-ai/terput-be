package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"skci/hr/internal/client"
	"skci/hr/internal/model"
)

type JobHandlers struct {
	client *client.Client
}

func NewJobHandlers(c *client.Client) *JobHandlers {
	return &JobHandlers{client: c}
}

// ContractReminders checks for employment contracts expiring within reminder days (AC-83-01).
func (h *JobHandlers) ContractReminders(ctx context.Context, db *sql.DB) error {
	today := time.Now().Format("2006-01-02")

	rows, err := db.QueryContext(ctx, `
		SELECT ea.id, ea.school_id, ea.employee_id, e.full_name, ea.contract_end,
		       COALESCE(s.contract_reminder_days, 30)
		FROM employment_assignments ea
		JOIN employees e ON ea.employee_id = e.id
		LEFT JOIN hr_settings s ON ea.school_id = s.school_id
		WHERE e.status = 'active'
		  AND ea.employment_type = 'kontrak'
		  AND ea.contract_end IS NOT NULL
		  AND ea.effective_from <= $1
		  AND (ea.effective_to IS NULL OR ea.effective_to >= $1)
		  AND ea.contract_end >= $1
		  AND ea.contract_end <= ($1::date + COALESCE(s.contract_reminder_days, 30))
	`, today)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			assignID     string
			schoolID     string
			empID        string
			fullName     string
			contractEnd  string
			reminderDays int
		)
		if err := rows.Scan(&assignID, &schoolID, &empID, &fullName, &contractEnd, &reminderDays); err != nil {
			return err
		}

		idempotencyKey := fmt.Sprintf("hr:contract_reminder:%s:%s", assignID, contractEnd)
		if h.client != nil {
			_ = h.client.SendNotification(ctx, client.NotificationPayload{
				SchoolID: schoolID,
				UserID:   "", // Broadcast to HR
				Title:    "Pengingat Kontrak Berakhir",
				Body:     fmt.Sprintf("Kontrak pegawai %s akan berakhir pada %s", fullName, contractEnd),
				Type:     "hr_contract_reminder",
				Data: map[string]any{
					"employee_id":   empID,
					"contract_end":  contractEnd,
					"assignment_id": assignID,
				},
			}, idempotencyKey)
		}
	}
	return rows.Err()
}

// DocumentReminders checks for employee documents expiring within reminder days (AC-85-01).
func (h *JobHandlers) DocumentReminders(ctx context.Context, db *sql.DB) error {
	today := time.Now().Format("2006-01-02")

	rows, err := db.QueryContext(ctx, `
		SELECT ed.id, ed.school_id, ed.employee_id, e.full_name, e.user_id, dt.name, ed.expires_on,
		       COALESCE(s.document_reminder_days, 30)
		FROM employee_documents ed
		JOIN employees e ON ed.employee_id = e.id
		JOIN document_types dt ON ed.document_type_id = dt.id
		LEFT JOIN hr_settings s ON ed.school_id = s.school_id
		WHERE e.status = 'active'
		  AND ed.superseded_at IS NULL
		  AND ed.expires_on IS NOT NULL
		  AND ed.expires_on >= $1
		  AND ed.expires_on <= ($1::date + COALESCE(s.document_reminder_days, 30))
	`, today)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			docID        string
			schoolID     string
			empID        string
			fullName     string
			userID       sql.NullString
			docName      string
			expiresOn    string
			reminderDays int
		)
		if err := rows.Scan(&docID, &schoolID, &empID, &fullName, &userID, &docName, &expiresOn, &reminderDays); err != nil {
			return err
		}

		idempotencyKey := fmt.Sprintf("hr:doc_reminder:%s:%s", docID, expiresOn)
		if h.client != nil {
			// Notify employee if user_id exists
			if userID.Valid && userID.String != "" {
				_ = h.client.SendNotification(ctx, client.NotificationPayload{
					SchoolID: schoolID,
					UserID:   userID.String,
					Title:    "Pengingat Dokumen Kedaluwarsa",
					Body:     fmt.Sprintf("Dokumen %s Anda akan kedaluwarsa pada %s", docName, expiresOn),
					Type:     "doc_expiry_reminder",
					Data: map[string]any{
						"document_id": docID,
						"expires_on":  expiresOn,
					},
				}, idempotencyKey+":emp")
			}
			// Notify HR
			_ = h.client.SendNotification(ctx, client.NotificationPayload{
				SchoolID: schoolID,
				UserID:   "",
				Title:    "Pengingat Dokumen Pegawai",
				Body:     fmt.Sprintf("Dokumen %s milik %s akan kedaluwarsa pada %s", docName, fullName, expiresOn),
				Type:     "doc_expiry_reminder_hr",
				Data: map[string]any{
					"employee_id": empID,
					"document_id": docID,
					"expires_on":  expiresOn,
				},
			}, idempotencyKey+":hr")
		}
	}
	return rows.Err()
}

// ScheduledOffboarding updates employee status to 'exited' when exit_date <= today (AC-83-03).
func (h *JobHandlers) ScheduledOffboarding(ctx context.Context, db *sql.DB) error {
	today := time.Now().Format("2006-01-02")

	rows, err := db.QueryContext(ctx, `
		SELECT id, school_id, user_id, exit_date
		FROM employees
		WHERE status = 'active'
		  AND exit_date IS NOT NULL
		  AND exit_date <= $1
	`, today)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			empID    string
			schoolID string
			userID   sql.NullString
			exitDate string
		)
		if err := rows.Scan(&empID, &schoolID, &userID, &exitDate); err != nil {
			return err
		}

		_, _ = db.ExecContext(ctx, `UPDATE employees SET status = 'exited', updated_at = now() WHERE id = $1`, empID)

		if userID.Valid && userID.String != "" && h.client != nil {
			now := time.Now()
			_ = h.client.DeactivateUser(ctx, userID.String, schoolID, now)
		}
	}
	return rows.Err()
}

// MonthlyAccrual processes monthly leave accrual for leave types with accrual = 'monthly'.
func (h *JobHandlers) MonthlyAccrual(ctx context.Context, db *sql.DB) error {
	now := time.Now()
	currentYear := now.Year()
	month := int(now.Month())

	rows, err := db.QueryContext(ctx, `
		SELECT lt.id, lt.school_id, lt.annual_quota_days, e.id
		FROM leave_types lt
		JOIN employees e ON lt.school_id = e.school_id
		WHERE lt.accrual = 'monthly'
		  AND lt.deleted_at IS NULL
		  AND e.status = 'active'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			leaveTypeID string
			schoolID    string
			annualQuota sql.NullFloat64
			empID       string
		)
		if err := rows.Scan(&leaveTypeID, &schoolID, &annualQuota, &empID); err != nil {
			return err
		}
		if !annualQuota.Valid || annualQuota.Float64 <= 0 {
			continue
		}

		// Calculate total entitled up to current month
		entitled := model.Accrue(annualQuota.Float64, month)

		_, _ = db.ExecContext(ctx, `
			INSERT INTO leave_balances (school_id, employee_id, leave_type_id, year, entitled, carried, used)
			VALUES ($1, $2, $3, $4, $5, 0, 0)
			ON CONFLICT (employee_id, leave_type_id, year)
			DO UPDATE SET entitled = GREATEST(leave_balances.entitled, EXCLUDED.entitled), updated_at = now()
		`, schoolID, empID, leaveTypeID, currentYear, entitled)
	}
	return rows.Err()
}
