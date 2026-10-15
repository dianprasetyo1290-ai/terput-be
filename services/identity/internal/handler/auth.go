package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/security"
	"skci/identity/internal/store"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user,omitempty"`
	// IdleTimeoutMinutes lets the SPA log out an idle tab on its own, in
	// addition to the server-side idle check.
	IdleTimeoutMinutes int `json:"idle_timeout_minutes,omitempty"`
}

// mfaChallengeResponse is the 200 answer to a correct password on an account
// that must pass MFA. No access token is issued until /auth/mfa/verify.
type mfaChallengeResponse struct {
	MFARequired bool   `json:"mfa_required"`
	MFAEnroll   bool   `json:"mfa_enroll"`
	MFAToken    string `json:"mfa_token"`
	Secret      string `json:"secret,omitempty"`
	OTPAuthURL  string `json:"otpauth_url,omitempty"`
	ExpiresIn   int    `json:"expires_in"`
}

const (
	mfaVerifyTTL   = 5 * time.Minute
	mfaEnrollTTL   = 10 * time.Minute
	mfaMaxAttempts = 5
	msgBadLogin    = "Email atau kata sandi salah"
)

// activeRoleRows drops role rows whose status is inactive (deactivated guru,
// offboarded employee): those must neither log in nor be switched to.
func activeRoleRows(roles []model.UserSchoolRole) []model.UserSchoolRole {
	out := roles[:0:0]
	for _, ro := range roles {
		if ro.Status == "" || ro.Status == "active" {
			out = append(out, ro)
		}
	}
	return out
}

func lockedMessage(until time.Time) string {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		loc = time.FixedZone("WIB", 7*3600)
	}
	return "Akun terkunci sementara karena terlalu banyak percobaan masuk yang gagal. Coba lagi setelah pukul " + until.In(loc).Format("15:04") + " WIB atau hubungi admin."
}

func (a *App) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format data tidak valid")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" || req.Password == "" {
		common.WriteError(w, http.StatusBadRequest, "Email dan kata sandi wajib diisi")
		return
	}
	ctx := r.Context()
	ip, ua := clientIP(r), truncate(r.UserAgent(), 300)
	corr := r.Header.Get("X-Correlation-ID")

	user, hash, err := store.FindUserByEmail(a.DB, req.Email)
	if err == store.ErrNotFound {
		_ = store.RecordLoginEvent(ctx, a.DB, nil, req.Email, false, "unknown_email", ip, ua, nil)
		a.RecordSecurityAudit(nil, "login_failed", req.Email, nil, nil, nil, nil, "email not found", corr)
		common.WriteError(w, http.StatusUnauthorized, msgBadLogin)
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk. Coba lagi")
		return
	}
	uid := user.ID
	st, err := store.GetLoginState(ctx, a.DB, user.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk. Coba lagi")
		return
	}
	p := a.policy(r)
	now := time.Now()
	if security.IsLocked(st.LockedUntil, now) {
		_ = store.RecordLoginEvent(ctx, a.DB, &uid, req.Email, false, "locked", ip, ua, nil)
		a.RecordSecurityAudit(nil, "login_blocked_locked", user.ID, &uid, nil, nil, nil, "login attempt while locked", corr)
		common.WriteError(w, http.StatusLocked, lockedMessage(*st.LockedUntil))
		return
	}
	if !common.CheckPassword(hash, req.Password) {
		res := security.RegisterFailure(st.FailedCount, p, now)
		if err := store.SaveLoginFailure(ctx, a.DB, user.ID, res); err != nil {
			common.CaptureError(err)
		}
		_ = store.RecordLoginEvent(ctx, a.DB, &uid, req.Email, false, "wrong_password", ip, ua, nil)
		a.RecordSecurityAudit(nil, "login_failed", req.Email, &uid, nil, nil, nil, "wrong password", corr)
		if res.LockedUntil != nil {
			a.RecordSecurityAudit(nil, "account_locked", user.ID, &uid, nil, nil, map[string]any{"locked_until": res.LockedUntil, "max_failed_logins": p.MaxFailedLogins}, "too many failed logins", corr)
			common.WriteError(w, http.StatusLocked, lockedMessage(*res.LockedUntil))
			return
		}
		common.WriteError(w, http.StatusUnauthorized, msgBadLogin)
		return
	}

	if user.DeactivateAt != nil && !user.DeactivateAt.After(now) {
		_ = store.RecordLoginEvent(ctx, a.DB, &uid, req.Email, false, "deactivated", ip, ua, nil)
		common.WriteError(w, http.StatusUnauthorized, "Akun dinonaktifkan. Hubungi admin unit")
		return
	}
	roles, err := store.UserRoles(a.DB, user.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk. Coba lagi")
		return
	}
	activeRole, schoolID, allRoles, ok := store.PickActiveRole(activeRoleRows(roles))
	if !ok {
		_ = store.RecordLoginEvent(ctx, a.DB, &uid, req.Email, false, "no_active_role", ip, ua, nil)
		common.WriteError(w, http.StatusForbidden, "Akun belum memiliki peran. Hubungi admin unit")
		return
	}
	if err := store.ClearLoginFailures(ctx, a.DB, user.ID); err != nil {
		common.CaptureError(err)
	}

	if security.RequiresMFA(allRoles, p.MFARoles) {
		a.startMFA(w, r, user, st.MFAEnabled)
		return
	}
	a.completeLogin(w, r, user, activeRole, schoolID, allRoles, false)
}

