package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/security"
	"skci/identity/internal/store"
)

// Session & account security endpoints (QA-IDN: BUG-022/024/025/083/023).

// policyCache keeps the security_policy row for a short time; every request
// consults it (idle timeout), and SaveSecurityPolicy invalidates it.
type policyCache struct {
	mu      sync.Mutex
	policy  security.Policy
	fetched time.Time
}

const policyCacheTTL = 30 * time.Second

func (a *App) policy(r *http.Request) security.Policy {
	a.policyCache.mu.Lock()
	defer a.policyCache.mu.Unlock()
	if time.Since(a.policyCache.fetched) < policyCacheTTL {
		return a.policyCache.policy
	}
	p, err := store.LoadPolicy(r.Context(), a.DB)
	if err != nil {
		common.CaptureError(err)
		return security.DefaultPolicy()
	}
	a.policyCache.policy, a.policyCache.fetched = p, time.Now()
	return p
}

func (a *App) invalidatePolicy() {
	a.policyCache.mu.Lock()
	a.policyCache.fetched = time.Time{}
	a.policyCache.mu.Unlock()
}

// ValidateSession is the common.SessionValidator wired into every JWT route of
// identity and, through /auth/session-check, into every other service.
func (a *App) ValidateSession(r *http.Request, c *common.Claims) string {
	st, err := store.LoadSessionState(r.Context(), a.DB, c.UserID, c.ActiveRole, c.SchoolID, c.SessionID)
	if err != nil {
		// Fail closed: an unverifiable session is not trusted.
		common.CaptureError(err)
		return "session_unverifiable"
	}
	if c.IssuedAt != nil {
		iat := c.IssuedAt.Time
		st.IssuedAt = &iat
	}
	p := a.policy(r)
	reason := security.CheckSession(st, p, time.Now())
	if reason == "" {
		if c.SessionID != "" {
			if err := store.TouchSession(r.Context(), a.DB, c.SessionID); err != nil {
				common.CaptureError(err)
			}
		}
		return ""
	}
	if c.SessionID != "" && st.SessionFound && !st.SessionRevoked {
		// Record why the session ended so it shows in the user's history.
		_, _ = store.RevokeSession(r.Context(), a.DB, c.SessionID, c.UserID, reason)
	}
	return reason
}

