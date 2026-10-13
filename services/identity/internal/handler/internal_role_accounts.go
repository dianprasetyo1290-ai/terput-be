package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"skci/identity/internal/common"
)

type internalRoleAccountRequest struct {
	SchoolID string `json:"school_id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

type internalRoleAccountResponse struct {
	UserID          string `json:"user_id"`
	Created         bool   `json:"created"`
	DefaultPassword string `json:"default_password,omitempty"`
}

var externalRoleWhitelist = map[string]bool{
	"pembimbing_industri": true,
	"canteen_operator":    true,
}

// InternalCreateOrLinkRoleAccount is POST /internal/role-accounts/create-or-link (§4.1, FR-139-003, T12 §4.2).
func (a *App) InternalCreateOrLinkRoleAccount(w http.ResponseWriter, r *http.Request) {
	var req internalRoleAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = strings.TrimSpace(req.SchoolID)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.FullName = strings.TrimSpace(req.FullName)
	req.Role = strings.TrimSpace(req.Role)
	if req.SchoolID == "" || req.Email == "" || req.FullName == "" || req.Role == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id, email, full_name, and role are required")
		return
	}
	if !externalRoleWhitelist[req.Role] {
		common.WriteError(w, http.StatusBadRequest, "role not allowed for external role accounts")
		return
	}

	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()

	var userID string
	var created bool
	var defaultPassword string

	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE lower(email) = $1 AND deleted_at IS NULL FOR UPDATE`, req.Email).Scan(&userID)
	switch {
	case err == sql.ErrNoRows:
		created = true
		defaultPassword = common.GenDefaultPassword()
		hash, herr := common.HashPassword(defaultPassword)
		if herr != nil {
			common.WriteError(w, http.StatusInternalServerError, "password hashing failed")
			return
		}
		// External role accounts do NOT set managed_by_hr
		err = tx.QueryRowContext(ctx, `INSERT INTO users (name, email, password_hash, password_changed, default_password)
			VALUES ($1, $2, $3, false, $4) RETURNING id`, req.FullName, req.Email, hash, defaultPassword).Scan(&userID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	case err != nil:
		common.WriteError(w, http.StatusInternalServerError, "database error")
		return
	default:
		created = false
	}

	if _, err = tx.ExecContext(ctx, `INSERT INTO user_school_roles (user_id, school_id, role, status)
		VALUES ($1, $2, $3, 'active')
		ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active'`,
		userID, req.SchoolID, req.Role); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to assign role")
		return
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	resp := internalRoleAccountResponse{
		UserID:  userID,
		Created: created,
	}
	if created {
		resp.DefaultPassword = defaultPassword
	}
	common.WriteJSON(w, http.StatusOK, resp, nil)
}
