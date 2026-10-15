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

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"

	"skci/identity/internal/common"
	"skci/identity/internal/rules"
	"skci/identity/internal/security"
)

// BUG-023 / AC-RBAC-08: an exception to a forbidden role combination is a
// request approved in the core Approval Center (request_type
// role_conflict_exception). The role is only granted when core calls back
// with decision=approved.
//
//	POST /users/{id}/roles {role, exception_reason, valid_until?}  -> 202 pending exception
//	GET  /security/role-conflict-exceptions?status=                 -> list (admin: own unit)
//	POST /security/role-conflict-exceptions/{id}/revoke {reason}    -> withdraw / end
//	POST /internal/role-conflict-exceptions/approval-callback       -> core callback
//	GET  /security/role-conflicts/cleanup-log                        -> rows removed by clean-up
//	POST /security/role-conflicts/violations/resolve {reason}        -> superadmin clean-up

const reqTypeRoleConflictException = "role_conflict_exception"

type roleConflictException struct {
	ID               string     `json:"id"`
	SchoolID         string     `json:"school_id"`
	UserID           string     `json:"user_id"`
	UserName         string     `json:"user_name"`
	UserEmail        string     `json:"user_email"`
	Role             string     `json:"role"`
	ConflictingRoles []string   `json:"conflicting_roles"`
	Reason           string     `json:"reason"`
	ValidUntil       *string    `json:"valid_until"`
	Status           string     `json:"status"`
	StatusMessage    *string    `json:"status_message"`
	ApprovalID       *string    `json:"approval_id"`
	RequestedBy      *string    `json:"requested_by"`
	DecidedBy        *string    `json:"decided_by"`
	DecisionComment  *string    `json:"decision_comment"`
	DecidedAt        *time.Time `json:"decided_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

const exceptionCols = `e.id, e.school_id, e.user_id, u.name, u.email, e.role, e.conflicting_roles::text[], e.reason,
	to_char(e.valid_until,'YYYY-MM-DD'), e.status, e.status_message, e.approval_id, e.requested_by::text,
	e.decided_by, e.decision_comment, e.decided_at, e.revoked_at, e.created_at`

func scanException(row interface{ Scan(...any) error }) (roleConflictException, error) {
	var e roleConflictException
	var roles pq.StringArray
	err := row.Scan(&e.ID, &e.SchoolID, &e.UserID, &e.UserName, &e.UserEmail, &e.Role, &roles, &e.Reason,
		&e.ValidUntil, &e.Status, &e.StatusMessage, &e.ApprovalID, &e.RequestedBy,
		&e.DecidedBy, &e.DecisionComment, &e.DecidedAt, &e.RevokedAt, &e.CreatedAt)
	e.ConflictingRoles = []string(roles)
	return e, err
}

func (a *App) getException(ctx context.Context, id string) (roleConflictException, error) {
	return scanException(a.DB.QueryRowContext(ctx, `SELECT `+exceptionCols+`
		FROM role_conflict_exceptions e JOIN users u ON u.id=e.user_id WHERE e.id::text=$1`, id))
}

// requestRoleConflictException stores a pending exception and submits it to
// core. Called by AssignUserRole after the conflict check found pairs.
func (a *App) requestRoleConflictException(w http.ResponseWriter, r *http.Request, schoolID, userID string, req assignRoleRequest, found []security.RoleConflict) {
	claims := common.FromCtx(r)
	reason := strings.TrimSpace(req.ExceptionReason)
	if len([]rune(reason)) < 20 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan pengecualian minimal 20 karakter",
			map[string]string{"exception_reason": "Minimal 20 karakter"})
		return
	}
	if schoolID == "" {
		common.WriteValidationError(w, http.StatusBadRequest, "Pengecualian hanya dapat diajukan untuk peran di sebuah unit",
			map[string]string{"school_id": "Unit wajib"})
		return
	}
	var validUntil any
	if v := strings.TrimSpace(req.ValidUntil); v != "" {
		d, err := time.Parse("2006-01-02", v)
		if err != nil || !d.After(time.Now()) {
			common.WriteValidationError(w, http.StatusBadRequest, "Tanggal berlaku harus setelah hari ini (format YYYY-MM-DD)",
				map[string]string{"valid_until": "Tanggal tidak valid"})
			return
		}
		validUntil = v
	}
	others := make([]string, 0, len(found))
	for _, c := range found {
		if c.RoleA == req.Role {
			others = append(others, c.RoleB)
		} else {
			others = append(others, c.RoleA)
		}
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(), `INSERT INTO role_conflict_exceptions
			(school_id, user_id, role, conflicting_roles, reason, valid_until, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		schoolID, userID, req.Role, stringArray(others), reason, validUntil, nullUUID(claims.UserID)).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "Sudah ada pengajuan pengecualian yang menunggu atau berlaku untuk peran ini")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan pengajuan pengecualian")
		return
	}
	var name, email string
	_ = a.DB.QueryRowContext(r.Context(), `SELECT name, email FROM users WHERE id=$1`, userID).Scan(&name, &email)
	subject := userID
	approvalID, subErr := a.submitApproval(withCorrelation(r.Context(), r), approvalSubmission{
		SchoolID:      schoolID,
		RequestType:   reqTypeRoleConflictException,
		SourceService: "identity",
		SourceID:      id,
		Title:         fmt.Sprintf("Pengecualian SoD: %s mendapat peran %s", name, req.Role),
		Summary: map[string]any{
			"user":              name,
			"email":             email,
			"role":              req.Role,
			"conflicting_roles": others,
			"reason":            reason,
			"valid_until":       validUntil,
		},
		MakerUserID:    claims.UserID,
		SubjectUserID:  &subject,
		CallbackURL:    a.selfURL() + "/internal/role-conflict-exceptions/approval-callback",
		CorrelationID:  correlationOr(r, id),
		IdempotencyKey: "role-conflict-exception:" + id,
	})
	if subErr != nil {
		msg := "Pengajuan tersimpan tetapi belum terkirim ke Approval Center. " + upstreamMessage("Approval Center", subErr)
		_, _ = a.DB.ExecContext(r.Context(), `UPDATE role_conflict_exceptions SET status='failed', status_message=$2, updated_at=now() WHERE id=$1`, id, msg)
		ex, _ := a.getException(r.Context(), id)
		common.WriteJSONMsg(w, http.StatusBadGateway, ex, nil, msg)
		return
	}
	_, _ = a.DB.ExecContext(r.Context(), `UPDATE role_conflict_exceptions SET approval_id=$2, updated_at=now() WHERE id=$1`, id, approvalID)
	sid := schoolID
	a.RecordSecurityAudit(&sid, "role_conflict_exception_requested", userID, &claims.UserID, &claims.ActiveRole, nil,
		map[string]any{"exception_id": id, "role": req.Role, "conflicting_roles": others, "approval_id": approvalID}, reason, r.Header.Get("X-Correlation-ID"))
	ex, err := a.getException(r.Context(), id)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengajuan")
		return
	}
	common.WriteJSONMsg(w, http.StatusAccepted, ex, nil,
		"Pengecualian diajukan ke Approval Center. Peran baru aktif setelah disetujui")
}

