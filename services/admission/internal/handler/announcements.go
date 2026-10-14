package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
)

// Selection results reach guardians only through a scheduled announcement of
// a committee-approved result set (BUG-034/036). Until publish_at the
// decisions stay invisible and the applications stay "ranked".

type announcementInput struct {
	SelectionApprovalID string `json:"selection_approval_id"`
	Title               string `json:"title"`
	Message             string `json:"message"`
	PublishAt           string `json:"publish_at"`
}

// CreateAnnouncement is POST /admin/selection/announcements.
func (a *App) CreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in announcementInput
	if !common.Decode(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || in.SelectionApprovalID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "judul dan hasil seleksi yang disetujui wajib diisi")
		return
	}
	publishAt, err := parseTime(in.PublishAt)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "jadwal pengumuman tidak valid")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var (
		periodID, waveID, status string
		decidedBy                sql.NullString
		decidedAt                sql.NullTime
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT period_id,wave_id,status,decided_by,decided_at FROM selection_approvals WHERE id::text=$1 AND school_id=$2 FOR UPDATE`,
		in.SelectionApprovalID, schoolID).Scan(&periodID, &waveID, &status, &decidedBy, &decidedAt)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "hasil seleksi tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != "approved" {
		common.WriteError(w, http.StatusUnprocessableEntity, "hasil seleksi belum disetujui panitia seleksi")
		return
	}
	var exists bool
	if err = tx.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM ppdb_announcements WHERE selection_approval_id=$1 AND status<>'cancelled')`,
		in.SelectionApprovalID).Scan(&exists); err != nil {
		writeErr(w, err)
		return
	}
	if exists {
		common.WriteError(w, http.StatusConflict, "hasil seleksi ini sudah dijadwalkan untuk diumumkan")
		return
	}
	var id string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO ppdb_announcements(school_id,period_id,wave_id,title,message,publish_at,status,created_by,created_by_name,
		                                approved_by,approved_by_name,approved_at,selection_approval_id)
		 VALUES($1,$2,$3,$4,$5,$6,'approved',$7,$8,$9,$9,$10,$11) RETURNING id`,
		schoolID, periodID, waveID, in.Title, in.Message, publishAt, claims.UserID, claims.Name,
		decidedBy, decidedAt, in.SelectionApprovalID).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'announcement',$2,'announcement_scheduled','admin',$3,$4,$5)`,
		schoolID, id, claims.UserID, claims.Name, jsonRaw(map[string]any{"publish_at": publishAt, "selection_approval_id": in.SelectionApprovalID})); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "approved", "publish_at": publishAt}, nil)
}

// ListAnnouncements is GET /admin/selection/announcements.
func (a *App) ListAnnouncements(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT pa.id,COALESCE(pa.wave_id::text,''),COALESCE(w.name,''),pa.title,pa.message,pa.publish_at,pa.status,
		        COALESCE(pa.created_by_name,''),pa.published_at,pa.notified_count,COALESCE(pa.cancelled_reason,'')
		 FROM ppdb_announcements pa LEFT JOIN admission_waves w ON w.id=pa.wave_id
		 WHERE pa.school_id=$1 ORDER BY pa.publish_at DESC LIMIT 100`, common.EffectiveSchoolID(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, waveID, waveName, title, message, status, createdBy, cancelled string
			publishAt                                                          time.Time
			publishedAt                                                        sql.NullTime
			notified                                                           int
		)
		if err := rows.Scan(&id, &waveID, &waveName, &title, &message, &publishAt, &status, &createdBy, &publishedAt, &notified, &cancelled); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{"id": id, "wave_id": waveID, "wave_name": waveName, "title": title, "message": message,
			"publish_at": publishAt, "status": status, "created_by_name": createdBy, "notified_count": notified}
		if publishedAt.Valid {
			item["published_at"] = publishedAt.Time
		}
		if cancelled != "" {
			item["cancelled_reason"] = cancelled
		}
		out = append(out, item)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// CancelAnnouncement is POST /admin/selection/announcements/{id}/cancel. Only
// a not yet published announcement can be cancelled.
func (a *App) CancelAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "alasan pembatalan wajib diisi")
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE ppdb_announcements SET status='cancelled',cancelled_reason=$3,updated_at=now()
		 WHERE id=$1 AND school_id=$2 AND status IN('draft','approved')`, param(r, "id"), schoolID, in.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusConflict, "pengumuman sudah terbit atau tidak ditemukan")
		return
	}
	a.audit(r.Context(), schoolID, "announcement", param(r, "id"), "announcement_cancelled", "admin", claims.UserID, claims.Name, in.Reason, nil, nil)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": param(r, "id"), "status": "cancelled"}, nil)
}

