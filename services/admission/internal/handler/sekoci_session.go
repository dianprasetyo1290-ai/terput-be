package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/admission/internal/common"
)

// SekociSession is POST /public/session/sekoci (BUG-108). A parent already
// signed in to SEKOCI (identity JWT, active role orang_tua) exchanges that
// session for an admission guardian token. The guardian row is found by
// (school_id, identity_user_id) or created and linked on first use, so the
// same parent account sees every application it registered via OTP once the
// OTP flow has linked identity_user_id as well.
func (a *App) SekociSession(w http.ResponseWriter, r *http.Request) {
	claims := common.Admin(r)
	if claims == nil || claims.UserID == "" || claims.SchoolID == "" {
		common.WriteError(w, http.StatusUnauthorized, "sesi SEKOCI tidak valid")
		return
	}
	schoolID := claims.SchoolID
	name := strings.TrimSpace(claims.Name)
	if name == "" {
		name = "Wali Calon Siswa"
	}
	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var guardianID, guardianName string
	created := false
	err = tx.QueryRowContext(ctx,
		`SELECT id,name FROM guardians WHERE school_id=$1 AND identity_user_id=$2`, schoolID, claims.UserID).
		Scan(&guardianID, &guardianName)
	if err == sql.ErrNoRows {
		err = tx.QueryRowContext(ctx,
			`INSERT INTO guardians(school_id,name,identity_user_id) VALUES($1,$2,$3)
			 ON CONFLICT (school_id,identity_user_id) WHERE identity_user_id IS NOT NULL DO UPDATE SET updated_at=now()
			 RETURNING id,name`, schoolID, name, claims.UserID).Scan(&guardianID, &guardianName)
		created = true
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	action := "guardian_sekoci_login"
	if created {
		action = "guardian_linked_sekoci"
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'guardian',$2,$3,'guardian',$2,$4,$5)`,
		schoolID, guardianID, action, guardianName, jsonRaw(map[string]any{"identity_user_id": claims.UserID})); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	token, expiresAt, err := a.guardianToken(guardianID, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat sesi")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_at": expiresAt,
		"expires_in": int64(a.Config.GuardianTTL.Seconds()),
		"guardian": map[string]any{
			"id":           guardianID,
			"name":         guardianName,
			"contact":      "",
			"contact_type": "sekoci",
			"verified_at":  time.Now(),
		},
	}, nil)
}