func correlationOr(r *http.Request, fallback string) string {
	if c := r.Header.Get("X-Correlation-ID"); c != "" {
		return c
	}
	return fallback
}

// ListRoleConflictExceptions: GET /security/role-conflict-exceptions.
func (a *App) ListRoleConflictExceptions(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	scope, _ := resolveSchoolScope(r)
	status := r.URL.Query().Get("status")
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+exceptionCols+`
		FROM role_conflict_exceptions e JOIN users u ON u.id=e.user_id
		WHERE ($1='' OR e.school_id::text=$1) AND ($2='' OR e.status=$2)
		ORDER BY e.created_at DESC LIMIT 200`, scope, status)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengecualian")
		return
	}
	defer rows.Close()
	out := []roleConflictException{}
	for rows.Next() {
		e, err := scanException(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat pengecualian")
			return
		}
		out = append(out, e)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// RevokeRoleConflictException: POST /security/role-conflict-exceptions/{id}/revoke {reason}.
// pending/failed -> withdrawn (core request cancelled); approved -> ended and
// the excepted role is taken off the account again.
func (a *App) RevokeRoleConflictException(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Reason = strings.TrimSpace(body.Reason)
	if len([]rune(body.Reason)) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan pencabutan minimal 10 karakter", map[string]string{"reason": "Minimal 10 karakter"})
		return
	}
	ex, err := a.getException(r.Context(), chi.URLParam(r, "id"))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && claims.ActiveRole != common.RoleSuperadmin && ex.SchoolID != claims.SchoolID) {
		common.WriteError(w, http.StatusNotFound, "Pengecualian tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengecualian")
		return
	}
	switch ex.Status {
	case "pending", "failed":
		if ex.ApprovalID != nil {
			_ = a.cancelApproval(withCorrelation(r.Context(), r), *ex.ApprovalID, claims.UserID, body.Reason)
		}
	case "approved":
	default:
		common.WriteError(w, http.StatusConflict, "Pengecualian ini sudah selesai ("+ex.Status+")")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencabut pengecualian")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE role_conflict_exceptions SET status='revoked', revoked_at=now(), revoked_by=$2,
			status_message=$3, updated_at=now() WHERE id=$1 AND status=$4`, ex.ID, nullUUID(claims.UserID), body.Reason, ex.Status)
	if err != nil {
		common.WriteDBError(w, err, "Gagal mencabut pengecualian")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusConflict, "Status pengecualian berubah. Muat ulang halaman")
		return
	}
	removed := false
	if ex.Status == "approved" {
		res, err := tx.Exec(`DELETE FROM user_school_roles WHERE user_id=$1 AND school_id=$2 AND role=$3`, ex.UserID, ex.SchoolID, ex.Role)
		if err != nil {
			common.WriteDBError(w, err, "Gagal mencabut pengecualian")
			return
		}
		n, _ := res.RowsAffected()
		removed = n > 0
		if removed {
			if _, err := tx.Exec(`UPDATE users SET tokens_valid_after=now() WHERE id=$1`, ex.UserID); err != nil {
				common.WriteDBError(w, err, "Gagal mencabut pengecualian")
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencabut pengecualian")
		return
	}
	sid := ex.SchoolID
	a.RecordSecurityAudit(&sid, "role_conflict_exception_revoked", ex.UserID, &claims.UserID, &claims.ActiveRole,
		map[string]any{"status": ex.Status}, map[string]any{"exception_id": ex.ID, "role_removed": removed}, body.Reason, r.Header.Get("X-Correlation-ID"))
	msg := "Pengajuan pengecualian dibatalkan"
	if ex.Status == "approved" {
		msg = "Pengecualian dicabut dan peran " + ex.Role + " dilepas dari akun"
	}
	out, _ := a.getException(r.Context(), ex.ID)
	common.WriteJSONMsg(w, http.StatusOK, out, nil, msg)
}

// RoleConflictExceptionCallback: POST /internal/role-conflict-exceptions/approval-callback.
// Idempotent: a decision for an exception that is no longer pending is
// acknowledged without change.
func (a *App) RoleConflictExceptionCallback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ApprovalID  string `json:"approval_id"`
		SourceID    string `json:"source_id"`
		RequestType string `json:"request_type"`
		Decision    string `json:"decision"`
		DecidedBy   string `json:"decided_by"`
		Comment     string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || !rules.IsUUID(in.SourceID) {
		common.WriteError(w, http.StatusBadRequest, "payload callback tidak valid")
		return
	}
	ex, err := a.getException(r.Context(), in.SourceID)
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "pengecualian tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "gagal memuat pengecualian")
		return
	}
	if ex.Status != "pending" && ex.Status != "failed" {
		common.WriteJSON(w, http.StatusOK, map[string]string{"status": ex.Status}, nil)
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memproses keputusan")
		return
	}
	defer tx.Rollback()
	newStatus := "rejected"
	switch in.Decision {
	case "approved":
		newStatus = "approved"
		if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)
				ON CONFLICT (user_id, school_id, role) DO UPDATE SET status='active', deactivated_at=NULL`,
			ex.UserID, ex.SchoolID, ex.Role); err != nil {
			common.WriteDBError(w, err, "gagal menerapkan peran")
			return
		}
	case "rejected", "cancelled", "canceled", "expired":
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "keputusan tidak dikenal: "+in.Decision)
		return
	}
	if _, err := tx.Exec(`UPDATE role_conflict_exceptions SET status=$2, approval_id=COALESCE(NULLIF($3,''), approval_id),
			decided_by=NULLIF($4,''), decision_comment=NULLIF($5,''), decided_at=now(), status_message=NULL, updated_at=now()
		WHERE id=$1`, ex.ID, newStatus, in.ApprovalID, in.DecidedBy, in.Comment); err != nil {
		common.WriteDBError(w, err, "gagal menyimpan keputusan")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan keputusan")
		return
	}
	sid := ex.SchoolID
	a.RecordSecurityAudit(&sid, "role_conflict_exception_"+newStatus, ex.UserID, nullStr(in.DecidedBy), nil, nil,
		map[string]any{"exception_id": ex.ID, "role": ex.Role, "approval_id": in.ApprovalID}, in.Comment, r.Header.Get("X-Correlation-ID"))
	if ex.RequestedBy != nil {
		title := "Pengecualian peran disetujui"
		body := fmt.Sprintf("Peran %s untuk %s telah aktif.", ex.Role, ex.UserName)
		if newStatus != "approved" {
			title = "Pengecualian peran ditolak"
			body = fmt.Sprintf("Pengajuan peran %s untuk %s ditolak. %s", ex.Role, ex.UserName, in.Comment)
		}
		go func(requester string) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = a.notifyUser(ctx, ex.SchoolID, requester, "role_conflict_exception", title, body,
				"role-conflict-exception:"+ex.ID+":"+newStatus, map[string]any{"exception_id": ex.ID})
		}(*ex.RequestedBy)
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": newStatus}, nil)
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type sodCleanupRow struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	UserName   *string    `json:"user_name"`
	UserEmail  *string    `json:"user_email"`
	SchoolID   *string    `json:"school_id"`
	Role       string     `json:"role"`
	RemovedAt  time.Time  `json:"removed_at"`
	Reason     string     `json:"reason"`
	RestoredAt *time.Time `json:"restored_at"`
}

