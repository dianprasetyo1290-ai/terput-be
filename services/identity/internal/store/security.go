package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	"skci/identity/internal/security"
)

// ---- policy -----------------------------------------------------------------

// LoadPolicy reads the single security_policy row. Callers fall back to
// security.DefaultPolicy() on error so a DB hiccup never weakens the rules.
func LoadPolicy(ctx context.Context, db *sql.DB) (security.Policy, error) {
	var p security.Policy
	var roles []string
	err := db.QueryRowContext(ctx, `SELECT max_failed_logins, lockout_minutes, idle_timeout_minutes, session_max_hours,
			reset_token_ttl_minutes, mfa_roles, updated_at FROM security_policy WHERE id = 1`).
		Scan(&p.MaxFailedLogins, &p.LockoutMinutes, &p.IdleTimeoutMinutes, &p.SessionMaxHours, &p.ResetTokenTTLMinutes, pq.Array(&roles), &p.UpdatedAt)
	p.MFARoles = roles
	return p, err
}

// PolicyOrDefault never fails: an unreadable row yields the seeded defaults.
func PolicyOrDefault(ctx context.Context, db *sql.DB) security.Policy {
	p, err := LoadPolicy(ctx, db)
	if err != nil {
		return security.DefaultPolicy()
	}
	return p
}

func SavePolicy(ctx context.Context, db *sql.DB, p security.Policy, actorID string) (security.Policy, error) {
	_, err := db.ExecContext(ctx, `UPDATE security_policy SET max_failed_logins=$1, lockout_minutes=$2, idle_timeout_minutes=$3,
			session_max_hours=$4, reset_token_ttl_minutes=$5, mfa_roles=$6, updated_at=now(), updated_by=$7 WHERE id = 1`,
		p.MaxFailedLogins, p.LockoutMinutes, p.IdleTimeoutMinutes, p.SessionMaxHours, p.ResetTokenTTLMinutes, pq.Array(nonNil(p.MFARoles)), nullable(actorID))
	if err != nil {
		return p, err
	}
	return LoadPolicy(ctx, db)
}

// ---- login state ------------------------------------------------------------

// LoginState is the security part of a users row.
type LoginState struct {
	FailedCount int
	LockedUntil *time.Time
	MFAEnabled  bool
	MFASecret   *string
}

func GetLoginState(ctx context.Context, db *sql.DB, userID string) (LoginState, error) {
	var s LoginState
	var mfaAt *time.Time
	err := db.QueryRowContext(ctx, `SELECT failed_login_count, locked_until, mfa_enabled_at, mfa_secret_enc FROM users WHERE id = $1`, userID).
		Scan(&s.FailedCount, &s.LockedUntil, &mfaAt, &s.MFASecret)
	s.MFAEnabled = mfaAt != nil && s.MFASecret != nil
	return s, err
}

// SaveLoginFailure persists the result of security.RegisterFailure.
func SaveLoginFailure(ctx context.Context, db *sql.DB, userID string, res security.LockoutResult) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET failed_login_count=$1, locked_until=COALESCE($2, locked_until) WHERE id=$3`,
		res.FailedCount, res.LockedUntil, userID)
	return err
}

func ClearLoginFailures(ctx context.Context, db *sql.DB, userID string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET failed_login_count=0, locked_until=NULL WHERE id=$1 AND (failed_login_count<>0 OR locked_until IS NOT NULL)`, userID)
	return err
}