// completeLogin opens the session, records the success and answers with the
// access token.
func (a *App) completeLogin(w http.ResponseWriter, r *http.Request, user model.User, activeRole, schoolID string, allRoles []string, mfa bool) {
	token, sid, err := a.issueToken(r, user.ID, schoolID, activeRole, allRoles, "", mfa)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat sesi. Coba lagi")
		return
	}
	uid := user.ID
	_ = store.RecordLoginEvent(r.Context(), a.DB, &uid, user.Email, true, map[bool]string{true: "success_mfa", false: "success"}[mfa], clientIP(r), truncate(r.UserAgent(), 300), &sid)
	var sID *string
	if schoolID != "" {
		sID = &schoolID
	}
	a.RecordSecurityAudit(sID, "login_success", user.ID, &user.ID, &activeRole, nil, map[string]any{"active_role": activeRole, "roles": allRoles, "mfa": mfa}, "successful authentication", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, loginResponse{Token: token, User: user, IdleTimeoutMinutes: a.policy(r).IdleTimeoutMinutes}, nil)
}

// startMFA answers a correct password on an MFA-required account with a
// challenge. Not enrolled yet -> enrollment challenge carrying a fresh TOTP
// secret (shown once, stored encrypted, activated only after a valid code).
func (a *App) startMFA(w http.ResponseWriter, r *http.Request, user model.User, enrolled bool) {
	token, hash, err := security.NewOpaqueToken()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk. Coba lagi")
		return
	}
	resp := mfaChallengeResponse{MFARequired: true, MFAToken: token}
	if enrolled {
		if err := store.CreateMFAChallenge(r.Context(), a.DB, user.ID, hash, "verify", nil, mfaVerifyTTL); err != nil {
			common.WriteDBError(w, err, "Gagal masuk. Coba lagi")
			return
		}
		resp.ExpiresIn = int(mfaVerifyTTL.Seconds())
		common.WriteJSON(w, http.StatusOK, resp, nil)
		return
	}
	secret, err := security.NewTOTPSecret()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk. Coba lagi")
		return
	}
	enc, err := security.Encrypt(a.MFAKey, secret)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk. Coba lagi")
		return
	}
	if err := store.CreateMFAChallenge(r.Context(), a.DB, user.ID, hash, "enroll", &enc, mfaEnrollTTL); err != nil {
		common.WriteDBError(w, err, "Gagal masuk. Coba lagi")
		return
	}
	resp.MFAEnroll = true
	resp.Secret = secret
	resp.OTPAuthURL = security.OTPAuthURL(a.MFAIssuer, user.Email, secret)
	resp.ExpiresIn = int(mfaEnrollTTL.Seconds())
	common.WriteJSON(w, http.StatusOK, resp, nil)
}

type mfaVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

