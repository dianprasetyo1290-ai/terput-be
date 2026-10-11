package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/store"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

func (a *App) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		common.WriteError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, hash, err := store.FindUserByEmail(a.DB, req.Email)
	if err == store.ErrNotFound {
		common.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if !common.CheckPassword(hash, req.Password) {
		common.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	roles, err := store.UserRoles(a.DB, user.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "login failed")
		return
	}
	activeRole, schoolID, allRoles, ok := store.PickActiveRole(roles)
	if !ok {
		common.WriteError(w, http.StatusForbidden, "account has no assigned role")
		return
	}

	token, err := a.issueToken(user.ID, schoolID, activeRole, allRoles)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	common.WriteJSON(w, http.StatusOK, loginResponse{Token: token, User: user}, nil)
}

type switchRoleRequest struct {
	Role string `json:"role"`
}

// SwitchRole reissues a JWT with a different active_role for the same
// account, no re-login — implements "kepala sekolah bisa berganti role
// secara langsung tanpa logout".
func (a *App) SwitchRole(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	var req switchRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role == "" {
		common.WriteError(w, http.StatusBadRequest, "role is required")
		return
	}

	roles, err := store.UserRoles(a.DB, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "switch role failed")
		return
	}
	var target *struct {
		SchoolID string
	}
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
	_ = target
	if !found {
		common.WriteError(w, http.StatusForbidden, "account does not hold role "+req.Role)
		return
	}

	var allRoles []string
	for _, ro := range roles {
		allRoles = append(allRoles, ro.Role)
	}

	token, err := a.issueToken(claims.UserID, schoolID, req.Role, allRoles)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	common.WriteJSON(w, http.StatusOK, loginResponse{Token: token}, nil)
}

// Me returns the caller's current claims + profile — convenience endpoint
// for the frontend to hydrate session state after login/switch-role.
func (a *App) Me(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	user, err := store.FindUserByID(a.DB, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "user not found")
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
		common.WriteError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}
	if len(req.NewPassword) < 6 {
		common.WriteError(w, http.StatusBadRequest, "new_password must be at least 6 characters")
		return
	}

	var hash string
	if err := a.DB.QueryRow(`SELECT password_hash FROM users WHERE id=$1 AND deleted_at IS NULL`, claims.UserID).Scan(&hash); err != nil {
		common.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if !common.CheckPassword(hash, req.CurrentPassword) {
		common.WriteError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := common.HashPassword(req.NewPassword)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	// Same post-change bookkeeping as ResetPassword: default_password no
	// longer applies once a real password is set, password_changed flips
	// so the UI stops showing a plaintext default password anywhere.
	if _, err := a.DB.Exec(`UPDATE users SET password_hash=$1, password_changed=true, default_password=NULL, updated_at=now() WHERE id=$2`,
		newHash, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to change password")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "password changed"}, nil)
}

func (a *App) issueToken(userID, schoolID, activeRole string, roles []string) (string, error) {
	// Display names ride in the token so the header shows them instead of UUIDs.
	var name, schoolName string
	_ = a.DB.QueryRow(`SELECT name FROM users WHERE id=$1`, userID).Scan(&name)
	if schoolID != "" {
		_ = a.DB.QueryRow(`SELECT name FROM schools WHERE id::text=$1`, schoolID).Scan(&schoolName)
	}
	claims := common.Claims{
		UserID:     userID,
		Name:       name,
		SchoolName: schoolName,
		SchoolID:   schoolID,
		ActiveRole: activeRole,
		Roles:      roles,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.JWTSecret))
}
