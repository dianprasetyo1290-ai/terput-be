package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"skci/billing/internal/canteen"
	"skci/billing/internal/client"
	"skci/billing/internal/common"
)

// ---------------------------------------------------------------------------
// Access helpers
// ---------------------------------------------------------------------------

// canteenStaff resolves a canteen request: role in roles, canteen feature on.
func (a *App) canteenStaff(w http.ResponseWriter, r *http.Request, roles ...string) (*common.Claims, string) {
	c, schoolID := common.RequireRoles(w, r, roles...)
	if c == nil || !a.requireFeature(w, r, schoolID, featureCanteen) {
		return nil, ""
	}
	return c, schoolID
}

// operatorTenantIDs lists the tenants an operator is actively assigned to.
func (a *App) operatorTenantIDs(ctx context.Context, schoolID, userID string) ([]string, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT o.tenant_id::text FROM canteen_tenant_operators o JOIN canteen_tenants t ON t.id=o.tenant_id
		 WHERE o.school_id=$1 AND o.user_id::text=$2 AND o.active AND t.deleted_at IS NULL ORDER BY t.code`, schoolID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// tenantScope returns the tenants a caller may act on: nil means "all" for
// canteen/school admins; operators get their own list (AC-123-01). A
// requested tenantID outside the operator's list is a 403.
func (a *App) tenantScope(ctx context.Context, c *common.Claims, schoolID, tenantID string) ([]string, error) {
	if c.ActiveRole != common.RoleCanteenOperator {
		if tenantID == "" {
			return nil, nil
		}
		return []string{tenantID}, nil
	}
	own, err := a.operatorTenantIDs(ctx, schoolID, c.UserID)
	if err != nil {
		return nil, err
	}
	if tenantID == "" {
		if len(own) == 0 {
			return nil, &httpError{http.StatusForbidden, "Akun operator belum ditugaskan ke tenant"}
		}
		return own, nil
	}
	for _, id := range own {
		if id == tenantID {
			return []string{tenantID}, nil
		}
	}
	return nil, &httpError{http.StatusForbidden, "Anda tidak memiliki akses ke tenant ini"}
}

// requireTenant checks the tenant exists in the school (and is not deleted).
func (a *App) requireTenant(ctx context.Context, schoolID, tenantID string) error {
	var ok bool
	err := a.DB.QueryRowContext(ctx, `SELECT true FROM canteen_tenants WHERE id::text=$1 AND school_id=$2 AND deleted_at IS NULL`, tenantID, schoolID).Scan(&ok)
	if err == sql.ErrNoRows {
		return &httpError{http.StatusNotFound, "tenant tidak ditemukan"}
	}
	return err
}

// maskAccount keeps only the last four digits of a bank account number.
func maskAccount(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return strings.Repeat("*", len(v)-4) + v[len(v)-4:]
}

// ---------------------------------------------------------------------------
// Tenants (FR-123-001)
// ---------------------------------------------------------------------------

type tenantInput struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	LocationLabel   string `json:"location_label"`
	RoomID          string `json:"room_id"`
	OwnerName       string `json:"owner_name"`
	OwnerPhone      string `json:"owner_phone"`
	BankName        string `json:"bank_name"`
	BankAccountNo   string `json:"bank_account_no"`
	BankAccountName string `json:"bank_account_name"`
	Status          string `json:"status"`
}

func (in *tenantInput) normalize() error {
	for _, v := range []*string{&in.Code, &in.Name, &in.LocationLabel, &in.RoomID, &in.OwnerName, &in.OwnerPhone, &in.BankName, &in.BankAccountNo, &in.BankAccountName, &in.Status} {
		*v = strings.TrimSpace(*v)
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.Code == "" || in.Name == "" {
		return &httpError{http.StatusUnprocessableEntity, "kode dan nama tenant wajib diisi"}
	}
	if in.Status != "active" && in.Status != "inactive" {
		return &httpError{http.StatusUnprocessableEntity, "status harus active atau inactive"}
	}
	return nil
}

// masked returns the audit-safe view of a tenant input.
func (in tenantInput) masked() tenantInput {
	in.BankAccountNo = maskAccount(in.BankAccountNo)
	return in
}

// validateRoom checks a room against ops (T9). Without OPS_URL the room is
// stored unvalidated (documented in the handoff).
func (a *App) validateRoom(ctx context.Context, schoolID, roomID string) error {
	if roomID == "" || !a.Ops.Configured() {
		return nil
	}
	err := a.Ops.Get(ctx, "/internal/facility/rooms/"+url.PathEscape(roomID)+"?school_id="+url.QueryEscape(schoolID), nil)
	var he *client.HTTPError
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return &httpError{http.StatusUnprocessableEntity, "ruang tidak ditemukan di master ruang"}
	}
	return err
}

const tenantCols = `id::text,code,name,location_label,COALESCE(room_id::text,''),owner_name,owner_phone,bank_name,bank_account_no,
	bank_account_name,status,is_open,open_changed_at,created_at`

func scanTenant(row interface{ Scan(...any) error }, reveal bool) (map[string]any, error) {
	var id, code, name, loc, room, owner, phone, bank, accNo, accName, status string
	var open bool
	var openAt sql.NullTime
	var created time.Time
	if err := row.Scan(&id, &code, &name, &loc, &room, &owner, &phone, &bank, &accNo, &accName, &status, &open, &openAt, &created); err != nil {
		return nil, err
	}
	if !reveal {
		accNo = maskAccount(accNo)
	}
	m := map[string]any{"id": id, "code": code, "name": name, "location_label": loc, "room_id": room, "owner_name": owner,
		"owner_phone": phone, "bank_name": bank, "bank_account_no": accNo, "bank_account_name": accName, "status": status,
		"is_open": open, "open_changed_at": nil, "created_at": created}
	if openAt.Valid {
		m["open_changed_at"] = openAt.Time
	}
	return m, nil
}

// ListTenants is GET /canteen/tenants (canteen_admin, admin).
func (a *App) ListTenants(w http.ResponseWriter, r *http.Request) {
	_, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT `+tenantCols+` FROM canteen_tenants WHERE school_id=$1 AND deleted_at IS NULL ORDER BY code`, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		m, err := scanTenant(rows, false)
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
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// GetTenant is GET /canteen/tenants/{id}[?for_edit=1]. The account number is
// masked except when loading the edit form.
func (a *App) GetTenant(w http.ResponseWriter, r *http.Request) {
	_, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	m, err := scanTenant(a.DB.QueryRowContext(r.Context(),
		`SELECT `+tenantCols+` FROM canteen_tenants WHERE id::text=$1 AND school_id=$2 AND deleted_at IS NULL`, param(r, "id"), schoolID),
		r.URL.Query().Get("for_edit") == "1")
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tenant tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, m, nil)
}

