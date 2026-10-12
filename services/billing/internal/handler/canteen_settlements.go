package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/canteen"
	"skci/billing/internal/client"
	"skci/billing/internal/common"
)

// Tenant settlements (FR-126-002/003, AC-126-01): draft with calculation ->
// core approval 'canteen_settlement' -> paid by finance. Sales in the period
// are locked to the settlement (no void) until it is rejected.

const settlementCols = `s.id::text,s.tenant_id::text,t.name,s.period_from,s.period_to,s.scheme_id::text,s.scheme_kind,s.wallet_sales_idr,
	s.cash_sales_idr,s.school_share_idr,s.tenant_payout_idr,s.status,COALESCE(s.approval_id,''),COALESCE(s.decision_comment,''),
	s.paid_at,COALESCE(s.paid_ref,''),s.created_by::text,s.created_at`

func scanSettlement(row interface{ Scan(...any) error }) (map[string]any, error) {
	var id, tenantID, tenantName, schemeID, kind, status, approval, comment, paidRef, by string
	var from, to, created time.Time
	var wallet, cash, share, payout int64
	var paidAt sql.NullTime
	if err := row.Scan(&id, &tenantID, &tenantName, &from, &to, &schemeID, &kind, &wallet, &cash, &share, &payout, &status,
		&approval, &comment, &paidAt, &paidRef, &by, &created); err != nil {
		return nil, err
	}
	m := map[string]any{"id": id, "tenant_id": tenantID, "tenant_name": tenantName, "period_from": from.Format("2006-01-02"),
		"period_to": to.Format("2006-01-02"), "scheme_id": schemeID, "scheme_kind": kind, "wallet_sales_idr": wallet,
		"cash_sales_idr": cash, "school_share_idr": share, "tenant_payout_idr": payout, "status": status, "approval_id": approval,
		"decision_comment": comment, "paid_at": nil, "paid_ref": paidRef, "created_by": by, "created_at": created}
	if paidAt.Valid {
		m["paid_at"] = paidAt.Time
	}
	return m, nil
}