// VerifyMFA: POST /auth/mfa/verify — second login step (also completes
// enrollment). Wrong codes count toward the account lockout too.
func (a *App) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MFAToken == "" || req.Code == "" {
		common.WriteError(w, http.StatusBadRequest, "Token MFA dan kode wajib diisi")
		return
	}
	ctx := r.Context()
	expired := "Verifikasi kedaluwarsa, silakan masuk kembali"
	ch, err := store.GetMFAChallenge(ctx, a.DB, security.HashToken(req.MFAToken))
	if err == store.ErrNotFound || (err == nil && (ch.UsedAt != nil || !ch.ExpiresAt.After(time.Now()) || ch.Attempts >= mfaMaxAttempts)) {
		common.WriteError(w, http.StatusUnauthorized, expired)
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Verifikasi gagal")
		return
	}
	user, err := store.FindUserByID(a.DB, ch.UserID)
	if err != nil {
		common.WriteError(w, http.StatusUnauthorized, expired)
		return
	}
	st, err := store.GetLoginState(ctx, a.DB, user.ID)
	if err != nil {
		common.WriteDBError(w, err, "Verifikasi gagal")
		return
	}
	if security.IsLocked(st.LockedUntil, time.Now()) {
		common.WriteError(w, http.StatusLocked, lockedMessage(*st.LockedUntil))
		return
	}
	var secretEnc *string
	if ch.Purpose == "enroll" {
		secretEnc = ch.PendingSecretEnc
	} else {
		secretEnc = st.MFASecret
	}
	if secretEnc == nil {
		common.WriteError(w, http.StatusUnauthorized, expired)
		return
	}
	secret, err := security.Decrypt(a.MFAKey, *secretEnc)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "Verifikasi gagal")
		return
	}
	uid := user.ID
	if !security.VerifyTOTP(secret, req.Code, time.Now()) {
		_ = store.IncrementMFAAttempts(ctx, a.DB, ch.ID)
		res := security.RegisterFailure(st.FailedCount, a.policy(r), time.Now())
		_ = store.SaveLoginFailure(ctx, a.DB, user.ID, res)
		_ = store.RecordLoginEvent(ctx, a.DB, &uid, user.Email, false, "mfa_failed", clientIP(r), truncate(r.UserAgent(), 300), nil)
		a.RecordSecurityAudit(nil, "mfa_failed", user.ID, &uid, nil, nil, nil, "wrong MFA code", r.Header.Get("X-Correlation-ID"))
		if res.LockedUntil != nil {
			common.WriteError(w, http.StatusLocked, lockedMessage(*res.LockedUntil))
			return
		}
		common.WriteError(w, http.StatusUnauthorized, "Kode verifikasi salah")
		return
	}
	if ok, err := store.ConsumeMFAChallenge(ctx, a.DB, ch.ID); err != nil || !ok {
		common.WriteError(w, http.StatusUnauthorized, expired)
		return
	}
	if ch.Purpose == "enroll" {
		if err := store.EnableMFA(ctx, a.DB, user.ID, *secretEnc); err != nil {
			common.WriteDBError(w, err, "Verifikasi gagal")
			return
		}
		a.RecordSecurityAudit(nil, "mfa_enrolled", user.ID, &uid, nil, nil, nil, "MFA TOTP enrolled", r.Header.Get("X-Correlation-ID"))
	}
	_ = store.ClearLoginFailures(ctx, a.DB, user.ID)
	roles, err := store.UserRoles(a.DB, user.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal masuk. Coba lagi")
		return
	}
	activeRole, schoolID, allRoles, ok := store.PickActiveRole(activeRoleRows(roles))
	if !ok {
		common.WriteError(w, http.StatusForbidden, "Akun belum memiliki peran. Hubungi admin unit")
		return
	}
	a.completeLogin(w, r, user, activeRole, schoolID, allRoles, true)
}

// ResetUserMFA: POST /users/{id}/mfa/reset (superadmin) — for a lost phone.
// The user enrolls again at next login; every session is ended.
func (a *App) ResetUserMFA(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	id := chi.URLParam(r, "id")
	ok, err := store.ResetMFA(r.Context(), a.DB, id)
	if err != nil {
		common.WriteDBError(w, err, "gagal mereset MFA")
		return
	}
	if !ok {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	}
	a.revokeAll(r, id, "mfa_reset", "")
	c := common.FromCtx(r)
	a.RecordSecurityAudit(nil, "mfa_reset", id, &c.UserID, &c.ActiveRole, nil, nil, "MFA reset by superadmin", r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]string{"user_id": id, "message": "MFA direset; pengguna mendaftar ulang saat masuk berikutnya"}, nil)
}

