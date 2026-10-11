package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/store"
)

type internalUserRequest struct {
	SchoolID      string `json:"school_id"`
	Source        string `json:"source"`
	ExternalRef   string `json:"external_ref"`
	ConversionRef string `json:"conversion_ref"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	NISN          string `json:"nisn"`
	Password      string `json:"password"`
}

func normalizeInternalUser(req *internalUserRequest) bool {
	req.SchoolID = strings.TrimSpace(req.SchoolID)
	req.Source = strings.TrimSpace(req.Source)
	req.ExternalRef = strings.TrimSpace(req.ExternalRef)
	req.ConversionRef = strings.TrimSpace(req.ConversionRef)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Source == "" {
		req.Source = "admission"
	}
	return req.SchoolID != "" && req.ExternalRef != "" && req.Name != "" && req.Email != ""
}

func upsertExternalUser(tx *sql.Tx, req internalUserRequest, entityType, role string) (model.User, error) {
	var existingID string
	err := tx.QueryRow(`SELECT user_id FROM identity_external_refs WHERE school_id=$1 AND source=$2 AND entity_type=$3 AND external_ref=$4 FOR UPDATE`,
		req.SchoolID, req.Source, entityType, req.ExternalRef).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return model.User{}, err
	}
	if existingID == "" {
		_ = tx.QueryRow(`SELECT id FROM users WHERE lower(email)=lower($1) AND deleted_at IS NULL`, req.Email).Scan(&existingID)
	}
	if existingID == "" {
		password := req.Password
		if password == "" {
			password = common.GenDefaultPassword()
		}
		hash, err := common.HashPassword(password)
		if err != nil {
			return model.User{}, err
		}
		err = tx.QueryRow(`INSERT INTO users (name,email,phone,password_hash,password_changed,nisn) VALUES ($1,$2,NULLIF($3,''),$4,false,NULLIF($5,'')) RETURNING id`,
			req.Name, req.Email, req.Phone, hash, req.NISN).Scan(&existingID)
		if err != nil {
			return model.User{}, err
		}
	} else {
		_, err = tx.Exec(`UPDATE users SET name=$1, phone=COALESCE(NULLIF($2,''),phone), nisn=COALESCE(NULLIF($3,''),nisn), updated_at=now() WHERE id=$4 AND deleted_at IS NULL`, req.Name, req.Phone, req.NISN, existingID)
		if err != nil {
			return model.User{}, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO user_school_roles (user_id,school_id,role) VALUES ($1,$2,$3) ON CONFLICT (user_id,school_id,role) DO UPDATE SET status='active'`, existingID, req.SchoolID, role); err != nil {
		return model.User{}, err
	}
	if _, err := tx.Exec(`INSERT INTO identity_external_refs (school_id,source,entity_type,external_ref,user_id) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (school_id,source,entity_type,external_ref) DO UPDATE SET updated_at=now()`, req.SchoolID, req.Source, entityType, req.ExternalRef, existingID); err != nil {
		return model.User{}, err
	}
	if req.ConversionRef != "" {
		column := "guardian_user_id"
		if entityType == "student" {
			column = "student_user_id"
		}
		if _, err := tx.Exec(`INSERT INTO admission_conversion_mappings (school_id,source,conversion_ref,`+column+`) VALUES ($1,$2,$3,$4)
			ON CONFLICT (school_id,source,conversion_ref) DO UPDATE SET `+column+`=COALESCE(admission_conversion_mappings.`+column+`,EXCLUDED.`+column+`), updated_at=now()`, req.SchoolID, req.Source, req.ConversionRef, existingID); err != nil {
			return model.User{}, err
		}
	}
	var u model.User
	err = tx.QueryRow(`SELECT id,name,email,phone,password_changed,photo,birth_date,birth_place,address,is_superadmin,created_at,updated_at FROM users WHERE id=$1`, existingID).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.PasswordChanged, &u.Photo, &u.BirthDate, &u.BirthPlace, &u.Address, &u.IsSuperadmin, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (a *App) internalUpsertUser(w http.ResponseWriter, r *http.Request, entityType, role string) {
	var req internalUserRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || !normalizeInternalUser(&req) {
		common.WriteError(w, http.StatusBadRequest, "school_id, external_ref, name and email are required")
		return
	}
	if entityType == "student" && strings.TrimSpace(req.NISN) == "" {
		common.WriteError(w, http.StatusBadRequest, "nisn is required")
		return
	}
	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to upsert user")
		return
	}
	defer tx.Rollback()
	u, err := upsertExternalUser(tx, req, entityType, role)
	if err != nil {
		common.WriteError(w, http.StatusConflict, "failed to upsert user: "+err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to upsert user")
		return
	}
	common.WriteJSON(w, http.StatusOK, u, nil)
}

func (a *App) InternalUpsertGuardian(w http.ResponseWriter, r *http.Request) {
	a.internalUpsertUser(w, r, "guardian", common.RoleOrangTua)
}

