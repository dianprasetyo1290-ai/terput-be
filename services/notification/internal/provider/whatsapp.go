package provider

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// ErrWhatsAppDisabled is returned when no WhatsApp provider is configured
// (WHATSAPP_PROVIDER empty). Campaign creation maps it to 409.
var ErrWhatsAppDisabled = errors.New("Kanal WhatsApp belum dikonfigurasi")

// WhatsApp delivery statuses reported by a provider webhook.
const (
	WAStatusDelivered = "delivered"
	WAStatusRead      = "read"
	WAStatusReplied   = "replied"
	WAStatusFailed    = "failed"
)

// StatusEvent is one normalized delivery/reply event from a provider webhook.
type StatusEvent struct {
	ProviderMessageID string
	Status            string
	From              string // sender phone for replies
	Text              string // reply text (used for stop keywords only, never stored)
	At                time.Time
	Error             string
}

// TemplateSender sends provider-approved WhatsApp templates (IF-ADV-02).
// Only the noop adapter exists until OQ-38 picks a provider; real adapters
// are added here behind the same contract.
type TemplateSender interface {
	Name() string
	SendTemplate(ctx context.Context, to, providerTemplateID, lang string, vars map[string]string) (string, error)
	ParseStatusWebhook(r *http.Request) ([]StatusEvent, error)
	// IsStopKeyword reports whether a reply text means "stop promotions".
	IsStopKeyword(text string) bool
}

// NewWhatsApp selects the WhatsApp adapter:
//
//   - "" or "disabled" -> nil, ErrWhatsAppDisabled is the caller's signal
//   - "noop"           -> logs only (development / tests; config forbids it in production)
//
// Any other name is rejected so a typo cannot pretend a provider is wired.
func NewWhatsApp(name, webhookSecret string, stopKeywords []string) (TemplateSender, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "disabled":
		return nil, nil
	case "noop":
		return &NoopWhatsApp{WebhookSecret: webhookSecret, StopKeywords: stopKeywords}, nil
	default:
		return nil, fmt.Errorf("unknown WHATSAPP_PROVIDER %q (want empty|noop; real adapters follow OQ-38)", name)
	}
}

// NoopWhatsApp is the development adapter: it never contacts a provider. Its
// webhook accepts a minimal SEKOCI-defined JSON body authenticated with the
// shared webhook secret so the status pipeline can be exercised end to end.
type NoopWhatsApp struct {
	WebhookSecret string
	StopKeywords  []string
}

// Name implements TemplateSender.
func (*NoopWhatsApp) Name() string { return "noop" }

// SendTemplate implements TemplateSender.
func (*NoopWhatsApp) SendTemplate(_ context.Context, to, providerTemplateID, _ string, _ map[string]string) (string, error) {
	id := "wa-noop-" + randomID()
	log.Printf("whatsapp noop: template=%s recipient=%s message_id=%s (not actually sent)", providerTemplateID, maskRecipient(to), id)
	return id, nil
}

// noopWebhookBody is the dev-only webhook shape.
type noopWebhookBody struct {
	Events []struct {
		ProviderMessageID string `json:"provider_message_id"`
		Status            string `json:"status"`
		From              string `json:"from"`
		Text              string `json:"text"`
		Error             string `json:"error"`
	} `json:"events"`
}

// ParseStatusWebhook implements TemplateSender. The request must carry
// X-Webhook-Secret equal to the configured secret.
func (n *NoopWhatsApp) ParseStatusWebhook(r *http.Request) ([]StatusEvent, error) {
	got := r.Header.Get("X-Webhook-Secret")
	if n.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(got), []byte(n.WebhookSecret)) != 1 {
		return nil, errors.New("invalid webhook signature")
	}
	var body noopWebhookBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid webhook body: %w", err)
	}
	now := time.Now()
	out := make([]StatusEvent, 0, len(body.Events))
	for _, e := range body.Events {
		switch e.Status {
		case WAStatusDelivered, WAStatusRead, WAStatusReplied, WAStatusFailed:
		default:
			continue
		}
		out = append(out, StatusEvent{ProviderMessageID: e.ProviderMessageID, Status: e.Status, From: e.From, Text: e.Text, Error: e.Error, At: now})
	}
	return out, nil
}

// IsStopKeyword implements TemplateSender (case-insensitive exact match).
func (n *NoopWhatsApp) IsStopKeyword(text string) bool {
	t := strings.TrimSpace(text)
	for _, k := range n.StopKeywords {
		if k != "" && strings.EqualFold(t, k) {
			return true
		}
	}
	return false
}