// ListSoDCleanupLog: GET /security/role-conflicts/cleanup-log (superadmin).
func (a *App) ListSoDCleanupLog(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r) {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT l.id, l.user_id, u.name, u.email, l.school_id::text, l.role,
			l.removed_at, l.reason, l.restored_at
		FROM role_sod_cleanup_log l LEFT JOIN users u ON u.id=l.user_id
		ORDER BY l.removed_at DESC, u.name LIMIT 500`)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat riwayat pembersihan")
		return
	}
	defer rows.Close()
	out := []sodCleanupRow{}
	for rows.Next() {
		var c sodCleanupRow
		if err := rows.Scan(&c.ID, &c.UserID, &c.UserName, &c.UserEmail, &c.SchoolID, &c.Role, &c.RemovedAt, &c.Reason, &c.RestoredAt); err != nil {
			common.WriteDBError(w, err, "Gagal memuat riwayat pembersihan")
			return
		}
		out = append(out, c)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// ResolveRoleConflictViolations: POST /security/role-conflicts/violations/resolve {reason}
// (superadmin). Runs the same clean-up as migration 0022: per account the
// role in most conflicts is removed until no unexcepted pair is left; every
// removed row is logged in role_sod_cleanup_log.
func (a *App) ResolveRoleConflictViolations(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r) {
		return
	}
	claims := common.FromCtx(r)
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Reason = strings.TrimSpace(body.Reason)
	if len([]rune(body.Reason)) < 10 {
		common.WriteValidationError(w, http.StatusBadRequest, "Alasan minimal 10 karakter", map[string]string{"reason": "Minimal 10 karakter"})
		return
	}
	var n int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT sod_resolve_violations($1)`, body.Reason).Scan(&n); err != nil {
		common.WriteDBError(w, err, "Gagal merapikan kombinasi peran")
		return
	}
	actor, role := "", ""
	if claims != nil {
		actor, role = claims.UserID, claims.ActiveRole
	}
	a.RecordSecurityAudit(nil, "role_conflicts_resolved", "", &actor, &role, nil, map[string]any{"roles_removed": n}, body.Reason, r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusOK, map[string]int{"roles_removed": n}, nil,
		fmt.Sprintf("%d peran dilepas dari akun yang melanggar SoD", n))
}