// UnlockUser is the admin action for a locked account.
func UnlockUser(ctx context.Context, db *sql.DB, userID string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE users SET failed_login_count=0, locked_until=NULL WHERE id=$1 AND deleted_at IS NULL`, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// LoginEvent is one login_events row.
type LoginEvent struct {
	ID        string    `json:"id"`
	UserID    *string   `json:"user_id"`
	Email     string    `json:"email"`
	Success   bool      `json:"success"`
	Reason    string    `json:"reason"`
	IP        *string   `json:"ip"`
	UserAgent *string   `json:"user_agent"`
	SessionID *string   `json:"session_id"`
	CreatedAt time.Time `json:"created_at"`
}

func RecordLoginEvent(ctx context.Context, db *sql.DB, userID *string, email string, success bool, reason, ip, ua string, sessionID *string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO login_events (user_id, email, success, reason, ip, user_agent, session_id) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		userID, email, success, reason, nullable(ip), nullable(ua), sessionID)
	return err
}

func ListLoginEvents(ctx context.Context, db *sql.DB, userID string, limit, offset int) ([]LoginEvent, int, error) {
	var total int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM login_events WHERE user_id=$1`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, user_id, email, success, reason, ip, user_agent, session_id, created_at
		FROM login_events WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []LoginEvent{}
	for rows.Next() {
		var e LoginEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.Email, &e.Success, &e.Reason, &e.IP, &e.UserAgent, &e.SessionID, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ---- sessions ---------------------------------------------------------------

// Session is one user_sessions row.
type Session struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSeenAt    time.Time  `json:"last_seen_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
	RevokedReason *string    `json:"revoked_reason"`
	IP            *string    `json:"ip"`
	UserAgent     *string    `json:"user_agent"`
	MFAVerified   bool       `json:"mfa_verified"`
	Current       bool       `json:"current"`
}

func CreateSession(ctx context.Context, db *sql.DB, userID string, expiresAt time.Time, ip, ua string, mfa bool) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `INSERT INTO user_sessions (user_id, expires_at, ip, user_agent, mfa_verified) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		userID, expiresAt, nullable(ip), nullable(ua), mfa).Scan(&id)
	return id, err
}