// CreateSettlement is POST /canteen/settlements {tenant_id, period_from, period_to}.
func (a *App) CreateSettlement(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	var in struct {
		TenantID   string `json:"tenant_id"`
		PeriodFrom string `json:"period_from"`
		PeriodTo   string `json:"period_to"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	from, err1 := time.Parse("2006-01-02", strings.TrimSpace(in.PeriodFrom))
	to, err2 := time.Parse("2006-01-02", strings.TrimSpace(in.PeriodTo))
	if err1 != nil || err2 != nil || to.Before(from) {
		common.WriteError(w, http.StatusUnprocessableEntity, "periode wajib berformat YYYY-MM-DD dan tanggal akhir tidak sebelum awal")
		return
	}
	if to.Format("2006-01-02") >= a.todayDate() {
		common.WriteError(w, http.StatusUnprocessableEntity, "periode penyelesaian harus sudah berakhir")
		return
	}
	if err := a.requireTenant(r.Context(), schoolID, in.TenantID); err != nil {
		writeErr(w, err)
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(r.Context(), `SELECT 1 FROM canteen_tenants WHERE id::text=$1 FOR UPDATE`, in.TenantID); err != nil {
		writeErr(w, err)
		return
	}
	var overlap bool
	_ = tx.QueryRowContext(r.Context(),
		`SELECT true FROM canteen_settlements WHERE tenant_id::text=$1 AND status<>'rejected' AND period_from <= $3::date AND period_to >= $2::date LIMIT 1`,
		in.TenantID, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&overlap)
	if overlap {
		common.WriteError(w, http.StatusConflict, "periode bertumpuk dengan penyelesaian lain")
		return
	}
	// The scheme must cover the whole period (otherwise: split the period).
	rows, err := tx.QueryContext(r.Context(),
		`SELECT id::text,kind,COALESCE(share_bp,0),COALESCE(share_base,''),COALESCE(rent_idr,0),effective_from,effective_to
		 FROM canteen_tenant_schemes WHERE tenant_id::text=$1 AND effective_from <= $3::date AND (effective_to IS NULL OR effective_to >= $2::date)
		 ORDER BY effective_from`, in.TenantID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		writeErr(w, err)
		return
	}
	var schemes []canteen.Scheme
	for rows.Next() {
		var s canteen.Scheme
		var effTo sql.NullTime
		if err := rows.Scan(&s.ID, &s.Kind, &s.ShareBP, &s.ShareBase, &s.RentIDR, &s.EffectiveFrom, &effTo); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		if effTo.Valid {
			s.EffectiveTo = &effTo.Time
		}
		schemes = append(schemes, s)
	}
	rows.Close()
	if len(schemes) == 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "tenant belum memiliki skema yang berlaku pada periode ini")
		return
	}
	if len(schemes) > 1 {
		common.WriteError(w, http.StatusUnprocessableEntity, canteen.ErrSplitPeriod.Error())
		return
	}
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, a.loc())
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, a.loc()).AddDate(0, 0, 1)
	var walletSales, cashSales int64
	if err = tx.QueryRowContext(r.Context(),
		`SELECT COALESCE(sum(total_idr) FILTER (WHERE pay_method='wallet'),0),COALESCE(sum(total_idr) FILTER (WHERE pay_method='cash'),0)
		 FROM canteen_sales WHERE tenant_id::text=$1 AND status='completed' AND settlement_id IS NULL AND recorded_at >= $2 AND recorded_at < $3`,
		in.TenantID, start, end).Scan(&walletSales, &cashSales); err != nil {
		writeErr(w, err)
		return
	}
	calc, err := canteen.Settle(schemes[0], walletSales, cashSales, from, to)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var id string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO canteen_settlements(school_id,tenant_id,period_from,period_to,scheme_id,scheme_kind,wallet_sales_idr,cash_sales_idr,
		   school_share_idr,tenant_payout_idr,created_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`,
		schoolID, in.TenantID, from.Format("2006-01-02"), to.Format("2006-01-02"), calc.SchemeID, calc.SchemeKind,
		calc.WalletSalesIDR, calc.CashSalesIDR, calc.SchoolShareIDR, calc.TenantPayoutIDR, c.UserID).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "penyelesaian untuk periode ini sudah ada")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE canteen_sales SET settlement_id=$1 WHERE tenant_id::text=$2 AND status='completed' AND settlement_id IS NULL AND recorded_at >= $3 AND recorded_at < $4`,
		id, in.TenantID, start, end); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_settlement", id, "settlement_created", c.ActiveRole, c.UserID, "", "", nil, calc)
	a.writeSettlement(w, r, schoolID, id, http.StatusCreated)
}

func (a *App) writeSettlement(w http.ResponseWriter, r *http.Request, schoolID, id string, status int) {
	m, err := scanSettlement(a.DB.QueryRowContext(r.Context(),
		`SELECT `+settlementCols+` FROM canteen_settlements s JOIN canteen_tenants t ON t.id=s.tenant_id WHERE s.id::text=$1 AND s.school_id=$2`, id, schoolID))
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "penyelesaian tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, status, m, nil)
}

// settlementReaders are the roles that may read settlements.
var settlementReaders = []string{common.RoleCanteenAdmin, common.RoleFinanceManager, common.RoleAdmin, common.RoleCanteenOperator}

// ListSettlements is GET /canteen/settlements?tenant_id=&status= (operators: own tenants).
func (a *App) ListSettlements(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, settlementReaders...)
	if schoolID == "" {
		return
	}
	scope, err := a.tenantScope(r.Context(), c, schoolID, strings.TrimSpace(r.URL.Query().Get("tenant_id")))
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT `+settlementCols+` FROM canteen_settlements s JOIN canteen_tenants t ON t.id=s.tenant_id
		 WHERE s.school_id=$1 AND ($2::text[] IS NULL OR s.tenant_id::text = ANY($2::text[])) AND ($3='' OR s.status=$3)
		 ORDER BY s.period_from DESC,t.code LIMIT 500`, schoolID, scope, strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		m, err := scanSettlement(rows)
		if err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, map[string]any{"generated_at": a.now()})
}

// GetSettlement is GET /canteen/settlements/{id}.
func (a *App) GetSettlement(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, settlementReaders...)
	if schoolID == "" {
		return
	}
	var tenantID string
	if err := a.DB.QueryRowContext(r.Context(), `SELECT tenant_id::text FROM canteen_settlements WHERE id::text=$1 AND school_id=$2`,
		param(r, "id"), schoolID).Scan(&tenantID); err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "penyelesaian tidak ditemukan")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := a.tenantScope(r.Context(), c, schoolID, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	a.writeSettlement(w, r, schoolID, param(r, "id"), http.StatusOK)
}

