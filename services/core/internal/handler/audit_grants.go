package handler

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

// maxAuditGrantDays bounds one auditor access window (AC-RBAC-10: access is
// always time-limited; a longer engagement gets a new grant).
const maxAuditGrantDays = 180

type auditGrantRequest struct {
	UserID    string    `json:"user_id"`
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"`
	Reason    string    `json:"reason"`
}

// validateAuditGrant checks the window of a new grant at now.
func validateAuditGrant(in auditGrantRequest, now time.Time) string {
	switch {
	case !isUUID(in.UserID):
		return "user_id auditor harus berupa UUID"
	case strings.TrimSpace(in.Reason) == "":
		return "alasan pemberian akses wajib diisi"
	case in.ValidFrom.IsZero() || in.ValidTo.IsZero():
		return "valid_from dan valid_to wajib diisi"
	case !in.ValidTo.After(in.ValidFrom):
		return "rentang tanggal tidak valid: akhir akses harus setelah awal akses"
	case !in.ValidTo.After(now):
		return "akhir akses sudah lewat"
	case in.ValidTo.Sub(in.ValidFrom) > maxAuditGrantDays*24*time.Hour:
		return "masa akses auditor maksimal 180 hari"
	}
	return ""
}

func superadminTenant(w http.ResponseWriter, r *http.Request) (*common.Claims, string, bool) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return nil, "", false
	}
	if claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "hanya Super Admin yang dapat mengatur masa akses auditor")
		return nil, "", false
	}
	return claims, schoolID, true
}

// ListAuditGrants GET /audit/access-grants (superadmin).
func (h *Handler) ListAuditGrants(w http.ResponseWriter, r *http.Request) {
	_, schoolID, ok := superadminTenant(w, r)
	if !ok {
		return
	}
	grants, err := h.Store.ListAuditGrants(r.Context(), schoolID)
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	common.WriteJSON(w, http.StatusOK, nonNil(grants), nil)
}

// CreateAuditGrant POST /audit/access-grants (superadmin): the grantee must
// hold the auditor role in the school (checked against identity).
func (h *Handler) CreateAuditGrant(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := superadminTenant(w, r)
	if !ok {
		return
	}
	var in auditGrantRequest
	if !common.Decode(w, r, &in) {
		return
	}
	if msg := validateAuditGrant(in, time.Now()); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if h.Client == nil || h.Client.IdentityURL == "" {
		common.WriteError(w, http.StatusServiceUnavailable, "layanan identitas belum dikonfigurasi; peran auditor tidak dapat diverifikasi")
		return
	}
	roles, err := h.Client.UserRoles(r.Context(), schoolID, in.UserID)
	switch {
	case errors.Is(err, client.ErrNotFound):
		common.WriteError(w, http.StatusUnprocessableEntity, "pengguna tidak ditemukan")
		return
	case err != nil:
		common.WriteError(w, http.StatusServiceUnavailable, "gagal memverifikasi peran pengguna di layanan identitas")
		return
	case !slices.Contains(roles, common.RoleAuditor):
		common.WriteError(w, http.StatusUnprocessableEntity, "pengguna tidak memiliki peran auditor di sekolah ini")
		return
	}
	g, err := h.Store.CreateAuditGrant(r.Context(), model.AuditAccessGrant{
		SchoolID: schoolID, UserID: in.UserID, ValidFrom: in.ValidFrom, ValidTo: in.ValidTo,
		Reason: strings.TrimSpace(in.Reason), GrantedBy: claims.GetUserID(),
	})
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "audit", EntityType: "audit_access_grant", EntityID: g.ID,
		Action: "auditor_access_granted", AfterData: map[string]any{"grant": g}, Reason: &g.Reason})
	common.WriteJSON(w, http.StatusCreated, g, nil)
}

// RevokeAuditGrant DELETE /audit/access-grants/{id} (superadmin).
func (h *Handler) RevokeAuditGrant(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := superadminTenant(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "masa akses tidak ditemukan")
		return
	}
	g, err := h.Store.RevokeAuditGrant(r.Context(), schoolID, id, claims.GetUserID())
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "masa akses tidak ditemukan atau sudah dicabut")
		return
	} else if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "audit", EntityType: "audit_access_grant", EntityID: g.ID,
		Action: "auditor_access_revoked", BeforeData: map[string]any{"revoked_at": nil}, AfterData: map[string]any{"grant": g}})
	common.WriteJSON(w, http.StatusOK, g, nil)
}