// CreateTenant is POST /canteen/tenants.
func (a *App) CreateTenant(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	var in tenantInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, err)
		return
	}
	if err := a.validateRoom(r.Context(), schoolID, in.RoomID); err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO canteen_tenants(school_id,code,name,location_label,room_id,owner_name,owner_phone,bank_name,bank_account_no,bank_account_name,status)
		 VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,$6,$7,$8,$9,$10,$11) RETURNING id::text`,
		schoolID, in.Code, in.Name, in.LocationLabel, in.RoomID, in.OwnerName, in.OwnerPhone, in.BankName, in.BankAccountNo, in.BankAccountName, in.Status).Scan(&id)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "kode tenant sudah dipakai")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_tenant", id, "tenant_created", c.ActiveRole, c.UserID, "", "", nil, in.masked())
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateTenant is PUT /canteen/tenants/{id}.
func (a *App) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	var in tenantInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, err)
		return
	}
	if err := a.validateRoom(r.Context(), schoolID, in.RoomID); err != nil {
		writeErr(w, err)
		return
	}
	id := param(r, "id")
	before, err := scanTenant(a.DB.QueryRowContext(r.Context(),
		`SELECT `+tenantCols+` FROM canteen_tenants WHERE id::text=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID), false)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "tenant tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	_, err = a.DB.ExecContext(r.Context(),
		`UPDATE canteen_tenants SET code=$1,name=$2,location_label=$3,room_id=NULLIF($4,'')::uuid,owner_name=$5,owner_phone=$6,bank_name=$7,
		 bank_account_no=$8,bank_account_name=$9,status=$10,updated_at=now() WHERE id::text=$11 AND school_id=$12`,
		in.Code, in.Name, in.LocationLabel, in.RoomID, in.OwnerName, in.OwnerPhone, in.BankName, in.BankAccountNo, in.BankAccountName, in.Status, id, schoolID)
	if isUniqueViolation(err) {
		common.WriteError(w, http.StatusConflict, "kode tenant sudah dipakai")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_tenant", id, "tenant_updated", c.ActiveRole, c.UserID, "", "", before, in.masked())
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id}, nil)
}

