package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/wallet"
)

// Feature keys (T9 §2).
const (
	featureCanteen = "canteen"
	featureWallet  = "wallet"
)

// Core request types owned by T12 (added to core KnownRequestTypes).
const (
	reqTypeCanteenSettlement = "canteen_settlement"
	reqTypeWalletTransfer    = "wallet_transfer"
)

// now is the handler clock (overridable in tests).
func (a *App) now() time.Time {
	if a.Clock != nil {
		return a.Clock()
	}
	return time.Now()
}

// loc is the school business time zone (APP_TIMEZONE, default Asia/Jakarta).
func (a *App) loc() *time.Location {
	if a.Config.Gateway.Location != nil {
		return a.Config.Gateway.Location
	}
	l, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*3600)
	}
	return l
}

// today is the current business date (midnight in loc).
func (a *App) today() time.Time {
	y, m, d := a.now().In(a.loc()).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, a.loc())
}

// featureOn asks identity whether a module is enabled for the school. It
// fails closed: no client or any error reads as off.
func (a *App) featureOn(ctx context.Context, schoolID, key string) bool {
	if !a.IdentitySvc.Configured() {
		return false
	}
	on, err := a.IdentitySvc.FeatureEnabled(ctx, schoolID, key)
	if err != nil {
		common.CaptureError(err)
		return false
	}
	return on
}

// requireFeature writes 403 when the module is switched off for the school.
func (a *App) requireFeature(w http.ResponseWriter, r *http.Request, schoolID, key string) bool {
	if a.featureOn(r.Context(), schoolID, key) {
		return true
	}
	label := map[string]string{featureCanteen: "Kantin & POS", featureWallet: "Dompet Siswa"}[key]
	common.WriteError(w, http.StatusForbidden, "Modul "+label+" belum diaktifkan untuk sekolah ini")
	return false
}

// walletRow is one wallets row.
type walletRow struct {
	ID            string `json:"id"`
	StudentUserID string `json:"student_user_id"`
	Kind          string `json:"kind"`
	BalanceIDR    int64  `json:"balance_idr"`
	DailyLimitIDR *int64 `json:"daily_limit_idr"`
	Status        string `json:"status"`
}

const walletCols = `id::text,student_user_id::text,kind,balance_idr,daily_limit_idr,status`

func scanWallet(row interface{ Scan(...any) error }) (walletRow, error) {
	var w walletRow
	var limit sql.NullInt64
	err := row.Scan(&w.ID, &w.StudentUserID, &w.Kind, &w.BalanceIDR, &limit, &w.Status)
	if limit.Valid {
		w.DailyLimitIDR = &limit.Int64
	}
	return w, err
}

// ensureWallet returns the student's wallet of a kind, creating it on first
// use, locked FOR UPDATE inside tx.
func ensureWallet(ctx context.Context, tx *sql.Tx, schoolID, studentID, kind string) (walletRow, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO wallets(school_id,student_user_id,kind) VALUES($1,$2,$3) ON CONFLICT (school_id,student_user_id,kind) DO NOTHING`,
		schoolID, studentID, kind); err != nil {
		return walletRow{}, err
	}
	return scanWallet(tx.QueryRowContext(ctx,
		`SELECT `+walletCols+` FROM wallets WHERE school_id=$1 AND student_user_id=$2 AND kind=$3 FOR UPDATE`,
		schoolID, studentID, kind))
}

// lockWallet locks an existing wallet by id.
func lockWallet(ctx context.Context, tx *sql.Tx, schoolID, walletID string) (walletRow, error) {
	w, err := scanWallet(tx.QueryRowContext(ctx,
		`SELECT `+walletCols+` FROM wallets WHERE id::text=$1 AND school_id=$2 FOR UPDATE`, walletID, schoolID))
	if err == sql.ErrNoRows {
		return w, &httpError{http.StatusNotFound, "dompet tidak ditemukan"}
	}
	return w, err
}

// movement is one wallet movement to append.
type movement struct {
	Kind, RefType, RefID, Actor, Key string
	AmountIDR                        int64
}

// applyMovement appends a movement and updates the (locked) wallet balance in
// the same transaction. A repeated idempotency key returns the original
// movement with dup=true and changes nothing.
func applyMovement(ctx context.Context, tx *sql.Tx, schoolID string, w *walletRow, m movement) (id string, dup bool, err error) {
	var after int64
	err = tx.QueryRowContext(ctx,
		`SELECT id::text,balance_after_idr FROM wallet_movements WHERE school_id=$1 AND idempotency_key=$2`,
		schoolID, m.Key).Scan(&id, &after)
	if err == nil {
		return id, true, nil
	}
	if err != sql.ErrNoRows {
		return "", false, err
	}
	next := w.BalanceIDR + m.AmountIDR
	if next < 0 {
		return "", false, &httpError{http.StatusUnprocessableEntity, wallet.ErrInsufficient.Error()}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE wallets SET balance_idr=$1,updated_at=now() WHERE id=$2`, next, w.ID); err != nil {
		return "", false, err
	}
	err = tx.QueryRowContext(ctx,
		`INSERT INTO wallet_movements(school_id,wallet_id,kind,amount_idr,balance_after_idr,ref_type,ref_id,created_by,idempotency_key)
		 VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$8,$9) RETURNING id::text`,
		schoolID, w.ID, m.Kind, m.AmountIDR, next, m.RefType, m.RefID, m.Actor, m.Key).Scan(&id)
	if err != nil {
		return "", false, err
	}
	w.BalanceIDR = next
	return id, false, nil
}

