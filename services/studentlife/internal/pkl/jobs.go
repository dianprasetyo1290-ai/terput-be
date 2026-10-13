package pkl

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"skci/studentlife/internal/client"
)

// DailyJob executes daily PKL maintenance tasks (§4.4):
// 1. Transition placements planned -> active when starts_on <= today, active -> completed when ends_on < today
// 2. Send daily reminder notification to industry mentors with unconfirmed attendances from yesterday
func DailyJob(ctx context.Context, db *sql.DB, cl *client.Client) error {
	now := time.Now()
	today := now.Format("2006-01-02")

	// 1a. Planned -> Active
	resActive, err := db.ExecContext(ctx, `
		UPDATE pkl_placements
		SET status = 'active', updated_at = $1
		WHERE status = 'planned' AND starts_on <= $2`, now, today)
	if err != nil {
		log.Printf("pkl_daily: error transitioning planned to active: %v", err)
	} else if n, _ := resActive.RowsAffected(); n > 0 {
		log.Printf("pkl_daily: transitioned %d placements from planned to active", n)
	}

	// 1b. Active -> Completed
	resComp, err := db.ExecContext(ctx, `
		UPDATE pkl_placements
		SET status = 'completed', updated_at = $1
		WHERE status = 'active' AND ends_on < $2`, now, today)
	if err != nil {
		log.Printf("pkl_daily: error transitioning active to completed: %v", err)
	} else if n, _ := resComp.RowsAffected(); n > 0 {
		log.Printf("pkl_daily: transitioned %d placements from active to completed", n)
	}

	// 2. Attendance confirmation reminders to industry mentors (§4.2)
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	rows, err := db.QueryContext(ctx, `
		SELECT im.user_id, att.school_id, COUNT(att.id)
		FROM pkl_attendance att
		JOIN pkl_placements pl ON att.placement_id = pl.id
		JOIN industry_mentors im ON pl.industry_mentor_id = im.id
		WHERE att.work_date = $1 AND att.confirmation = 'pending' AND im.active = true
		GROUP BY im.user_id, att.school_id`, yesterday)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var mentorUserID, schoolID string
			var count int
			if err := rows.Scan(&mentorUserID, &schoolID, &count); err == nil && cl != nil {
				key := fmt.Sprintf("studentlife:pkl_confirm:%s:%s", mentorUserID, yesterday)
				_ = cl.Notify(ctx, client.Notification{
					SchoolID: schoolID,
					UserID:   mentorUserID,
					Type:     "pkl_attendance_confirm",
					Title:    "Konfirmasi Presensi PKL",
					Body:     fmt.Sprintf("Ada %d presensi siswa PKL kemarin yang menunggu konfirmasi Anda.", count),
					Data:     map[string]any{"pending_count": count, "work_date": yesterday},
				}, key)
			}
		}
	}

	return nil
}

// DailyJob on Handler implements the runner.Job signature.
func (h *Handler) DailyJob(ctx context.Context, db *sql.DB) error {
	return DailyJob(ctx, db, h.Client)
}

