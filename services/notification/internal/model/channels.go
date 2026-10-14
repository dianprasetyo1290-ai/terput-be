package model

import "strings"

// MandatoryTypePrefixes are the event types that ignore channel opt-outs
// (FR-07-006, BUG-090): billing (tagihan) and security notices always go to
// every external channel the user can be reached on.
var MandatoryTypePrefixes = []string{"bill_", "invoice_", "security_"}

// TypeCampaign is the marketing campaign type: its channels and consent are
// decided by the campaign module, never widened here.
const TypeCampaign = "campaign"

// IsMandatoryType reports whether a notification type is mandatory.
func IsMandatoryType(t string) bool {
	for _, p := range MandatoryTypePrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// externalChannels are the provider-backed channels preferences govern.
var externalChannels = []string{ChannelEmail, ChannelWhatsApp}

// ResolveChannels decides which channels a notification is delivered on
// (BUG-029/090). The caller's list is a minimum, not a ceiling:
//   - in_app is always included (the inbox row is the record);
//   - mandatory: every external channel, regardless of preferences;
//   - otherwise: an external channel is used when the user enabled it, and a
//     requested external channel the user switched off is dropped;
//   - campaign: exactly what was requested (campaign consent applies).
//
// enabled holds the user's effective switch per external channel. Push is
// passed through as requested (it has its own preference and auto-dispatch).
func ResolveChannels(requested []string, notifType string, mandatory bool, enabled map[string]bool) []string {
	if notifType == TypeCampaign {
		return requested
	}
	want := map[string]bool{ChannelInApp: true}
	for _, c := range requested {
		want[c] = true
	}
	for _, c := range externalChannels {
		want[c] = mandatory || enabled[c]
	}
	out := make([]string, 0, len(want))
	for _, c := range []string{ChannelInApp, ChannelEmail, ChannelWhatsApp, ChannelPush} {
		if want[c] {
			out = append(out, c)
		}
	}
	return out
}
