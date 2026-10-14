package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"skci/notification/internal/common"
	"skci/notification/internal/model"
)

type pushSubscribePayload struct {
	Endpoint  string `json:"endpoint"`
	Keys      struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	UserAgent *string `json:"user_agent,omitempty"`
}

// GetPushConfig handles GET /push/config.
func (h *Handler) GetPushConfig(w http.ResponseWriter, r *http.Request) {
	enabled := false
	pubKey := ""
	if h.PushProvider != nil {
		enabled = h.PushProvider.IsConfigured()
		pubKey = h.PushProvider.VAPIDPublicKey()
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":          enabled,
		"vapid_public_key": pubKey,
	}, nil)
}

// SubscribePush handles POST /push/subscriptions.
func (h *Handler) SubscribePush(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}

	var req pushSubscribePayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	req.Endpoint = strings.TrimSpace(req.Endpoint)
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		common.WriteError(w, http.StatusBadRequest, "endpoint, keys.p256dh, dan keys.auth wajib diisi")
		return
	}

	sub := model.PushSubscription{
		SchoolID:  schoolID,
		UserID:    claims.UserID,
		Endpoint:  req.Endpoint,
		P256dh:    req.Keys.P256dh,
		Auth:      req.Keys.Auth,
		UserAgent: req.UserAgent,
	}

	out, err := h.Store.UpsertPushSubscription(r.Context(), sub)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan langganan push")
		return
	}

	common.WriteJSONMsg(w, http.StatusCreated, out, nil, "Langganan push notifikasi berhasil didaftarkan")
}

// UnsubscribePush handles DELETE /push/subscriptions.
func (h *Handler) UnsubscribePush(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
		return
	}

	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}

	endpoint := strings.TrimSpace(r.URL.Query().Get("endpoint"))
	if endpoint == "" {
		var body struct {
			Endpoint string `json:"endpoint"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			endpoint = strings.TrimSpace(body.Endpoint)
		}
	}

	if endpoint == "" {
		common.WriteError(w, http.StatusBadRequest, "endpoint wajib diisi")
		return
	}

	if err := h.Store.DeletePushSubscription(r.Context(), schoolID, claims.UserID, endpoint); err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal menghapus langganan push")
		return
	}

	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"message": "Langganan push dihapus"}, nil, "Langganan push dihapus")
}