// SubmitSettlement is POST /canteen/settlements/{id}/submit (-> core approval).
func (a *App) SubmitSettlement(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	if !a.Core.Configured() {
		common.WriteError(w, http.StatusServiceUnavailable, "Approval Center belum tersedia")
		return
	}
	id := param(r, "id")
	var status, tenantName string
	var share, payout int64
	var from, to time.Time
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT s.status,t.name,s.school_share_idr,s.tenant_payout_idr,s.period_from,s.period_to FROM canteen_settlements s
		 JOIN canteen_tenants t ON t.id=s.tenant_id WHERE s.id::text=$1 AND s.school_id=$2`, id, schoolID).
		Scan(&status, &tenantName, &share, &payout, &from, &to)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "penyelesaian tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != "draft" {
		common.WriteError(w, http.StatusConflict, "hanya draft yang dapat diajukan")
		return
	}
	resp, err := a.Core.RequestApproval(r.Context(), client.ApprovalRequest{
		SchoolID: schoolID, RequestType: reqTypeCanteenSettlement, SourceID: id,
		Title:   "Penyelesaian bagi hasil kantin " + tenantName,
		Summary: map[string]any{"tenant": tenantName, "period_from": from.Format("2006-01-02"), "period_to": to.Format("2006-01-02"), "school_share_idr": share, "tenant_payout_idr": payout},
		AmountIDR: &payout, MakerUserID: c.UserID,
		CallbackURL:   a.Config.Gateway.BillingInternalURL + "/internal/canteen/settlements/" + id + "/callback",
		CorrelationID: id, IdempotencyKey: "canteen-settlement:" + id,
	})
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "gagal mengajukan approval: "+err.Error())
		return
	}
	if _, err = a.DB.ExecContext(r.Context(),
		`UPDATE canteen_settlements SET status='pending_approval',approval_id=$1,updated_at=now() WHERE id::text=$2 AND status='draft'`, resp.ID, id); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_settlement", id, "settlement_submitted", c.ActiveRole, c.UserID, "", "",
		map[string]any{"status": "draft"}, map[string]any{"status": "pending_approval", "approval_id": resp.ID})
	a.writeSettlement(w, r, schoolID, id, http.StatusOK)
}

// SettlementCallback is POST /internal/canteen/settlements/{id}/callback
// (core). Idempotent: anything but pending_approval is acknowledged as-is.
func (a *App) SettlementCallback(w http.ResponseWriter, r *http.Request) {
	var cb client.ApprovalCallback
	if !common.Decode(w, r, &cb) {
		return
	}
	id := param(r, "id")
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var schoolID, status string
	err = tx.QueryRowContext(r.Context(), `SELECT school_id::text,status FROM canteen_settlements WHERE id::text=$1 FOR UPDATE`, id).Scan(&schoolID, &status)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "penyelesaian tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if status != "pending_approval" {
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": status}, nil)
		return
	}
	next := "rejected"
	if cb.Decision == "approved" {
		next = "approved"
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE canteen_settlements SET status=$1,decided_by=NULLIF($2,''),decision_comment=NULLIF($3,''),updated_at=now() WHERE id::text=$4`,
		next, cb.DecidedBy, cb.Comment, id); err != nil {
		writeErr(w, err)
		return
	}
	if next == "rejected" {
		if _, err = tx.ExecContext(r.Context(), `UPDATE canteen_sales SET settlement_id=NULL WHERE settlement_id::text=$1`, id); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_settlement", id, "settlement_"+next, "core", cb.DecidedBy, "", cb.Comment,
		map[string]any{"status": status}, map[string]any{"status": next})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "status": next}, nil)
}

// PaySettlement is POST /canteen/settlements/{id}/pay {paid_ref} (finance_manager).
func (a *App) PaySettlement(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleFinanceManager)
	if schoolID == "" {
		return
	}
	var in struct {
		PaidRef string `json:"paid_ref"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.PaidRef = strings.TrimSpace(in.PaidRef)
	if in.PaidRef == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "referensi pembayaran wajib diisi")
		return
	}
	id := param(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE canteen_settlements SET status='paid',paid_at=now(),paid_ref=$1,paid_by=$2,updated_at=now()
		 WHERE id::text=$3 AND school_id=$4 AND status='approved'`, in.PaidRef, c.UserID, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusConflict, "hanya penyelesaian yang sudah disetujui yang dapat dibayar")
		return
	}
	a.audit(r.Context(), schoolID, "canteen_settlement", id, "settlement_paid", c.ActiveRole, c.UserID, "", "",
		map[string]any{"status": "approved"}, map[string]any{"status": "paid", "paid_ref": in.PaidRef})
	a.writeSettlement(w, r, schoolID, id, http.StatusOK)
}