// DeleteTenant is DELETE /canteen/tenants/{id} (soft delete, BR-GEN-07).
func (a *App) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	id := param(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE canteen_tenants SET deleted_at=now(),status='inactive',is_open=false,updated_at=now() WHERE id::text=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "tenant tidak ditemukan")
		return
	}
	_, _ = a.DB.ExecContext(r.Context(), `UPDATE canteen_tenant_operators SET active=false,updated_at=now() WHERE tenant_id::text=$1`, id)
	a.audit(r.Context(), schoolID, "canteen_tenant", id, "tenant_deleted", c.ActiveRole, c.UserID, "", "", nil, map[string]any{"deleted": true})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true}, nil)
}

// ---------------------------------------------------------------------------
// Schemes (FR-123-002): a change is a new history row.
// ---------------------------------------------------------------------------

// ListTenantSchemes is GET /canteen/tenants/{id}/schemes.
func (a *App) ListTenantSchemes(w http.ResponseWriter, r *http.Request) {
	_, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text,kind,share_bp,share_base,rent_idr,effective_from,effective_to,created_by::text,created_at
		 FROM canteen_tenant_schemes WHERE tenant_id::text=$1 AND school_id=$2 ORDER BY effective_from DESC`, param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, kind, by string
		var bp sql.NullInt64
		var base sql.NullString
		var rent sql.NullInt64
		var from time.Time
		var to sql.NullTime
		var created time.Time
		if err := rows.Scan(&id, &kind, &bp, &base, &rent, &from, &to, &by, &created); err != nil {
			writeErr(w, err)
			return
		}
		m := map[string]any{"id": id, "kind": kind, "share_bp": nil, "share_base": nil, "rent_idr": nil,
			"effective_from": from.Format("2006-01-02"), "effective_to": nil, "created_by": by, "created_at": created}
		if bp.Valid {
			m["share_bp"] = bp.Int64
		}
		if base.Valid {
			m["share_base"] = base.String
		}
		if rent.Valid {
			m["rent_idr"] = rent.Int64
		}
		if to.Valid {
			m["effective_to"] = to.Time.Format("2006-01-02")
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// CreateTenantScheme is POST /canteen/tenants/{id}/schemes. An open-ended
// current scheme is closed the day before the new one starts.
func (a *App) CreateTenantScheme(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	var in struct {
		Kind          string  `json:"kind"`
		ShareBP       *int    `json:"share_bp"`
		ShareBase     *string `json:"share_base"`
		RentIDR       *int64  `json:"rent_idr"`
		EffectiveFrom string  `json:"effective_from"`
		EffectiveTo   string  `json:"effective_to"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	from, err := time.Parse("2006-01-02", strings.TrimSpace(in.EffectiveFrom))
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "tanggal mulai berlaku wajib berformat YYYY-MM-DD")
		return
	}
	var to *time.Time
	if strings.TrimSpace(in.EffectiveTo) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(in.EffectiveTo))
		if err != nil || t.Before(from) {
			common.WriteError(w, http.StatusUnprocessableEntity, "tanggal akhir tidak valid")
			return
		}
		to = &t
	}
	switch in.Kind {
	case canteen.SchemeRevenueShare:
		if in.ShareBP == nil || *in.ShareBP < 0 || *in.ShareBP > 10000 || in.ShareBase == nil ||
			(*in.ShareBase != canteen.BaseWallet && *in.ShareBase != canteen.BaseAll) {
			common.WriteError(w, http.StatusUnprocessableEntity, "bagi hasil wajib berisi persentase (0-10000 bp) dan dasar bagi hasil (wallet/all)")
			return
		}
		in.RentIDR = nil
	case canteen.SchemeRent:
		if in.RentIDR == nil || *in.RentIDR < 0 {
			common.WriteError(w, http.StatusUnprocessableEntity, "sewa wajib berisi nominal per bulan")
			return
		}
		in.ShareBP, in.ShareBase = nil, nil
	default:
		common.WriteError(w, http.StatusUnprocessableEntity, "jenis skema harus revenue_share atau rent")
		return
	}
	tenantID := param(r, "id")
	if err := a.requireTenant(r.Context(), schoolID, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(r.Context(), `SELECT 1 FROM canteen_tenants WHERE id::text=$1 FOR UPDATE`, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE canteen_tenant_schemes SET effective_to=$1::date - 1
		 WHERE tenant_id::text=$2 AND effective_to IS NULL AND effective_from < $1::date`, from, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	var id string
	err = tx.QueryRowContext(r.Context(),
		`INSERT INTO canteen_tenant_schemes(school_id,tenant_id,kind,share_bp,share_base,rent_idr,effective_from,effective_to,created_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text`,
		schoolID, tenantID, in.Kind, in.ShareBP, in.ShareBase, in.RentIDR, from, to, c.UserID).Scan(&id)
	if err != nil && strings.Contains(err.Error(), "overlaps") {
		common.WriteError(w, http.StatusConflict, "periode skema tumpang tindih dengan skema lain")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_tenant", tenantID, "scheme_created", c.ActiveRole, c.UserID, "", "", nil, in)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// ---------------------------------------------------------------------------
// Operators (FR-123-003)
// ---------------------------------------------------------------------------

// ListTenantOperators is GET /canteen/tenants/{id}/operators.
func (a *App) ListTenantOperators(w http.ResponseWriter, r *http.Request) {
	_, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin)
	if schoolID == "" {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT user_id::text,email,full_name,active,created_at FROM canteen_tenant_operators
		 WHERE tenant_id::text=$1 AND school_id=$2 ORDER BY active DESC,full_name`, param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var uid, email, name string
		var active bool
		var created time.Time
		if err := rows.Scan(&uid, &email, &name, &active, &created); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{"user_id": uid, "email": email, "full_name": name, "active": active, "created_at": created})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

// AddTenantOperator is POST /canteen/tenants/{id}/operators {email, full_name}.
// The account is created/linked by identity; a new account's default
// password is returned once.
func (a *App) AddTenantOperator(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	var in struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.FullName = strings.TrimSpace(in.FullName)
	if in.Email == "" || !strings.Contains(in.Email, "@") || in.FullName == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "email dan nama operator wajib diisi")
		return
	}
	tenantID := param(r, "id")
	if err := a.requireTenant(r.Context(), schoolID, tenantID); err != nil {
		writeErr(w, err)
		return
	}
	if !a.IdentitySvc.Configured() {
		common.WriteError(w, http.StatusServiceUnavailable, "layanan identitas belum tersedia")
		return
	}
	acct, err := a.IdentitySvc.CreateOrLinkRoleAccount(r.Context(), schoolID, in.Email, in.FullName, common.RoleCanteenOperator)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusBadGateway, "gagal membuat akun operator di layanan identitas")
		return
	}
	if _, err = a.DB.ExecContext(r.Context(),
		`INSERT INTO canteen_tenant_operators(tenant_id,user_id,school_id,email,full_name,active) VALUES($1,$2,$3,$4,$5,true)
		 ON CONFLICT (tenant_id,user_id) DO UPDATE SET active=true,email=EXCLUDED.email,full_name=EXCLUDED.full_name,updated_at=now()`,
		tenantID, acct.UserID, schoolID, in.Email, in.FullName); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_tenant", tenantID, "operator_added", c.ActiveRole, c.UserID, "", "", nil,
		map[string]any{"user_id": acct.UserID, "email": in.Email, "created": acct.Created})
	common.WriteJSON(w, http.StatusCreated, map[string]any{"user_id": acct.UserID, "created": acct.Created, "default_password": acct.DefaultPassword}, nil)
}