type switchRoleRequest struct {
	Role string `json:"role"`
}

// SwitchRole reissues a JWT with a different active_role for the same
// account, no re-login — implements "kepala sekolah bisa berganti role
// secara langsung tanpa logout". The session (sid, expiry, idle clock) is
// kept: switching role is not a new login.
func (a *App) SwitchRole(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	var req switchRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format data tidak valid")
		return
	}
	if req.Role == "" {
		common.WriteError(w, http.StatusBadRequest, "Peran wajib diisi")
		return
	}

	// A deactivated (offboarded) account must not mint fresh tokens either.
	if user, err := store.FindUserByID(a.DB, claims.UserID); err != nil || (user.DeactivateAt != nil && !user.DeactivateAt.After(time.Now())) {
		common.WriteError(w, http.StatusUnauthorized, "Akun dinonaktifkan. Hubungi admin unit")
		return
	}

	roles, err := store.UserRoles(a.DB, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal berpindah peran")
		return
	}
	roles = activeRoleRows(roles)
	found := false
	schoolID := ""
	for _, ro := range roles {
		if ro.Role == req.Role {
			found = true
			if ro.SchoolID != nil {
				schoolID = *ro.SchoolID
			}
			break
		}
	}
	if !found {
		common.WriteError(w, http.StatusForbidden, "Akun Anda tidak memiliki peran "+req.Role)
		return
	}

	var allRoles []string
	for _, ro := range roles {
		allRoles = append(allRoles, ro.Role)
	}

	token, _, err := a.issueToken(r, claims.UserID, schoolID, req.Role, allRoles, claims.SessionID, false)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat sesi. Coba lagi")
		return
	}

	var sID *string
	if schoolID != "" {
		sID = &schoolID
	}
	a.RecordSecurityAudit(sID, "role_switched", claims.UserID, &claims.UserID, &req.Role, map[string]any{"from_role": claims.ActiveRole}, map[string]any{"to_role": req.Role}, "switched active role", r.Header.Get("X-Correlation-ID"))

	common.WriteJSON(w, http.StatusOK, loginResponse{Token: token}, nil)
}

// Me returns the caller's current claims + profile — convenience endpoint
// for the frontend to hydrate session state after login/switch-role.
func (a *App) Me(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	user, err := store.FindUserByID(a.DB, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	}
	roles, _ := store.UserRoles(a.DB, claims.UserID)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"claims": claims,
		"user":   user,
		"roles":  roles,
	}, nil)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword: self-service password change for the logged-in user
// (Admin's "Ubah Password" 3-step form). Unlike ResetPassword in
// users.go (an admin resetting someone ELSE's password without knowing
// it), this requires the caller's own current password to verify identity
// before allowing the change — a genuinely different, security-sensitive
// path, not a variant of the existing reset endpoint.
func (a *App) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CurrentPassword == "" || req.NewPassword == "" {
		common.WriteError(w, http.StatusBadRequest, "Kata sandi lama dan baru wajib diisi")
		return
	}
	if msg := security.ValidPassword(req.NewPassword); msg != "" {
		common.WriteValidationError(w, http.StatusBadRequest, msg, map[string]string{"new_password": msg})
		return
	}

	var hash string
	if err := a.DB.QueryRow(`SELECT password_hash FROM users WHERE id=$1 AND deleted_at IS NULL`, claims.UserID).Scan(&hash); err != nil {
		common.WriteError(w, http.StatusNotFound, "Pengguna tidak ditemukan")
		return
	}
	if !common.CheckPassword(hash, req.CurrentPassword) {
		common.WriteError(w, http.StatusUnauthorized, "Kata sandi lama salah")
		return
	}

	newHash, err := common.HashPassword(req.NewPassword)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengubah kata sandi")
		return
	}
	// Same post-change bookkeeping as ResetPassword: default_password no
	// longer applies once a real password is set, password_changed flips
	// so the UI stops showing a plaintext default password anywhere.
	if _, err := a.DB.Exec(`UPDATE users SET password_hash=$1, password_changed=true, default_password=NULL, updated_at=now() WHERE id=$2`,
		newHash, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengubah kata sandi")
		return
	}
	// Other devices may be using the old password: end them, keep this one.
	a.revokeAll(r, claims.UserID, "password_changed", claims.SessionID)
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "password changed"}, nil)
}
