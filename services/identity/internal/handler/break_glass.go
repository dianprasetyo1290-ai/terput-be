package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"

	"skci/identity/internal/common"
	"skci/identity/internal/rules"
)

// BUG-120 break-glass: a superadmin grants a role for an emergency, for at
// most 24 hours, with a reason. The grant ends by itself (RunBreakGlassExpiry,
// every minute) or by hand; ending removes the role row the grant created,
// ends every session of the account, and sends an activity summary to the
// Kepala Yayasan of the unit (or of the units in the same foundation).
//
//	GET  /security/break-glass?active=true
//	POST /security/break-glass {user_id, school_id, role, duration_minutes, reason, incident_ref?}
//	POST /security/break-glass/{id}/revoke {reason}
//	POST /security/break-glass/{id}/resend-summary

const (
	breakGlassMinMinutes = 15
	breakGlassMaxMinutes = 24 * 60
)

type breakGlassGrant struct {
	ID            string          `json:"id"`
	SchoolID      string          `json:"school_id"`
	SchoolName    *string         `json:"school_name"`
	UserID        string          `json:"user_id"`
	UserName      string          `json:"user_name"`
	UserEmail     string          `json:"user_email"`
	Role          string          `json:"role"`
	Reason        string          `json:"reason"`
	IncidentRef   *string         `json:"incident_ref"`
	GrantedBy     *string         `json:"granted_by"`
	GrantedByName *string         `json:"granted_by_name"`
	StartsAt      time.Time       `json:"starts_at"`
	ExpiresAt     time.Time       `json:"expires_at"`
	EndedAt       *time.Time      `json:"ended_at"`
	EndReason     *string         `json:"end_reason"`
	Summary       json.RawMessage `json:"summary"`
	SummarySentAt *time.Time      `json:"summary_sent_at"`
	SummaryStatus *string         `json:"summary_status"`
	Active        bool            `json:"active"`
}

const breakGlassCols = `g.id, g.school_id, s.name, g.user_id, u.name, u.email, g.role, g.reason, g.incident_ref,
	g.granted_by::text, gb.name, g.starts_at, g.expires_at, g.ended_at, g.end_reason, g.summary, g.summary_sent_at, g.summary_status`

const breakGlassFrom = ` FROM break_glass_grants g
	JOIN users u ON u.id=g.user_id
	LEFT JOIN users gb ON gb.id=g.granted_by
	LEFT JOIN schools s ON s.id=g.school_id`

func scanBreakGlass(row interface{ Scan(...any) error }) (breakGlassGrant, error) {
	var g breakGlassGrant
	var summary []byte
	err := row.Scan(&g.ID, &g.SchoolID, &g.SchoolName, &g.UserID, &g.UserName, &g.UserEmail, &g.Role, &g.Reason, &g.IncidentRef,
		&g.GrantedBy, &g.GrantedByName, &g.StartsAt, &g.ExpiresAt, &g.EndedAt, &g.EndReason, &summary, &g.SummarySentAt, &g.SummaryStatus)
	if len(summary) > 0 {
		g.Summary = summary
	}
	g.Active = g.EndedAt == nil
	return g, err
}

func (a *App) getBreakGlass(ctx context.Context, id string) (breakGlassGrant, error) {
	return scanBreakGlass(a.DB.QueryRowContext(ctx, `SELECT `+breakGlassCols+breakGlassFrom+` WHERE g.id::text=$1`, id))
}

