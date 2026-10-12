package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/canteen"
	"skci/billing/internal/common"
	"skci/billing/internal/wallet"
)

// maxSyncBatch bounds one offline sync request (T12 §4.3).
const maxSyncBatch = 100

// todayDate is the current business date as YYYY-MM-DD (for date columns).
func (a *App) todayDate() string { return a.today().Format("2006-01-02") }

// posOperator resolves a POS request (canteen_operator, canteen feature on).
func (a *App) posOperator(w http.ResponseWriter, r *http.Request) (*common.Claims, string) {
	return a.canteenStaff(w, r, common.RoleCanteenOperator)
}

// PosSession is GET /pos/session?tenant_id= : the operator's tenants, the
// selected tenant's active menu and the offline window.
func (a *App) PosSession(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.posOperator(w, r)
	if schoolID == "" {
		return
	}
	own, err := a.tenantScope(r.Context(), c, schoolID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	selected := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if selected == "" {
		selected = own[0]
	}
	if _, err := a.tenantScope(r.Context(), c, schoolID, selected); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text,code,name,is_open,status FROM canteen_tenants WHERE id::text = ANY($1::text[]) AND deleted_at IS NULL ORDER BY code`, own)
	if err != nil {
		writeErr(w, err)
		return
	}
	tenants := []map[string]any{}
	for rows.Next() {
		var id, code, name, status string
		var open bool
		if err := rows.Scan(&id, &code, &name, &open, &status); err != nil {
			rows.Close()
			writeErr(w, err)
			return
		}
		tenants = append(tenants, map[string]any{"id": id, "code": code, "name": name, "is_open": open, "status": status})
	}
	rows.Close()
	menu, err := a.menuItems(r.Context(), schoolID, selected, true)
	if err != nil {
		writeErr(w, err)
		return
	}
	settings, err := loadWalletSettings(r.Context(), a.DB, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"tenants": tenants, "tenant_id": selected, "menu": menu,
		"pos_offline_max_minutes": settings.PosOfflineMaxMinutes,
		"wallet_enabled":          a.featureOn(r.Context(), schoolID, featureWallet),
		"server_time":             a.now(),
	}, nil)
}

// PosOpenState is PUT /pos/open-state {tenant_id, is_open} (read by T25).
func (a *App) PosOpenState(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.posOperator(w, r)
	if schoolID == "" {
		return
	}
	var in struct {
		TenantID string `json:"tenant_id"`
		IsOpen   bool   `json:"is_open"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	scope, err := a.tenantScope(r.Context(), c, schoolID, strings.TrimSpace(in.TenantID))
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(scope) != 1 {
		common.WriteError(w, http.StatusUnprocessableEntity, "tenant_id wajib diisi")
		return
	}
	if _, err := a.DB.ExecContext(r.Context(),
		`UPDATE canteen_tenants SET is_open=$1,open_changed_at=now(),updated_at=now() WHERE id::text=$2 AND school_id=$3`,
		in.IsOpen, scope[0], schoolID); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_tenant", scope[0], "open_state_changed", c.ActiveRole, c.UserID, "", "", nil, map[string]any{"is_open": in.IsOpen})
	common.WriteJSON(w, http.StatusOK, map[string]any{"tenant_id": scope[0], "is_open": in.IsOpen}, nil)
}

// saleItemInput is one cart line.
type saleItemInput struct {
	ItemID string `json:"item_id"`
	Qty    int    `json:"qty"`
}

// saleInput is a POS sale (online or from the offline queue).
type saleInput struct {
	TenantID    string          `json:"tenant_id"`
	ClientTxnID string          `json:"client_txn_id"`
	PayMethod   string          `json:"pay_method"`
	CardToken   string          `json:"card_token"`
	Items       []saleItemInput `json:"items"`
	DeviceID    string          `json:"device_id"`
	RecordedAt  string          `json:"recorded_at"`
}

// saleLine is a priced cart line.
type saleLine struct {
	ItemID, Name string
	Qty          int
	UnitIDR      int64
}

// priceLines prices the cart from the tenant's active menu (server prices only).
func priceLines(ctx context.Context, tx *sql.Tx, schoolID, tenantID string, items []saleItemInput) ([]saleLine, int64, error) {
	if len(items) == 0 {
		return nil, 0, &httpError{http.StatusUnprocessableEntity, "keranjang kosong"}
	}
	var lines []saleLine
	var total int64
	for _, it := range items {
		if it.Qty <= 0 {
			return nil, 0, &httpError{http.StatusUnprocessableEntity, "jumlah item harus lebih dari 0"}
		}
		var l saleLine
		err := tx.QueryRowContext(ctx,
			`SELECT id::text,name,price_idr FROM canteen_menu_items WHERE id::text=$1 AND tenant_id::text=$2 AND school_id=$3 AND active AND deleted_at IS NULL`,
			it.ItemID, tenantID, schoolID).Scan(&l.ItemID, &l.Name, &l.UnitIDR)
		if err == sql.ErrNoRows {
			return nil, 0, &httpError{http.StatusUnprocessableEntity, "item menu tidak tersedia"}
		}
		if err != nil {
			return nil, 0, err
		}
		l.Qty = it.Qty
		total += l.UnitIDR * int64(l.Qty)
		lines = append(lines, l)
	}
	return lines, total, nil
}

// existingSale returns the sale recorded under a client transaction id.
func (a *App) existingSale(ctx context.Context, q queryer, schoolID, tenantID, clientTxnID string) (string, bool, error) {
	var id string
	err := q.QueryRowContext(ctx,
		`SELECT id::text FROM canteen_sales WHERE school_id=$1 AND tenant_id::text=$2 AND client_txn_id::text=$3`,
		schoolID, tenantID, clientTxnID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return id, err == nil, err
}

// recordSale stores one sale in its own transaction. offline sales are cash
// only and keep their device time; online wallet sales run BR-WAL-01 under a
// row lock on the wallet (NFR-OPS-01). A repeated client_txn_id returns the
// stored sale with dup=true (FR-124-005).
func (a *App) recordSale(ctx context.Context, c *common.Claims, schoolID string, in saleInput, offline bool, maxOfflineMinutes int) (map[string]any, bool, error) {
	in.TenantID, in.ClientTxnID = strings.TrimSpace(in.TenantID), strings.TrimSpace(in.ClientTxnID)
	if in.ClientTxnID == "" {
		return nil, false, &httpError{http.StatusUnprocessableEntity, "client_txn_id wajib diisi"}
	}
	if in.PayMethod != canteen.PayCash && in.PayMethod != canteen.PayWallet {
		return nil, false, &httpError{http.StatusUnprocessableEntity, "metode bayar harus wallet atau cash"}
	}
	if offline && in.PayMethod == canteen.PayWallet {
		return nil, false, &httpError{http.StatusUnprocessableEntity, "penjualan dompet tidak boleh luring"}
	}
	if _, err := a.tenantScope(ctx, c, schoolID, in.TenantID); err != nil {
		return nil, false, err
	}
	if id, ok, err := a.existingSale(ctx, a.DB, schoolID, in.TenantID, in.ClientTxnID); err != nil || ok {
		if err != nil {
			return nil, false, err
		}
		sale, err := a.saleView(ctx, schoolID, id)
		return sale, true, err
	}
	now := a.now()
	recordedAt := now
	flags := canteen.OfflineResult{}
	if offline {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(in.RecordedAt))
		if err != nil {
			return nil, false, &httpError{http.StatusUnprocessableEntity, "recorded_at wajib RFC3339"}
		}
		if t.After(now.Add(time.Minute)) {
			return nil, false, &httpError{http.StatusUnprocessableEntity, "recorded_at berada di masa depan"}
		}
		recordedAt = t
		flags = canteen.OfflineAccept(t, now, maxOfflineMinutes)
	}
	if in.PayMethod == canteen.PayWallet && !a.featureOn(ctx, schoolID, featureWallet) {
		return nil, false, &httpError{http.StatusForbidden, "Modul Dompet Siswa belum diaktifkan, gunakan tunai"}
	}

	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var tenantCode, tenantStatus string
	if err = tx.QueryRowContext(ctx,
		`SELECT code,status FROM canteen_tenants WHERE id::text=$1 AND school_id=$2 AND deleted_at IS NULL`, in.TenantID, schoolID).
		Scan(&tenantCode, &tenantStatus); err == sql.ErrNoRows {
		return nil, false, &httpError{http.StatusNotFound, "tenant tidak ditemukan"}
	} else if err != nil {
		return nil, false, err
	}
	if tenantStatus != "active" {
		return nil, false, &httpError{http.StatusUnprocessableEntity, "tenant tidak aktif"}
	}
	lines, total, err := priceLines(ctx, tx, schoolID, in.TenantID, in.Items)
	if err != nil {
		return nil, false, err
	}
	stockDate := recordedAt.In(a.loc()).Format("2006-01-02")
	for _, l := range lines {
		res, err := tx.ExecContext(ctx,
			`UPDATE canteen_daily_stock SET qty_sold=qty_sold+$1 WHERE item_id::text=$2 AND stock_date=$3::date AND ($4 OR qty_start-qty_sold >= $1)`,
			l.Qty, l.ItemID, stockDate, offline)
		if err != nil {
			return nil, false, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			var limited bool
			_ = tx.QueryRowContext(ctx, `SELECT true FROM canteen_daily_stock WHERE item_id::text=$1 AND stock_date=$2::date`, l.ItemID, stockDate).Scan(&limited)
			if limited {
				return nil, false, &httpError{http.StatusUnprocessableEntity, "Stok " + l.Name + " tidak mencukupi"}
			}
		}
	}

	var studentID, movementID string
	var balanceAfter int64
	if in.PayMethod == canteen.PayWallet {
		if strings.TrimSpace(in.CardToken) == "" {
			return nil, false, &httpError{http.StatusUnprocessableEntity, "pindai kartu siswa terlebih dahulu"}
		}
		if studentID, _, err = resolveCard(ctx, tx, schoolID, in.CardToken); err != nil {
			return nil, false, err
		}
		wl, err := ensureWallet(ctx, tx, schoolID, studentID, wallet.KindAllowance)
		if err != nil {
			return nil, false, err
		}
		if wl.Status != "active" {
			return nil, false, &httpError{http.StatusUnprocessableEntity, "dompet siswa sedang ditutup"}
		}
		spent, err := spentTodayTx(ctx, tx, wl.ID, now, a.loc())
		if err != nil {
			return nil, false, err
		}
		if err := wallet.CanSpend(wl.BalanceIDR, wl.DailyLimitIDR, spent, total); err != nil {
			return nil, false, errHTTP(err)
		}
		if movementID, _, err = applyMovement(ctx, tx, schoolID, &wl, movement{Kind: wallet.MovePosSale, AmountIDR: -total,
			RefType: "canteen_sale", Actor: c.UserID, Key: "pos-sale:" + in.TenantID + ":" + in.ClientTxnID}); err != nil {
			return nil, false, err
		}
		balanceAfter = wl.BalanceIDR
	}

	saleDate := recordedAt.In(a.loc())
	var seq int
	if err = tx.QueryRowContext(ctx,
		`INSERT INTO canteen_sale_sequences(tenant_id,sale_date,last) VALUES($1,$2::date,1)
		 ON CONFLICT (tenant_id,sale_date) DO UPDATE SET last=canteen_sale_sequences.last+1 RETURNING last`,
		in.TenantID, saleDate.Format("2006-01-02")).Scan(&seq); err != nil {
		return nil, false, err
	}
	saleNo := fmt.Sprintf("%s-%s-%04d", tenantCode, saleDate.Format("20060102"), seq)
	var saleID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO canteen_sales(school_id,tenant_id,sale_no,pay_method,total_idr,student_user_id,wallet_movement_id,operator_user_id,
		   device_id,client_txn_id,recorded_at,offline,late_sync)
		 VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,$8,$9,$10::uuid,$11,$12,$13) RETURNING id::text`,
		schoolID, in.TenantID, saleNo, in.PayMethod, total, studentID, movementID, c.UserID, strings.TrimSpace(in.DeviceID),
		in.ClientTxnID, recordedAt, flags.Offline, flags.LateSync).Scan(&saleID)
	if isUniqueViolation(err) {
		// Lost a race with the same client_txn_id: return the stored sale.
		_ = tx.Rollback()
		id, _, lookupErr := a.existingSale(ctx, a.DB, schoolID, in.TenantID, in.ClientTxnID)
		if lookupErr != nil || id == "" {
			return nil, false, err
		}
		sale, err := a.saleView(ctx, schoolID, id)
		return sale, true, err
	}
	if err != nil {
		return nil, false, err
	}
	for _, l := range lines {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO canteen_sale_items(sale_id,item_id,name_snapshot,qty,unit_price_idr,line_total_idr) VALUES($1,$2,$3,$4,$5,$6)`,
			saleID, l.ItemID, l.Name, l.Qty, l.UnitIDR, l.UnitIDR*int64(l.Qty)); err != nil {
			return nil, false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	sale, err := a.saleView(ctx, schoolID, saleID)
	if err == nil && in.PayMethod == canteen.PayWallet {
		sale["balance_after_idr"] = balanceAfter
	}
	return sale, false, err
}