// SessionExpiry returns a live session's expiry so a role switch can keep it.
func SessionExpiry(ctx context.Context, db *sql.DB, sessionID, userID string) (time.Time, bool, error) {
	var exp time.Time
	var mfa bool
	err := db.QueryRowContext(ctx, `SELECT expires_at, mfa_verified FROM user_sessions WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, sessionID, userID).Scan(&exp, &mfa)
	return exp, mfa, err
}

// LoadSessionState gathers everything security.CheckSession needs in one
// round trip. schoolID "" means the role row is the unit-less superadmin one.
func LoadSessionState(ctx context.Context, db *sql.DB, userID, activeRole, schoolID, sessionID string) (security.SessionState, error) {
	var st security.SessionState
	var deleted, deactivated bool
	var sessID *string
	var revokedAt, expiresAt, lastSeen *time.Time
	err := db.QueryRowContext(ctx, `
		SELECT u.deleted_at IS NOT NULL,
		       (u.deactivate_at IS NOT NULL AND u.deactivate_at <= now()),
		       u.tokens_valid_after,
		       EXISTS (SELECT 1 FROM user_school_roles r WHERE r.user_id = u.id AND r.role = $2 AND r.status = 'active'
		               AND (($3 = '' AND r.school_id IS NULL) OR r.school_id::text = $3)),
		       s.id, s.revoked_at, s.expires_at, s.last_seen_at
		FROM users u
		LEFT JOIN user_sessions s ON $4 <> '' AND s.id::text = $4 AND s.user_id = u.id
		WHERE u.id::text = $1`, userID, activeRole, schoolID, sessionID).
		Scan(&deleted, &deactivated, &st.TokensValidAfter, &st.RoleHeld, &sessID, &revokedAt, &expiresAt, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil // UserExists=false
	}
	if err != nil {
		return st, err
	}
	st.UserExists = !deleted
	st.UserActive = !deactivated
	st.HasSessionID = sessionID != ""
	st.SessionFound = sessID != nil
	st.SessionRevoked = revokedAt != nil
	if expiresAt != nil {
		st.SessionExpiresAt = *expiresAt
	}
	if lastSeen != nil {
		st.SessionLastSeenAt = *lastSeen
	}
	return st, nil
}

// TouchSession records activity at most once a minute per session so the
// idle check stays accurate without a write on every request.
func TouchSession(ctx context.Context, db *sql.DB, sessionID string) error {
	_, err := db.ExecContext(ctx, `UPDATE user_sessions SET last_seen_at = now()
		WHERE id::text = $1 AND revoked_at IS NULL AND last_seen_at < now() - interval '1 minute'`, sessionID)
	return err
}

func RevokeSession(ctx context.Context, db *sql.DB, sessionID, userID, reason string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE user_sessions SET revoked_at = now(), revoked_reason = $3
		WHERE id::text = $1 AND user_id::text = $2 AND revoked_at IS NULL`, sessionID, userID, reason)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// RevokeAllSessions ends every session of the user except keepSessionID (""
// keeps none) and invalidates every sid-less token issued so far.
func RevokeAllSessions(ctx context.Context, db *sql.DB, userID, reason, keepSessionID string) (int64, error) {
	if _, err := db.ExecContext(ctx, `UPDATE users SET tokens_valid_after = now() WHERE id::text = $1`, userID); err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx, `UPDATE user_sessions SET revoked_at = now(), revoked_reason = $2
		WHERE user_id::text = $1 AND revoked_at IS NULL AND ($3 = '' OR id::text <> $3)`, userID, reason, keepSessionID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListActiveSessions lists sessions that are neither revoked nor expired.
func ListActiveSessions(ctx context.Context, db *sql.DB, userID, currentSessionID string, idleMinutes int) ([]Session, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, user_id, created_at, last_seen_at, expires_at, revoked_at, revoked_reason, ip, user_agent, mfa_verified
		FROM user_sessions WHERE user_id::text = $1 AND revoked_at IS NULL AND expires_at > now()
		  AND last_seen_at > now() - make_interval(mins => $2)
		ORDER BY last_seen_at DESC`, userID, idleMinutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.RevokedAt, &s.RevokedReason, &s.IP, &s.UserAgent, &s.MFAVerified); err != nil {
			return nil, err
		}
		s.Current = s.ID == currentSessionID
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---- MFA --------------------------------------------------------------------

// MFAChallenge is a pending second login step.
type MFAChallenge struct {
	ID               string
	UserID           string
	Purpose          string
	PendingSecretEnc *string
	Attempts         int
	ExpiresAt        time.Time
	UsedAt           *time.Time
}

func CreateMFAChallenge(ctx context.Context, db *sql.DB, userID, tokenHash, purpose string, pendingSecretEnc *string, ttl time.Duration) error {
	_, err := db.ExecContext(ctx, `INSERT INTO mfa_challenges (user_id, token_hash, purpose, pending_secret_enc, expires_at)
		VALUES ($1,$2,$3,$4, now() + make_interval(secs => $5))`, userID, tokenHash, purpose, pendingSecretEnc, int(ttl.Seconds()))
	return err
}

func GetMFAChallenge(ctx context.Context, db *sql.DB, tokenHash string) (MFAChallenge, error) {
	var c MFAChallenge
	err := db.QueryRowContext(ctx, `SELECT id, user_id, purpose, pending_secret_enc, attempts, expires_at, used_at FROM mfa_challenges WHERE token_hash=$1`, tokenHash).
		Scan(&c.ID, &c.UserID, &c.Purpose, &c.PendingSecretEnc, &c.Attempts, &c.ExpiresAt, &c.UsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func IncrementMFAAttempts(ctx context.Context, db *sql.DB, id string) error {
	_, err := db.ExecContext(ctx, `UPDATE mfa_challenges SET attempts = attempts + 1 WHERE id=$1`, id)
	return err
}

// ConsumeMFAChallenge marks the challenge used; false if it was already used
// (two concurrent submissions of the same code).
func ConsumeMFAChallenge(ctx context.Context, db *sql.DB, id string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE mfa_challenges SET used_at = now() WHERE id=$1 AND used_at IS NULL`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func EnableMFA(ctx context.Context, db *sql.DB, userID, secretEnc string) error {
	_, err := db.ExecContext(ctx, `UPDATE users SET mfa_secret_enc=$1, mfa_enabled_at=now() WHERE id=$2`, secretEnc, userID)
	return err
}

func ResetMFA(ctx context.Context, db *sql.DB, userID string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE users SET mfa_secret_enc=NULL, mfa_enabled_at=NULL WHERE id=$1 AND deleted_at IS NULL`, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---- password reset ---------------------------------------------------------

func CreatePasswordResetToken(ctx context.Context, db *sql.DB, userID, tokenHash, ip string, ttl time.Duration) error {
	// A new link supersedes every older unused one.
	if _, err := db.ExecContext(ctx, `UPDATE password_reset_tokens SET used_at = now() WHERE user_id=$1 AND used_at IS NULL`, userID); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO password_reset_tokens (user_id, token_hash, expires_at, requested_ip)
		VALUES ($1,$2, now() + make_interval(secs => $3), $4)`, userID, tokenHash, int(ttl.Seconds()), nullable(ip))
	return err
}

// ResetTokenStatus values.
const (
	ResetTokenValid   = "valid"
	ResetTokenUnknown = "unknown"
	ResetTokenUsed    = "used"
	ResetTokenExpired = "expired"
)

// CheckPasswordResetToken classifies a presented token and returns its user.
func CheckPasswordResetToken(ctx context.Context, db *sql.DB, tokenHash string) (string, string, error) {
	var userID string
	var usedAt *time.Time
	var expired bool
	err := db.QueryRowContext(ctx, `SELECT user_id, used_at, expires_at <= now() FROM password_reset_tokens WHERE token_hash=$1`, tokenHash).
		Scan(&userID, &usedAt, &expired)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", ResetTokenUnknown, nil
	case err != nil:
		return "", "", err
	case usedAt != nil:
		return userID, ResetTokenUsed, nil
	case expired:
		return userID, ResetTokenExpired, nil
	}
	return userID, ResetTokenValid, nil
}

// ConsumePasswordResetToken sets the new password and burns the token in one
// transaction; false when the token was used/expired in the meantime.
func ConsumePasswordResetToken(ctx context.Context, db *sql.DB, tokenHash, newHash string) (string, bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	var userID string
	err = tx.QueryRowContext(ctx, `UPDATE password_reset_tokens SET used_at = now()
		WHERE token_hash=$1 AND used_at IS NULL AND expires_at > now() RETURNING user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=$1, password_changed=true, default_password=NULL,
			failed_login_count=0, locked_until=NULL, updated_at=now() WHERE id=$2 AND deleted_at IS NULL`, newHash, userID); err != nil {
		return "", false, err
	}
	return userID, true, tx.Commit()
}

// ---- role conflicts (SoD) ---------------------------------------------------

func ListRoleConflicts(ctx context.Context, db *sql.DB) ([]security.RoleConflict, error) {
	rows, err := db.QueryContext(ctx, `SELECT role_a, role_b, reason FROM role_conflicts ORDER BY role_a, role_b`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []security.RoleConflict{}
	for rows.Next() {
		var c security.RoleConflict
		if err := rows.Scan(&c.RoleA, &c.RoleB, &c.Reason); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func UpsertRoleConflict(ctx context.Context, db *sql.DB, c security.RoleConflict) error {
	a, b := security.NormalizePair(c.RoleA, c.RoleB)
	_, err := db.ExecContext(ctx, `INSERT INTO role_conflicts (role_a, role_b, reason) VALUES ($1,$2,$3)
		ON CONFLICT (role_a, role_b) DO UPDATE SET reason = EXCLUDED.reason`, a, b, c.Reason)
	return err
}

func DeleteRoleConflict(ctx context.Context, db *sql.DB, roleA, roleB string) (bool, error) {
	a, b := security.NormalizePair(roleA, roleB)
	res, err := db.ExecContext(ctx, `DELETE FROM role_conflicts WHERE role_a=$1 AND role_b=$2`, a, b)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// HeldRoleNames lists the distinct roles (any status) an account holds.
func HeldRoleNames(ctx context.Context, db *sql.DB, userID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT role FROM user_school_roles WHERE user_id::text=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RoleConflictViolation is an existing account that already breaks a rule
// (accounts created before SoD existed); listed for clean-up, not auto-fixed.
type RoleConflictViolation struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	RoleA  string `json:"role_a"`
	RoleB  string `json:"role_b"`
	Reason string `json:"reason"`
}

func ListRoleConflictViolations(ctx context.Context, db *sql.DB, schoolID string) ([]RoleConflictViolation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT u.id, u.name, u.email, c.role_a, c.role_b, c.reason
		FROM role_conflicts c
		JOIN user_school_roles ra ON ra.role = c.role_a
		JOIN user_school_roles rb ON rb.role = c.role_b AND rb.user_id = ra.user_id
		JOIN users u ON u.id = ra.user_id AND u.deleted_at IS NULL
		WHERE $1 = '' OR ra.school_id::text = $1 OR rb.school_id::text = $1
		ORDER BY u.name, c.role_a, c.role_b`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoleConflictViolation{}
	for rows.Next() {
		var v RoleConflictViolation
		if err := rows.Scan(&v.UserID, &v.Name, &v.Email, &v.RoleA, &v.RoleB, &v.Reason); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---- small helpers ----------------------------------------------------------

func nullable(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nonNil keeps a NOT NULL text[] column from receiving SQL NULL (pq.Array of
// a nil slice is NULL).
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
