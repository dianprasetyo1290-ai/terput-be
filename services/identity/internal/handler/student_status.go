package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/rules"
)

// Student status calon/aktif/lulus/keluar (BUG-017) with controlled
// transitions (BUG-075). The status lives on the siswa row of
// user_school_roles (migration 0020); user_school_roles.status follows it
// (only calon/aktif may sign in) so the session check revokes access of a
// student who left or graduated. Every change is kept in
// student_status_history and audited.

type studentStatusRequest struct {
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	Correction bool   `json:"correction"`
}

// studentSchool resolves which unit's siswa row is meant: the caller's unit,
// or for superadmin ?school_id= (or the student's only unit).
func (a *App) studentSchool(r *http.Request, userID string) (string, error) {
	claims := common.FromCtx(r)
	if claims.ActiveRole != common.RoleSuperadmin {
		return claims.SchoolID, nil
	}
	if q := r.URL.Query().Get("school_id"); q != "" {
		return q, nil
	}
	var sid string
	err := a.DB.QueryRow(`SELECT school_id::text FROM user_school_roles WHERE user_id=$1 AND role='siswa' AND school_id IS NOT NULL
		ORDER BY created_at LIMIT 1`, userID).Scan(&sid)
	return sid, err
}

// ChangeStudentStatus: POST /students/{id}/status {status, reason, correction}.
func (a *App) ChangeStudentStatus(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin, common.RoleKepalaSekolah) {
		return
	}
	claims := common.FromCtx(r)
	userID := chi.URLParam(r, "id")
	var req studentStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format data tidak valid")
		return
	}
	req.Status = strings.TrimSpace(strings.ToLower(req.Status))
	schoolID, err := a.studentSchool(r, userID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && schoolID == "") {
		common.WriteError(w, http.StatusNotFound, "Siswa tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal mengubah status siswa")
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengubah status siswa")
		return
	}
	defer tx.Rollback()
	var roleStatus string
	var cur sql.NullString
	err = tx.QueryRow(`SELECT usr.status, usr.student_status FROM user_school_roles usr
		JOIN users u ON u.id = usr.user_id AND u.deleted_at IS NULL
		WHERE usr.user_id=$1 AND usr.school_id=$2 AND usr.role='siswa' FOR UPDATE OF usr`, userID, schoolID).Scan(&roleStatus, &cur)
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "Siswa tidak ditemukan di unit ini")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal mengubah status siswa")
		return
	}
	from := cur.String
	if !cur.Valid { // rows created before migration 0020 by other paths
		from = rules.StudentAktif
		if roleStatus != "active" {
			from = rules.StudentKeluar
		}
	}
	if msg := rules.StudentTransition(from, req.Status, req.Correction, claims.ActiveRole, req.Reason); msg != "" {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, msg, map[string]string{"status": msg})
		return
	}
	if _, err := tx.Exec(`UPDATE user_school_roles SET student_status=$1, status=$2 WHERE user_id=$3 AND school_id=$4 AND role='siswa'`,
		req.Status, rules.RoleStatusFor(req.Status), userID, schoolID); err != nil {
		common.WriteDBError(w, err, "Gagal mengubah status siswa")
		return
	}
	if _, err := tx.Exec(`INSERT INTO student_status_history (user_id, school_id, from_status, to_status, reason, correction, changed_by)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7)`, userID, schoolID, from, req.Status, strings.TrimSpace(req.Reason), req.Correction, nullUUID(claims.UserID)); err != nil {
		common.WriteDBError(w, err, "Gagal mengubah status siswa")
		return
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengubah status siswa")
		return
	}
	action := "student_status_changed"
	if req.Correction {
		action = "student_status_corrected"
	}
	a.RecordSecurityAudit(&schoolID, action, userID, &claims.UserID, &claims.ActiveRole,
		map[string]any{"student_status": from}, map[string]any{"student_status": req.Status}, req.Reason, r.Header.Get("X-Correlation-ID"))
	common.WriteJSON(w, http.StatusOK, map[string]any{"user_id": userID, "school_id": schoolID, "from": from, "student_status": req.Status}, nil)
}

type studentStatusEvent struct {
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Reason     *string   `json:"reason"`
	Correction bool      `json:"correction"`
	ChangedBy  *string   `json:"changed_by"`
	ChangedAt  time.Time `json:"changed_at"`
}

// StudentStatusHistory: GET /students/{id}/status-history.
func (a *App) StudentStatusHistory(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin, common.RoleKepalaSekolah) {
		return
	}
	userID := chi.URLParam(r, "id")
	schoolID, err := a.studentSchool(r, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		common.WriteDBError(w, err, "Gagal memuat riwayat status")
		return
	}
	rows, err := a.DB.Query(`SELECT from_status, to_status, reason, correction, changed_by::text, changed_at
		FROM student_status_history WHERE user_id=$1 AND school_id::text=$2 ORDER BY changed_at DESC LIMIT 200`, userID, schoolID)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat riwayat status")
		return
	}
	defer rows.Close()
	out := []studentStatusEvent{}
	for rows.Next() {
		var e studentStatusEvent
		if err := rows.Scan(&e.FromStatus, &e.ToStatus, &e.Reason, &e.Correction, &e.ChangedBy, &e.ChangedAt); err != nil {
			common.WriteDBError(w, err, "Gagal memuat riwayat status")
			return
		}
		out = append(out, e)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
