package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// reminderMaxAttempts bounds delivery retries of one reminder before it is
// handed to core's exception queue. It is a transport retry limit, not a
// school business rule.
const reminderMaxAttempts = 3

var monthNamesID = []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func periodLabel(year, month int) string {
	if month >= 1 && month <= 12 {
		return fmt.Sprintf("%s %d", monthNamesID[month], year)
	}
	return fmt.Sprintf("%d/%d", month, year)
}

// ---- settings --------------------------------------------------------------

func (a *App) reminderPolicy(ctx context.Context, schoolID string) (model.ReminderPolicy, bool, error) {
	var p model.ReminderPolicy
	var before, after, channels string
	err := a.DB.QueryRowContext(ctx,
		`SELECT enabled, array_to_json(days_before)::text, on_due, array_to_json(days_after)::text, send_hour, array_to_json(channels)::text
		   FROM billing_reminder_settings WHERE school_id = $1`, schoolID).
		Scan(&p.Enabled, &before, &p.OnDue, &after, &p.SendHour, &channels)
	if errors.Is(err, sql.ErrNoRows) {
		return model.SuggestedReminderPolicy(), false, nil
	}
	if err != nil {
		return p, false, err
	}
	_ = json.Unmarshal([]byte(before), &p.DaysBefore)
	_ = json.Unmarshal([]byte(after), &p.DaysAfter)
	_ = json.Unmarshal([]byte(channels), &p.Channels)
	return p, true, nil
}