// publishDueAnnouncements publishes every approved announcement whose time
// has come: each approved decision becomes visible, the application moves
// to offered / waitlisted / rejected, offers get their re-registration bill
// and every guardian is notified.
func (a *App) publishDueAnnouncements(ctx context.Context, now time.Time) error {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id FROM ppdb_announcements WHERE status='approved' AND publish_at<=$1 AND selection_approval_id IS NOT NULL
		 ORDER BY publish_at LIMIT 20`, now)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := a.publishAnnouncement(ctx, id, now); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) publishAnnouncement(ctx context.Context, announcementID string, now time.Time) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var schoolID, approvalID, status, title string
	if err := tx.QueryRowContext(ctx,
		`SELECT school_id,selection_approval_id,status,title FROM ppdb_announcements WHERE id=$1 FOR UPDATE`, announcementID).
		Scan(&schoolID, &approvalID, &status, &title); err != nil {
		return err
	}
	if status != "approved" {
		return nil
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT d.id,d.application_id,d.decision,COALESCE(d.reason,''),a.status
		 FROM decisions d JOIN applications a ON a.id=d.application_id
		 WHERE d.selection_approval_id=$1 AND d.status='approved' ORDER BY a.registration_number`, approvalID)
	if err != nil {
		return err
	}
	type pending struct{ id, applicationID, decision, reason, status string }
	var list []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.applicationID, &p.decision, &p.reason, &p.status); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	rows.Close()
	notified := 0
	for _, p := range list {
		if p.status != string(model.StatusRanked) {
			// Withdrawn meanwhile: the decision is superseded, not applied.
			if _, err := tx.ExecContext(ctx, `UPDATE decisions SET status='superseded' WHERE id=$1`, p.id); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE decisions SET status='published',published_at=$2 WHERE id=$1`, p.id, now); err != nil {
			return err
		}
		switch p.decision {
		case "offered":
			if err := a.offerSeat(ctx, tx, schoolID, p.applicationID, "announcement", now, p.reason); err != nil {
				return err
			}
		case "waitlisted":
			if _, err := a.transitionTx(ctx, tx, schoolID, p.applicationID, "system", "admission", "Sistem PPDB",
				model.StatusWaitlisted, p.reason, 0, ""); err != nil {
				return err
			}
			if err := a.notify(ctx, tx, schoolID, p.applicationID, "waitlisted", "Hasil seleksi: cadangan",
				"Ananda masuk daftar cadangan. Panitia akan menghubungi bila ada kursi yang kosong.", "result:"+p.id, nil); err != nil {
				return err
			}
		default:
			if _, err := a.transitionTx(ctx, tx, schoolID, p.applicationID, "system", "admission", "Sistem PPDB",
				model.StatusRejected, "Hasil seleksi", 0, ""); err != nil {
				return err
			}
			if err := a.notify(ctx, tx, schoolID, p.applicationID, "rejected", "Hasil seleksi",
				"Mohon maaf, ananda belum dapat diterima pada seleksi ini. Terima kasih atas partisipasinya.", "result:"+p.id, nil); err != nil {
				return err
			}
		}
		notified++
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE ppdb_announcements SET status='published',published_at=$2,notified_count=$3,updated_at=now() WHERE id=$1`,
		announcementID, now, notified); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'announcement',$2,'announcement_published','system','admission','Sistem PPDB',$3)`,
		schoolID, announcementID, jsonRaw(map[string]any{"title": title, "decisions": notified})); err != nil {
		return err
	}
	return tx.Commit()
}
