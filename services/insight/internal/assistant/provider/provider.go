package provider

import (
	"context"
	"fmt"
	"strings"
)

type Provider interface {
	Name() string
	GenerateText(ctx context.Context, prompt string) (string, error)
}

// DevProvider provides deterministic templates for development and testing
type DevProvider struct{}

func (DevProvider) Name() string { return "dev" }

func (DevProvider) GenerateText(_ context.Context, prompt string) (string, error) {
	return fmt.Sprintf("[Dev AI Response] Menjawab permintaan: %s", prompt), nil
}

// UnconfiguredProvider is the default: refuses every prompt generation
type UnconfiguredProvider struct{}

func (UnconfiguredProvider) Name() string { return "unconfigured" }

func (UnconfiguredProvider) GenerateText(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("Penyedia AI belum dikonfigurasi: set AI_PROVIDER dan kredensial penyedia")
}

// New selects the AI adapter.
// - "dev": development only (error in production)
// - "unconfigured" or "": default, returns 503 error on calls
// - unknown: startup error
func New(name string, production bool) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dev":
		if production {
			return nil, fmt.Errorf("AI_PROVIDER=dev is forbidden in production")
		}
		return DevProvider{}, nil
	case "", "unconfigured":
		return UnconfiguredProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown AI_PROVIDER %q (want dev|unconfigured)", name)
	}
}