// walletSettings is a school's wallet_settings row (defaults when absent).
type walletSettings struct {
	MaxAllowanceBalanceIDR *int64 `json:"max_allowance_balance_idr"`
	MaxSavingsBalanceIDR   *int64 `json:"max_savings_balance_idr"`
	PosOfflineMaxMinutes   int    `json:"pos_offline_max_minutes"`
}

// MaxFor returns the balance cap of a kind (nil = no cap).
func (s walletSettings) MaxFor(kind string) *int64 {
	if kind == wallet.KindSavings {
		return s.MaxSavingsBalanceIDR
	}
	return s.MaxAllowanceBalanceIDR
}

func loadWalletSettings(ctx context.Context, q queryer, schoolID string) (walletSettings, error) {
	var s walletSettings
	var maxA, maxS sql.NullInt64
	// The row is created on first read so the defaults come from the
	// table's column defaults, not from code.
	if _, err := q.ExecContext(ctx, `INSERT INTO wallet_settings(school_id) VALUES($1) ON CONFLICT (school_id) DO NOTHING`, schoolID); err != nil {
		return s, err
	}
	err := q.QueryRowContext(ctx,
		`SELECT max_allowance_balance_idr,max_savings_balance_idr,pos_offline_max_minutes FROM wallet_settings WHERE school_id=$1`,
		schoolID).Scan(&maxA, &maxS, &s.PosOfflineMaxMinutes)
	if maxA.Valid {
		s.MaxAllowanceBalanceIDR = &maxA.Int64
	}
	if maxS.Valid {
		s.MaxSavingsBalanceIDR = &maxS.Int64
	}
	return s, err
}

// loadWalletAccounts reads the school's account mapping; ok=false when unmapped.
func loadWalletAccounts(ctx context.Context, q queryer, schoolID string) (wallet.Accounts, bool, error) {
	var acc wallet.Accounts
	var rent, recv sql.NullString
	err := q.QueryRowContext(ctx,
		`SELECT allowance_liability_acc,savings_liability_acc,cash_acc,tenant_payable_acc,canteen_revenue_acc,rent_revenue_acc,tenant_receivable_acc
		 FROM wallet_accounts WHERE school_id=$1`, schoolID).
		Scan(&acc.AllowanceLiability, &acc.SavingsLiability, &acc.Cash, &acc.TenantPayable, &acc.CanteenRevenue, &rent, &recv)
	if err == sql.ErrNoRows {
		return acc, false, nil
	}
	acc.RentRevenue, acc.TenantReceivable = rent.String, recv.String
	return acc, err == nil, err
}