// ListBreakGlass: superadmin sees all; kepala_yayasan sees the units it
// holds a role in (it receives the summaries).
func (a *App) ListBreakGlass(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleKepalaYayasan) {
		return
	}
	claims := common.FromCtx(r)
	q := `SELECT ` + breakGlassCols + breakGlassFrom + ` WHERE ($1 = false OR g.ended_at IS NULL)`
	args := []any{r.URL.Query().Get("active") == "true"}
	if claims != nil && claims.ActiveRole != common.RoleSuperadmin {
		q += ` AND g.school_id IN (SELECT school_id FROM user_school_roles WHERE user_id=$2 AND role='kepala_yayasan' AND status='active')`
		args = append(args, claims.UserID)
	}
	q += ` ORDER BY g.ended_at IS NULL DESC, g.starts_at DESC LIMIT 200`
	rows, err := a.DB.QueryContext(r.Context(), q, args...)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat akses darurat")
		return
	}
	defer rows.Close()
	out := []breakGlassGrant{}
	for rows.Next() {
		g, err := scanBreakGlass(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat akses darurat")
			return
		}
		out = append(out, g)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type breakGlassRequest struct {
	UserID          string `json:"user_id"`
	SchoolID        string `json:"school_id"`
	Role            string `json:"role"`
	DurationMinutes int    `json:"duration_minutes"`
	Reason          string `json:"reason"`
	IncidentRef     string `json:"incident_ref"`
}

// validateBreakGlass checks the request shape (no DB).
func validateBreakGlass(in *breakGlassRequest, granterID string) map[string]string {
	errs := map[string]string{}
	in.Reason = strings.TrimSpace(in.Reason)
	in.IncidentRef = strings.TrimSpace(in.IncidentRef)
	if !rules.IsUUID(in.UserID) {
		errs["user_id"] = "Pilih akun penerima"
	} else if in.UserID == granterID {
		errs["user_id"] = "Akses darurat tidak boleh diberikan kepada diri sendiri"
	}
	if !rules.IsUUID(in.SchoolID) {
		errs["school_id"] = "Pilih unit"
	}
	if !common.IsValidRole(in.Role) || in.Role == common.RoleSuperadmin {
		errs["role"] = "Peran tidak valid (superadmin tidak dapat diberikan lewat akses darurat)"
	}
	if in.DurationMinutes < breakGlassMinMinutes || in.DurationMinutes > breakGlassMaxMinutes {
		errs["duration_minutes"] = fmt.Sprintf("Durasi %d–%d menit", breakGlassMinMinutes, breakGlassMaxMinutes)
	}
	if len([]rune(in.Reason)) < 20 {
		errs["reason"] = "Alasan minimal 20 karakter"
	}
	return errs
}

// CreateBreakGlass: POST /security/break-glass (superadmin only).
func (a *App) CreateBreakGlass(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r) {
		return
	}
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return
	}
	var in breakGlassRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	if errs := validateBreakGlass(&in, claims.UserID); len(errs) > 0 {
		common.WriteValidationError(w, http.StatusBadRequest, "Periksa kembali isian akses darurat", errs)
		return
	}
	var active bool
	err := a.DB.QueryRowContext(r.Context(), `SELECT deleted_at IS NULL FROM users WHERE id=$1`, in.UserID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !active) {
		common.WriteValidationError(w, http.StatusBadRequest, "Akun penerima tidak ditemukan", map[string]string{"user_id": "Akun tidak ditemukan"})
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa akun")
		return
	}
	var schoolOK bool
	if err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM schools WHERE id=$1 AND deleted_at IS NULL)`, in.SchoolID).Scan(&schoolOK); err != nil || !schoolOK {
		common.WriteValidationError(w, http.StatusBadRequest, "Unit tidak ditemukan", map[string]string{"school_id": "Unit tidak ditemukan"})
		return
	}
	var hasRole bool
	_ = a.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$1 AND school_id=$2 AND role=$3)`,
		in.UserID, in.SchoolID, in.Role).Scan(&hasRole)
	if hasRole {
		common.WriteError(w, http.StatusConflict, "Akun sudah memegang peran ini di unit tersebut; akses darurat tidak diperlukan")
		return
	}
	// SoD still applies in an emergency.
	if !a.checkRoleConflicts(w, r, in.UserID, in.Role) {
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memberi akses darurat")
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRow(`INSERT INTO break_glass_grants (school_id, user_id, role, reason, incident_ref, granted_by, starts_at, expires_at, role_row_created)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6, now(), now() + make_interval(mins => $7), true) RETURNING id`,
		in.SchoolID, in.UserID, in.Role, in.Reason, in.IncidentRef, claims.UserID, in.DurationMinutes).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "Akses darurat untuk peran ini masih berjalan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memberi akses darurat")
		return
	}
	if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)`, in.UserID, in.SchoolID, in.Role); err != nil {
		common.WriteDBError(w, err, "Gagal memberi akses darurat")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memberi akses darurat")
		return
	}
	sid := in.SchoolID
	a.RecordSecurityAudit(&sid, "break_glass_granted", in.UserID, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"break_glass_id": id, "role": in.Role, "duration_minutes": in.DurationMinutes, "incident_ref": in.IncidentRef},
		in.Reason, correlationOr(r, "break-glass:"+id))
	g, err := a.getBreakGlass(r.Context(), id)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat akses darurat")
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, g, nil,
		fmt.Sprintf("Akses darurat %s aktif sampai %s dan akan dicabut otomatis", in.Role, g.ExpiresAt.In(jakarta()).Format("02-01-2006 15:04 WIB")))
}

func jakarta() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*3600)
}

// RevokeBreakGlass: POST /security/break-glass/{id}/revoke {reason}.
func (a *App) RevokeBreakGlass(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r) {
		return
	}
	claims := common.FromCtx(r)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if len([]rune(strings.TrimSpace(body.Reason))) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan pencabutan minimal 10 karakter", map[string]string{"reason": "Minimal 10 karakter"})
		return
	}
	actor := ""
	if claims != nil {
		actor = claims.UserID
	}
	ended, err := a.endBreakGlass(withCorrelation(r.Context(), r), chi.URLParam(r, "id"), "revoked", actor, strings.TrimSpace(body.Reason))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusConflict, "Akses darurat tidak ditemukan atau sudah berakhir")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal mencabut akses darurat")
		return
	}
	g, _ := a.getBreakGlass(r.Context(), ended)
	common.WriteJSONMsg(w, http.StatusOK, g, nil, "Akses darurat dicabut. "+summaryStatusText(g.SummaryStatus))
}

// ResendBreakGlassSummary: POST /security/break-glass/{id}/resend-summary.
func (a *App) ResendBreakGlassSummary(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r) {
		return
	}
	g, err := a.getBreakGlass(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "Akses darurat tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat akses darurat")
		return
	}
	if g.EndedAt == nil {
		common.WriteError(w, http.StatusConflict, "Ringkasan dikirim setelah akses darurat berakhir")
		return
	}
	a.sendBreakGlassSummary(withCorrelation(r.Context(), r), g.ID)
	g, _ = a.getBreakGlass(r.Context(), g.ID)
	common.WriteJSONMsg(w, http.StatusOK, g, nil, summaryStatusText(g.SummaryStatus))
}

func summaryStatusText(s *string) string {
	if s == nil {
		return ""
	}
	switch {
	case *s == "sent":
		return "Ringkasan aktivitas terkirim ke Kepala Yayasan"
	case *s == "no_recipient":
		return "Ringkasan tersimpan, tetapi unit ini belum memiliki akun Kepala Yayasan aktif"
	default:
		return "Ringkasan tersimpan, pengiriman gagal: " + *s
	}
}

// endBreakGlass closes one active grant (expired or revoked), builds the
// activity summary and sends it. Returns sql.ErrNoRows when it is not active.
func (a *App) endBreakGlass(ctx context.Context, id, endReason, actorID, note string) (string, error) {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var userID, schoolID, role string
	var created bool
	var starts time.Time
	if err := tx.QueryRowContext(ctx, `SELECT user_id, school_id, role, role_row_created, starts_at FROM break_glass_grants
		WHERE id::text=$1 AND ended_at IS NULL FOR UPDATE`, id).Scan(&userID, &schoolID, &role, &created, &starts); err != nil {
		return "", err
	}
	if created {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_school_roles WHERE user_id=$1 AND school_id=$2 AND role=$3`, userID, schoolID, role); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET tokens_valid_after=now() WHERE id=$1`, userID); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE user_sessions SET revoked_at=now(), revoked_reason='break_glass_ended' WHERE user_id=$1 AND revoked_at IS NULL`, userID); err != nil {
		return "", err
	}
	summary, err := breakGlassActivity(ctx, tx, userID, starts)
	if err != nil {
		return "", err
	}
	summary["end_reason"] = endReason
	if note != "" {
		summary["note"] = note
	}
	raw, _ := json.Marshal(summary)
	if _, err := tx.ExecContext(ctx, `UPDATE break_glass_grants SET ended_at=now(), end_reason=$2, ended_by=$3, summary=$4 WHERE id::text=$1`,
		id, endReason, nullUUID(actorID), raw); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	var actor *string
	if actorID != "" {
		actor = &actorID
	}
	sid := schoolID
	a.RecordSecurityAudit(&sid, "break_glass_"+endReason, userID, actor, nil, nil,
		map[string]any{"break_glass_id": id, "role": role, "summary": summary}, note, "break-glass:"+id)
	a.sendBreakGlassSummary(ctx, id)
	return id, nil
}

