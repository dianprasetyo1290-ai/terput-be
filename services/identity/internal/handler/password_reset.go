package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/security"
	"skci/identity/internal/store"
)

// Password reset by link (BUG-085, NFR-SEC). Flow:
//   POST /auth/password/forgot {email}         -> always 200 (no account enumeration)
//   GET  /auth/password/reset/{token}          -> 200 valid | 410 used/expired | 404 unknown
//   POST /auth/password/reset {token,password} -> 200, single use, then all sessions revoked
// Only the SHA-256 of the token is stored; the link is delivered by the
// notification service (email channel) to a unit the account belongs to.

const msgForgotAccepted = "Jika email terdaftar, tautan untuk mengatur ulang kata sandi telah dikirim. Periksa kotak masuk Anda."

var resetStatusMessages = map[string]string{
	store.ResetTokenUnknown: "Tautan reset kata sandi tidak dikenal. Minta tautan baru.",
	store.ResetTokenUsed:    "Tautan reset kata sandi sudah dipakai. Minta tautan baru bila perlu.",
	store.ResetTokenExpired: "Tautan reset kata sandi sudah kedaluwarsa. Minta tautan baru.",
}

func resetStatusCode(status string) int {
	if status == store.ResetTokenUnknown {
		return http.StatusNotFound
	}
	return http.StatusGone
}

// ForgotPassword issues a reset link. The answer never reveals whether the
// email exists, is locked or could be mailed.
func (a *App) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		common.WriteValidationError(w, http.StatusBadRequest, "Email wajib diisi", map[string]string{"email": "Email wajib diisi"})
		return
	}
	email := strings.TrimSpace(req.Email)
	ctx := r.Context()
	corr := r.Header.Get("X-Correlation-ID")
	user, _, err := store.FindUserByEmail(a.DB, email)
	if err == nil && (user.DeactivateAt == nil || user.DeactivateAt.After(time.Now())) {
		token, hash, terr := security.NewOpaqueToken()
		if terr == nil {
			ttl := time.Duration(a.policy(r).ResetTokenTTLMinutes) * time.Minute
			if err := store.CreatePasswordResetToken(ctx, a.DB, user.ID, hash, clientIP(r), ttl); err != nil {
				common.CaptureError(err)
			} else {
				uid := user.ID
				delivered := a.sendResetLink(ctx, user.ID, token, ttl)
				a.RecordSecurityAudit(nil, "password_reset_requested", user.ID, &uid, nil, nil,
					map[string]any{"delivered": delivered, "ttl_minutes": int(ttl.Minutes())}, "forgot password", corr)
			}
		}
	} else if err != nil && err != store.ErrNotFound {
		common.CaptureError(err)
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]bool{"accepted": true}, nil, msgForgotAccepted)
}

// CheckResetToken lets the reset page tell the user up front that a link is
// used or expired instead of failing after they typed a new password.
func (a *App) CheckResetToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	_, status, err := store.CheckPasswordResetToken(r.Context(), a.DB, security.HashToken(token))
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa tautan")
		return
	}
	if status != store.ResetTokenValid {
		common.WriteError(w, resetStatusCode(status), resetStatusMessages[status])
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": status}, nil)
}

// ResetPasswordWithToken sets a new password from a valid link, burns the
// link, clears any lockout and ends every existing session.
func (a *App) ResetPasswordWithToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		common.WriteError(w, http.StatusBadRequest, "Token dan kata sandi baru wajib diisi")
		return
	}
	if msg := security.ValidPassword(req.Password); msg != "" {
		common.WriteValidationError(w, http.StatusBadRequest, msg, map[string]string{"password": msg})
		return
	}
	ctx := r.Context()
	hash := security.HashToken(req.Token)
	_, status, err := store.CheckPasswordResetToken(ctx, a.DB, hash)
	if err != nil {
		common.WriteDBError(w, err, "Gagal mengatur ulang kata sandi")
		return
	}
	if status != store.ResetTokenValid {
		common.WriteError(w, resetStatusCode(status), resetStatusMessages[status])
		return
	}
	newHash, err := common.HashPassword(req.Password)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengatur ulang kata sandi")
		return
	}
	userID, ok, err := store.ConsumePasswordResetToken(ctx, a.DB, hash, newHash)
	if err != nil {
		common.WriteDBError(w, err, "Gagal mengatur ulang kata sandi")
		return
	}
	if !ok { // raced with another use or expiry
		common.WriteError(w, http.StatusGone, resetStatusMessages[store.ResetTokenUsed])
		return
	}
	a.revokeAll(r, userID, "password_reset", "")
	a.RecordSecurityAudit(nil, "password_reset_completed", userID, &userID, nil, nil, nil, "password reset via link", r.Header.Get("X-Correlation-ID"))
	common.WriteJSONMsg(w, http.StatusOK, map[string]bool{"reset": true}, nil, "Kata sandi berhasil diubah. Silakan masuk dengan kata sandi baru.")
}

// resetLink builds the SPA URL carrying the token.
func (a *App) resetLink(token string) string {
	return strings.TrimRight(a.PublicAppURL, "/") + "/reset-password?token=" + url.QueryEscape(token)
}

// sendResetLink asks the notification service to email the link. The
// notification service is tenant scoped, so the message goes to the first
// unit where the account holds an active role; an account without a unit
// (pure superadmin) cannot be mailed and must be reset by another
// superadmin (documented in the handoff). Returns whether it was accepted.
func (a *App) sendResetLink(ctx context.Context, userID, token string, ttl time.Duration) bool {
	if a.NotificationURL == "" {
		return false
	}
	var schoolID string
	err := a.DB.QueryRowContext(ctx, `SELECT school_id FROM user_school_roles
		WHERE user_id=$1 AND school_id IS NOT NULL AND COALESCE(status,'active')='active'
		ORDER BY created_at LIMIT 1`, userID).Scan(&schoolID)
	if err != nil {
		return false
	}
	payload := map[string]any{
		"school_id": schoolID,
		"user_id":   userID,
		"type":      "password_reset",
		"title":     "Atur ulang kata sandi SEKOCI",
		"body": fmt.Sprintf("Kami menerima permintaan mengatur ulang kata sandi akun Anda. Buka tautan berikut dalam %d menit: %s\n"+
			"Tautan hanya dapat dipakai sekali. Abaikan pesan ini bila Anda tidak memintanya.", int(ttl.Minutes()), a.resetLink(token)),
		"channels": []string{"email"},
	}
	body, _ := json.Marshal(payload)
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, strings.TrimRight(a.NotificationURL, "/")+"/internal/notifications", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", a.InternalKey)
	req.Header.Set("Idempotency-Key", "password-reset:"+security.HashToken(token)[:32])
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		common.CaptureError(err)
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 300
}
