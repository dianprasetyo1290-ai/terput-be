package provider

import (
	"context"
	"strings"
	"testing"
)

func TestNewRejectsDevInProduction(t *testing.T) {
	if _, err := New("dev", true); err == nil {
		t.Fatal("New(\"dev\", production=true) succeeded; a no-op provider must never be selectable in production")
	}
	if _, err := New("dev", false); err != nil {
		t.Fatalf("New(\"dev\", development): unexpected error %v", err)
	}
}

func TestUnconfiguredProviderRefusesSend(t *testing.T) {
	p, err := New("", true)
	if err != nil {
		t.Fatalf("New(\"\", production): %v", err)
	}
	if p.Name() != "unconfigured" {
		t.Fatalf("production default provider = %q, want \"unconfigured\"", p.Name())
	}
	if _, err := p.Send(context.Background(), Message{Channel: "email", Recipient: "a@b.c"}); err == nil {
		t.Fatal("the unconfigured provider accepted a send; it must refuse loudly")
	} else if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("refusal error = %q, want it to mention the missing configuration", err.Error())
	}
}

func TestNoopProviderSendsInDevelopment(t *testing.T) {
	p, err := New("dev", false)
	if err != nil {
		t.Fatalf("New(\"dev\", development): %v", err)
	}
	id, err := p.Send(context.Background(), Message{Channel: "whatsapp", Recipient: "08123456789", Body: "hi"})
	if err != nil {
		t.Fatalf("no-op send: %v", err)
	}
	if id == "" {
		t.Error("no-op send returned an empty provider message id")
	}
}

func TestUnknownProviderRejected(t *testing.T) {
	if _, err := New("smtp", false); err == nil {
		t.Fatal("an unknown provider name must be rejected, not silently ignored")
	}
}

func TestMaskRecipientHidesAddress(t *testing.T) {
	if got := maskRecipient("wali@example.com"); strings.Contains(got, "wali") {
		t.Errorf("maskRecipient leaked the address: %q", got)
	}
	if maskRecipient("") != "***" {
		t.Errorf("maskRecipient(\"\") = %q, want \"***\"", maskRecipient(""))
	}
}
