package handler

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/core/internal/common"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

// auditLocation is the school timezone used for date-only filters.
func (h *Handler) auditLocation() *time.Location {
	if loc, err := time.LoadLocation(h.Config.SchoolTimezone); err == nil && h.Config.SchoolTimezone != "" {
		return loc
	}
	return time.UTC
}

// auditorHasAccess (BUG-087, AC-RBAC-10): an auditor reads a school's audit
// trail / exception queue only inside an active grant window given by a
// superadmin. Writes 403/500 and returns false otherwise.
func (h *Handler) auditorHasAccess(w http.ResponseWriter, r *http.Request, claims *common.Claims, schoolID string) bool {
	if claims.ActiveRole != common.RoleAuditor {
		return true
	}
	if !isUUID(schoolID) {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return false
	}
	ok, err := h.Store.HasActiveAuditGrant(r.Context(), schoolID, claims.GetUserID())
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memeriksa masa akses auditor")
		return false
	}
	if !ok {
		common.WriteError(w, http.StatusForbidden, "akses auditor Anda tidak aktif atau sudah berakhir; minta Super Admin memberikan masa akses")
		return false
	}
	return true
}

func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	if claims.ActiveRole != common.RoleSuperadmin && claims.ActiveRole != common.RoleAuditor {
		common.WriteError(w, http.StatusForbidden, "hanya superadmin dan auditor yang dapat mengakses audit trail")
		return
	}

	var schoolIDPtr *string
	if claims.ActiveRole != common.RoleSuperadmin || claims.SchoolID != "" {
		sid := common.EffectiveSchoolID(r)
		if sid != "" {
			schoolIDPtr = &sid
		}
	}
	if claims.ActiveRole == common.RoleAuditor {
		sid := ""
		if schoolIDPtr != nil {
			sid = *schoolIDPtr
		}
		if !h.auditorHasAccess(w, r, claims, sid) {
			return
		}
	}

	q := r.URL.Query()
	from, to, err := model.ParseAuditRange(q.Get("from"), q.Get("to"), h.auditLocation())
	switch {
	case errors.Is(err, model.ErrAuditDateRange):
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	case err != nil:
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	format := q.Get("format")

	filter := store.AuditFilter{
		SchoolID:      schoolIDPtr,
		UserID:        q.Get("user_id"),
		Module:        q.Get("module"),
		Service:       q.Get("service"),
		Action:        q.Get("action"),
		EntityID:      q.Get("entity_id"),
		CorrelationID: q.Get("correlation_id"),
		From:          from,
		To:            to,
	}

	if format == "csv" || strings.Contains(r.Header.Get("Accept"), "text/csv") {
		// Export CSV (FR-06-003, CC-04)
		maxRows := h.Config.AuditExportMaxRows
		events, total, err := h.Store.ListAudit(r.Context(), filter, 1, maxRows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "gagal mengekspor log audit: "+err.Error())
			return
		}
		if total > maxRows {
			common.WriteError(w, http.StatusUnprocessableEntity,
				fmt.Sprintf("hasil %d baris melebihi batas ekspor %d baris; persempit filter tanggal/modul", total, maxRows))
			return
		}

		// Log audit event for the export action itself (CC-04)
		_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
			SchoolID:    schoolIDPtr,
			Service:     "core",
			Module:      "audit",
			EntityType:  "audit_trail",
			EntityID:    "export_csv",
			Action:      "audit_exported",
			ActorUserID: ptr(claims.GetUserID()),
			ActorRole:   ptr(claims.ActiveRole),
			Reason:      ptr(fmt.Sprintf("ekspor %d baris audit log", len(events))),
			AfterData: map[string]any{"rows": len(events), "filter": map[string]any{
				"user_id": filter.UserID, "module": filter.Module, "service": filter.Service, "action": filter.Action,
				"entity_id": filter.EntityID, "correlation_id": filter.CorrelationID, "from": q.Get("from"), "to": q.Get("to")}},
			CorrelationID: correlationPtr(r),
			IPHash:        HashIP(ClientIP(r)),
		})

		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=audit-trail-%s.csv", time.Now().Format("20060102-150405")))

		csvWriter := csv.NewWriter(w)
		_ = csvWriter.Write([]string{"ID", "Waktu", "Layanan", "Modul", "Aksi", "Tipe Entitas", "ID Entitas", "User ID", "Peran", "Alasan", "Correlation ID"})

		for _, ev := range events {
			actorID := ""
			if ev.ActorUserID != nil {
				actorID = *ev.ActorUserID
			}
			role := ""
			if ev.ActorRole != nil {
				role = *ev.ActorRole
			}
			reason := ""
			if ev.Reason != nil {
				reason = *ev.Reason
			}
			corrID := ""
			if ev.CorrelationID != nil {
				corrID = *ev.CorrelationID
			}

			_ = csvWriter.Write([]string{
				strconv.FormatInt(ev.ID, 10),
				ev.CreatedAt.Format(time.RFC3339),
				ev.Service,
				ev.Module,
				ev.Action,
				ev.EntityType,
				ev.EntityID,
				actorID,
				role,
				reason,
				corrID,
			})
		}
		csvWriter.Flush()
		return
	}

	page, perPage := common.Page(r)
	events, total, err := h.Store.ListAudit(r.Context(), filter, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat log audit: "+err.Error())
		return
	}

	meta := map[string]any{
		"page":     page,
		"per_page": perPage,
		"total":    total,
	}
	common.WriteJSON(w, http.StatusOK, events, meta)
}