// SessionCheck: GET /auth/session-check, called by nginx auth_request before
// proxying to any other service (web/nginx.conf). 204 = let the request
// through, 401 = refuse. Requests without a bearer token, and bearer tokens
// identity did not sign (admission guardian / exam participant tokens use
// their own secrets), pass through untouched: the target service still does
// its own authentication; this only adds revocation on top.
func (a *App) SessionCheck(w http.ResponseWriter, r *http.Request) {
	tok := common.BearerToken(r)
	if tok == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	claims, err := common.ParseToken(tok, a.JWTSecret)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if claims.ActiveRole == "" || claims.UserID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if reason := a.ValidateSession(r, claims); reason != "" {
		w.Header().Set("X-Session-Reject-Reason", reason)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// issueToken mints an access token. sessionID "" opens a new session row;
// otherwise the existing session is kept (role switch) with its expiry.
func (a *App) issueToken(r *http.Request, userID, schoolID, activeRole string, roles []string, sessionID string, mfa bool) (string, string, error) {
	ctx := r.Context()
	p := a.policy(r)
	var expiresAt time.Time
	if sessionID != "" {
		exp, sessMFA, err := store.SessionExpiry(ctx, a.DB, sessionID, userID)
		if err != nil {
			return "", "", err
		}
		expiresAt, mfa = exp, sessMFA
	} else {
		expiresAt = time.Now().Add(time.Duration(p.SessionMaxHours) * time.Hour)
		sid, err := store.CreateSession(ctx, a.DB, userID, expiresAt, clientIP(r), truncate(r.UserAgent(), 300), mfa)
		if err != nil {
			return "", "", err
		}
		sessionID = sid
	}
	// Display names ride in the token so the header shows them instead of UUIDs.
	var name, schoolName string
	_ = a.DB.QueryRowContext(ctx, `SELECT name FROM users WHERE id=$1`, userID).Scan(&name)
	if schoolID != "" {
		_ = a.DB.QueryRowContext(ctx, `SELECT name FROM schools WHERE id::text=$1`, schoolID).Scan(&schoolName)
	}
	claims := common.Claims{
		UserID:     userID,
		Name:       name,
		SchoolName: schoolName,
		SchoolID:   schoolID,
		ActiveRole: activeRole,
		Roles:      roles,
		SessionID:  sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.JWTSecret))
	return signed, sessionID, err
}

// revokeAll ends every session of a user and writes a security audit event.
func (a *App) revokeAll(r *http.Request, userID, reason, keepSessionID string) {
	n, err := store.RevokeAllSessions(r.Context(), a.DB, userID, reason, keepSessionID)
	if err != nil {
		common.CaptureError(err)
		return
	}
	var actor, role *string
	if c := common.FromCtx(r); c != nil {
		actor, role = &c.UserID, &c.ActiveRole
	}
	a.RecordSecurityAudit(nil, "sessions_revoked", userID, actor, role, nil, map[string]any{"revoked_sessions": n, "reason": reason}, reason, r.Header.Get("X-Correlation-ID"))
}

// Logout: POST /auth/logout — ends the caller's current session.
func (a *App) Logout(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if c.SessionID != "" {
		if _, err := store.RevokeSession(r.Context(), a.DB, c.SessionID, c.UserID, "logout"); err != nil {
			common.WriteDBError(w, err, "gagal keluar")
			return
		}
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Berhasil keluar"}, nil)
}

// MySessions: GET /auth/sessions — the caller's active sessions.
func (a *App) MySessions(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	out, err := store.ListActiveSessions(r.Context(), a.DB, c.UserID, c.SessionID, a.policy(r).IdleTimeoutMinutes)
	if err != nil {
		common.WriteDBError(w, err, "gagal memuat sesi")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// RevokeMySession: DELETE /auth/sessions/{sid}.
func (a *App) RevokeMySession(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	ok, err := store.RevokeSession(r.Context(), a.DB, chi.URLParam(r, "sid"), c.UserID, "revoked_by_user")
	if err != nil {
		common.WriteDBError(w, err, "gagal mengakhiri sesi")
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "Sesi tidak ditemukan atau sudah berakhir")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Sesi diakhiri"}, nil)
}

type revokeAllRequest struct {
	KeepCurrent bool `json:"keep_current"`
}

// RevokeAllMySessions: POST /auth/sessions/revoke-all — "keluar dari semua
// sesi". keep_current=true keeps the device the user is on.
func (a *App) RevokeAllMySessions(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	var req revokeAllRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	keep := ""
	if req.KeepCurrent {
		keep = c.SessionID
	}
	a.revokeAll(r, c.UserID, "logout_all", keep)
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Semua sesi diakhiri"}, nil)
}

// MyLoginHistory: GET /auth/login-history.
func (a *App) MyLoginHistory(w http.ResponseWriter, r *http.Request) {
	a.writeLoginHistory(w, r, common.FromCtx(r).UserID)
}

func (a *App) writeLoginHistory(w http.ResponseWriter, r *http.Request, userID string) {
	page, perPage := common.ParsePage(r)
	out, total, err := store.ListLoginEvents(r.Context(), a.DB, userID, perPage, (page-1)*perPage)
	if err != nil {
		common.WriteDBError(w, err, "gagal memuat riwayat login")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// userInScope: superadmin manages everyone; an admin only accounts holding a
// role at their own unit (closes the by-ID reach across units).
func (a *App) userInScope(r *http.Request, userID string) (bool, error) {
	c := common.FromCtx(r)
	if c == nil {
		return false, nil
	}
	if c.ActiveRole == common.RoleSuperadmin {
		var exists bool
		err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id::text=$1 AND deleted_at IS NULL)`, userID).Scan(&exists)
		return exists, err
	}
	var exists bool
	err := a.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM user_school_roles usr JOIN users u ON u.id = usr.user_id
		WHERE usr.user_id::text=$1 AND usr.school_id::text=$2 AND u.deleted_at IS NULL)`, userID, c.SchoolID).Scan(&exists)
	return exists, err
}

// requireUserInScope combines the admin role check with userInScope.
func (a *App) requireUserInScope(w http.ResponseWriter, r *http.Request, userID string) bool {
	if !requireRole(w, r, common.RoleAdmin) {
		return false
	}
	ok, err := a.userInScope(r, userID)
	if err != nil {
		common.WriteDBError(w, err, "gagal memeriksa pengguna")
		return false
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "user not found")
		return false
	}
	return true
}

// UserSessions: GET /users/{id}/sessions (admin of the user's unit).
func (a *App) UserSessions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !a.requireUserInScope(w, r, id) {
		return
	}
	out, err := store.ListActiveSessions(r.Context(), a.DB, id, "", a.policy(r).IdleTimeoutMinutes)
	if err != nil {
		common.WriteDBError(w, err, "gagal memuat sesi")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// UserLoginHistory: GET /users/{id}/login-history.
func (a *App) UserLoginHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !a.requireUserInScope(w, r, id) {
		return
	}
	a.writeLoginHistory(w, r, id)
}

// RevokeUserSessions: POST /users/{id}/sessions/revoke-all (force logout).
func (a *App) RevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !a.requireUserInScope(w, r, id) {
		return
	}
	a.revokeAll(r, id, "revoked_by_admin", "")
	common.WriteJSON(w, http.StatusOK, map[string]string{"user_id": id, "message": "Semua sesi pengguna diakhiri"}, nil)
}

// UnlockUserAccount: POST /users/{id}/unlock — clears a login lockout.
func (a *App) UnlockUserAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !a.requireUserInScope(w, r, id) {
		return
	}
	if _, err := store.UnlockUser(r.Context(), a.DB, id); err != nil {
		common.WriteDBError(w, err, "gagal membuka kunci akun")
		return
	}
	c := common.FromCtx(r)
	a.RecordSecurityAudit(nil, "account_unlocked", id, &c.UserID, &c.ActiveRole, nil, nil, "admin unlocked account", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]string{"user_id": id, "message": "Kunci akun dibuka"}, nil)
}

// GetSecurityPolicy: GET /security/policy (superadmin).
func (a *App) GetSecurityPolicy(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	p, err := store.LoadPolicy(r.Context(), a.DB)
	if err != nil {
		common.WriteDBError(w, err, "gagal memuat kebijakan keamanan")
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

// UpdateSecurityPolicy: PUT /security/policy (superadmin).
func (a *App) UpdateSecurityPolicy(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	var p security.Policy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if errs := security.ValidatePolicy(p, common.IsValidRole); len(errs) > 0 {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, "Kebijakan keamanan tidak valid", errs)
		return
	}
	c := common.FromCtx(r)
	before, _ := store.LoadPolicy(r.Context(), a.DB)
	saved, err := store.SavePolicy(r.Context(), a.DB, p, c.UserID)
	if err != nil {
		common.WriteDBError(w, err, "gagal menyimpan kebijakan keamanan")
		return
	}
	a.invalidatePolicy()
	a.RecordSecurityAudit(nil, "security_policy_updated", "security_policy", &c.UserID, &c.ActiveRole, before, saved, "updated security policy", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, saved, nil)
}

// ListRoleConflicts: GET /security/role-conflicts (admin, superadmin).
func (a *App) ListRoleConflicts(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	out, err := store.ListRoleConflicts(r.Context(), a.DB)
	if err != nil {
		common.WriteDBError(w, err, "gagal memuat aturan kombinasi peran")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// UpsertRoleConflict: POST /security/role-conflicts (superadmin).
func (a *App) UpsertRoleConflict(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	var c security.RoleConflict
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	errs := map[string]string{}
	if !common.IsValidRole(c.RoleA) {
		errs["role_a"] = "Peran tidak dikenal"
	}
	if !common.IsValidRole(c.RoleB) {
		errs["role_b"] = "Peran tidak dikenal"
	}
	if c.RoleA == c.RoleB {
		errs["role_b"] = "Dua peran harus berbeda"
	}
	if strings.TrimSpace(c.Reason) == "" {
		errs["reason"] = "Alasan wajib diisi"
	}
	if len(errs) > 0 {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, "Aturan kombinasi peran tidak valid", errs)
		return
	}
	if err := store.UpsertRoleConflict(r.Context(), a.DB, c); err != nil {
		common.WriteDBError(w, err, "gagal menyimpan aturan kombinasi peran")
		return
	}
	cl := common.FromCtx(r)
	a.RecordSecurityAudit(nil, "role_conflict_saved", c.RoleA+"+"+c.RoleB, &cl.UserID, &cl.ActiveRole, nil, c, c.Reason, r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusCreated, c, nil)
}

// DeleteRoleConflict: DELETE /security/role-conflicts/{roleA}/{roleB} (superadmin).
func (a *App) DeleteRoleConflict(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	ra, rb := chi.URLParam(r, "roleA"), chi.URLParam(r, "roleB")
	ok, err := store.DeleteRoleConflict(r.Context(), a.DB, ra, rb)
	if err != nil {
		common.WriteDBError(w, err, "gagal menghapus aturan kombinasi peran")
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "Aturan tidak ditemukan")
		return
	}
	cl := common.FromCtx(r)
	a.RecordSecurityAudit(nil, "role_conflict_deleted", ra+"+"+rb, &cl.UserID, &cl.ActiveRole, map[string]string{"role_a": ra, "role_b": rb}, nil, "deleted SoD rule", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]string{"role_a": ra, "role_b": rb}, nil)
}

// ListRoleConflictViolations: GET /security/role-conflicts/violations —
// accounts that already break a rule (assigned before SoD existed).
func (a *App) ListRoleConflictViolations(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	schoolID, _ := resolveSchoolScope(r)
	out, err := store.ListRoleConflictViolations(r.Context(), a.DB, schoolID)
	if err != nil {
		common.WriteDBError(w, err, "gagal memuat pelanggaran kombinasi peran")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// checkRoleConflicts writes 409 and returns false when granting role to
// userID would create a forbidden combination (BUG-023, AC-RBAC-08).
func (a *App) checkRoleConflicts(w http.ResponseWriter, r *http.Request, userID, role string) bool {
	found, err := a.findRoleConflicts(r.Context(), userID, role)
	if err != nil {
		common.WriteDBError(w, err, "gagal memeriksa kombinasi peran")
		return false
	}
	if len(found) == 0 {
		return true
	}
	writeRoleConflict(w, found)
	return false
}

// writeRoleConflict is the 409 for a forbidden combination. The UI offers
// "Ajukan pengecualian" (exception_allowed) which goes to the Approval Center.
func writeRoleConflict(w http.ResponseWriter, found []security.RoleConflict) {
	msgs := make([]string, 0, len(found))
	for _, f := range found {
		msgs = append(msgs, f.RoleA+" + "+f.RoleB+": "+f.Reason)
	}
	msg := "Kombinasi peran tidak diizinkan (" + strings.Join(msgs, "; ") + ")"
	common.WriteValidationError(w, http.StatusConflict, msg, map[string]any{"role": msg, "conflicts": found, "exception_allowed": true})
}

// findRoleConflicts lists the forbidden pairs granting role to userID would
// create, minus the pairs covered by an approved, unexpired exception.
func (a *App) findRoleConflicts(ctx context.Context, userID, role string) ([]security.RoleConflict, error) {
	held, err := store.HeldRoleNames(ctx, a.DB, userID)
	if err != nil {
		return nil, err
	}
	rules, err := store.ListRoleConflicts(ctx, a.DB)
	if err != nil {
		return nil, err
	}
	excepted, err := store.ExceptedRolePairs(ctx, a.DB, userID)
	if err != nil {
		return nil, err
	}
	return security.WithoutExcepted(security.ConflictsFor(held, role, rules), excepted), nil
}

func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
