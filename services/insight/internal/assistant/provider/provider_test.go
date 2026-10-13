package provider

import (
	"context"
	"testing"
)

func TestProviderLifecycle(t *testing.T) {
	// 1. Unconfigured provider (default)
	pUnconf, err := New("unconfigured", false)
	if err != nil {
		t.Fatalf("unexpected error creating unconfigured provider: %v", err)
	}
	_, err = pUnconf.GenerateText(context.Background(), "test prompt")
	if err == nil {
		t.Errorf("unconfigured provider must reject calls with an error")
	}

	// 2. Dev provider in non-prod
	pDev, err := New("dev", false)
	if err != nil {
		t.Fatalf("unexpected error creating dev provider: %v", err)
	}
	resp, err := pDev.GenerateText(context.Background(), "halo")
	if err != nil || resp == "" {
		t.Errorf("dev provider failed to generate deterministic text: %v", err)
	}

	// 3. Dev provider forbidden in production
	_, err = New("dev", true)
	if err == nil {
		t.Errorf("dev provider must be forbidden in production")
	}

	// 4. Unknown provider -> error
	_, err = New("unknown-vendor-xyz", false)
	if err == nil {
		t.Errorf("unknown provider name must return error")
	}
}
