package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/core/internal/common"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

func parseAuditTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
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

	userID := r.URL.Query().Get("user_id")
	module := r.URL.Query().Get("module")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	format := r.URL.Query().Get("format")

	filter := store.AuditFilter{
		SchoolID: schoolIDPtr,
		UserID:   userID,
		Module:   module,
		From:     parseAuditTime(from),
		To:       parseAuditTime(to),
	}

	if format == "csv" || strings.Contains(r.Header.Get("Accept"), "text/csv") {
		// Export CSV (FR-06-003, CC-04)
		events, _, err := h.Store.ListAudit(r.Context(), filter, 1, 10000)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "gagal mengekspor log audit: "+err.Error())
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
			IPHash:      HashIP(ClientIP(r)),
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
