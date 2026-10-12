package handler

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
)

// Canteen directory (contract with T25 / menu 156): active tenants and their
// active menu, never owner or bank data, never cached so a price change is
// visible immediately (AC-156-01, BR-3D-01).

func (a *App) directoryTenants(ctx context.Context, schoolID, tenantID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id::text,code,name,location_label,COALESCE(room_id::text,''),is_open,open_changed_at FROM canteen_tenants
		 WHERE school_id=$1 AND status='active' AND deleted_at IS NULL AND ($2='' OR id::text=$2) ORDER BY code`, schoolID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, code, name, loc, room string
		var open bool
		var openAt sql.NullTime
		if err := rows.Scan(&id, &code, &name, &loc, &room, &open, &openAt); err != nil {
			return nil, err
		}
		m := map[string]any{"tenant_id": id, "code": code, "name": name, "location_label": loc, "room_id": nil, "is_open": open, "open_changed_at": nil}
		if room != "" {
			m["room_id"] = room
		}
		if openAt.Valid {
			m["open_changed_at"] = openAt.Time
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// directoryMenu is the public projection of a tenant's active menu.
func (a *App) directoryMenu(ctx context.Context, schoolID, tenantID string) ([]map[string]any, error) {
	items, err := a.menuItems(ctx, schoolID, tenantID, true)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{"item_id": it["item_id"], "name": it["name"], "category": it["category"],
			"price_idr": it["price_idr"], "available": it["available"]})
	}
	return out, nil
}

func (a *App) writeDirectory(w http.ResponseWriter, r *http.Request, schoolID string) {
	tenants, err := a.directoryTenants(r.Context(), schoolID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, t := range tenants {
		menu, err := a.directoryMenu(r.Context(), schoolID, t["tenant_id"].(string))
		if err != nil {
			writeErr(w, err)
			return
		}
		t["menu"] = menu
	}
	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, tenants, map[string]any{"generated_at": time.Now()})
}

// CanteenDirectory is GET /canteen/directory (any role of the school).
func (a *App) CanteenDirectory(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if c == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if c.ActiveRole != common.RoleSuperadmin {
		schoolID = c.SchoolID
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id is required")
		return
	}
	if !a.requireFeature(w, r, schoolID, featureCanteen) {
		return
	}
	a.writeDirectory(w, r, schoolID)
}

// internalSchool reads the mandatory ?school_id= of an internal read.
func internalSchool(w http.ResponseWriter, r *http.Request) string {
	s := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if s == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
	}
	return s
}

// InternalCanteenDirectory is GET /internal/canteen/directory?school_id=.
func (a *App) InternalCanteenDirectory(w http.ResponseWriter, r *http.Request) {
	if schoolID := internalSchool(w, r); schoolID != "" {
		a.writeDirectory(w, r, schoolID)
	}
}

// InternalCanteenTenants is GET /internal/canteen/tenants?school_id= (T25).
func (a *App) InternalCanteenTenants(w http.ResponseWriter, r *http.Request) {
	schoolID := internalSchool(w, r)
	if schoolID == "" {
		return
	}
	tenants, err := a.directoryTenants(r.Context(), schoolID, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, tenants, map[string]any{"generated_at": time.Now()})
}

// InternalCanteenTenantMenu is GET /internal/canteen/tenants/{id}/menu?school_id= (T25).
func (a *App) InternalCanteenTenantMenu(w http.ResponseWriter, r *http.Request) {
	schoolID := internalSchool(w, r)
	if schoolID == "" {
		return
	}
	tenants, err := a.directoryTenants(r.Context(), schoolID, param(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(tenants) == 0 {
		common.WriteError(w, http.StatusNotFound, "tenant tidak ditemukan")
		return
	}
	menu, err := a.directoryMenu(r.Context(), schoolID, param(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	common.WriteJSON(w, http.StatusOK, menu, map[string]any{"generated_at": time.Now()})
}
