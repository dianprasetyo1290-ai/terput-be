package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
)

// Student cards (OQ-30): a static QR per student, shared by POS, library and
// transport. The code is derived from the card id with an HMAC so the
// student portal can show it again without the database storing it; only
// sha256(code) is stored and looked up. Revoking a card kills its code.

const cardCodePrefix = "SC"

// cardCode derives the printable code of a card.
func (a *App) cardCode(cardID string) string {
	mac := hmac.New(sha256.New, []byte(a.Config.Wallet.CardSecret))
	mac.Write([]byte("student-card:" + cardID))
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(mac.Sum(nil))
	return cardCodePrefix + enc[:24]
}

// cardHash is the lookup hash of a scanned code (case/space-insensitive).
func cardHash(code string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(code))))
	return hex.EncodeToString(sum[:])
}

// resolveCard maps a scanned code to the student of an active card.
func resolveCard(ctx context.Context, q queryer, schoolID, code string) (studentID, cardID string, err error) {
	err = q.QueryRowContext(ctx,
		`SELECT student_user_id::text,id::text FROM student_cards WHERE school_id=$1 AND token_hash=$2 AND status='active'`,
		schoolID, cardHash(code)).Scan(&studentID, &cardID)
	if err == sql.ErrNoRows {
		return "", "", &httpError{http.StatusNotFound, "Kartu tidak berlaku"}
	}
	return studentID, cardID, err
}

// AdminIssueStudentCard is POST /admin/student-cards {student_user_id}. Any
// active card of the student is revoked; the new code is returned once.
func (a *App) AdminIssueStudentCard(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	var in struct {
		StudentUserID string `json:"student_user_id"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.StudentUserID = strings.TrimSpace(in.StudentUserID)
	if in.StudentUserID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "student_user_id wajib diisi")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var revoked []string
	rows, err := tx.QueryContext(r.Context(),
		`UPDATE student_cards SET status='revoked',revoked_at=now() WHERE school_id=$1 AND student_user_id::text=$2 AND status='active' RETURNING id::text`,
		schoolID, in.StudentUserID)
	if err != nil {
		writeErr(w, err)
		return
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		revoked = append(revoked, id)
	}
	rows.Close()
	var cardID string
	if err = tx.QueryRowContext(r.Context(), `SELECT gen_random_uuid()::text`).Scan(&cardID); err != nil {
		writeErr(w, err)
		return
	}
	code := a.cardCode(cardID)
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO student_cards(id,school_id,student_user_id,token_hash,issued_by) VALUES($1,$2,$3,$4,$5)`,
		cardID, schoolID, in.StudentUserID, cardHash(code), c.UserID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "student_card", cardID, "student_card_issued", c.ActiveRole, c.UserID, "", "",
		map[string]any{"revoked": revoked}, map[string]any{"student_user_id": in.StudentUserID})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"card_id": cardID, "student_user_id": in.StudentUserID, "code": code, "revoked": revoked}, nil)
}

// AdminRevokeStudentCard is POST /admin/student-cards/{id}/revoke.
func (a *App) AdminRevokeStudentCard(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	id := param(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE student_cards SET status='revoked',revoked_at=now() WHERE id::text=$1 AND school_id=$2 AND status='active'`, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists bool
		_ = a.DB.QueryRowContext(r.Context(), `SELECT true FROM student_cards WHERE id::text=$1 AND school_id=$2`, id, schoolID).Scan(&exists)
		if !exists {
			common.WriteError(w, http.StatusNotFound, "kartu tidak ditemukan")
			return
		}
	} else {
		a.audit(r.Context(), schoolID, "student_card", id, "student_card_revoked", c.ActiveRole, c.UserID, "", "", map[string]any{"status": "active"}, map[string]any{"status": "revoked"})
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"card_id": id, "status": "revoked"}, nil)
}

// AdminListStudentCards is GET /admin/student-cards?student_user_id=.
func (a *App) AdminListStudentCards(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	student := strings.TrimSpace(r.URL.Query().Get("student_user_id"))
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text,student_user_id::text,status,issued_by::text,revoked_at,created_at FROM student_cards
		 WHERE school_id=$1 AND ($2='' OR student_user_id::text=$2) ORDER BY created_at DESC LIMIT 200`, schoolID, student)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, sid, status, by string
		var revokedAt sql.NullTime
		var created time.Time
		if err := rows.Scan(&id, &sid, &status, &by, &revokedAt, &created); err != nil {
			writeErr(w, err)
			return
		}
		m := map[string]any{"id": id, "student_user_id": sid, "status": status, "issued_by": by, "created_at": created, "revoked_at": nil}
		if revokedAt.Valid {
			m["revoked_at"] = revokedAt.Time
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// MyStudentCard is GET /me/student-card (siswa): the active card's code for
// the QR shown in the student portal.
func (a *App) MyStudentCard(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil {
		return
	}
	var id string
	var created time.Time
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT id::text,created_at FROM student_cards WHERE school_id=$1 AND student_user_id::text=$2 AND status='active'`,
		c.SchoolID, c.UserID).Scan(&id, &created)
	if err == sql.ErrNoRows {
		common.WriteJSON(w, http.StatusOK, nil, nil)
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"card_id": id, "code": a.cardCode(id), "issued_at": created}, nil)
}

// InternalResolveStudentCard is POST /internal/student-cards/resolve {school_id, code}
// (§4.8): 200 {student_user_id, card_id} or 404 when unknown or revoked.
func (a *App) InternalResolveStudentCard(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SchoolID string `json:"school_id"`
		Code     string `json:"code"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.SchoolID) == "" || strings.TrimSpace(in.Code) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id dan code wajib diisi")
		return
	}
	student, cardID, err := resolveCard(r.Context(), a.DB, strings.TrimSpace(in.SchoolID), in.Code)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"student_user_id": student, "card_id": cardID}, nil)
}
