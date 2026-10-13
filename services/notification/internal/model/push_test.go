package model

import "testing"

func TestShouldPush(t *testing.T) {
	// Case 1: no active subscription -> false
	if ShouldPush(nil, false) {
		t.Errorf("expected false when no active subscription")
	}

	// Case 2: active subscription + no preference row -> true (default on per T26 §3)
	if !ShouldPush(nil, true) {
		t.Errorf("expected true when subscription exists and no preference row")
	}

	// Case 3: active subscription + preference row disabled -> false (CC-08)
	disabledPref := &NotificationPreference{
		Channel: ChannelPush,
		Enabled: false,
	}
	if ShouldPush(disabledPref, true) {
		t.Errorf("expected false when preference explicitly disabled")
	}

	// Case 4: active subscription + preference row enabled -> true
	enabledPref := &NotificationPreference{
		Channel: ChannelPush,
		Enabled: true,
	}
	if !ShouldPush(enabledPref, true) {
		t.Errorf("expected true when preference explicitly enabled")
	}
}

func TestIsGoneStatus(t *testing.T) {
	if !IsGoneStatus(404) {
		t.Errorf("expected 404 to be gone")
	}
	if !IsGoneStatus(410) {
		t.Errorf("expected 410 to be gone")
	}
	if IsGoneStatus(200) || IsGoneStatus(500) || IsGoneStatus(400) {
		t.Errorf("200/500/400 should not be gone")
	}
}
