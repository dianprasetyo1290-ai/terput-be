package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"time"

	"skci/notification/internal/common"
	"skci/notification/internal/model"
	"skci/notification/internal/store"
)

// isMandatory: a mandatory type (billing/security) or an explicit
// "mandatory": true in the data object (FR-07-006, BUG-090).
func isMandatory(notifType string, data json.RawMessage) bool {
	if model.IsMandatoryType(notifType) {
		return true
	}
	var obj struct {
		Mandatory bool `json:"mandatory"`
	}
	return len(data) > 0 && json.Unmarshal(data, &obj) == nil && obj.Mandatory
}

// externalPrefs reads the user's effective switch for each external channel.
// A read error counts as "off" (the in-app row is still written).
func (h *Handler) externalPrefs(ctx context.Context, n *model.Notification) map[string]bool {
	out := map[string]bool{}
	for _, ch := range []string{model.ChannelEmail, model.ChannelWhatsApp} {
		on, err := h.Store.IsChannelEnabled(ctx, n.SchoolID, n.UserID, ch)
		if err != nil {
			common.CaptureError(err)
			continue
		}
		out[ch] = on
	}
	return out
}

// renderTemplate applies the latest template for key+channel (BUG-088).
// ok=false when there is no template or a variable has no value.
func (h *Handler) renderTemplate(ctx context.Context, schoolID, key, channel, title, body string, data json.RawMessage) (string, string, bool) {
	if key == "" {
		return "", "", false
	}
	tpl, err := h.Store.FindNotificationTemplate(ctx, schoolID, key, channel)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			common.CaptureError(err)
		}
		return "", "", false
	}
	vals := model.TemplateValues(title, body, data)
	subject, okS := model.RenderTemplate(tpl.Subject, vals)
	text, okB := model.RenderTemplate(tpl.Body, vals)
	if !okS || !okB {
		log.Printf("template %s/%s v%d: missing variable, using the sender's text", key, channel, tpl.Version)
		return "", "", false
	}
	if subject == "" {
		subject = title
	}
	return subject, text, true
}

type failureKind int

const (
	failSend      failureKind = iota // provider/gateway error: retryable per delivery
	failProvider                     // channel adapter not configured: one item per school+channel
	failRecipient                    // no address on file: one item per user+channel
)

// reportDeliveryFailure puts a failed external delivery in core's exception
// queue (BUG-113, FR-80-001) without blocking the send. Grouping keys keep a
// configuration problem from flooding the queue with one item per message.
func (h *Handler) reportDeliveryFailure(ctx context.Context, n *model.Notification, channel string, kind failureKind, cause string) {
	if h.Exceptions == nil {
		return
	}
	corr := common.CorrelationID(ctx)
	if corr == "" {
		corr = "notification:" + n.ID
	}
	rep := common.ExceptionReport{
		SchoolID: n.SchoolID, Kind: common.ExceptionKindNotification, Severity: common.ExceptionSeverityNormal, CorrelationID: corr,
		ErrorMessage: "notifikasi " + channel + " gagal: " + cause, MaxAttempts: 3,
		PayloadSummary: map[string]any{"notification_id": n.ID, "type": n.Type, "channel": channel, "user_id": n.UserID},
	}
	switch kind {
	case failProvider:
		rep.IdempotencyKey = "notify-provider:" + channel
		rep.ErrorMessage = "kanal " + channel + " belum dikonfigurasi di layanan notifikasi: " + cause
	case failRecipient:
		rep.IdempotencyKey = "notify-recipient:" + n.UserID + ":" + channel
		rep.ErrorMessage = "pengguna tidak memiliki alamat " + channel + " di data identitas"
	default:
		rep.IdempotencyKey = "notify-delivery:" + n.ID + ":" + channel
		if h.InternalURL != "" {
			u := h.InternalURL + "/internal/notifications/" + url.PathEscape(n.ID) + "/retry?school_id=" + url.QueryEscape(n.SchoolID)
			rep.RetryURL = &u
		}
	}
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		bctx, cancel := context.WithTimeout(common.WithCorrelationID(context.Background(), corr), 10*time.Second)
		defer cancel()
		if err := h.Exceptions(bctx, rep); err != nil {
			log.Printf("report delivery failure %s: %v", rep.IdempotencyKey, err)
		}
	}()
}

// Wait blocks until background reports finish (tests, graceful shutdown).
func (h *Handler) Wait() { h.wg.Wait() }
