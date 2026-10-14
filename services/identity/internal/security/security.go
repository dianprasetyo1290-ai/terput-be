// Package security holds the pure (I/O-free) rules behind account and
// session security: lockout, session validity, MFA requirement, TOTP,
// secret encryption, opaque tokens and forbidden role combinations.
// Handlers and the store call these; tests cover them table-driven.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Policy mirrors the security_policy row (migration 0019).
type Policy struct {
	MaxFailedLogins      int       `json:"max_failed_logins"`
	LockoutMinutes       int       `json:"lockout_minutes"`
	IdleTimeoutMinutes   int       `json:"idle_timeout_minutes"`
	SessionMaxHours      int       `json:"session_max_hours"`
	ResetTokenTTLMinutes int       `json:"reset_token_ttl_minutes"`
	MFARoles             []string  `json:"mfa_roles"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// DefaultPolicy is only used when the policy row cannot be read; it matches
// the seeded row so a DB hiccup never weakens the rules.
func DefaultPolicy() Policy {
	return Policy{MaxFailedLogins: 5, LockoutMinutes: 15, IdleTimeoutMinutes: 30, SessionMaxHours: 24, ResetTokenTTLMinutes: 30,
		MFARoles: []string{"superadmin", "finance_manager", "payroll_officer", "hr_manager"}}
}

// ValidatePolicy returns field -> message for an invalid policy edit. The
// bounds match the CHECK constraints in migration 0019.
func ValidatePolicy(p Policy, isValidRole func(string) bool) map[string]string {
	errs := map[string]string{}
	if p.MaxFailedLogins < 1 || p.MaxFailedLogins > 50 {
		errs["max_failed_logins"] = "Batas gagal login harus 1–50"
	}
	if p.LockoutMinutes < 1 || p.LockoutMinutes > 1440 {
		errs["lockout_minutes"] = "Lama penguncian harus 1–1440 menit"
	}
	if p.IdleTimeoutMinutes < 5 || p.IdleTimeoutMinutes > 1440 {
		errs["idle_timeout_minutes"] = "Batas sesi tidak aktif harus 5–1440 menit"
	}
	if p.SessionMaxHours < 1 || p.SessionMaxHours > 720 {
		errs["session_max_hours"] = "Umur sesi maksimum harus 1–720 jam"
	}
	if p.ResetTokenTTLMinutes < 5 || p.ResetTokenTTLMinutes > 1440 {
		errs["reset_token_ttl_minutes"] = "Masa berlaku tautan reset harus 5–1440 menit"
	}
	for _, r := range p.MFARoles {
		if !isValidRole(r) {
			errs["mfa_roles"] = "Peran tidak dikenal: " + r
			break
		}
	}
	return errs
}

// LockoutResult is what a failed password attempt does to the account.
type LockoutResult struct {
	FailedCount int
	LockedUntil *time.Time
}

// RegisterFailure applies one failed attempt. Reaching the limit locks the
// account for LockoutMinutes and resets the counter, so the next window
// starts fresh once the lock expires (NFR-SEC-04).
func RegisterFailure(currentCount int, p Policy, now time.Time) LockoutResult {
	next := currentCount + 1
	if next >= p.MaxFailedLogins {
		until := now.Add(time.Duration(p.LockoutMinutes) * time.Minute)
		return LockoutResult{FailedCount: 0, LockedUntil: &until}
	}
	return LockoutResult{FailedCount: next}
}

// IsLocked reports whether an account lock is still in force.
func IsLocked(lockedUntil *time.Time, now time.Time) bool {
	return lockedUntil != nil && lockedUntil.After(now)
}

// SessionState is everything needed to decide whether a presented access
// token is still acceptable.
type SessionState struct {
	UserExists        bool
	UserActive        bool // not deactivated (deactivate_at in the past)
	RoleHeld          bool // active_role still assigned (status active) at the claimed unit
	TokensValidAfter  *time.Time
	IssuedAt          *time.Time
	HasSessionID      bool
	SessionFound      bool
	SessionRevoked    bool
	SessionExpiresAt  time.Time
	SessionLastSeenAt time.Time
}

// Session rejection reasons (also stored as user_sessions.revoked_reason).
const (
	ReasonUserGone       = "user_not_found"
	ReasonUserInactive   = "user_deactivated"
	ReasonRoleRevoked    = "role_revoked"
	ReasonTokenRevoked   = "token_revoked"
	ReasonSessionRevoked = "session_revoked"
	ReasonSessionExpired = "session_expired"
	ReasonIdleTimeout    = "idle_timeout"
)

// CheckSession returns "" when the token may be used, otherwise the reason.
// Tokens minted before sessions existed carry no sid; those are still
// bounded by tokens_valid_after (revoke-all) and the user/role checks.
func CheckSession(s SessionState, p Policy, now time.Time) string {
	switch {
	case !s.UserExists:
		return ReasonUserGone
	case !s.UserActive:
		return ReasonUserInactive
	case !s.RoleHeld:
		return ReasonRoleRevoked
	}
	if !s.HasSessionID {
		if s.TokensValidAfter != nil && (s.IssuedAt == nil || s.IssuedAt.Before(s.TokensValidAfter.Truncate(time.Second))) {
			return ReasonTokenRevoked
		}
		return ""
	}
	switch {
	case !s.SessionFound || s.SessionRevoked:
		return ReasonSessionRevoked
	case !s.SessionExpiresAt.After(now):
		return ReasonSessionExpired
	case now.Sub(s.SessionLastSeenAt) > time.Duration(p.IdleTimeoutMinutes)*time.Minute:
		return ReasonIdleTimeout
	}
	return ""
}

// RequiresMFA reports whether any role the account holds is in the policy's
// MFA list. Checked against every held role, not only the active one, since
// switching role never asks for the password again.
func RequiresMFA(heldRoles, mfaRoles []string) bool {
	for _, h := range heldRoles {
		for _, m := range mfaRoles {
			if h == m {
				return true
			}
		}
	}
	return false
}

// RoleConflict is one forbidden combination (role_conflicts row).
type RoleConflict struct {
	RoleA  string `json:"role_a"`
	RoleB  string `json:"role_b"`
	Reason string `json:"reason"`
}

// NormalizePair orders a pair the way role_conflicts stores it (role_a < role_b).
func NormalizePair(a, b string) (string, string) {
	if a > b {
		return b, a
	}
	return a, b
}

// ConflictsFor lists the forbidden combinations that granting newRole to an
// account already holding heldRoles would create.
func ConflictsFor(heldRoles []string, newRole string, conflicts []RoleConflict) []RoleConflict {
	var out []RoleConflict
	for _, h := range heldRoles {
		if h == newRole {
			continue
		}
		a, b := NormalizePair(h, newRole)
		for _, c := range conflicts {
			if c.RoleA == a && c.RoleB == b {
				out = append(out, c)
			}
		}
	}
	return out
}

// NewOpaqueToken returns a 256-bit URL-safe random token and its SHA-256 hex
// digest; only the digest is persisted.
func NewOpaqueToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken is the persisted form of an opaque token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ---- TOTP (RFC 6238, SHA-1, 6 digits, 30 s) --------------------------------

const totpPeriod = 30

// NewTOTPSecret returns a random 160-bit base32 secret (no padding).
func NewTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// TOTPCode computes the code for secret at time t.
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("invalid totp secret: %w", err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(t.Unix()/totpPeriod))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000), nil
}

// VerifyTOTP accepts the code for the current step and one step either side
// (clock drift between phone and server).
func VerifyTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	for _, d := range []int{0, -1, 1} {
		want, err := TOTPCode(secret, t.Add(time.Duration(d*totpPeriod)*time.Second))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// OTPAuthURL is the provisioning URI authenticator apps import.
func OTPAuthURL(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// ---- secret encryption (AES-256-GCM) ----------------------------------------

// DeriveKey turns a configured passphrase into a 32-byte AES key.
func DeriveKey(passphrase string) []byte {
	sum := sha256.Sum256([]byte("sekoci-identity-mfa:" + passphrase))
	return sum[:]
}

// Encrypt seals plaintext; output is base64(nonce||ciphertext).
func Encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), nil)), nil
}

// Decrypt reverses Encrypt.
func Decrypt(key []byte, enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	pt, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// ValidPassword is the minimum for a password chosen by the user (change or
// reset link). Admin-generated defaults are always 12 random characters.
func ValidPassword(p string) string {
	if len(p) < 8 {
		return "Kata sandi minimal 8 karakter"
	}
	hasLetter, hasDigit := false, false
	for _, r := range p {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	if !hasLetter || !hasDigit {
		return "Kata sandi harus memuat huruf dan angka"
	}
	return ""
}