func (a *App) InternalCreateOrLinkStudent(w http.ResponseWriter, r *http.Request) {
	a.internalUpsertUser(w, r, "student", common.RoleSiswa)
}

type internalLinkRequest struct {
	SchoolID       string `json:"school_id"`
	Source         string `json:"source"`
	ConversionRef  string `json:"conversion_ref"`
	GuardianUserID string `json:"guardian_user_id"`
	StudentUserID  string `json:"student_user_id"`
	Relationship   string `json:"relationship"`
	IsPrimary      bool   `json:"is_primary"`
}

func (a *App) InternalCreateGuardianLink(w http.ResponseWriter, r *http.Request) {
	var req internalLinkRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.SchoolID == "" || req.ConversionRef == "" || req.GuardianUserID == "" || req.StudentUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id, conversion_ref, guardian_user_id and student_user_id are required")
		return
	}
	if req.Source == "" {
		req.Source = "admission"
	}
	if req.Relationship == "" {
		req.Relationship = "orang_tua"
	}
	tx, err := a.DB.Begin()
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create guardian link")
		return
	}
	defer tx.Rollback()
	var valid bool
	if err := tx.QueryRow(`SELECT
		EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$2 AND school_id=$1 AND role='orang_tua' AND status='active') AND
		EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$3 AND school_id=$1 AND role='siswa' AND status='active')`, req.SchoolID, req.GuardianUserID, req.StudentUserID).Scan(&valid); err != nil || !valid {
		common.WriteError(w, http.StatusBadRequest, "guardian and student must belong to school")
		return
	}
	link, err := store.ScanGuardianLink(tx.QueryRow(`INSERT INTO guardian_links (school_id,guardian_user_id,student_user_id,relationship,is_primary,status,verified_at)
		VALUES ($1,$2,$3,$4,$5,'active',now()) ON CONFLICT (school_id,guardian_user_id,student_user_id) DO UPDATE SET relationship=EXCLUDED.relationship,is_primary=EXCLUDED.is_primary,status='active',updated_at=now()
		RETURNING `+store.GuardianLinkColumns, req.SchoolID, req.GuardianUserID, req.StudentUserID, req.Relationship, req.IsPrimary))
	if err != nil {
		common.WriteError(w, http.StatusConflict, "failed to create guardian link")
		return
	}
	_, err = tx.Exec(`INSERT INTO admission_conversion_mappings (school_id,source,conversion_ref,guardian_user_id,student_user_id,guardian_link_id)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (school_id,source,conversion_ref) DO UPDATE SET guardian_user_id=EXCLUDED.guardian_user_id,student_user_id=EXCLUDED.student_user_id,guardian_link_id=EXCLUDED.guardian_link_id,updated_at=now()`,
		req.SchoolID, req.Source, req.ConversionRef, req.GuardianUserID, req.StudentUserID, link.ID)
	if err != nil {
		common.WriteError(w, http.StatusConflict, "conversion reference already maps to different identities")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to create guardian link")
		return
	}
	common.WriteJSON(w, http.StatusOK, link, nil)
}

func (a *App) InternalConversionMapping(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source == "" {
		source = "admission"
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	var m model.ConversionMapping
	err := a.DB.QueryRow(`SELECT school_id,source,conversion_ref,guardian_user_id,student_user_id,guardian_link_id FROM admission_conversion_mappings WHERE school_id=$1 AND source=$2 AND conversion_ref=$3`, schoolID, source, chi.URLParam(r, "conversionRef")).
		Scan(&m.SchoolID, &m.Source, &m.ConversionRef, &m.GuardianID, &m.StudentID, &m.GuardianLinkID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "conversion mapping not found")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get conversion mapping")
		return
	}
	common.WriteJSON(w, http.StatusOK, m, nil)
}

type internalDeactivateRequest struct {
	SchoolID string `json:"school_id"`
	Reason   string `json:"reason"`
}

// InternalDeactivateUser handles POST /internal/users/{id}/deactivate (called on withdrawal/exit).
func (a *App) InternalDeactivateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if strings.TrimSpace(userID) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "user_id is required")
		return
	}

	var req internalDeactivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	schoolID := strings.TrimSpace(req.SchoolID)
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}

	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Deactivate user_school_roles for this user and school
	_, err = tx.ExecContext(ctx, `
		UPDATE user_school_roles
		SET status = 'inactive', deactivated_at = now()
		WHERE user_id = $1 AND school_id = $2 AND status = 'active'
	`, userID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Deactivate guardian_links for this student and school
	_, err = tx.ExecContext(ctx, `
		UPDATE guardian_links
		SET status = 'inactive', deactivated_at = now(), updated_at = now()
		WHERE student_user_id = $1 AND school_id = $2 AND status = 'active'
	`, userID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"deactivated": true,
		"user_id":     userID,
		"school_id":   schoolID,
	}, nil)
}