// breakGlassActivity summarises what the account did during the grant from
// identity's own records (sign-ins, failures, sessions, IPs). Actions inside
// other services are in the central audit trail, filterable by the
// correlation id "break-glass:<id>" and the actor.
func breakGlassActivity(ctx context.Context, tx *sql.Tx, userID string, since time.Time) (map[string]any, error) {
	var ok, failed, sessions int
	var ips pq.StringArray
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE success), count(*) FILTER (WHERE NOT success),
			COALESCE(array_agg(DISTINCT ip) FILTER (WHERE ip IS NOT NULL), '{}')
		FROM login_events WHERE user_id=$1 AND created_at >= $2`, userID, since).Scan(&ok, &failed, &ips); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_sessions WHERE user_id=$1 AND created_at >= $2`, userID, since).Scan(&sessions); err != nil {
		return nil, err
	}
	return map[string]any{
		"started_at":        since,
		"ended_at":          time.Now(),
		"successful_logins": ok,
		"failed_logins":     failed,
		"sessions_opened":   sessions,
		"ip_addresses":      []string(ips),
	}, nil
}

// sendBreakGlassSummary notifies every active Kepala Yayasan of the unit, or
// of the units in the same foundation when the unit has none.
func (a *App) sendBreakGlassSummary(ctx context.Context, id string) {
	g, err := a.getBreakGlass(ctx, id)
	if err != nil {
		return
	}
	rows, err := a.DB.QueryContext(ctx, `
		SELECT DISTINCT usr.user_id::text, usr.school_id::text FROM user_school_roles usr
		JOIN users u ON u.id=usr.user_id AND u.deleted_at IS NULL
		WHERE usr.role='kepala_yayasan' AND usr.status='active'
		  AND (usr.school_id=$1 OR usr.school_id IN (
		      SELECT s2.id FROM schools s1 JOIN schools s2 ON s2.foundation_id=s1.foundation_id
		      WHERE s1.id=$1 AND s1.foundation_id IS NOT NULL))`, g.SchoolID)
	if err != nil {
		a.setSummaryStatus(ctx, id, "gagal memuat penerima")
		return
	}
	type rcpt struct{ user, school string }
	var to []rcpt
	for rows.Next() {
		var x rcpt
		if rows.Scan(&x.user, &x.school) == nil {
			to = append(to, x)
		}
	}
	rows.Close()
	if len(to) == 0 {
		a.setSummaryStatus(ctx, id, "no_recipient")
		return
	}
	var sum map[string]any
	_ = json.Unmarshal(g.Summary, &sum)
	school := ""
	if g.SchoolName != nil {
		school = *g.SchoolName
	}
	body := fmt.Sprintf("Akses darurat peran %s untuk %s (%s) di %s berakhir (%s).\nAlasan: %s\nMasa akses: %s s.d. %s.\n"+
		"Login berhasil: %v, gagal: %v, sesi dibuka: %v. Jejak tindakan lengkap ada di Audit Trail dengan correlation id break-glass:%s.",
		g.Role, g.UserName, g.UserEmail, school, deref(g.EndReason), g.Reason,
		g.StartsAt.In(jakarta()).Format("02-01-2006 15:04"), g.EndedAt.In(jakarta()).Format("02-01-2006 15:04 WIB"),
		sum["successful_logins"], sum["failed_logins"], sum["sessions_opened"], g.ID)
	failed := 0
	var lastErr error
	for _, x := range to {
		if err := a.notifyUser(ctx, x.school, x.user, "security_break_glass_summary", "Ringkasan akses darurat (break-glass)", body,
			"break-glass-summary:"+g.ID+":"+x.user, map[string]any{"break_glass_id": g.ID, "mandatory": true}); err != nil {
			failed++
			lastErr = err
		}
	}
	switch {
	case failed == 0:
		_, _ = a.DB.ExecContext(ctx, `UPDATE break_glass_grants SET summary_status='sent', summary_sent_at=now() WHERE id=$1`, id)
	default:
		a.setSummaryStatus(ctx, id, upstreamMessage("Layanan notifikasi", lastErr))
	}
}

func (a *App) setSummaryStatus(ctx context.Context, id, status string) {
	_, _ = a.DB.ExecContext(ctx, `UPDATE break_glass_grants SET summary_status=$2 WHERE id=$1`, id, status)
}

// RunBreakGlassExpiry ends every grant past expires_at. Called every minute
// from main; safe to run on several replicas (FOR UPDATE + ended_at check).
func (a *App) RunBreakGlassExpiry(ctx context.Context) int {
	rows, err := a.DB.QueryContext(ctx, `SELECT id::text FROM break_glass_grants WHERE ended_at IS NULL AND expires_at <= now() LIMIT 100`)
	if err != nil {
		log.Printf("break-glass expiry: %v", err)
		return 0
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if _, err := a.endBreakGlass(ctx, id, "expired", "", ""); err == nil {
			n++
		} else if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("break-glass expiry %s: %v", id, err)
		}
	}
	return n
}