// RemoveTenantOperator is DELETE /canteen/tenants/{id}/operators/{userId}.
func (a *App) RemoveTenantOperator(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin)
	if schoolID == "" {
		return
	}
	tenantID, userID := param(r, "id"), param(r, "userId")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE canteen_tenant_operators SET active=false,updated_at=now() WHERE tenant_id::text=$1 AND user_id::text=$2 AND school_id=$3 AND active`,
		tenantID, userID, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "operator tidak ditemukan")
		return
	}
	a.audit(r.Context(), schoolID, "canteen_tenant", tenantID, "operator_removed", c.ActiveRole, c.UserID, "", "", nil, map[string]any{"user_id": userID})
	common.WriteJSON(w, http.StatusOK, map[string]any{"user_id": userID, "active": false}, nil)
}

// ---------------------------------------------------------------------------
// Menu items & daily stock (FR-124-001)
// ---------------------------------------------------------------------------

// menuItemAccess resolves the caller for menu endpoints: canteen admins on
// any tenant, operators on their own tenant only.
func (a *App) menuItemAccess(w http.ResponseWriter, r *http.Request) (*common.Claims, string, string) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin, common.RoleCanteenOperator)
	if schoolID == "" {
		return nil, "", ""
	}
	tenantID := param(r, "id")
	if _, err := a.tenantScope(r.Context(), c, schoolID, tenantID); err != nil {
		writeErr(w, err)
		return nil, "", ""
	}
	if err := a.requireTenant(r.Context(), schoolID, tenantID); err != nil {
		writeErr(w, err)
		return nil, "", ""
	}
	return c, schoolID, tenantID
}

// menuItems lists a tenant's items with today's stock (activeOnly for POS).
func (a *App) menuItems(ctx context.Context, schoolID, tenantID string, activeOnly bool) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT i.id::text,i.name,i.category,i.price_idr,i.active,s.qty_start,s.qty_sold
		 FROM canteen_menu_items i LEFT JOIN canteen_daily_stock s ON s.item_id=i.id AND s.stock_date=$3::date
		 WHERE i.tenant_id::text=$1 AND i.school_id=$2 AND i.deleted_at IS NULL AND (NOT $4 OR i.active)
		 ORDER BY i.category,i.name`, tenantID, schoolID, a.todayDate(), activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, cat string
		var price int64
		var active bool
		var start, sold sql.NullInt64
		if err := rows.Scan(&id, &name, &cat, &price, &active, &start, &sold); err != nil {
			return nil, err
		}
		m := map[string]any{"id": id, "item_id": id, "name": name, "category": cat, "price_idr": price, "active": active,
			"stock_remaining": nil, "available": active}
		if start.Valid {
			remaining := start.Int64 - sold.Int64
			m["stock_remaining"] = remaining
			m["available"] = active && remaining > 0
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// ListMenuItems is GET /canteen/tenants/{id}/menu-items.
func (a *App) ListMenuItems(w http.ResponseWriter, r *http.Request) {
	_, schoolID, tenantID := a.menuItemAccess(w, r)
	if schoolID == "" {
		return
	}
	items, err := a.menuItems(r.Context(), schoolID, tenantID, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, nil)
}

type menuItemInput struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	PriceIDR int64  `json:"price_idr"`
	Active   *bool  `json:"active"`
}

