package activity

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// DailyJob executes maintenance tasks:
// 1. Cancels pending_consent registrations past event registration_closes_at (freeing seats).
// 2. Marks events past ends_at as 'completed'.
func DailyJob(ctx context.Context, db *sql.DB) error {
	now := time.Now()

	// 1. Cancel expired pending_consent registrations
	resReg, err := db.ExecContext(ctx, `
		UPDATE act_event_registrations r
		SET status = 'cancelled', updated_at = $1
		FROM act_events e
		WHERE r.event_id = e.id
		  AND r.status = 'pending_consent'
		  AND e.registration_closes_at IS NOT NULL
		  AND e.registration_closes_at < $1`, now)
	if err != nil {
		log.Printf("activity_daily: error cancelling expired registrations: %v", err)
	} else if n, _ := resReg.RowsAffected(); n > 0 {
		log.Printf("activity_daily: cancelled %d expired pending registrations", n)
	}

	// 2. Mark past published events as completed
	resEvt, err := db.ExecContext(ctx, `
		UPDATE act_events
		SET status = 'completed', updated_at = $1
		WHERE status = 'published' AND ends_at < $1 AND deleted_at IS NULL`, now)
	if err != nil {
		log.Printf("activity_daily: error completing past events: %v", err)
	} else if n, _ := resEvt.RowsAffected(); n > 0 {
		log.Printf("activity_daily: marked %d past events as completed", n)
	}

	return nil
}
