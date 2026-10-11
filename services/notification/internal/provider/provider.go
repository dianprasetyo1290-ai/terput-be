// Package provider is the outbound delivery adapter. It is deliberately tiny
// and injectable: the development adapter is an explicit no-op, and the
// "unconfigured" adapter refuses every send so a production deployment without
// real credentials fails loudly (and visibly, as a failed delivery row) instead
// of pretending a message went out.
package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
)

// Message is one outbound delivery handed to a provider.
type Message struct {
	Channel   string
	Recipient string // email address or phone number
	Subject   string
	Body      string
}

// Provider delivers a message to an external channel and returns the upstream
// receipt id.
type Provider interface {
	Name() string
	Send(ctx context.Context, msg Message) (string, error)
}

// NoopProvider logs the fact of a delivery without contacting anything. It is
// only ever constructed for a non-production deployment.
type NoopProvider struct{}

// Name implements Provider.
func (NoopProvider) Name() string { return "dev-noop" }

// Send implements Provider: it returns a synthetic receipt so the delivery row
// can be marked sent in local development.
func (NoopProvider) Send(_ context.Context, msg Message) (string, error) {
	id := "dev-noop-" + randomID()
	// Never log the body; it may contain personal data. Channel + recipient
	// suffix is enough for local debugging.
	log.Printf("dev provider: channel=%s recipient=%s message_id=%s (not actually sent)", msg.Channel, maskRecipient(msg.Recipient), id)
	return id, nil
}

// UnconfiguredProvider is the production default: it refuses every send with a
// clear, actionable error until a real provider is wired in.
type UnconfiguredProvider struct{}

// Name implements Provider.
func (UnconfiguredProvider) Name() string { return "unconfigured" }

// Send implements Provider. The error is intentionally explicit so it lands in
// notification_deliveries.last_error and makes a misconfigured deploy obvious.
func (UnconfiguredProvider) Send(_ context.Context, msg Message) (string, error) {
	return "", fmt.Errorf("delivery provider not configured: refusing to send %s message; set NOTIFICATION_PROVIDER and provider credentials", msg.Channel)
}

// New selects the adapter for a provider name.
//
//   - "dev"  -> no-op, development only (New returns an error in production).
//   - "noop" -> no-op, explicitly chosen (allowed anywhere).
//   - "" or "unconfigured" -> refusing adapter (production safe default).
//
// Any other name is rejected so a typo cannot silently disable delivery.
func New(name string, production bool) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dev":
		if production {
			return nil, fmt.Errorf("NOTIFICATION_PROVIDER=dev is forbidden in production")
		}
		return NoopProvider{}, nil
	case "noop":
		return NoopProvider{}, nil
	case "", "unconfigured":
		return UnconfiguredProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown NOTIFICATION_PROVIDER %q (want dev|noop|unconfigured)", name)
	}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func maskRecipient(v string) string {
	if len(v) <= 3 {
		return "***"
	}
	return "***" + v[len(v)-3:]
}
