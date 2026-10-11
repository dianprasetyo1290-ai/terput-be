package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
)

// rolePegawai is the HRIS staff role. The shared constant (and the CHECK
// widening) is owned by migration 0014 / T1; a literal here keeps this file
// free of a duplicate declaration once both branches meet.
const rolePegawai = "pegawai"

type internalEmployeeRequest struct {
	SchoolID string `json:"school_id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

// InternalCreateOrLinkEmployee is POST /internal/employees/create-or-link
// (BR-HR-01). It reuses an account with the same email or creates one with a
// default password, marks it managed_by_hr, clears any pending deactivation
// (rehire) and grants the guru/pegawai role in the school.
func (a *App) InternalCreateOrLinkEmployee(w http.ResponseWriter, r *http.Request) {
	var req internalEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = strings.TrimSpace(req.SchoolID)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.FullName = strings.TrimSpace(req.FullName)
	if req.SchoolID == "" || req.Email == "" || req.FullName == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id, email, and full_name are required")
		return
	}
	if req.Role != common.RoleGuru && req.Role != rolePegawai {
		common.WriteError(w, http.StatusBadRequest, "role must be guru or pegawai")
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
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE lower(email) = $1 AND deleted_at IS NULL FOR UPDATE`, req.Email).Scan(&userID)
	switch {
	case err == sql.ErrNoRows:
		pass := common.GenDefaultPassword()
		hash, herr := common.HashPassword(pass)
		if herr != nil {
			common.WriteError(w, http.StatusInternalServerError, "password hashing failed")
			return
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO users (name, email, password_hash, password_changed, default_password, managed_by_hr)
			VALUES ($1, $2, $3, false, $4, true) RETURNING id`, req.FullName, req.Email, hash, pass).Scan(&userID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
	case err != nil:
		common.WriteError(w, http.StatusInternalServerError, "database error")
		return
	default:
		if _, err = tx.ExecContext(ctx, `UPDATE users SET name = $2, managed_by_hr = true, deactivate_at = NULL, updated_at = now() WHERE id = $1`, userID, req.FullName); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to update user")
			return
		}
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
	common.WriteJSON(w, http.StatusOK, map[string]string{"user_id": userID}, nil)
}

type internalDeactivateRequest struct {
	SchoolID    string     `json:"school_id"`
	Reason      string     `json:"reason,omitempty"`
	EffectiveAt *time.Time `json:"effective_at,omitempty"`
}

// InternalDeactivateUser is POST /internal/users/{id}/deactivate (AC-83-03 & T7a student exit).
// It records users.deactivate_at when effective_at is provided, and deactivates active roles
// and guardian links for the user in the school when reason or immediate deactivation is requested.
func (a *App) InternalDeactivateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req internalDeactivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.SchoolID = strings.TrimSpace(req.SchoolID)
	req.Reason = strings.TrimSpace(req.Reason)
	if id == "" || req.SchoolID == "" || (req.EffectiveAt == nil && req.Reason == "") {
		common.WriteError(w, http.StatusBadRequest, "id, school_id and effective_at (or reason) are required")
		return
	}

	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback()

	// Ensure user exists and has a role in the school
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM user_school_roles WHERE user_id = $1 AND school_id = $2)`, id, req.SchoolID).Scan(&exists)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to check user in school")
		return
	}
	if !exists {
		common.WriteError(w, http.StatusNotFound, "user not found in school")
		return
	}

	if req.EffectiveAt != nil {
		_, err = tx.ExecContext(ctx, `UPDATE users SET deactivate_at = $1, updated_at = now()
			WHERE id = $2 AND deleted_at IS NULL`,
			*req.EffectiveAt, id)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to set deactivation date")
			return
		}
	}

	if req.Reason != "" || req.EffectiveAt == nil || !req.EffectiveAt.After(time.Now()) {
		// Deactivate active roles for this user and school
		_, err = tx.ExecContext(ctx, `
			UPDATE user_school_roles
			SET status = 'inactive', deactivated_at = now()
			WHERE user_id = $1 AND school_id = $2 AND status = 'active'
		`, id, req.SchoolID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to deactivate user roles")
			return
		}

		// Deactivate guardian_links for this student and school
		_, err = tx.ExecContext(ctx, `
			UPDATE guardian_links
			SET status = 'inactive', deactivated_at = now(), updated_at = now()
			WHERE student_user_id = $1 AND school_id = $2 AND status = 'active'
		`, id, req.SchoolID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to deactivate guardian links")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"deactivated":   true,
		"user_id":       id,
		"school_id":     req.SchoolID,
		"deactivate_at": req.EffectiveAt,
	}, nil)
}