// studentWallets returns both wallet kinds of a student (zero rows when not
// yet created are reported as empty active wallets).
func (a *App) studentWallets(ctx context.Context, schoolID, studentID string) ([]walletRow, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT `+walletCols+` FROM wallets WHERE school_id=$1 AND student_user_id::text=$2`, schoolID, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byKind := map[string]walletRow{}
	for rows.Next() {
		w, err := scanWallet(rows)
		if err != nil {
			return nil, err
		}
		byKind[w.Kind] = w
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]walletRow, 0, len(wallet.Kinds))
	for _, k := range wallet.Kinds {
		w, ok := byKind[k]
		if !ok {
			w = walletRow{StudentUserID: studentID, Kind: k, Status: "active"}
		}
		out = append(out, w)
	}
	return out, nil
}

// walletMovements lists a wallet's movements, newest first, paginated.
func (a *App) walletMovements(w http.ResponseWriter, r *http.Request, schoolID, studentID, kind string) {
	if !wallet.ValidKind(kind) {
		common.WriteError(w, http.StatusNotFound, "jenis dompet tidak dikenal")
		return
	}
	page, perPage := common.Page(r)
	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(*) FROM wallet_movements m JOIN wallets w ON w.id=m.wallet_id
		 WHERE w.school_id=$1 AND w.student_user_id::text=$2 AND w.kind=$3`, schoolID, studentID, kind).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT m.id::text,m.kind,m.amount_idr,m.balance_after_idr,m.ref_type,COALESCE(m.ref_id::text,''),m.created_at
		 FROM wallet_movements m JOIN wallets w ON w.id=m.wallet_id
		 WHERE w.school_id=$1 AND w.student_user_id::text=$2 AND w.kind=$3
		 ORDER BY m.created_at DESC LIMIT $4 OFFSET $5`, schoolID, studentID, kind, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, mk, refType, refID string
		var amount, after int64
		var at time.Time
		if err := rows.Scan(&id, &mk, &amount, &after, &refType, &refID, &at); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "kind": mk, "amount_idr": amount, "balance_after_idr": after,
			"ref_type": refType, "ref_id": refID, "created_at": at})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// AdminListWallets is GET /admin/wallets?q=&status= (admin, canteen_admin, and
// finance_manager read-only: PRD menu 125 "Finance Admin" = finance_manager,
// the only finance role identity issues; QA-PAY round 4).
// Grouped per student: both wallet kinds and the active card, if any.
func (a *App) AdminListWallets(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleCanteenAdmin, common.RoleFinanceManager)
	if schoolID == "" || !a.requireFeature(w, r, schoolID, featureWallet) {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	page, perPage := common.Page(r)
	var total int
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT count(DISTINCT student_user_id) FROM wallets
		 WHERE school_id=$1 AND ($2='' OR student_user_id::text LIKE $2||'%') AND ($3='' OR status=$3)`,
		schoolID, q, status).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT s.student_user_id::text,
		        COALESCE(max(CASE WHEN w.kind='allowance' THEN w.balance_idr END),0),
		        COALESCE(max(CASE WHEN w.kind='savings' THEN w.balance_idr END),0),
		        max(CASE WHEN w.kind='allowance' THEN w.daily_limit_idr END),
		        string_agg(DISTINCT w.status, ','),
		        (SELECT c.id::text FROM student_cards c WHERE c.school_id=s.school_id AND c.student_user_id=s.student_user_id AND c.status='active')
		 FROM (SELECT DISTINCT school_id,student_user_id FROM wallets
		        WHERE school_id=$1 AND ($2='' OR student_user_id::text LIKE $2||'%') AND ($3='' OR status=$3)) s
		 JOIN wallets w ON w.school_id=s.school_id AND w.student_user_id=s.student_user_id
		 GROUP BY s.school_id,s.student_user_id ORDER BY s.student_user_id LIMIT $4 OFFSET $5`,
		schoolID, q, status, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var student, statuses string
		var allowance, savings int64
		var limit sql.NullInt64
		var cardID sql.NullString
		if err := rows.Scan(&student, &allowance, &savings, &limit, &statuses, &cardID); err != nil {
			writeErr(w, err)
			return
		}
		m := map[string]any{"student_user_id": student, "allowance_idr": allowance, "savings_idr": savings,
			"daily_limit_idr": nil, "statuses": strings.Split(statuses, ","), "card_id": cardID.String}
		if limit.Valid {
			m["daily_limit_idr"] = limit.Int64
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// AdminStudentWallets is GET /admin/students/{studentUserId}/wallets.
func (a *App) AdminStudentWallets(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleCanteenAdmin, common.RoleFinanceManager)
	if schoolID == "" || !a.requireFeature(w, r, schoolID, featureWallet) {
		return
	}
	ws, err := a.studentWallets(r.Context(), schoolID, param(r, "studentUserId"))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, ws, nil)
}

// AdminCloseWallet is POST /admin/wallets/{id}/close {disposition, target_student_user_id?}
// (FR-125-005). refund -> a menu 25 refund request with wallet_id, decided
// in core (request type refund) and paid out from menu 25; transfer -> core
// approval wallet_transfer, executed by the callback.
func (a *App) AdminCloseWallet(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireRoles(w, r, common.RoleAdmin)
	if schoolID == "" || !a.requireFeature(w, r, schoolID, featureWallet) {
		return
	}
	var in struct {
		Disposition         string `json:"disposition"`
		TargetStudentUserID string `json:"target_student_user_id"`
		Reason              string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.TargetStudentUserID = strings.TrimSpace(in.TargetStudentUserID)
	if in.Disposition != "refund" && in.Disposition != "transfer" {
		common.WriteError(w, http.StatusUnprocessableEntity, "disposition harus refund atau transfer")
		return
	}
	if in.Disposition == "transfer" && in.TargetStudentUserID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "siswa tujuan wajib diisi untuk pemindahan saldo")
		return
	}
	if in.Disposition == "transfer" && in.TargetStudentUserID != "" && !a.Core.Configured() {
		common.WriteError(w, http.StatusServiceUnavailable, "Approval Center belum tersedia")
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "Penutupan dompet siswa"
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	wl, err := lockWallet(r.Context(), tx, schoolID, param(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if wl.Status != "active" {
		common.WriteError(w, http.StatusConflict, "dompet sedang/sudah ditutup")
		return
	}
	if in.Disposition == "transfer" && in.TargetStudentUserID == wl.StudentUserID {
		common.WriteError(w, http.StatusUnprocessableEntity, "siswa tujuan harus berbeda")
		return
	}
	if wl.BalanceIDR == 0 {
		if _, err = tx.ExecContext(r.Context(), `UPDATE wallets SET status='closed',updated_at=now() WHERE id=$1`, wl.ID); err != nil {
			writeErr(w, err)
			return
		}
		if err = tx.Commit(); err != nil {
			writeErr(w, err)
			return
		}
		a.audit(r.Context(), schoolID, "wallet", wl.ID, "wallet_closed", "admin", c.UserID, "", reason, nil, map[string]any{"status": "closed"})
		common.WriteJSON(w, http.StatusOK, map[string]any{"wallet_id": wl.ID, "status": "closed"}, nil)
		return
	}
	// Both dispositions wait for core's Approval Center: a transfer as
	// wallet_transfer, a refund as a Menu 25 refund (request type refund).
	status := "pending_approval"
	var closureID string
	if err = tx.QueryRowContext(r.Context(),
		`INSERT INTO wallet_closures(school_id,wallet_id,disposition,target_student_user_id,amount_idr,status,requested_by)
		 VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5,$6,$7) RETURNING id::text`,
		schoolID, wl.ID, in.Disposition, in.TargetStudentUserID, wl.BalanceIDR, status, c.UserID).Scan(&closureID); err != nil {
		writeErr(w, err)
		return
	}
	var refundID string
	if in.Disposition == "refund" {
		if err = tx.QueryRowContext(r.Context(),
			`INSERT INTO bill_refunds(school_id,student_bill_id,student_user_id,amount_idr,reason,requested_by,idempotency_key,wallet_id)
			 VALUES($1,NULL,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
			schoolID, wl.StudentUserID, wl.BalanceIDR, reason, c.UserID, "wallet-close:"+closureID, wl.ID).Scan(&refundID); err != nil {
			writeErr(w, err)
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE wallet_closures SET refund_id=$1 WHERE id=$2`, refundID, closureID); err != nil {
			writeErr(w, err)
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE wallets SET status='closing',updated_at=now() WHERE id=$1`, wl.ID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	if in.Disposition == "refund" {
		amount := wl.BalanceIDR
		resp, err := a.submitApproval(r.Context(), client.ApprovalRequest{
			SchoolID: schoolID, RequestType: reqTypeRefund, SourceID: refundID,
			Title: "Refund saldo dompet siswa",
			Summary: map[string]any{"wallet_id": wl.ID, "kind": wl.Kind, "student_user_id": wl.StudentUserID,
				"amount_idr": amount, "reason": reason, "origin": "wallet_closure", "closure_id": closureID},
			AmountIDR: &amount, MakerUserID: c.UserID, SubjectUserID: wl.StudentUserID,
			CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/refunds/" + refundID + "/callback",
			CorrelationID: refundID, IdempotencyKey: "refund:" + refundID,
		})
		if err != nil {
			// Nothing moved yet: drop the request and reopen the wallet so the
			// admin can retry once the refund flow exists in core.
			_, _ = a.DB.ExecContext(r.Context(), `UPDATE wallet_closures SET status='rejected',refund_id=NULL,updated_at=now() WHERE id=$1`, closureID)
			_, _ = a.DB.ExecContext(r.Context(), `DELETE FROM bill_refunds WHERE id::text=$1 AND status='requested'`, refundID)
			_, _ = a.DB.ExecContext(r.Context(), `UPDATE wallets SET status='active',updated_at=now() WHERE id=$1`, wl.ID)
			a.reportRefundApprovalFailure(r.Context(), schoolID, wl.StudentUserID, refundID, "wallet-close:"+closureID, reason, amount, err)
			writeErr(w, err)
			return
		}
		_, _ = a.DB.ExecContext(r.Context(), `UPDATE bill_refunds SET approval_id=$1,updated_at=now() WHERE id::text=$2`, resp.ID, refundID)
		_, _ = a.DB.ExecContext(r.Context(), `UPDATE wallet_closures SET approval_id=$1,updated_at=now() WHERE id=$2`, resp.ID, closureID)
	}
	if in.Disposition == "transfer" {
		amount := wl.BalanceIDR
		resp, err := a.Core.RequestApproval(r.Context(), client.ApprovalRequest{
			SchoolID: schoolID, RequestType: reqTypeWalletTransfer, SourceID: closureID,
			Title:     "Pemindahan saldo dompet siswa",
			Summary:   map[string]any{"wallet_id": wl.ID, "kind": wl.Kind, "from_student_user_id": wl.StudentUserID, "to_student_user_id": in.TargetStudentUserID, "reason": reason},
			AmountIDR: &amount, MakerUserID: c.UserID, SubjectUserID: wl.StudentUserID,
			CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/wallets/transfer-callback",
			CorrelationID: closureID, IdempotencyKey: "wallet-transfer:" + closureID,
		})
		if err != nil {
			// Roll the closure back so the admin can retry; nothing moved yet.
			_, _ = a.DB.ExecContext(r.Context(), `UPDATE wallet_closures SET status='rejected',updated_at=now() WHERE id=$1`, closureID)
			_, _ = a.DB.ExecContext(r.Context(), `UPDATE wallets SET status='active',updated_at=now() WHERE id=$1`, wl.ID)
			common.WriteError(w, http.StatusBadGateway, "gagal mengajukan approval: "+err.Error())
			return
		}
		_, _ = a.DB.ExecContext(r.Context(), `UPDATE wallet_closures SET approval_id=$1,updated_at=now() WHERE id=$2`, resp.ID, closureID)
	}
	a.audit(r.Context(), schoolID, "wallet", wl.ID, "wallet_close_requested", "admin", c.UserID, "", reason, nil,
		map[string]any{"closure_id": closureID, "disposition": in.Disposition, "amount_idr": wl.BalanceIDR, "refund_id": refundID})
	common.WriteJSON(w, http.StatusOK, map[string]any{"wallet_id": wl.ID, "closure_id": closureID, "status": status, "refund_id": refundID}, nil)
}

