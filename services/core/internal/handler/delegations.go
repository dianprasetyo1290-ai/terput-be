package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/model"
)

type createDelegationRequest struct {
	ToUserID  string    `json:"to_user_id"`
	ValidFrom time.Time `json:"valid_from"`
	ValidTo   time.Time `json:"valid_to"`
}

func (h *Handler) ListDelegations(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}

	delegations, err := h.Store.ListDelegations(r.Context(), schoolID, claims.GetUserID())
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat delegasi: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, delegations, nil)
}

func (h *Handler) CreateDelegation(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}

	var req createDelegationRequest
	if !common.Decode(w, r, &req) {
		return
	}

	if strings.TrimSpace(req.ToUserID) == "" {
		common.WriteError(w, http.StatusBadRequest, "to_user_id wajib diisi")
		return
	}
	if req.ToUserID == claims.GetUserID() {
		common.WriteError(w, http.StatusBadRequest, "tidak dapat mendelegasikan wewenang kepada diri sendiri")
		return
	}
	if !isUUID(req.ToUserID) {
		common.WriteError(w, http.StatusBadRequest, "to_user_id harus berupa UUID")
		return
	}
	if !req.ValidTo.After(req.ValidFrom) {
		common.WriteError(w, http.StatusBadRequest, "valid_to harus lebih lambat dari valid_from")
		return
	}
	shared, status, msg := h.delegationPeerRoles(r, claims, schoolID, req.ToUserID)
	if msg != "" {
		common.WriteError(w, status, msg)
		return
	}

	del := model.ApprovalDelegation{
		SchoolID:   schoolID,
		FromUserID: claims.GetUserID(),
		ToUserID:   req.ToUserID,
		ValidFrom:  req.ValidFrom,
		ValidTo:    req.ValidTo,
	}

	created, err := h.Store.CreateDelegation(r.Context(), del)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat delegasi: "+err.Error())
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "approval",
		EntityType:  "approval_delegation",
		EntityID:    created.ID,
		Action:      "delegation_created",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		AfterData:   map[string]any{"delegation": created, "shared_roles": shared},
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// delegationPeerRoles enforces FR-79-005 (BUG-002): the delegate must hold at
// least one approver role the delegator holds, in the same school. The
// delegator's roles come from the signed JWT; the delegate's from identity.
// Without identity the delegation cannot be verified and is refused (503)
// rather than accepted unchecked.
func (h *Handler) delegationPeerRoles(r *http.Request, claims *common.Claims, schoolID, toUserID string) ([]string, int, string) {
	if h.Client == nil || h.Client.IdentityURL == "" {
		return nil, http.StatusServiceUnavailable, "layanan identitas belum dikonfigurasi sehingga penerima delegasi tidak dapat diverifikasi"
	}
	toRoles, err := h.Client.UserRoles(r.Context(), schoolID, toUserID)
	switch {
	case errors.Is(err, client.ErrNotFound):
		return nil, http.StatusUnprocessableEntity, "pengguna penerima delegasi tidak ditemukan"
	case err != nil:
		log.Printf("delegation: resolve roles of %s: %v", toUserID, err)
		return nil, http.StatusServiceUnavailable, "layanan identitas tidak dapat dihubungi untuk memverifikasi penerima delegasi"
	}
	fromRoles := append([]string{claims.ActiveRole}, claims.Roles...)
	shared := model.SharedApproverRoles(fromRoles, toRoles, common.ApproverRoles)
	if len(shared) == 0 {
		return nil, http.StatusUnprocessableEntity, "delegasi hanya dapat diberikan kepada pengguna dengan peran penyetuju yang setara di sekolah yang sama"
	}
	return shared, 0, ""
}

func (h *Handler) RevokeDelegation(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if id == "" {
		common.WriteError(w, http.StatusBadRequest, "id delegasi diperlukan")
		return
	}

	if err := h.Store.RevokeDelegation(r.Context(), schoolID, id, claims.GetUserID()); err != nil {
		// Only the delegator can revoke an active delegation; anything else is 404.
		writeStoreErr(w, err, "delegasi aktif milik Anda tidak ditemukan")
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "approval",
		EntityType:  "approval_delegation",
		EntityID:    id,
		Action:      "delegation_revoked",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "delegasi berhasil dicabut"}, nil)
}