// GetReminderSettings is GET /admin/billing/reminder-settings. An
// unconfigured school gets the suggested schedule with configured=false;
// nothing is sent until the school saves it.
func (a *App) GetReminderSettings(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	p, configured, err := a.reminderPolicy(r.Context(), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteSuccess(w, map[string]any{"configured": configured, "policy": p}, nil)
}

// PutReminderSettings is PUT /admin/billing/reminder-settings.
func (a *App) PutReminderSettings(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in model.ReminderPolicy
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.Normalize(); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	before, configured, err := a.reminderPolicy(r.Context(), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	b, _ := json.Marshal(in.DaysBefore)
	af, _ := json.Marshal(in.DaysAfter)
	ch, _ := json.Marshal(in.Channels)
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO billing_reminder_settings (school_id, enabled, days_before, on_due, days_after, send_hour, channels, updated_by)
		 VALUES ($1, $2, ARRAY(SELECT jsonb_array_elements_text($3::jsonb)::int), $4,
		         ARRAY(SELECT jsonb_array_elements_text($5::jsonb)::int), $6,
		         ARRAY(SELECT jsonb_array_elements_text($7::jsonb)), $8)
		 ON CONFLICT (school_id) DO UPDATE SET enabled = EXCLUDED.enabled, days_before = EXCLUDED.days_before,
		   on_due = EXCLUDED.on_due, days_after = EXCLUDED.days_after, send_hour = EXCLUDED.send_hour,
		   channels = EXCLUDED.channels, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		schoolID, in.Enabled, string(b), in.OnDue, string(af), in.SendHour, string(ch), c.UserID); err != nil {
		writeErr(w, err)
		return
	}
	var prev any
	if configured {
		prev = before
	}
	a.audit(r.Context(), schoolID, "reminder_settings", schoolID, "reminder_settings_updated", c.ActiveRole, c.UserID, "", "", prev, in)
	common.WriteSuccess(w, map[string]any{"configured": true, "policy": in}, nil)
}

// ---- delivery --------------------------------------------------------------

type reminderDelivery struct {
	SchoolID    string
	StudentID   string
	BillID      string // "" for a per-student manual reminder
	GuardianID  string
	Kind        model.ReminderKind
	Offset      int
	Outstanding int64
	SentBy      string
	Key         string
	Title, Body string
	Channels    []string
}

type reminderResult struct {
	StudentUserID  string `json:"student_user_id"`
	BillID         string `json:"bill_id,omitempty"`
	GuardianUserID string `json:"guardian_user_id,omitempty"`
	Status         string `json:"status"` // sent | failed | skipped | duplicate
	Error          string `json:"error,omitempty"`
}

// deliverReminder claims the log row for d.Key, sends it and records the
// outcome. A key that was already sent (or is in flight) is reported as
// duplicate; a failed one is retried up to reminderMaxAttempts and then
// raised in core's exception queue.
func (a *App) deliverReminder(ctx context.Context, d reminderDelivery) reminderResult {
	res := reminderResult{StudentUserID: d.StudentID, BillID: d.BillID, GuardianUserID: d.GuardianID}
	var id string
	var attempts int
	err := a.DB.QueryRowContext(ctx,
		`INSERT INTO bill_reminder_log (school_id, student_user_id, bill_id, guardian_user_id, kind, offset_days, outstanding_idr, status, attempts, sent_by, idempotency_key)
		 VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, $5, $6, $7, 'pending', 1, NULLIF($8, '')::uuid, $9)
		 ON CONFLICT (school_id, idempotency_key) DO UPDATE
		   SET status = 'pending', attempts = bill_reminder_log.attempts + 1, updated_at = now()
		 WHERE (bill_reminder_log.status = 'failed' AND bill_reminder_log.attempts < $10)
		    OR (bill_reminder_log.status = 'pending' AND bill_reminder_log.updated_at < now() - interval '10 minutes')
		 RETURNING id::text, attempts`,
		d.SchoolID, d.StudentID, d.BillID, d.GuardianID, string(d.Kind), d.Offset, d.Outstanding, d.SentBy, d.Key, reminderMaxAttempts).Scan(&id, &attempts)
	if errors.Is(err, sql.ErrNoRows) {
		res.Status = "duplicate"
		return res
	}
	if err != nil {
		res.Status, res.Error = "failed", err.Error()
		return res
	}
	data := map[string]any{"student_user_id": d.StudentID, "kind": string(d.Kind), "url": "/orang-tua/tagihan"}
	if d.BillID != "" {
		data["bill_id"] = d.BillID
		data["url"] = "/orang-tua/tagihan/" + d.BillID
	}
	sendErr := a.Notification.Notify(ctx, d.Key, client.Notification{
		SchoolID: d.SchoolID, UserID: d.GuardianID, Type: "bill_reminder",
		Title: d.Title, Body: d.Body, Data: data, Channels: d.Channels,
	})
	if sendErr == nil {
		_, _ = a.DB.ExecContext(ctx, `UPDATE bill_reminder_log SET status = 'sent', error = NULL, updated_at = now() WHERE id = $1::uuid`, id)
		res.Status = "sent"
		return res
	}
	_, _ = a.DB.ExecContext(ctx, `UPDATE bill_reminder_log SET status = 'failed', error = $2, updated_at = now() WHERE id = $1::uuid`, id, sendErr.Error())
	res.Status, res.Error = "failed", sendErr.Error()
	if attempts >= reminderMaxAttempts || d.Kind == model.ReminderManual {
		a.raiseNotificationFailure(ctx, d.SchoolID, "reminder:"+d.Key, sendErr.Error(), map[string]any{
			"type": "bill_reminder", "student_user_id": d.StudentID, "bill_id": d.BillID,
			"guardian_user_id": d.GuardianID, "kind": string(d.Kind), "attempts": attempts,
		})
	}
	return res
}

// skipReminder records that a reminder could not be addressed (no guardian).
func (a *App) skipReminder(ctx context.Context, d reminderDelivery, reason string) reminderResult {
	_, _ = a.DB.ExecContext(ctx,
		`INSERT INTO bill_reminder_log (school_id, student_user_id, bill_id, kind, offset_days, outstanding_idr, status, error, sent_by, idempotency_key)
		 VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, 'skipped', $7, NULLIF($8, '')::uuid, $9)
		 ON CONFLICT (school_id, idempotency_key) DO NOTHING`,
		d.SchoolID, d.StudentID, d.BillID, string(d.Kind), d.Offset, d.Outstanding, reason, d.SentBy, d.Key)
	return reminderResult{StudentUserID: d.StudentID, BillID: d.BillID, Status: "skipped", Error: reason}
}

// raiseNotificationFailure puts a failed guardian notification in core's
// Integration Exception Queue (kind "notification"), once per key.
func (a *App) raiseNotificationFailure(ctx context.Context, schoolID, key, msg string, summary map[string]any) {
	if a.Core == nil || !a.Core.Configured() {
		common.CaptureError(fmt.Errorf("notification %s failed (core not configured): %s", key, msg))
		return
	}
	if err := a.Core.CreateException(ctx, client.ExceptionEvent{
		SchoolID: schoolID, Kind: "notification", Severity: "normal",
		CorrelationID: key, ErrorMessage: msg, PayloadSummary: summary,
		MaxAttempts: 1, IdempotencyKey: "notify:" + key,
	}); err != nil {
		common.CaptureError(err)
	}
}

func (a *App) notifyReady() bool {
	return a.Notification != nil && a.Notification.Configured() && a.IdentitySvc != nil && a.IdentitySvc.Configured()
}

// JobBillReminders sends the scheduled H-x / H / H+x reminders of every
// school that switched them on, once its local send hour has passed.
func (a *App) JobBillReminders(ctx context.Context, _ *sql.DB) error {
	rows, err := a.DB.QueryContext(ctx, `SELECT school_id::text FROM billing_reminder_settings WHERE enabled`)
	if err != nil {
		return err
	}
	var schools []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		schools = append(schools, s)
	}
	rows.Close()
	if len(schools) == 0 {
		return nil
	}
	if !a.notifyReady() {
		return errors.New("pengingat tagihan aktif tetapi NOTIFICATION_URL/IDENTITY_URL belum dikonfigurasi")
	}
	var errs []error
	for _, s := range schools {
		if _, err := a.runScheduledReminders(ctx, s, a.now()); err != nil {
			errs = append(errs, fmt.Errorf("school %s: %w", s, err))
		}
	}
	return errors.Join(errs...)
}