func (in *menuItemInput) normalize() error {
	in.Name, in.Category = strings.TrimSpace(in.Name), strings.TrimSpace(in.Category)
	if in.Name == "" || in.PriceIDR <= 0 {
		return &httpError{http.StatusUnprocessableEntity, "nama dan harga (> 0) wajib diisi"}
	}
	if in.Active == nil {
		t := true
		in.Active = &t
	}
	return nil
}

// CreateMenuItem is POST /canteen/tenants/{id}/menu-items.
func (a *App) CreateMenuItem(w http.ResponseWriter, r *http.Request) {
	c, schoolID, tenantID := a.menuItemAccess(w, r)
	if schoolID == "" {
		return
	}
	var in menuItemInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, err)
		return
	}
	var id string
	if err := a.DB.QueryRowContext(r.Context(),
		`INSERT INTO canteen_menu_items(school_id,tenant_id,name,category,price_idr,active) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`,
		schoolID, tenantID, in.Name, in.Category, in.PriceIDR, *in.Active).Scan(&id); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_menu_item", id, "menu_item_created", c.ActiveRole, c.UserID, "", "", nil, in)
	common.WriteJSON(w, http.StatusCreated, map[string]any{"id": id}, nil)
}

// UpdateMenuItem is PUT /canteen/tenants/{id}/menu-items/{itemId}.
func (a *App) UpdateMenuItem(w http.ResponseWriter, r *http.Request) {
	c, schoolID, tenantID := a.menuItemAccess(w, r)
	if schoolID == "" {
		return
	}
	var in menuItemInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := in.normalize(); err != nil {
		writeErr(w, err)
		return
	}
	itemID := param(r, "itemId")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE canteen_menu_items SET name=$1,category=$2,price_idr=$3,active=$4,updated_at=now()
		 WHERE id::text=$5 AND tenant_id::text=$6 AND school_id=$7 AND deleted_at IS NULL`,
		in.Name, in.Category, in.PriceIDR, *in.Active, itemID, tenantID, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "item menu tidak ditemukan")
		return
	}
	a.audit(r.Context(), schoolID, "canteen_menu_item", itemID, "menu_item_updated", c.ActiveRole, c.UserID, "", "", nil, in)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": itemID}, nil)
}

// DeleteMenuItem is DELETE /canteen/tenants/{id}/menu-items/{itemId} (soft).
func (a *App) DeleteMenuItem(w http.ResponseWriter, r *http.Request) {
	c, schoolID, tenantID := a.menuItemAccess(w, r)
	if schoolID == "" {
		return
	}
	itemID := param(r, "itemId")
	res, err := a.DB.ExecContext(r.Context(),
		`UPDATE canteen_menu_items SET deleted_at=now(),active=false,updated_at=now()
		 WHERE id::text=$1 AND tenant_id::text=$2 AND school_id=$3 AND deleted_at IS NULL`, itemID, tenantID, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "item menu tidak ditemukan")
		return
	}
	a.audit(r.Context(), schoolID, "canteen_menu_item", itemID, "menu_item_deleted", c.ActiveRole, c.UserID, "", "", nil, nil)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": itemID, "deleted": true}, nil)
}

// SetMenuItemStock is PUT /canteen/tenants/{id}/menu-items/{itemId}/stock
// {qty_start} for today (optional per item; null removes the limit).
func (a *App) SetMenuItemStock(w http.ResponseWriter, r *http.Request) {
	c, schoolID, tenantID := a.menuItemAccess(w, r)
	if schoolID == "" {
		return
	}
	var in struct {
		QtyStart *int `json:"qty_start"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	itemID := param(r, "itemId")
	var ok bool
	if err := a.DB.QueryRowContext(r.Context(),
		`SELECT true FROM canteen_menu_items WHERE id::text=$1 AND tenant_id::text=$2 AND school_id=$3 AND deleted_at IS NULL`,
		itemID, tenantID, schoolID).Scan(&ok); err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "item menu tidak ditemukan")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	var err error
	if in.QtyStart == nil {
		_, err = a.DB.ExecContext(r.Context(), `DELETE FROM canteen_daily_stock WHERE item_id::text=$1 AND stock_date=$2::date`, itemID, a.todayDate())
	} else if *in.QtyStart < 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "stok tidak boleh negatif")
		return
	} else {
		_, err = a.DB.ExecContext(r.Context(),
			`INSERT INTO canteen_daily_stock(item_id,stock_date,qty_start) VALUES($1,$2,$3)
			 ON CONFLICT (item_id,stock_date) DO UPDATE SET qty_start=EXCLUDED.qty_start`, itemID, a.todayDate(), *in.QtyStart)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "canteen_menu_item", itemID, "menu_item_stock_set", c.ActiveRole, c.UserID, "", "", nil, in)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": itemID, "qty_start": in.QtyStart}, nil)
}
