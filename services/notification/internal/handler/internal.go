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
		common.WriteError(w, http.StatusBadRequest, "user_id wajib diisi")
		return
	}
	if strings.TrimSpace(payload.Type) == "" {
		common.WriteError(w, http.StatusBadRequest, "type wajib diisi")
		return
	}

	title := payload.Title
	body := payload.Body
	// BUG-088: the latest admin-edited template for the event wins over the
	// caller's text (template_key, else the event type). A template whose
	// variables cannot all be filled falls back to the caller's text.
	tplKey := strings.TrimSpace(payload.TemplateKey)
	if tplKey == "" {
		tplKey = payload.Type
	}
	if t, b, ok := h.renderTemplate(r.Context(), schoolID, tplKey, model.ChannelInApp, title, body, payload.Data); ok {
		title, body = t, b
	}
	if strings.TrimSpace(title) == "" {
		common.WriteError(w, http.StatusBadRequest, "judul (atau template_key dengan subjek) wajib diisi")
		return
	}

	requested, err := normalizeChannels(payload.Channels)
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
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat notifikasi")
		return
	}

	if created {
		campaign := n.Type == model.TypeCampaign
		channels := model.ResolveChannels(requested, n.Type, isMandatory(n.Type, payload.Data), h.externalPrefs(r.Context(), n))
		hasPushInChannels := false
		for _, channel := range channels {
			if channel == model.ChannelPush {
				hasPushInChannels = true
			}
			h.dispatchChannel(r.Context(), n, channel, campaign)
		}

		if !hasPushInChannels && n.Type != "campaign" {
			h.maybeAutoDispatchPush(r.Context(), n)
		}
	}

	deliveries, err := h.Store.ListDeliveries(r.Context(), n.ID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat status pengiriman")
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
		common.WriteError(w, http.StatusNotFound, "notifikasi tidak ditemukan")
		return
	}
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat notifikasi")
		return
	}

	deliveries, err := h.Store.ListDeliveries(r.Context(), n.ID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat status pengiriman")
		return
	}
	retried := 0
	for _, d := range deliveries {
		if d.Status != model.DeliveryFailed {
			continue
		}
		h.dispatchChannel(r.Context(), n, d.Channel, false)
		retried++
	}

	deliveries, err = h.Store.ListDeliveries(r.Context(), n.ID)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat ulang status pengiriman")
		return
	}
	n.Deliveries = deliveries
	// Core's exception queue resolves an item only on 2xx: a delivery that is
	// still failing must not look like a successful retry.
	for _, d := range deliveries {
		if d.Status == model.DeliveryFailed {
			common.WriteError(w, http.StatusBadGateway, "pengiriman "+d.Channel+" masih gagal: "+d.LastErrorText())
			return
		}
	}
	common.WriteJSONMsg(w, http.StatusOK, n, map[string]int{"retried": retried}, "Retry pengiriman selesai")
}

// dispatchChannel creates the delivery ledger row for one channel and records
// its outcome. in_app is terminal immediately (the row IS the in-app message);
// external channels go through the provider adapter, and an unavailable
// recipient address is a recorded failure rather than an error response.
// checkPreference re-applies the user's opt-out (campaign sends); normal
// sends were already resolved by model.ResolveChannels.
func (h *Handler) dispatchChannel(ctx context.Context, n *model.Notification, channel string, checkPreference bool) {
	if checkPreference && channel != model.ChannelInApp {
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
		msg := "recipient address unavailable for channel " + channel
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         msg,
			IncrementAttempts: true,
		})
		h.reportDeliveryFailure(ctx, n, channel, failRecipient, msg)
		return
	}

	if h.Provider == nil {
		msg := "no delivery provider configured"
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         msg,
			IncrementAttempts: true,
		})
		h.reportDeliveryFailure(ctx, n, channel, failProvider, msg)
		return
	}

	subject, text := n.Title, n.Body
	if t, b, ok := h.renderTemplate(ctx, n.SchoolID, n.Type, channel, n.Title, n.Body, n.Data); ok {
		subject, text = t, b
	}
	msgID, err := h.Provider.Send(ctx, provider.Message{
		Channel:   channel,
		Recipient: recipient,
		Subject:   subject,
		Body:      text,
	})
	if err != nil {
		_, _ = h.Store.UpdateDelivery(ctx, d.ID, store.UpdateDeliveryInput{
			Status:            model.DeliveryFailed,
			LastError:         err.Error(),
			IncrementAttempts: true,
		})
		kind := failSend
		if errors.Is(err, provider.ErrNotConfigured) {
			kind = failProvider
		}
		h.reportDeliveryFailure(ctx, n, channel, kind, err.Error())
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
