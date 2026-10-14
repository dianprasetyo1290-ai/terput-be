package handler

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strconv"
	"time"

	"skci/admission/internal/common"
)

// PPDB guardians are not identity users, so the shared notification service
// (which addresses identity user ids) cannot reach them. Admission keeps its
// own guardian inbox + email outbox: every notification is stored once
// (dedupe_key UNIQUE per school, so re-running a job never notifies twice),
// shown in the guardian portal and, when SMTP is configured, emailed by the
// background job.

// notify enqueues one guardian notification for an application.
func (a *App) notify(ctx context.Context, q queryer, schoolID, applicationID, kind, title, body, dedupeKey string, data map[string]any) error {
	emailStatus := "skipped"
	if a.Mailer != nil {
		emailStatus = "pending"
	}
	if data == nil {
		data = map[string]any{}
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO guardian_notifications(school_id,guardian_id,application_id,type,title,body,data,dedupe_key,email_status)
		 SELECT a.school_id,ap.guardian_id,a.id,$3,$4,$5,$6,$7,$8
		 FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
		 WHERE a.id=$1 AND a.school_id=$2
		 ON CONFLICT (school_id,dedupe_key) DO NOTHING`,
		applicationID, schoolID, kind, title, body, jsonRaw(data), dedupeKey, emailStatus)
	return err
}

// formatIDR renders an amount as "Rp1.500.000".
func formatIDR(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	digits := strconv.FormatInt(v, 10)
	out := make([]byte, 0, len(digits)+len(digits)/3)
	for i := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, digits[i])
	}
	if neg {
		return "-Rp" + string(out)
	}
	return "Rp" + string(out)
}

// ListGuardianNotifications is GET /guardian/notifications.
func (a *App) ListGuardianNotifications(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id,COALESCE(application_id::text,''),type,title,body,read_at,created_at
		 FROM guardian_notifications WHERE school_id=$1 AND guardian_id=$2 ORDER BY created_at DESC LIMIT 100`,
		c.SchoolID, c.GuardianID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	unread := 0
	for rows.Next() {
		var (
			id, applicationID, kind, title, body string
			readAt                               sql.NullTime
			createdAt                            time.Time
		)
		if err := rows.Scan(&id, &applicationID, &kind, &title, &body, &readAt, &createdAt); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{"id": id, "type": kind, "title": title, "body": body, "created_at": createdAt, "read": readAt.Valid}
		if applicationID != "" {
			item["application_id"] = applicationID
		}
		if !readAt.Valid {
			unread++
		}
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": out, "unread": unread}, nil)
}

// MarkGuardianNotificationRead is POST /guardian/notifications/{id}/read.
func (a *App) MarkGuardianNotificationRead(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE guardian_notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND school_id=$2 AND guardian_id=$3`,
		param(r, "id"), c.SchoolID, c.GuardianID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "notifikasi tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id"), "read": true}, nil)
}

// deliverNotificationEmails sends pending notification emails (at most 50 per
// run, 5 attempts each). Guardians without a verified email are skipped.
func (a *App) deliverNotificationEmails(ctx context.Context) error {
	if a.Mailer == nil {
		return nil
	}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT n.id,n.title,n.body,COALESCE((SELECT gc.normalized_value FROM guardian_contacts gc
		         WHERE gc.guardian_id=n.guardian_id AND gc.kind='email' ORDER BY gc.created_at LIMIT 1),'')
		 FROM guardian_notifications n
		 WHERE n.email_status='pending' AND n.email_attempts<5 ORDER BY n.created_at LIMIT 50`)
	if err != nil {
		return err
	}
	type pending struct{ id, title, body, email string }
	var batch []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.title, &p.body, &p.email); err != nil {
			rows.Close()
			return err
		}
		batch = append(batch, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range batch {
		if p.email == "" {
			_, _ = a.DB.ExecContext(ctx, `UPDATE guardian_notifications SET email_status='skipped' WHERE id=$1`, p.id)
			continue
		}
		body := p.body + "\r\n\r\nSalam,\r\nPanitia PPDB " + a.Config.SchoolName + "\r\n"
		if err := a.Mailer.SendMail(ctx, p.email, "[PPDB] "+p.title, body); err != nil {
			log.Printf("admission: notification email %s failed: %v", p.id, err)
			_, _ = a.DB.ExecContext(ctx,
				`UPDATE guardian_notifications SET email_attempts=email_attempts+1,email_error=$2,
				        email_status=CASE WHEN email_attempts+1>=5 THEN 'failed' ELSE 'pending' END WHERE id=$1`,
				p.id, err.Error())
			continue
		}
		_, _ = a.DB.ExecContext(ctx,
			`UPDATE guardian_notifications SET email_status='sent',email_attempts=email_attempts+1,email_error=NULL WHERE id=$1`, p.id)
	}
	return nil
}

var wib = time.FixedZone("WIB", 7*3600)

// formatDateWIB renders a deadline as "02-01-2006 15:04 WIB".
func formatDateWIB(t time.Time) string {
	return t.In(wib).Format("02-01-2006 15:04") + " WIB"
}
