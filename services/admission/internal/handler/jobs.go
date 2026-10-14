package handler

import (
	"context"
	"database/sql"
	"log"
	"time"

	"skci/admission/internal/model"
)

// RunJobs runs the PPDB background jobs every interval until ctx ends. Each
// job is idempotent (row locks + dedupe keys), so running several replicas
// or re-running after a crash is safe.
func (a *App) RunJobs(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		a.runJobsOnce(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *App) runJobsOnce(ctx context.Context, now time.Time) {
	jobs := []struct {
		name string
		run  func(context.Context, time.Time) error
	}{
		{"drop_unpaid_forms", a.dropUnpaidForms},
		{"drop_unpaid_reregistration", a.dropUnpaidReregistration},
		{"publish_announcements", a.publishDueAnnouncements},
		{"notification_emails", func(ctx context.Context, _ time.Time) error { return a.deliverNotificationEmails(ctx) }},
	}
	for _, j := range jobs {
		if err := j.run(ctx, now); err != nil {
			log.Printf("admission job %s: %v", j.name, err)
		}
	}
}

// overdue lists applications in the given statuses whose stage deadline has
// passed and whose stage charge is still active.
func (a *App) overdue(ctx context.Context, now time.Time, stage string, statuses []string) ([][2]string, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT a.school_id::text,a.id::text FROM applications a
		 JOIN LATERAL (SELECT due_at,cancelled_at FROM charges c WHERE c.application_id=a.id AND c.stage=$1
		               ORDER BY revision DESC LIMIT 1) c ON true
		 WHERE a.status=ANY($2) AND c.cancelled_at IS NULL
		   AND COALESCE(CASE WHEN $1='reregistration' THEN a.reregistration_due_at END,c.due_at)<$3
		 ORDER BY a.id LIMIT 200`, stage, statuses, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var school, id string
		if err := rows.Scan(&school, &id); err != nil {
			return nil, err
		}
		out = append(out, [2]string{school, id})
	}
	return out, rows.Err()
}

// dropUnpaidForms applies BR-PPDB-07: a submitted application whose form fee
// is still unpaid after the payment deadline is dropped (gugur), unless a
// proof transferred on time is waiting for review.
func (a *App) dropUnpaidForms(ctx context.Context, now time.Time) error {
	list, err := a.overdue(ctx, now, stageRegistration, []string{string(model.StatusSubmitted)})
	if err != nil {
		return err
	}
	for _, it := range list {
		if err := a.dropIfOverdue(ctx, it[0], it[1], stageRegistration, now); err != nil {
			log.Printf("admission: drop form %s: %v", it[1], err)
		}
	}
	return nil
}

// dropUnpaidReregistration applies BR-PPDB-08: an offered applicant who has
// not paid the re-registration minimum by the deadline is dropped and the
// seat is released for the waitlist. Amounts already paid become a refund
// record under the period refund policy (see refunds.go).
func (a *App) dropUnpaidReregistration(ctx context.Context, now time.Time) error {
	list, err := a.overdue(ctx, now, stageReregistration,
		[]string{string(model.StatusOffered), string(model.StatusReregistration)})
	if err != nil {
		return err
	}
	for _, it := range list {
		if err := a.dropIfOverdue(ctx, it[0], it[1], stageReregistration, now); err != nil {
			log.Printf("admission: drop reregistration %s: %v", it[1], err)
		}
	}
	return nil
}

func (a *App) dropIfOverdue(ctx context.Context, schoolID, applicationID, stage string, now time.Time) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var (
		status string
		due    sql.NullTime
	)
	if err := tx.QueryRowContext(ctx,
		`SELECT status,reregistration_due_at FROM applications WHERE id=$1 AND school_id=$2 FOR UPDATE`,
		applicationID, schoolID).Scan(&status, &due); err != nil {
		return err
	}
	charge, err := latestCharge(ctx, tx, schoolID, applicationID, stage)
	if err != nil || charge == nil || charge.Cancelled.Valid {
		return err
	}
	deadline := charge.DueAt
	if stage == stageReregistration && due.Valid {
		deadline = due
	}
	if !deadline.Valid {
		return nil
	}
	paid, err := stagePaid(ctx, tx, schoolID, applicationID, stage)
	if err != nil {
		return err
	}
	onTime, err := pendingOnTime(ctx, tx, schoolID, applicationID, stage, deadline.Time)
	if err != nil {
		return err
	}
	var drop bool
	reason := "Biaya formulir tidak dibayar sampai batas waktu (BR-PPDB-07)"
	if stage == stageRegistration {
		drop = status == string(model.StatusSubmitted) && model.ShouldDropUnpaidForm(now, &deadline.Time, paid >= charge.MinPayment, onTime)
	} else {
		reason = "Pembayaran minimum daftar ulang tidak terpenuhi sampai batas waktu (BR-PPDB-08)"
		drop = (status == string(model.StatusOffered) || status == string(model.StatusReregistration)) &&
			model.ShouldDropReregistration(now, &deadline.Time, paid, charge.MinPayment, onTime)
	}
	if !drop {
		return nil
	}
	if _, err := a.transitionTx(ctx, tx, schoolID, applicationID, "system", "admission", "Sistem PPDB",
		model.StatusDropped, reason, 0, ""); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE charges SET cancelled_at=$2,cancel_reason=$3 WHERE id=$1`, charge.ID, now, reason); err != nil {
		return err
	}
	refundable, err := recordRefundDue(ctx, tx, schoolID, applicationID, stage, paid)
	if err != nil {
		return err
	}
	body := "Pendaftaran dinyatakan gugur: " + reason + "."
	if stage == stageReregistration {
		body += " Kursi telah dilepas untuk pendaftar cadangan."
	}
	if paid > 0 {
		if refundable > 0 {
			body += " Sesuai kebijakan sekolah, dana sebesar " + formatIDR(refundable) + " dari " + formatIDR(paid) +
				" yang sudah dibayar akan dikembalikan oleh panitia."
		} else {
			body += " Sesuai kebijakan sekolah, dana yang sudah dibayar (" + formatIDR(paid) + ") tidak dapat dikembalikan."
		}
	}
	if err := a.notify(ctx, tx, schoolID, applicationID, "dropped", "Pendaftaran gugur", body, "dropped:"+applicationID, nil); err != nil {
		return err
	}
	return tx.Commit()
}