// WalletTransferCallback is POST /internal/wallets/transfer-callback (core).
// Idempotent: a closure that is no longer pending is acknowledged as-is.
func (a *App) WalletTransferCallback(w http.ResponseWriter, r *http.Request) {
	var cb client.ApprovalCallback
	if !common.Decode(w, r, &cb) {
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var schoolID, walletID, target, status string
	err = tx.QueryRowContext(r.Context(),
		`SELECT school_id::text,wallet_id::text,COALESCE(target_student_user_id::text,''),status FROM wallet_closures
		 WHERE id::text=$1 AND disposition='transfer' FOR UPDATE`, cb.SourceID).Scan(&schoolID, &walletID, &target, &status)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "permintaan pemindahan tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != "pending_approval" {
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": cb.SourceID, "status": status}, nil)
		return
	}
	src, err := lockWallet(r.Context(), tx, schoolID, walletID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if cb.Decision != "approved" {
		if _, err = tx.ExecContext(r.Context(), `UPDATE wallet_closures SET status='rejected',updated_at=now() WHERE id::text=$1`, cb.SourceID); err == nil {
			_, err = tx.ExecContext(r.Context(), `UPDATE wallets SET status='active',updated_at=now() WHERE id=$1`, src.ID)
		}
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			writeErr(w, err)
			return
		}
		a.audit(r.Context(), schoolID, "wallet", src.ID, "wallet_transfer_rejected", "core", cb.DecidedBy, "", cb.Comment, nil, nil)
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": cb.SourceID, "status": "rejected"}, nil)
		return
	}
	dst, err := ensureWallet(r.Context(), tx, schoolID, target, src.Kind)
	if err != nil {
		writeErr(w, err)
		return
	}
	amount := src.BalanceIDR
	if amount > 0 {
		if _, _, err = applyMovement(r.Context(), tx, schoolID, &src, movement{Kind: wallet.MoveTransferOut, AmountIDR: -amount,
			RefType: "wallet_closure", RefID: cb.SourceID, Actor: "core:" + cb.DecidedBy, Key: "wallet-transfer-out:" + cb.SourceID}); err != nil {
			writeErr(w, err)
			return
		}
		if _, _, err = applyMovement(r.Context(), tx, schoolID, &dst, movement{Kind: wallet.MoveTransferIn, AmountIDR: amount,
			RefType: "wallet_closure", RefID: cb.SourceID, Actor: "core:" + cb.DecidedBy, Key: "wallet-transfer-in:" + cb.SourceID}); err != nil {
			writeErr(w, err)
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE wallets SET status='closed',updated_at=now() WHERE id=$1`, src.ID); err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE wallet_closures SET status='completed',approval_id=COALESCE(approval_id,NULLIF($2,'')),updated_at=now() WHERE id::text=$1`, cb.SourceID, cb.ApprovalID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "wallet", src.ID, "wallet_transferred", "core", cb.DecidedBy, "", cb.Comment, nil,
		map[string]any{"to_wallet_id": dst.ID, "amount_idr": amount})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": cb.SourceID, "status": "completed"}, nil)
}

// GetWalletSettings is GET /admin/wallet-settings (finance_manager, admin).
func (a *App) GetWalletSettings(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleFinanceManager)
	if schoolID == "" {
		return
	}
	s, err := loadWalletSettings(r.Context(), a.DB, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, s, nil)
}

