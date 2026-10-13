package counseling

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"skci/studentlife/internal/client"
)

// JobSessionReminders sends session reminders according to school settings (FR-130-003).
// Never leaks notes or sensitive details in notification body (NFR-OPS-03).
func (h *Handler) JobSessionReminders(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.school_id, s.scheduled_at, s.counselor_user_id,
		       COALESCE(cs.reminder_days_before, 0)
		FROM counseling_sessions s
		LEFT JOIN counseling_settings cs ON s.school_id = cs.school_id
		WHERE s.status = 'scheduled' AND s.reminder_sent_at IS NULL
		LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var id, schoolID, counselorID string
		var scheduledAt time.Time
		var reminderDaysBefore int

		if err := rows.Scan(&id, &schoolID, &scheduledAt, &counselorID, &reminderDaysBefore); err != nil {
			continue
		}

		if ReminderDue(scheduledAt, reminderDaysBefore, nil, SessionStatusScheduled, now, time.Local) {
			// Send notification
			timeStr := scheduledAt.Format("15:04 WIB")
			dateStr := scheduledAt.Format("02 Jan 2006")
			_ = h.Client.Notify(ctx, client.Notification{
				SchoolID: schoolID,
				UserID:   counselorID,
				Type:     "counseling_session_reminder",
				Title:    "Pengingat Sesi Konseling",
				Body:     fmt.Sprintf("Anda memiliki jadwal sesi konseling pada %s pukul %s.", dateStr, timeStr),
				Data:     map[string]any{"session_id": id},
			}, fmt.Sprintf("studentlife:session:%s:%s", id, now.Format("2006-01-02")))

			_, _ = db.ExecContext(ctx, `
				UPDATE counseling_sessions
				SET reminder_sent_at = now()
				WHERE id = $1`, id)
		}
	}
	return nil
}

// JobTaskReminders sends reminders to staff assignees for upcoming or overdue follow-up tasks (FR-132-002).
func (h *Handler) JobTaskReminders(ctx context.Context, db *sql.DB) error {
	today := time.Now().Format("2006-01-02")
	rows, err := db.QueryContext(ctx, `
		SELECT id, school_id, title, assignee_user_id, due_date::text
		FROM case_tasks
		WHERE done_at IS NULL AND due_date <= $1::date
		LIMIT 100`, today)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, schoolID, title, assigneeID, dueDate string
		if err := rows.Scan(&id, &schoolID, &title, &assigneeID, &dueDate); err != nil {
			continue
		}

		statusLabel := "jatuh tempo hari ini"
		if dueDate < today {
			statusLabel = "telah melewati tenggat waktu"
		}

		_ = h.Client.Notify(ctx, client.Notification{
			SchoolID: schoolID,
			UserID:   assigneeID,
			Type:     "counseling_task_reminder",
			Title:    "Pengingat Tindak Lanjut Konseling",
			Body:     fmt.Sprintf("Tugas '%s' %s (%s).", title, statusLabel, dueDate),
			Data:     map[string]any{"task_id": id, "due_date": dueDate},
		}, fmt.Sprintf("studentlife:task:%s:%s", id, today))
	}
	return nil
}
