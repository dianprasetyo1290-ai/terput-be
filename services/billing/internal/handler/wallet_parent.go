package handler

import (
	"net/http"

	"skci/billing/internal/common"
	"skci/billing/internal/wallet"
)

// Parent and student wallet views (FR-125-001/003).

// requireWalletChild is requireChild + the wallet feature.
func (a *App) requireWalletChild(w http.ResponseWriter, r *http.Request) (*common.Claims, string, bool) {
	c, studentID, ok := a.requireChild(w, r)
	if !ok || !a.requireFeature(w, r, c.SchoolID, featureWallet) {
		return nil, "", false
	}
	return c, studentID, true
}

// ParentChildWallets is GET /parent/children/{studentUserId}/wallets.
func (a *App) ParentChildWallets(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireWalletChild(w, r)
	if !ok {
		return
	}
	a.writeWallets(w, r, c.SchoolID, studentID)
}

// MyWallets is GET /me/wallets (siswa).
func (a *App) MyWallets(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil || !a.requireFeature(w, r, c.SchoolID, featureWallet) {
		return
	}
	a.writeWallets(w, r, c.SchoolID, c.UserID)
}

func (a *App) writeWallets(w http.ResponseWriter, r *http.Request, schoolID, studentID string) {
	ws, err := a.studentWallets(r.Context(), schoolID, studentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	settings, err := loadWalletSettings(r.Context(), a.DB, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	var spent int64
	for _, wl := range ws {
		if wl.Kind == wallet.KindAllowance && wl.ID != "" {
			tx, err := a.DB.BeginTx(r.Context(), nil)
			if err != nil {
				writeErr(w, err)
				return
			}
			spent, err = spentTodayTx(r.Context(), tx, wl.ID, a.now(), a.loc())
			_ = tx.Rollback()
			if err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"student_user_id": studentID, "wallets": ws, "spent_today_idr": spent,
		"max_allowance_balance_idr": settings.MaxAllowanceBalanceIDR, "max_savings_balance_idr": settings.MaxSavingsBalanceIDR,
	}, map[string]any{"generated_at": a.now()})
}

// ParentSetDailyLimit is PUT /parent/children/{studentUserId}/wallets/allowance/limit
// {daily_limit_idr|null} (FR-125-003). null removes the limit.
func (a *App) ParentSetDailyLimit(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireWalletChild(w, r)
	if !ok {
		return
	}
	var in struct {
		DailyLimitIDR *int64 `json:"daily_limit_idr"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if in.DailyLimitIDR != nil && *in.DailyLimitIDR <= 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "batas harian harus lebih dari 0 (kosongkan untuk tanpa batas)")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	wl, err := ensureWallet(r.Context(), tx, c.SchoolID, studentID, wallet.KindAllowance)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE wallets SET daily_limit_idr=$1,updated_at=now() WHERE id=$2`, in.DailyLimitIDR, wl.ID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), c.SchoolID, "wallet", wl.ID, "daily_limit_set", common.RoleParent, c.UserID, "", "",
		map[string]any{"daily_limit_idr": wl.DailyLimitIDR}, map[string]any{"daily_limit_idr": in.DailyLimitIDR})
	common.WriteJSON(w, http.StatusOK, map[string]any{"wallet_id": wl.ID, "daily_limit_idr": in.DailyLimitIDR}, nil)
}

// ParentWalletMovements is GET /parent/children/{studentUserId}/wallets/{kind}/movements.
func (a *App) ParentWalletMovements(w http.ResponseWriter, r *http.Request) {
	c, studentID, ok := a.requireWalletChild(w, r)
	if !ok {
		return
	}
	a.walletMovements(w, r, c.SchoolID, studentID, param(r, "kind"))
}

// MyWalletMovements is GET /me/wallets/{kind}/movements (siswa).
func (a *App) MyWalletMovements(w http.ResponseWriter, r *http.Request) {
	c := common.RequireStudent(w, r)
	if c == nil || !a.requireFeature(w, r, c.SchoolID, featureWallet) {
		return
	}
	a.walletMovements(w, r, c.SchoolID, c.UserID, param(r, "kind"))
}
