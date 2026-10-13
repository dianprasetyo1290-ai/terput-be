package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	webpush "github.com/SherClockHolmes/webpush-go"
	"skci/notification/internal/model"
)

type WebPushConfig struct {
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	Subject         string
	TTLSeconds      int
}

type PushPayload struct {
	Title          string `json:"title"`
	Body           string `json:"body"`
	URL            string `json:"url,omitempty"`
	NotificationID string `json:"notification_id,omitempty"`
}

type PushSender interface {
	IsConfigured() bool
	VAPIDPublicKey() string
	SendPush(ctx context.Context, sub model.PushSubscription, payload PushPayload) (string, bool, error)
}

type WebPushProvider struct {
	Config WebPushConfig
}

func NewWebPushProvider(cfg WebPushConfig) *WebPushProvider {
	if cfg.TTLSeconds <= 0 {
		cfg.TTLSeconds = 86400
	}
	return &WebPushProvider{Config: cfg}
}

func (p *WebPushProvider) IsConfigured() bool {
	return p.Config.VAPIDPublicKey != "" && p.Config.VAPIDPrivateKey != ""
}

func (p *WebPushProvider) VAPIDPublicKey() string {
	return p.Config.VAPIDPublicKey
}

// SendPush sends an encrypted WebPush notification to a subscription.
// Returns (receiptID, isGone, error).
func (p *WebPushProvider) SendPush(ctx context.Context, sub model.PushSubscription, payload PushPayload) (string, bool, error) {
	if !p.IsConfigured() {
		return "", false, errors.New("push belum dikonfigurasi: VAPID keys belum diatur")
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}

	s := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	res, err := webpush.SendNotificationWithContext(ctx, payloadBytes, s, &webpush.Options{
		Subscriber:      p.Config.Subject,
		VAPIDPublicKey:  p.Config.VAPIDPublicKey,
		VAPIDPrivateKey: p.Config.VAPIDPrivateKey,
		TTL:             p.Config.TTLSeconds,
	})
	if err != nil {
		return "", false, err
	}
	defer res.Body.Close()

	if model.IsGoneStatus(res.StatusCode) {
		return "", true, fmt.Errorf("push endpoint expired (%d)", res.StatusCode)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", false, fmt.Errorf("push service returned HTTP %d", res.StatusCode)
	}

	return fmt.Sprintf("webpush-%d", res.StatusCode), false, nil
}
