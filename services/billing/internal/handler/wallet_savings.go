package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/wallet"
)

// Savings deposits and withdrawals (FR-125-004). Deposits by an admin with a
// proof complete immediately. Withdrawals asked by an admin or the student
// wait for a guardian (pending_parent); a guardian's own request is approved
// at once. An admin then hands over the cash with a proof (completed).
// Deviation (recorded): the guardian approval is stored here, not in core,
// because the approver is the guardian of that one student, while core flows
// are role-based.

// uploadProof stores the multipart "file" as a private proof object.
func (a *App) uploadProof(r *http.Request, schoolID, folder string) (string, error) {
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", &httpError{http.StatusUnprocessableEntity, "bukti (file PDF/JPG/PNG) wajib diunggah"}
	}
	defer file.Close()
	key := fmt.Sprintf("%s/%s/%s", folder, schoolID, randomKey())
	obj, err := a.Storage.Put(r.Context(), key, header.Header.Get("Content-Type"), file, header.Size)
	if err != nil {
		return "", &httpError{http.StatusUnprocessableEntity, "bukti tidak valid: " + err.Error()}
	}
	return obj.Key, nil
}

// savingsRequest is one savings_requests row with its student.
type savingsRequest struct {
	ID            string     `json:"id"`
	WalletID      string     `json:"wallet_id"`
	StudentUserID string     `json:"student_user_id"`
	Kind          string     `json:"kind"`
	AmountIDR     int64      `json:"amount_idr"`
	Note          string     `json:"note"`
	HasProof      bool       `json:"has_proof"`
	RequestedBy   string     `json:"requested_by"`
	RequestedRole string     `json:"requested_role"`
	Status        string     `json:"status"`
	ParentUserID  string     `json:"parent_user_id"`
	CompletedAt   *time.Time `json:"completed_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

const savingsCols = `r.id::text,r.wallet_id::text,w.student_user_id::text,r.kind,r.amount_idr,r.note,r.proof_object_key IS NOT NULL,
	r.requested_by::text,r.requested_role,r.status,COALESCE(r.parent_user_id::text,''),r.completed_at,r.created_at`

func scanSavings(row interface{ Scan(...any) error }) (savingsRequest, error) {
	var s savingsRequest
	var completed sql.NullTime
	err := row.Scan(&s.ID, &s.WalletID, &s.StudentUserID, &s.Kind, &s.AmountIDR, &s.Note, &s.HasProof, &s.RequestedBy,
		&s.RequestedRole, &s.Status, &s.ParentUserID, &completed, &s.CreatedAt)
	if completed.Valid {
		s.CompletedAt = &completed.Time
	}
	return s, err
}

func (a *App) listSavings(ctx context.Context, schoolID string, students []string, status string) ([]savingsRequest, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT `+savingsCols+` FROM savings_requests r JOIN wallets w ON w.id=r.wallet_id
		 WHERE r.school_id=$1 AND ($2::text[] IS NULL OR w.student_user_id::text = ANY($2::text[])) AND ($3='' OR r.status=$3)
		 ORDER BY r.created_at DESC LIMIT 500`, schoolID, students, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []savingsRequest{}
	for rows.Next() {
		s, err := scanSavings(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// createWithdrawal records a withdrawal request after checking the balance.
func (a *App) createWithdrawal(ctx context.Context, schoolID, studentID string, amount int64, note, actorID, role, status string) (string, error) {
	if amount <= 0 {
		return "", &httpError{http.StatusUnprocessableEntity, wallet.ErrAmount.Error()}
	}
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	wl, err := ensureWallet(ctx, tx, schoolID, studentID, wallet.KindSavings)
	if err != nil {
		return "", err
	}
	if wl.Status != "active" {
		return "", &httpError{http.StatusUnprocessableEntity, "dompet tabungan sedang ditutup"}
	}
	var pending int64
	if err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(sum(amount_idr),0) FROM savings_requests WHERE wallet_id=$1 AND kind='withdrawal' AND status IN ('pending_parent','approved')`,
		wl.ID).Scan(&pending); err != nil {
		return "", err
	}
	if amount+pending > wl.BalanceIDR {
		return "", &httpError{http.StatusUnprocessableEntity, "saldo tabungan tidak mencukupi"}
	}
	var parent any
	if role == common.RoleParent {
		parent = actorID
	}
	var id string
	if err = tx.QueryRowContext(ctx,
		`INSERT INTO savings_requests(school_id,wallet_id,kind,amount_idr,note,requested_by,requested_role,status,parent_user_id,parent_decided_at)
		 VALUES($1,$2,'withdrawal',$3,$4,$5,$6,$7,$8,CASE WHEN $8::uuid IS NULL THEN NULL ELSE now() END) RETURNING id::text`,
		schoolID, wl.ID, amount, strings.TrimSpace(note), actorID, role, status, parent).Scan(&id); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	a.audit(ctx, schoolID, "savings_request", id, "withdrawal_requested", role, actorID, "", note, nil,
		map[string]any{"student_user_id": studentID, "amount_idr": amount, "status": status})
	if status == "pending_parent" {
		a.notifyGuardians(ctx, schoolID, studentID, id, amount)
	}
	return id, nil
}

// notifyGuardians tells the student's guardians a withdrawal waits for them.
// Best effort: the request itself is already stored.
func (a *App) notifyGuardians(ctx context.Context, schoolID, studentID, requestID string, amount int64) {
	if !a.IdentitySvc.Configured() || !a.Notification.Configured() {
		return
	}
	guardians, err := a.IdentitySvc.StudentGuardianIDs(ctx, schoolID, studentID)
	if err != nil {
		common.CaptureError(err)
		return
	}
	for _, g := range guardians {
		if err := a.Notification.Notify(ctx, "savings-withdrawal:"+requestID+":"+g, client.Notification{
			SchoolID: schoolID, UserID: g, Type: "wallet_savings_withdrawal",
			Title: "Persetujuan penarikan tabungan",
			Body:  fmt.Sprintf("Ada pengajuan penarikan tabungan anak sebesar Rp%d yang menunggu persetujuan Anda.", amount),
			Data:  map[string]any{"savings_request_id": requestID, "student_user_id": studentID}, Channels: []string{"in_app"},
		}); err != nil {
			common.CaptureError(err)
		}
	}
}

// ParentListSavingsRequests is GET /parent/savings-requests?status= : requests
// of every linked child.
func (a *App) ParentListSavingsRequests(w http.ResponseWriter, r *http.Request) {
	c := common.RequireParent(w, r)
	if c == nil || !a.requireFeature(w, r, c.SchoolID, featureWallet) {
		return
	}
	if !a.IdentitySvc.Configured() {
		common.WriteError(w, http.StatusServiceUnavailable, "layanan identitas belum tersedia")
		return
	}
	children, err := a.IdentitySvc.GuardianChildIDs(r.Context(), c.SchoolID, c.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusBadGateway, "gagal memuat daftar anak")
		return
	}
	if len(children) == 0 {
		common.WriteJSON(w, http.StatusOK, []savingsRequest{}, nil)
		return
	}
	items, err := a.listSavings(r.Context(), c.SchoolID, children, strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// ParentDecideSavingsRequest is POST /parent/savings-requests/{id}/decision {decision: approve|reject}.
func (a *App) ParentDecideSavingsRequest(w http.ResponseWriter, r *http.Request) {
	c := common.RequireParent(w, r)
	if c == nil || !a.requireFeature(w, r, c.SchoolID, featureWallet) {
		return
	}
	var in struct {
		Decision string `json:"decision"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	next := map[string]string{"approve": "approved", "reject": "rejected"}[in.Decision]
	if next == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "decision harus approve atau reject")
		return
	}
	id := param(r, "id")
	s, err := scanSavings(a.DB.QueryRowContext(r.Context(),
		`SELECT `+savingsCols+` FROM savings_requests r JOIN wallets w ON w.id=r.wallet_id WHERE r.id::text=$1 AND r.school_id=$2`, id, c.SchoolID))
	if err == sql.ErrNoRows || (err == nil && !a.guardianCanAccess(r.Context(), c, s.StudentUserID)) {
		common.WriteError(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE savings_requests SET status=$1,parent_user_id=$2,parent_decided_at=now(),updated_at=now() WHERE id::text=$3 AND status='pending_parent'`,
		next, c.UserID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusConflict, "pengajuan sudah diputuskan")
		return
	}
	a.audit(r.Context(), c.SchoolID, "savings_request", id, "withdrawal_"+next, common.RoleParent, c.UserID, "", "",
		map[string]any{"status": s.Status}, map[string]any{"status": next})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": next}, nil)
}

type withdrawalInput struct {
	AmountIDR int64  `json:"amount_idr"`
	Note      string `json:"note"`
}

// ParentCreateWithdrawal is POST /parent/children/{studentUserId}/savings-withdrawals
// (approved at once: the guardian is the approver).
func (a *App) ParentCreateWithdrawal(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireWalletChild(w, r)
	if !ok {
		return
	}
	var in withdrawalInput
	if !common.Decode(w, r, &in) {
		return
	}
	id, err := a.createWithdrawal(r.Context(), c.SchoolID, studentID, in.AmountIDR, in.Note, c.UserID, common.RoleParent, "approved")
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "approved"}, nil)
}

// MyCreateWithdrawal is POST /me/savings-withdrawals (siswa, waits for a guardian).
func (a *App) MyCreateWithdrawal(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil || !a.requireFeature(w, r, c.SchoolID, featureWallet) {
		return
	}
	var in withdrawalInput
	if !common.Decode(w, r, &in) {
		return
	}
	id, err := a.createWithdrawal(r.Context(), c.SchoolID, c.UserID, in.AmountIDR, in.Note, c.UserID, common.RoleStudent, "pending_parent")
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending_parent"}, nil)
}

// MySavingsRequests is GET /me/savings-requests (siswa).
func (a *App) MySavingsRequests(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil || !a.requireFeature(w, r, c.SchoolID, featureWallet) {
		return
	}
	items, err := a.listSavings(r.Context(), c.SchoolID, []string{c.UserID}, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// AdminListSavingsRequests is GET /admin/savings-requests?status=&student_user_id=.
func (a *App) AdminListSavingsRequests(w http.ResponseWriter, r *http.Request) {
	c := common.RequireCashier(w, r)
	if c == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" || !a.requireFeature(w, r, schoolID, featureWallet) {
		return
	}
	var students []string
	if s := strings.TrimSpace(r.URL.Query().Get("student_user_id")); s != "" {
		students = []string{s}
	}
	items, err := a.listSavings(r.Context(), schoolID, students, strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// AdminCreateSavingsRequest is POST /admin/savings-requests (multipart:
// student_user_id, kind deposit|withdrawal, amount_idr, note, file). A
// deposit needs the proof and completes at once; a withdrawal waits for a
// guardian.
func (a *App) AdminCreateSavingsRequest(w http.ResponseWriter, r *http.Request) {
	c := common.RequireCashier(w, r)
	if c == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" || !a.requireFeature(w, r, schoolID, featureWallet) {
		return
	}
	if err := r.ParseMultipartForm(a.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "form tidak valid")
		return
	}
	studentID := strings.TrimSpace(r.FormValue("student_user_id"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	amount, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("amount_idr")), 10, 64)
	note := strings.TrimSpace(r.FormValue("note"))
	if studentID == "" || amount <= 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "siswa dan nominal (> 0) wajib diisi")
		return
	}
	switch kind {
	case "withdrawal":
		id, err := a.createWithdrawal(r.Context(), schoolID, studentID, amount, note, c.UserID, c.ActiveRole, "pending_parent")
		if err != nil {
			writeErr(w, err)
			return
		}
		common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "pending_parent"}, nil)
		return
	case "deposit":
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "kind harus deposit atau withdrawal")
		return
	}
	key, err := a.uploadProof(r, schoolID, "savings")
	if err != nil {
		writeErr(w, err)
		return
	}
	settings, err := loadWalletSettings(r.Context(), a.DB, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	wl, err := ensureWallet(r.Context(), tx, schoolID, studentID, wallet.KindSavings)
	if err != nil {
		writeErr(w, err)
		return
	}
	if wl.Status != "active" {
		common.WriteError(w, http.StatusUnprocessableEntity, "dompet tabungan sedang ditutup")
		return
	}
	if err := wallet.CanTopup(wl.BalanceIDR, amount, settings.MaxFor(wallet.KindSavings)); err != nil {
		writeErr(w, errHTTP(err))
		return
	}
	var id string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO savings_requests(school_id,wallet_id,kind,amount_idr,note,proof_object_key,requested_by,requested_role,status,completed_by,completed_at)
		 VALUES($1,$2,'deposit',$3,$4,$5,$6,$7,'completed',$6,now()) RETURNING id::text`,
		schoolID, wl.ID, amount, note, key, c.UserID, c.ActiveRole).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	if _, _, err = applyMovement(r.Context(), tx, schoolID, &wl, movement{Kind: wallet.MoveSavingsDeposit, AmountIDR: amount,
		RefType: "savings_request", RefID: id, Actor: c.UserID, Key: "savings-deposit:" + id}); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "savings_request", id, "savings_deposited", c.ActiveRole, c.UserID, "", note, nil,
		map[string]any{"student_user_id": studentID, "amount_idr": amount})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "completed", "balance_idr": wl.BalanceIDR}, nil)
}

