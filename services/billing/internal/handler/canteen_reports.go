package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/canteen"
	"skci/billing/internal/common"
)

// parseDateRange reads ?from=&to= (YYYY-MM-DD, inclusive), defaulting to the
// current month in the school time zone.
func (a *App) parseDateRange(r *http.Request) (time.Time, time.Time, error) {
	today := a.today()
	from := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, a.loc())
	to := from.AddDate(0, 1, -1)
	if v := strings.TrimSpace(r.URL.Query().Get("from")); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, a.loc())
		if err != nil {
			return from, to, &httpError{http.StatusUnprocessableEntity, "format tanggal from harus YYYY-MM-DD"}
		}
		from = t
	}
	if v := strings.TrimSpace(r.URL.Query().Get("to")); v != "" {
		t, err := time.ParseInLocation("2006-01-02", v, a.loc())
		if err != nil {
			return from, to, &httpError{http.StatusUnprocessableEntity, "format tanggal to harus YYYY-MM-DD"}
		}
		to = t
	}
	if to.Before(from) {
		return from, to, &httpError{http.StatusUnprocessableEntity, "tanggal akhir sebelum tanggal awal"}
	}
	return from, to, nil
}

// saleLines loads sale item rows of the scoped tenants in [from, to] (dates
// in the school time zone). tenants nil = every tenant of the school.
func (a *App) saleLines(ctx context.Context, schoolID string, tenants []string, method string, from, to time.Time) ([]canteen.SaleLine, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT s.id::text,s.tenant_id::text,t.name,i.item_id::text,i.name_snapshot,s.pay_method,s.status,s.recorded_at,i.qty,i.line_total_idr
		 FROM canteen_sales s JOIN canteen_tenants t ON t.id=s.tenant_id JOIN canteen_sale_items i ON i.sale_id=s.id
		 WHERE s.school_id=$1 AND ($2::text[] IS NULL OR s.tenant_id::text = ANY($2::text[])) AND ($3='' OR s.pay_method=$3)
		   AND s.recorded_at >= $4 AND s.recorded_at < $5`,
		schoolID, tenants, method, from, to.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []canteen.SaleLine
	for rows.Next() {
		var l canteen.SaleLine
		if err := rows.Scan(&l.SaleID, &l.TenantID, &l.TenantName, &l.ItemID, &l.ItemName, &l.PayMethod, &l.Status, &l.RecordedAt, &l.Qty, &l.LineIDR); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// SalesReport is GET /canteen/reports/sales?tenant_id=&from=&to=&group_by=&method=
// (FR-126-001). Operators only ever see their own tenants (AC-123-01): a
// foreign tenant_id is 403, no tenant_id means "my tenants".
func (a *App) SalesReport(w http.ResponseWriter, r *http.Request) {
	c, schoolID := a.canteenStaff(w, r, common.RoleCanteenAdmin, common.RoleAdmin, common.RoleFinanceManager, common.RoleCanteenOperator)
	if schoolID == "" {
		return
	}
	scope, err := a.tenantScope(r.Context(), c, schoolID, strings.TrimSpace(r.URL.Query().Get("tenant_id")))
	if err != nil {
		writeErr(w, err)
		return
	}
	groupBy := strings.TrimSpace(r.URL.Query().Get("group_by"))
	if groupBy == "" {
		groupBy = "tenant"
	}
	if !canteen.GroupBys[groupBy] {
		common.WriteError(w, http.StatusUnprocessableEntity, "group_by harus tenant, item, day atau method")
		return
	}
	method := strings.TrimSpace(r.URL.Query().Get("method"))
	if method != "" && method != canteen.PayCash && method != canteen.PayWallet {
		common.WriteError(w, http.StatusUnprocessableEntity, "method harus wallet atau cash")
		return
	}
	from, to, err := a.parseDateRange(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	lines, err := a.saleLines(r.Context(), schoolID, scope, method, from, to)
	if err != nil {
		writeErr(w, err)
		return
	}
	rows := canteen.SalesSummary(lines, groupBy, a.loc())
	var total int64
	sales := map[string]bool{}
	var voided int
	voidedSeen := map[string]bool{}
	tenantSeen := map[string]bool{}
	for _, l := range lines {
		if l.Status == canteen.SaleVoided {
			if !voidedSeen[l.SaleID] {
				voidedSeen[l.SaleID] = true
				voided++
			}
			continue
		}
		total += l.LineIDR
		sales[l.SaleID] = true
		tenantSeen[l.TenantID] = true
	}
	tenantIDs := make([]string, 0, len(tenantSeen))
	for id := range tenantSeen {
		tenantIDs = append(tenantIDs, id)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"rows": rows, "total_idr": total, "sales_count": len(sales), "voided_count": voided, "tenant_ids": tenantIDs,
		"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "group_by": groupBy,
	}, map[string]any{"generated_at": a.now()})
}