// PutWalletSettings is PUT /admin/wallet-settings (OQ-29, NFR-OPS-02).
func (a *App) PutWalletSettings(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleFinanceManager)
	if schoolID == "" {
		return
	}
	var in walletSettings
	if !common.Decode(w, r, &in) {
		return
	}
	if in.PosOfflineMaxMinutes <= 0 || (in.MaxAllowanceBalanceIDR != nil && *in.MaxAllowanceBalanceIDR <= 0) ||
		(in.MaxSavingsBalanceIDR != nil && *in.MaxSavingsBalanceIDR <= 0) {
		common.WriteError(w, http.StatusUnprocessableEntity, "batas saldo harus > 0 (kosongkan untuk tanpa batas) dan batas luring > 0 menit")
		return
	}
	before, _ := loadWalletSettings(r.Context(), a.DB, schoolID)
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO wallet_settings(school_id,max_allowance_balance_idr,max_savings_balance_idr,pos_offline_max_minutes)
		 VALUES($1,$2,$3,$4) ON CONFLICT (school_id) DO UPDATE SET max_allowance_balance_idr=EXCLUDED.max_allowance_balance_idr,
		 max_savings_balance_idr=EXCLUDED.max_savings_balance_idr,pos_offline_max_minutes=EXCLUDED.pos_offline_max_minutes,updated_at=now()`,
		schoolID, in.MaxAllowanceBalanceIDR, in.MaxSavingsBalanceIDR, in.PosOfflineMaxMinutes); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "wallet_settings", schoolID, "wallet_settings_updated", c.ActiveRole, c.UserID, "", "", before, in)
	common.WriteJSON(w, http.StatusOK, in, nil)
}

// GetWalletAccounts is GET /admin/wallet-accounts.
func (a *App) GetWalletAccounts(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleFinanceManager)
	if schoolID == "" {
		return
	}
	acc, ok, err := loadWalletAccounts(r.Context(), a.DB, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !ok {
		common.WriteJSON(w, http.StatusOK, nil, nil)
		return
	}
	common.WriteJSON(w, http.StatusOK, acc, nil)
}

// PutWalletAccounts is PUT /admin/wallet-accounts. Codes are the school's own
// COA codes; nothing is defaulted.
func (a *App) PutWalletAccounts(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleFinanceManager)
	if schoolID == "" {
		return
	}
	var in wallet.Accounts
	if !common.Decode(w, r, &in) {
		return
	}
	for _, v := range []*string{&in.AllowanceLiability, &in.SavingsLiability, &in.Cash, &in.TenantPayable, &in.CanteenRevenue, &in.RentRevenue, &in.TenantReceivable} {
		*v = strings.TrimSpace(*v)
	}
	if in.AllowanceLiability == "" || in.SavingsLiability == "" || in.Cash == "" || in.TenantPayable == "" || in.CanteenRevenue == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "akun titipan uang saku, titipan tabungan, kas, utang tenant dan pendapatan kantin wajib diisi")
		return
	}
	before, _, _ := loadWalletAccounts(r.Context(), a.DB, schoolID)
	if _, err := a.DB.ExecContext(r.Context(),
		`INSERT INTO wallet_accounts(school_id,allowance_liability_acc,savings_liability_acc,cash_acc,tenant_payable_acc,canteen_revenue_acc,rent_revenue_acc,tenant_receivable_acc)
		 VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''))
		 ON CONFLICT (school_id) DO UPDATE SET allowance_liability_acc=EXCLUDED.allowance_liability_acc,savings_liability_acc=EXCLUDED.savings_liability_acc,
		 cash_acc=EXCLUDED.cash_acc,tenant_payable_acc=EXCLUDED.tenant_payable_acc,canteen_revenue_acc=EXCLUDED.canteen_revenue_acc,
		 rent_revenue_acc=EXCLUDED.rent_revenue_acc,tenant_receivable_acc=EXCLUDED.tenant_receivable_acc,updated_at=now()`,
		schoolID, in.AllowanceLiability, in.SavingsLiability, in.Cash, in.TenantPayable, in.CanteenRevenue, in.RentRevenue, in.TenantReceivable); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "wallet_accounts", schoolID, "wallet_accounts_updated", c.ActiveRole, c.UserID, "", "", before, in)
	common.WriteJSON(w, http.StatusOK, in, nil)
}

// ListWalletReconciliations is GET /admin/wallet-reconciliations (NFR-OPS-04).
func (a *App) ListWalletReconciliations(w http.ResponseWriter, r *http.Request) {
	_, schoolID := common.RequireRoles(w, r, common.RoleAdmin, common.RoleFinanceManager)
	if schoolID == "" {
		return
	}
	page, perPage := common.Page(r)
	var total int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM wallet_reconciliations WHERE school_id=$1`, schoolID).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text,run_on,wallet_kind,wallet_total_idr,ledger_idr,diff_idr,status,created_at FROM wallet_reconciliations
		 WHERE school_id=$1 ORDER BY run_on DESC,wallet_kind LIMIT $2 OFFSET $3`, schoolID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, kind, status string
		var runOn, created time.Time
		var total, ledger, diff int64
		if err := rows.Scan(&id, &runOn, &kind, &total, &ledger, &diff, &status, &created); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "run_on": runOn.Format("2006-01-02"), "wallet_kind": kind,
			"wallet_total_idr": total, "ledger_idr": ledger, "diff_idr": diff, "status": status, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, map[string]any{"page": page, "per_page": perPage, "total": total, "generated_at": a.now()})
}

// errHTTP converts a pure-rule error into a 422 httpError.
func errHTTP(err error) error {
	var he *httpError
	if errors.As(err, &he) {
		return err
	}
	return &httpError{http.StatusUnprocessableEntity, err.Error()}
}