// AdminCompleteSavingsRequest is POST /admin/savings-requests/{id}/complete
// (multipart file): the cash of an approved withdrawal was handed over.
func (a *App) AdminCompleteSavingsRequest(w http.ResponseWriter, r *http.Request) {
	c := common.RequireCashier(w, r)
	if c == nil {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" || !a.requireFeature(w, r, schoolID, featureWallet) {
		return
	}
	if err := r.ParseMultipartForm(a.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "form tidak valid")
		return
	}
	key, err := a.uploadProof(r, schoolID, "savings")
	if err != nil {
		writeErr(w, err)
		return
	}
	id := param(r, "id")
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var walletID, kind, status string
	var amount int64
	err = tx.QueryRowContext(r.Context(),
		`SELECT wallet_id::text,kind,status,amount_idr FROM savings_requests WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, id, schoolID).
		Scan(&walletID, &kind, &status, &amount)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "pengajuan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if kind != "withdrawal" || status != "approved" {
		common.WriteError(w, http.StatusConflict, "hanya penarikan yang sudah disetujui orang tua yang dapat diselesaikan")
		return
	}
	wl, err := lockWallet(r.Context(), tx, schoolID, walletID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, _, err = applyMovement(r.Context(), tx, schoolID, &wl, movement{Kind: wallet.MoveSavingsWithdrawal, AmountIDR: -amount,
		RefType: "savings_request", RefID: id, Actor: c.UserID, Key: "savings-withdrawal:" + id}); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE savings_requests SET status='completed',proof_object_key=$1,completed_by=$2,completed_at=now(),updated_at=now() WHERE id::text=$3`,
		key, c.UserID, id); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "savings_request", id, "withdrawal_completed", c.ActiveRole, c.UserID, "", "",
		map[string]any{"status": status}, map[string]any{"status": "completed", "amount_idr": amount})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": "completed", "balance_idr": wl.BalanceIDR}, nil)
}