// RunRoleConflictExceptionExpiry ends approved exceptions whose valid_until
// has passed: the excepted role is taken off the account again so the
// forbidden combination does not silently outlive its approval. Run by the
// main.go minute ticker; returns how many were ended.
func (a *App) RunRoleConflictExceptionExpiry(ctx context.Context) int {
	rows, err := a.DB.QueryContext(ctx, `SELECT id FROM role_conflict_exceptions
		WHERE status='approved' AND valid_until IS NOT NULL AND valid_until < (now() AT TIME ZONE 'Asia/Jakarta')::date`)
	if err != nil {
		common.CaptureError(err)
		return 0
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if a.expireException(ctx, id) {
			n++
		}
	}
	return n
}

func (a *App) expireException(ctx context.Context, id string) bool {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		common.CaptureError(err)
		return false
	}
	defer tx.Rollback()
	var userID, schoolID, role string
	err = tx.QueryRowContext(ctx, `UPDATE role_conflict_exceptions SET status='revoked', revoked_at=now(),
			status_message='Masa berlaku pengecualian berakhir', updated_at=now()
		WHERE id=$1 AND status='approved' RETURNING user_id, school_id, role`, id).Scan(&userID, &schoolID, &role)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			common.CaptureError(err)
		}
		return false
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM user_school_roles WHERE user_id=$1 AND school_id=$2 AND role=$3`, userID, schoolID, role)
	if err != nil {
		common.CaptureError(err)
		return false
	}
	if k, _ := res.RowsAffected(); k > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET tokens_valid_after=now() WHERE id=$1`, userID); err != nil {
			common.CaptureError(err)
			return false
		}
	}
	if err := tx.Commit(); err != nil {
		common.CaptureError(err)
		return false
	}
	a.RecordSecurityAudit(&schoolID, "role_conflict_exception_expired", userID, nil, nil, nil,
		map[string]any{"exception_id": id, "role": role}, "valid_until terlewati", "")
	return true
}