// runScheduledReminders sends today's due reminders for one school.
func (a *App) runScheduledReminders(ctx context.Context, schoolID string, now time.Time) ([]reminderResult, error) {
	p, configured, err := a.reminderPolicy(ctx, schoolID)
	if err != nil || !configured || !p.Enabled {
		return nil, err
	}
	loc := a.location()
	if now.In(loc).Hour() < p.SendHour {
		return nil, nil
	}
	before, after := p.Window()
	y, m, d := now.In(loc).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, loc)
	rows, err := a.DB.QueryContext(ctx,
		`SELECT b.id::text, b.student_user_id::text, b.due_at, b.period_year, b.period_month,
		        COALESCE(s.name, initcap(b.source_module), 'Tagihan')
		   FROM student_bills b LEFT JOIN billing_schemes s ON s.id = b.scheme_id
		  WHERE b.school_id = $1 AND b.status <> 'void' AND b.due_at IS NOT NULL
		    AND b.due_at >= $2 AND b.due_at < $3
		  ORDER BY b.due_at, b.id`,
		schoolID, today.AddDate(0, 0, -after), today.AddDate(0, 0, before+1))
	if err != nil {
		return nil, err
	}
	type cand struct {
		id, student, label string
		due                time.Time
		year, month        int
	}
	var cands []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.student, &c.due, &c.year, &c.month, &c.label); err != nil {
			rows.Close()
			return nil, err
		}
		cands = append(cands, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []reminderResult
	for _, c := range cands {
		kind, offset, ok := model.ReminderFor(p, now, c.due, loc)
		if !ok {
			continue
		}
		_, _, _, outstanding, err := billTotalsQ(ctx, a.DB, schoolID, c.id)
		if err != nil {
			return out, err
		}
		if outstanding <= 0 {
			continue
		}
		title, body := model.ReminderText(kind, offset, c.label+" "+periodLabel(c.year, c.month), formatRupiah(outstanding), c.due.In(loc).Format("02-01-2006"))
		base := fmt.Sprintf("auto:%s:%s:%d:%s", c.id, kind, offset, c.due.In(loc).Format("20060102"))
		d := reminderDelivery{SchoolID: schoolID, StudentID: c.student, BillID: c.id, Kind: kind, Offset: offset,
			Outstanding: outstanding, Title: title, Body: body, Channels: p.Channels}
		guardians, err := a.IdentitySvc.StudentGuardianIDs(ctx, schoolID, c.student)
		if err != nil {
			// Identity unreachable: nothing is logged, the next run retries.
			common.CaptureError(err)
			continue
		}
		if len(guardians) == 0 {
			d.Key = base + ":no-guardian"
			out = append(out, a.skipReminder(ctx, d, "siswa belum memiliki wali terhubung"))
			continue
		}
		for _, g := range guardians {
			d.GuardianID, d.Key = g, base+":"+g
			out = append(out, a.deliverReminder(ctx, d))
		}
	}
	return out, nil
}

