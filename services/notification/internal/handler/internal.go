package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/provider"
	"skci/notification/internal/store"
)

// internalSendPayload is the POST /internal/notifications contract other
// services (billing, admission, ...) use to reach a wali/siswa.
type internalSendPayload struct {
	SchoolID    string          `json:"school_id"`
	UserID      string          `json:"user_id"`
	Type        string          `json:"type"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	Data        json.RawMessage `json:"data"`
	Channels    []string        `json:"channels"`
	TemplateKey string          `json:"template_key"`
}

// InternalSendNotification handles POST /internal/notifications.
//
// The Idempotency-Key header is REQUIRED: a retried send (network timeout,
// worker restart) must converge on exactly one notification and one set of
// delivery rows. Replaying a known key returns 200 with the existing
// notification instead of 201.
func (h *Handler) InternalSendNotification(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		common.WriteError(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}

	var payload internalSendPayload
	if !common.Decode(w, r, &payload) {
		return
	}

	schoolID := strings.TrimSpace(payload.SchoolID)
	if schoolID == "" {
		schoolID = strings.TrimSpace(r.URL.Query().Get("school_id"))
	}
	if !requireSchool(w, schoolID) {
		return
	}
	if strings.TrimSpace(payload.UserID) == "" {
		common.WriteError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if strings.TrimSpace(payload.Type) == "" {
		common.WriteError(w, http.StatusBadRequest, "type is required")
		return
	}

	title := payload.Title
	body := payload.Body
	if payload.TemplateKey != "" && (title == "" || body == "") {
		tpl, err := h.Store.FindNotificationTemplate(r.Context(), schoolID, payload.TemplateKey, model.ChannelInApp)
		if err == nil && tpl != nil {
			if title == "" {
				title = tpl.Subject
			}
			if body == "" {
				body = tpl.Body
			}
		} else if err != nil && !errors.Is(err, store.ErrNotFound) {
			common.CaptureError(err)
		}
	}
	if strings.TrimSpace(title) == "" {
		common.WriteError(w, http.StatusBadRequest, "title (or template_key with a subject) is required")
		return
	}

	channels, err := normalizeChannels(payload.Channels)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	n, created, err := h.Store.CreateNotification(r.Context(), store.CreateNotificationInput{
		SchoolID:       schoolID,
		UserID:         payload.UserID,
		Type:           payload.Type,
		Title:          title,
		Body:           body,
		Data:           payload.Data,
		IdempotencyKey: &idempotencyKey,
	})
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to create notification")
		return
	}

	if created {
		hasPushInChannels := false
		for _, channel := range channels {
			if channel == model.ChannelPush {
				hasPushInChannels = true
			}
			h.dispatchChannel(r.Context(), n, channel)
		}

		if !hasPushInChannels && n.Type != "campaign" {
			h.maybeAutoDispatchPush(r.Context(), n)
		}
	}

	deliveries, err := h.Store.ListDeliveries(r.Context(), n.ID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to load deliveries")
		return
	}
	n.Deliveries = deliveries

	if created {
		common.WriteJSONMsg(w, http.StatusCreated, n, nil, "Notifikasi dibuat")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, n, nil, "Notifikasi sudah ada (idempoten)")
}

// InternalRetryDelivery handles POST /internal/notifications/{id}/retry.
// Only failed deliveries are retried; already-sent channels are left alone.
func (h *Handler) InternalRetryDelivery(w http.ResponseWriter, r *http.Request) {
	schoolID := templateSchoolID(r, "")
	if !requireSchool(w, schoolID) {
		return
	}
	n, err := h.Store.GetNotificationByID(r.Context(), param(r, "id"), schoolID)
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
	retried := 0
	for _, d := range deliveries {
		if d.Status != model.DeliveryFailed {
			continue
		}
		h.dispatchChannel(r.Context(), n, d.Channel)
		retried++
	}

	deliveries, err = h.Store.ListDeliveries(r.Context(), n.ID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "failed to reload deliveries")
		return
	}
	n.Deliveries = deliveries
	common.WriteJSONMsg(w, http.StatusOK, n, map[string]int{"retried": retried}, "Retry pengiriman selesai")
}

// dispatchChannel creates the delivery ledger row for one channel and records
// its outcome. in_app is terminal immediately (the row IS the in-app message);
// external channels go through the provider adapter, and an unavailable
// recipient address is a recorded failure rather than an error response.
func (h *Handler) dispatchChannel(ctx context.Context, n *model.Notification, channel string) {
	if channel != model.ChannelInApp {
		enabled, err := h.Store.IsChannelEnabled(ctx, n.SchoolID, n.UserID, channel)
		if err != nil {
			common.CaptureError(err)
			return
		}
		if !enabled {
			// User opted out: no delivery row is created at all.
			return
		}
	}

	d, err := h.Store.CreateDelivery(ctx, n.ID, channel)
	if err != nil {
		common.CaptureError(err)
		return
	}

	if channel == model.ChannelInApp {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliverySent,
			ProviderMessageID: "in_app",
			IncrementAttempts: true,
		})
		return
	}

	if channel == model.ChannelPush {
		h.dispatchPushWithDelivery(ctx, n, d)
		return
	}

	recipient := h.recipientFor(ctx, n.UserID, channel)
	if recipient == "" {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         "recipient address unavailable for channel " + channel,
			IncrementAttempts: true,
		})
		return
	}

	if h.Provider == nil {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         "no delivery provider configured",
			IncrementAttempts: true,
		})
		return
	}

	msgID, err := h.Provider.Send(ctx, provider.Message{
		Channel:   channel,
		Recipient: recipient,
		Subject:   n.Title,
		Body:      n.Body,
	})
	if err != nil {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         err.Error(),
			IncrementAttempts: true,
		})
		return
	}
	_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
		Status:            model.DeliverySent,
		ProviderMessageID: msgID,
		IncrementAttempts: true,
	})
}

// recipientFor resolves an email/whatsapp address through identity-service.
// Best-effort: an unreachable identity-service yields an empty recipient, which
// the caller records as a failed delivery.
func (h *Handler) recipientFor(ctx context.Context, userID, channel string) string {
	if h.Client == nil || !h.Client.Configured() {
		return ""
	}
	u, err := h.Client.GetUser(ctx, userID)
	if err != nil || u == nil {
		return ""
	}
	if channel == model.ChannelEmail {
		return strings.TrimSpace(u.Email)
	}
	return strings.TrimSpace(u.Phone)
}

// normalizeChannels validates, dedupes and defaults the requested channels.
func normalizeChannels(in []string) ([]string, error) {
	if len(in) == 0 {
		return []string{model.ChannelInApp}, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, c := range in {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !model.ValidChannels[c] {
			return nil, errors.New("unknown channel " + c + " (want in_app|email|whatsapp|push)")
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return []string{model.ChannelInApp}, nil
	}
	return out, nil
}

func (h *Handler) maybeAutoDispatchPush(ctx context.Context, n *model.Notification) {
	subs, err := h.Store.ListActivePushSubscriptions(ctx, n.SchoolID, n.UserID)
	if err != nil {
		common.CaptureError(err)
		return
	}
	pref, err := h.Store.GetUserPushPreference(ctx, n.SchoolID, n.UserID)
	if err != nil {
		common.CaptureError(err)
		return
	}
	if !model.ShouldPush(pref, len(subs) > 0) {
		return
	}

	d, err := h.Store.CreateDelivery(ctx, n.ID, model.ChannelPush)
	if err != nil {
		common.CaptureError(err)
		return
	}

	h.executePushDelivery(ctx, n, d, subs)
}

func (h *Handler) dispatchPushWithDelivery(ctx context.Context, n *model.Notification, d *model.NotificationDelivery) {
	subs, err := h.Store.ListActivePushSubscriptions(ctx, n.SchoolID, n.UserID)
	if err != nil {
		common.CaptureError(err)
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         "failed to list push subscriptions: " + err.Error(),
			IncrementAttempts: true,
		})
		return
	}
	h.executePushDelivery(ctx, n, d, subs)
}

func (h *Handler) executePushDelivery(ctx context.Context, n *model.Notification, d *model.NotificationDelivery, subs []model.PushSubscription) {
	if h.PushProvider == nil || !h.PushProvider.IsConfigured() {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         "push belum dikonfigurasi: VAPID keys belum diatur",
			IncrementAttempts: true,
		})
		return
	}

	if len(subs) == 0 {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         "no active push subscriptions",
			IncrementAttempts: true,
		})
		return
	}

	var dataMap map[string]any
	if len(n.Data) > 0 {
		_ = json.Unmarshal(n.Data, &dataMap)
	}
	urlStr := ""
	if u, ok := dataMap["url"].(string); ok {
		urlStr = u
	}

	payload := provider.PushPayload{
		Title:          n.Title,
		Body:           n.Body,
		URL:            urlStr,
		NotificationID: n.ID,
	}

	var errMsgs []string
	var firstReceipt string
	successCount := 0

	for _, sub := range subs {
		receiptID, isGone, err := h.PushProvider.SendPush(ctx, sub, payload)
		if isGone {
			_ = h.Store.RevokePushSubscription(ctx, sub.Endpoint)
		}
		if err != nil {
			errMsgs = append(errMsgs, err.Error())
		} else {
			_ = h.Store.TouchPushSubscription(ctx, sub.Endpoint)
			if firstReceipt == "" {
				firstReceipt = receiptID
			}
			successCount++
		}
	}

	if successCount > 0 {
		lastErr := ""
		if len(errMsgs) > 0 {
			lastErr = strings.Join(errMsgs, "; ")
		}
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliverySent,
			ProviderMessageID: firstReceipt,
			LastError:         lastErr,
			IncrementAttempts: true,
		})
	} else {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         strings.Join(errMsgs, "; "),
			IncrementAttempts: true,
		})
	}
}
