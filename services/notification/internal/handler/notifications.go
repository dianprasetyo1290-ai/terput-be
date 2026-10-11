package handler

import (
	"errors"
	"net/http"

	"skci/notification/internal/common"
	"skci/notification/internal/store"
)

// ListNotifications handles GET /notifications — the caller's own inbox.
// Query: ?unread=true, ?type=, ?page=, ?per_page=.
func (h *Handler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	page, perPage := common.Page(r)

	filter := store.NotificationFilter{
		SchoolID: schoolID,
		UserID:   claims.UserID, // always the caller — never a client-supplied user id
		Type:     r.URL.Query().Get("type"),
		Unread:   r.URL.Query().Get("unread") == "true" || r.URL.Query().Get("unread") == "1",
		Page:     page,
		PerPage:  perPage,
	}
	items, total, err := h.Store.ListNotifications(r.Context(), filter)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to list notifications")
		return
	}
	common.WriteJSON(w, http.StatusOK, items, common.NewPageMeta(page, perPage, total))
}

// UnreadCount handles GET /notifications/unread-count — the inbox badge.
func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	count, err := h.Store.CountUnreadNotifications(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to count unread notifications")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]int{"count": count}, nil)
}

// MarkRead handles POST /notifications/{id}/read. Idempotent: re-marking an
// already-read notification keeps its original read_at.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	n, err := h.Store.MarkNotificationRead(r.Context(), param(r, "id"), schoolID, claims.UserID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "notification not found")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to mark notification read")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, n, nil, "Notifikasi ditandai sudah dibaca")
}

// MarkAllRead handles POST /notifications/read-all.
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	marked, err := h.Store.MarkAllNotificationsRead(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to mark notifications read")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]int{"marked": marked}, nil, "Semua notifikasi ditandai sudah dibaca")
}

// ListNotificationDeliveries handles GET /notifications/{id}/deliveries — the
// per-channel delivery status for one of the caller's own notifications.
func (h *Handler) ListNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	schoolID := effectiveSchoolID(r, claims)
	if !requireSchool(w, schoolID) {
		return
	}
	n, err := h.Store.GetNotification(r.Context(), param(r, "id"), schoolID, claims.UserID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "notification not found")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load notification")
		return
	}
	deliveries, err := h.Store.ListDeliveries(r.Context(), n.ID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load deliveries")
		return
	}
	common.WriteJSON(w, http.StatusOK, deliveries, nil)
}