// AdminBillReminders is POST /admin/bills/reminders: a finance officer
// reminds the guardians of the given students (or of the given bills) about
// their outstanding balance now. Students without an open balance are
// skipped. Idempotency-Key makes a resubmitted request a no-op.
func (a *App) AdminBillReminders(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var req struct {
		StudentUserIDs []string `json:"student_user_ids"`
		BillIDs        []string `json:"bill_ids"`
		Message        string   `json:"message"`
	}
	if !common.Decode(w, r, &req) {
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if len(req.StudentUserIDs) == 0 && len(req.BillIDs) == 0 {
		common.WriteError(w, http.StatusBadRequest, "pilih minimal satu siswa atau tagihan")
		return
	}
	if len(req.StudentUserIDs)+len(req.BillIDs) > 500 {
		common.WriteError(w, http.StatusBadRequest, "maksimal 500 siswa/tagihan per pengiriman")
		return
	}
	if len(req.Message) > 1000 {
		common.WriteError(w, http.StatusBadRequest, "pesan maksimal 1000 karakter")
		return
	}
	if !a.notifyReady() {
		common.WriteError(w, http.StatusServiceUnavailable, "layanan notifikasi belum dikonfigurasi")
		return
	}
	p, _, err := a.reminderPolicy(r.Context(), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	requestKey := idempotencyKey(r)

	// Resolve the target students and, when bills were picked, restrict the
	// balance to those bills.
	onlyBills := map[string]bool{}
	students := map[string]bool{}
	for _, s := range req.StudentUserIDs {
		if s = strings.TrimSpace(s); s != "" {
			students[s] = true
		}
	}
	if len(req.BillIDs) > 0 {
		rows, err := a.DB.QueryContext(r.Context(),
			`SELECT id::text, student_user_id::text FROM student_bills WHERE school_id = $1 AND id::text = ANY($2) AND status <> 'void'`,
			schoolID, req.BillIDs)
		if err != nil {
			writeErr(w, err)
			return
		}
		for rows.Next() {
			var id, st string
			if err := rows.Scan(&id, &st); err != nil {
				rows.Close()
				writeErr(w, err)
				return
			}
			onlyBills[id] = true
			students[st] = true
		}
		rows.Close()
		if len(onlyBills) != len(req.BillIDs) {
			common.WriteError(w, http.StatusUnprocessableEntity, "sebagian tagihan tidak ditemukan atau sudah dibatalkan")
			return
		}
	}

	var results []reminderResult
	counts := map[string]int{}
	for st := range students {
		open, err := loadOpenBills(r.Context(), a.DB, schoolID, st, false)
		if err != nil {
			writeErr(w, err)
			return
		}
		var total int64
		n := 0
		for _, b := range open {
			if len(onlyBills) > 0 && !onlyBills[b.BillID] {
				continue
			}
			total += b.OutstandingIDR
			n++
		}
		d := reminderDelivery{SchoolID: schoolID, StudentID: st, Kind: model.ReminderManual, Outstanding: total,
			SentBy: c.UserID, Channels: p.Channels, Title: "Pengingat tagihan"}
		if n == 0 {
			res := reminderResult{StudentUserID: st, Status: "skipped", Error: "tidak ada tagihan terbuka"}
			results = append(results, res)
			counts[res.Status]++
			continue
		}
		d.Body = fmt.Sprintf("Sisa tagihan sebesar %s (%d tagihan).", formatRupiah(total), n)
		if req.Message != "" {
			d.Body += " " + req.Message
		}
		guardians, err := a.IdentitySvc.StudentGuardianIDs(r.Context(), schoolID, st)
		if err != nil {
			res := reminderResult{StudentUserID: st, Status: "failed", Error: "data wali tidak dapat dibaca: " + err.Error()}
			results = append(results, res)
			counts[res.Status]++
			continue
		}
		base := "manual:" + requestKey + ":" + st
		if len(guardians) == 0 {
			d.Key = base + ":no-guardian"
			res := a.skipReminder(r.Context(), d, "siswa belum memiliki wali terhubung")
			results = append(results, res)
			counts[res.Status]++
			continue
		}
		for _, g := range guardians {
			d.GuardianID, d.Key = g, base+":"+g
			res := a.deliverReminder(r.Context(), d)
			results = append(results, res)
			counts[res.Status]++
		}
	}
	a.audit(r.Context(), schoolID, "bill_reminder", requestKey, "bill_reminders_sent", c.ActiveRole, c.UserID, "", req.Message,
		nil, map[string]any{"students": len(students), "bills": req.BillIDs, "counts": counts})
	common.WriteSuccess(w, map[string]any{
		"sent": counts["sent"], "failed": counts["failed"], "skipped": counts["skipped"], "duplicate": counts["duplicate"],
		"results": results, "idempotency_key": requestKey,
	}, nil)
}

// ListReminderLog is GET /admin/bills/reminder-log.
func (a *App) ListReminderLog(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	q := r.URL.Query()
	page, perPage := common.Page(r)
	where := `WHERE school_id = $1 AND ($2 = '' OR student_user_id::text = $2) AND ($3 = '' OR bill_id::text = $3) AND ($4 = '' OR status = $4)`
	args := []any{schoolID, q.Get("student_user_id"), q.Get("bill_id"), q.Get("status")}
	var total int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM bill_reminder_log `+where, args...).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text, student_user_id::text, COALESCE(bill_id::text, ''), COALESCE(guardian_user_id::text, ''), kind, offset_days,
		        outstanding_idr, status, attempts, COALESCE(error, ''), COALESCE(sent_by::text, ''), created_at, updated_at
		   FROM bill_reminder_log `+where+` ORDER BY created_at DESC, id LIMIT $5 OFFSET $6`,
		append(args, perPage, common.Offset(page, perPage))...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	type row struct {
		ID             string    `json:"id"`
		StudentUserID  string    `json:"student_user_id"`
		BillID         string    `json:"bill_id"`
		GuardianUserID string    `json:"guardian_user_id"`
		Kind           string    `json:"kind"`
		OffsetDays     int       `json:"offset_days"`
		OutstandingIDR int64     `json:"outstanding_idr"`
		Status         string    `json:"status"`
		Attempts       int       `json:"attempts"`
		Error          string    `json:"error"`
		SentBy         string    `json:"sent_by"`
		CreatedAt      time.Time `json:"created_at"`
		UpdatedAt      time.Time `json:"updated_at"`
	}
	items := []row{}
	for rows.Next() {
		var it row
		if err := rows.Scan(&it.ID, &it.StudentUserID, &it.BillID, &it.GuardianUserID, &it.Kind, &it.OffsetDays,
			&it.OutstandingIDR, &it.Status, &it.Attempts, &it.Error, &it.SentBy, &it.CreatedAt, &it.UpdatedAt); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, it)
	}
	common.WriteSuccess(w, items, &common.Meta{Page: page, PerPage: perPage, Total: total})
}

// ---- follow-ups (BUG-133) ----------------------------------------------------

var followupChannels = map[string]bool{"phone": true, "whatsapp": true, "email": true, "visit": true, "letter": true, "meeting": true, "other": true}

// CreateFollowup is POST /admin/billing/followups.
func (a *App) CreateFollowup(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	var in struct {
		StudentUserID    string `json:"student_user_id"`
		BillID           string `json:"bill_id"`
		Channel          string `json:"channel"`
		Outcome          string `json:"outcome"`
		PromiseDate      string `json:"promise_date"`
		PromiseAmountIDR *int64 `json:"promise_amount_idr"`
		NextActionOn     string `json:"next_action_on"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Outcome = strings.TrimSpace(in.Outcome)
	switch {
	case in.StudentUserID == "":
		common.WriteError(w, http.StatusBadRequest, "student_user_id wajib diisi")
		return
	case !followupChannels[in.Channel]:
		common.WriteError(w, http.StatusBadRequest, "kanal tindak lanjut tidak dikenal")
		return
	case in.Outcome == "":
		common.WriteError(w, http.StatusBadRequest, "hasil tindak lanjut wajib diisi")
		return
	case in.PromiseAmountIDR != nil && *in.PromiseAmountIDR <= 0:
		common.WriteError(w, http.StatusBadRequest, "nominal janji bayar harus positif")
		return
	}
	for _, d := range []string{in.PromiseDate, in.NextActionOn} {
		if d != "" {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				common.WriteError(w, http.StatusBadRequest, "tanggal harus berformat YYYY-MM-DD")
				return
			}
		}
	}
	if in.BillID != "" {
		var ok bool
		if err := a.DB.QueryRowContext(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM student_bills WHERE id::text = $1 AND school_id = $2 AND student_user_id::text = $3)`,
			in.BillID, schoolID, in.StudentUserID).Scan(&ok); err != nil {
			writeErr(w, err)
			return
		}
		if !ok {
			common.WriteError(w, http.StatusUnprocessableEntity, "tagihan bukan milik siswa ini")
			return
		}
	}
	var id string
	var created time.Time
	if err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO bill_followups (school_id, student_user_id, bill_id, channel, outcome, promise_date, promise_amount_idr, next_action_on, created_by)
		 VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, NULLIF($6, '')::date, $7, NULLIF($8, '')::date, $9)
		 RETURNING id::text, created_at`,
		schoolID, in.StudentUserID, in.BillID, in.Channel, in.Outcome, in.PromiseDate, in.PromiseAmountIDR, in.NextActionOn, c.UserID).Scan(&id, &created); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "bill_followup", id, "followup_recorded", c.ActiveRole, c.UserID, "", "", nil, in)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "created_at": created}, nil)
}

// ListFollowups is GET /admin/billing/followups.
func (a *App) ListFollowups(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	page, perPage := common.Page(r)
	student := r.URL.Query().Get("student_user_id")
	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM bill_followups WHERE school_id = $1 AND ($2 = '' OR student_user_id::text = $2)`, schoolID, student).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text, student_user_id::text, COALESCE(bill_id::text, ''), channel, outcome,
		        COALESCE(to_char(promise_date, 'YYYY-MM-DD'), ''), promise_amount_idr, COALESCE(to_char(next_action_on, 'YYYY-MM-DD'), ''),
		        created_by::text, created_at
		   FROM bill_followups WHERE school_id = $1 AND ($2 = '' OR student_user_id::text = $2)
		  ORDER BY created_at DESC, id LIMIT $3 OFFSET $4`, schoolID, student, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	type row struct {
		ID               string    `json:"id"`
		StudentUserID    string    `json:"student_user_id"`
		BillID           string    `json:"bill_id"`
		Channel          string    `json:"channel"`
		Outcome          string    `json:"outcome"`
		PromiseDate      string    `json:"promise_date"`
		PromiseAmountIDR *int64    `json:"promise_amount_idr"`
		NextActionOn     string    `json:"next_action_on"`
		CreatedBy        string    `json:"created_by"`
		CreatedAt        time.Time `json:"created_at"`
	}
	items := []row{}
	for rows.Next() {
		var it row
		var amt sql.NullInt64
		if err := rows.Scan(&it.ID, &it.StudentUserID, &it.BillID, &it.Channel, &it.Outcome, &it.PromiseDate, &amt,
			&it.NextActionOn, &it.CreatedBy, &it.CreatedAt); err != nil {
			writeErr(w, err)
			return
		}
		if amt.Valid {
			v := amt.Int64
			it.PromiseAmountIDR = &v
		}
		items = append(items, it)
	}
	common.WriteSuccess(w, items, &common.Meta{Page: page, PerPage: perPage, Total: total})
}