// spentTodayTx is today's POS spending of a wallet (BR-WAL-01).
func spentTodayTx(ctx context.Context, tx *sql.Tx, walletID string, now time.Time, loc *time.Location) (int64, error) {
	y, m, d := now.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	rows, err := tx.QueryContext(ctx,
		`SELECT kind,amount_idr,created_at FROM wallet_movements WHERE wallet_id=$1 AND kind IN ('pos_sale','pos_void') AND created_at >= $2`,
		walletID, start)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var mv []wallet.Movement
	for rows.Next() {
		var m wallet.Movement
		if err := rows.Scan(&m.Kind, &m.AmountIDR, &m.CreatedAt); err != nil {
			return 0, err
		}
		mv = append(mv, m)
	}
	return wallet.SpentToday(mv, now, loc), rows.Err()
}

// saleView loads one sale with its items (struk / receipt data).
func (a *App) saleView(ctx context.Context, schoolID, saleID string) (map[string]any, error) {
	var id, tenantID, tenantName, saleNo, method, status, operator, clientTxn, voidReason string
	var student sql.NullString
	var total int64
	var recordedAt, syncedAt time.Time
	var offline, late bool
	var voidedAt sql.NullTime
	err := a.DB.QueryRowContext(ctx,
		`SELECT s.id::text,s.tenant_id::text,t.name,s.sale_no,s.pay_method,s.status,s.operator_user_id::text,s.client_txn_id::text,
		        COALESCE(s.void_reason,''),s.student_user_id::text,s.total_idr,s.recorded_at,s.synced_at,s.offline,s.late_sync,s.voided_at
		 FROM canteen_sales s JOIN canteen_tenants t ON t.id=s.tenant_id WHERE s.id::text=$1 AND s.school_id=$2`, saleID, schoolID).
		Scan(&id, &tenantID, &tenantName, &saleNo, &method, &status, &operator, &clientTxn, &voidReason, &student, &total,
			&recordedAt, &syncedAt, &offline, &late, &voidedAt)
	if err == sql.ErrNoRows {
		return nil, &httpError{http.StatusNotFound, "transaksi tidak ditemukan"}
	}
	if err != nil {
		return nil, err
	}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT item_id::text,name_snapshot,qty,unit_price_idr,line_total_idr FROM canteen_sale_items WHERE sale_id=$1 ORDER BY name_snapshot`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var itemID, name string
		var qty int
		var unit, line int64
		if err := rows.Scan(&itemID, &name, &qty, &unit, &line); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"item_id": itemID, "name": name, "qty": qty, "unit_price_idr": unit, "line_total_idr": line})
	}
	m := map[string]any{"id": id, "tenant_id": tenantID, "tenant_name": tenantName, "sale_no": saleNo, "pay_method": method,
		"status": status, "operator_user_id": operator, "client_txn_id": clientTxn, "void_reason": voidReason,
		"student_user_id": student.String, "total_idr": total, "recorded_at": recordedAt, "synced_at": syncedAt,
		"offline": offline, "late_sync": late, "voided_at": nil, "items": items}
	if voidedAt.Valid {
		m["voided_at"] = voidedAt.Time
	}
	return m, rows.Err()
}

// PosCreateSale is POST /pos/sales (FR-124-002/003, AC-124-01, NFR-OPS-01).
func (a *App) PosCreateSale(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.posOperator(w, r)
	if schoolID == "" {
		return
	}
	var in saleInput
	if !common.Decode(w, r, &in) {
		return
	}
	sale, dup, err := a.recordSale(r.Context(), c, schoolID, in, false, 0)
	if err != nil {
		writeErr(w, err)
		return
	}
	status := http.StatusCreated
	if dup {
		status = http.StatusOK
	}
	common.WriteJSON(w, status, sale, map[string]any{"duplicate": dup})
}

// PosSyncSales is POST /pos/sales/sync {device_id, sales[]} (FR-124-005,
// NFR-OPS-02): offline cash sales, idempotent per client_txn_id.
func (a *App) PosSyncSales(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.posOperator(w, r)
	if schoolID == "" {
		return
	}
	var in struct {
		DeviceID string      `json:"device_id"`
		Sales    []saleInput `json:"sales"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if len(in.Sales) == 0 || len(in.Sales) > maxSyncBatch {
		common.WriteError(w, http.StatusUnprocessableEntity, fmt.Sprintf("batch sinkron harus berisi 1-%d transaksi", maxSyncBatch))
		return
	}
	settings, err := loadWalletSettings(r.Context(), a.DB, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	ids := make([]string, 0, len(in.Sales))
	for _, s := range in.Sales {
		ids = append(ids, strings.TrimSpace(s.ClientTxnID))
	}
	existing := map[string]bool{}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT client_txn_id::text FROM canteen_sales WHERE school_id=$1 AND client_txn_id::text = ANY($2::text[])`, schoolID, ids)
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
		existing[id] = true
	}
	rows.Close()
	fresh, duplicate := canteen.MergeSync(existing, ids)
	isFresh := map[string]bool{}
	for _, id := range fresh {
		isFresh[id] = true
	}
	accepted := []map[string]any{}
	rejected := []map[string]any{}
	for _, s := range in.Sales {
		id := strings.TrimSpace(s.ClientTxnID)
		if !isFresh[id] {
			continue
		}
		delete(isFresh, id) // a repeated id inside the batch is processed once
		if s.DeviceID == "" {
			s.DeviceID = in.DeviceID
		}
		sale, dup, err := a.recordSale(r.Context(), c, schoolID, s, true, settings.PosOfflineMaxMinutes)
		switch {
		case err != nil:
			msg := "internal error"
			if he, ok := err.(*httpError); ok {
				msg = he.msg
			} else {
				common.CaptureError(err)
			}
			rejected = append(rejected, map[string]any{"client_txn_id": id, "error": msg})
		case dup:
			duplicate = append(duplicate, id)
		default:
			accepted = append(accepted, map[string]any{"client_txn_id": id, "sale_id": sale["id"], "sale_no": sale["sale_no"], "late_sync": sale["late_sync"]})
		}
	}
	if duplicate == nil {
		duplicate = []string{}
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"accepted": accepted, "duplicate": duplicate, "rejected": rejected}, nil)
}

// saleAccess checks the caller may act on a sale (operator: own tenant).
func (a *App) saleAccess(w http.ResponseWriter, r *http.Request) (*common.Claims, string, string) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenOperator, common.RoleCanteenAdmin)
	if schoolID == "" {
		return nil, "", ""
	}
	var tenantID string
	err := a.DB.QueryRowContext(r.Context(), `SELECT tenant_id::text FROM canteen_sales WHERE id::text=$1 AND school_id=$2`, param(r, "id"), schoolID).Scan(&tenantID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "transaksi tidak ditemukan")
		return nil, "", ""
	}
	if err != nil {
		writeErr(w, err)
		return nil, "", ""
	}
	if _, err := a.tenantScope(r.Context(), c, schoolID, tenantID); err != nil {
		writeErr(w, err)
		return nil, "", ""
	}
	return c, schoolID, tenantID
}

// PosVoidSale is POST /pos/sales/{id}/void {reason} (FR-124-004): wallet
// sales get their balance back through a pos_void movement.
func (a *App) PosVoidSale(w http.ResponseWriter, r *http.Request) {
	c, schoolID, _ := a.saleAccess(w, r)
	if schoolID == "" {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	saleID := param(r, "id")
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	var status, method, settlement, movementWallet string
	var total int64
	var recordedAt time.Time
	if err = tx.QueryRowContext(r.Context(),
		`SELECT s.status,s.pay_method,COALESCE(s.settlement_id::text,''),COALESCE(m.wallet_id::text,''),s.total_idr,s.recorded_at
		 FROM canteen_sales s LEFT JOIN wallet_movements m ON m.id=s.wallet_movement_id
		 WHERE s.id::text=$1 AND s.school_id=$2 FOR UPDATE OF s`, saleID, schoolID).
		Scan(&status, &method, &settlement, &movementWallet, &total, &recordedAt); err != nil {
		writeErr(w, err)
		return
	}
	if err := canteen.CanVoid(canteen.SaleState{Status: status, SettlementID: settlement}, in.Reason); err != nil {
		code := http.StatusConflict
		if err == canteen.ErrReasonRequired {
			code = http.StatusUnprocessableEntity
		}
		common.WriteError(w, code, err.Error())
		return
	}
	var balanceAfter *int64
	if method == canteen.PayWallet {
		wl, err := lockWallet(r.Context(), tx, schoolID, movementWallet)
		if err != nil {
			writeErr(w, err)
			return
		}
		if _, _, err = applyMovement(r.Context(), tx, schoolID, &wl, movement{Kind: wallet.MovePosVoid, AmountIDR: total,
			RefType: "canteen_sale", RefID: saleID, Actor: c.UserID, Key: "pos-void:" + saleID}); err != nil {
			writeErr(w, err)
			return
		}
		balanceAfter = &wl.BalanceIDR
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE canteen_daily_stock d SET qty_sold=GREATEST(d.qty_sold-i.qty,0)
		 FROM canteen_sale_items i WHERE i.sale_id::text=$1 AND d.item_id=i.item_id AND d.stock_date=$2::date`,
		saleID, recordedAt.In(a.loc()).Format("2006-01-02")); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE canteen_sales SET status='voided',void_reason=$1,voided_by=$2,voided_at=now() WHERE id::text=$3`,
		strings.TrimSpace(in.Reason), c.UserID, saleID); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_sale", saleID, "sale_voided", c.ActiveRole, c.UserID, "", strings.TrimSpace(in.Reason),
		map[string]any{"status": status}, map[string]any{"status": canteen.SaleVoided, "total_idr": total})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": saleID, "status": canteen.SaleVoided, "balance_after_idr": balanceAfter}, nil)
}

// PosSaleReceipt is GET /pos/sales/{id}/receipt (struk data for print CSS).
func (a *App) PosSaleReceipt(w http.ResponseWriter, r *http.Request) {
	_, schoolID, _ := a.saleAccess(w, r)
	if schoolID == "" {
		return
	}
	sale, err := a.saleView(r.Context(), schoolID, param(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, sale, nil)
}

// PosListSales is GET /pos/sales?tenant_id= : today's sales of a tenant
// (for the void list on the POS screen).
func (a *App) PosListSales(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenOperator, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	scope, err := a.tenantScope(r.Context(), c, schoolID, strings.TrimSpace(r.URL.Query().Get("tenant_id")))
	if err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text,sale_no,pay_method,status,total_idr,recorded_at,offline,late_sync FROM canteen_sales
		 WHERE school_id=$1 AND ($2::text[] IS NULL OR tenant_id::text = ANY($2::text[])) AND recorded_at >= $3
		 ORDER BY recorded_at DESC LIMIT 200`, schoolID, scope, a.today())
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, no, method, status string
		var total int64
		var at time.Time
		var offline, late bool
		if err := rows.Scan(&id, &no, &method, &status, &total, &at, &offline, &late); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "sale_no": no, "pay_method": method, "status": status, "total_idr": total,
			"recorded_at": at, "offline": offline, "late_sync": late})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}
