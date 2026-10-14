package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"skci/notification/internal/provider"
)

// Round 2: without WA credentials the admin UI must show "belum
// dikonfigurasi" rather than a failing button.
func TestChannelStatus(t *testing.T) {
	_, router, h, _, _ := newDeliveryServer(t, nil)
	h.Provider = provider.ChannelRouter{Default: provider.UnconfiguredProvider{}, Email: provider.NoopProvider{}}

	if rec := do(router, http.MethodGet, "/notification-channels/status", token(t, userB, schoolA, "guru"), nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("guru: want 403, got %d", rec.Code)
	}
	rec := do(router, http.MethodGet, "/notification-channels/status", token(t, userB, schoolA, roleAdmin), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct{ Data []provider.ChannelState }
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range resp.Data {
		got[s.Channel] = s.State
	}
	want := map[string]string{
		"email": provider.ChannelStateSimulated, "whatsapp": provider.ChannelStateUnconfigured,
		"push": provider.ChannelStateUnconfigured, "whatsapp_campaign": provider.ChannelStateUnconfigured,
	}
	for ch, st := range want {
		if got[ch] != st {
			t.Errorf("%s = %q, want %q (all: %v)", ch, got[ch], st, got)
		}
	}
}
