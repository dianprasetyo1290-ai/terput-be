package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

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
	if !req.ValidTo.After(req.ValidFrom) {
		common.WriteError(w, http.StatusBadRequest, "valid_to harus lebih lambat dari valid_from")
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
		AfterData:   created,
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusCreated, created, nil)
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
		common.WriteError(w, http.StatusInternalServerError, "gagal mencabut delegasi: "+err.Error())
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
