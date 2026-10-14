package config

import "testing"

// QA-PAY BUG-004: the payment simulator needs an explicit flag plus a
// webhook secret, and is refused in production.
func TestSimulatorGuards(t *testing.T) {
	cases := []struct {
		name, appEnv, enabled, secret string
		wantErr, wantAllowed          bool
	}{
		{"off by default", "development", "", "", false, false},
		{"enabled without secret", "development", "true", "", true, false},
		{"enabled with short secret", "development", "true", "short", true, false},
		{"enabled with secret", "development", "true", "sim-webhook-secret-0001", false, true},
		{"production refuses simulator", "production", "true", "sim-webhook-secret-0001", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("APP_ENV", c.appEnv)
			t.Setenv("PAYMENT_SIMULATOR_ENABLED", c.enabled)
			t.Setenv("PAYMENT_SIM_WEBHOOK_SECRET", c.secret)
			t.Setenv("SEKOCI_JWT_SECRET", "prod-jwt-secret-value-xyz")
			t.Setenv("SEKOCI_INTERNAL_KEY", "prod-internal-key-value-xyz")
			t.Setenv("IDENTITY_URL", "http://identity:8081")
			for _, k := range []string{"NICEPAY_BASE_URL", "NICEPAY_CLIENT_ID", "NICEPAY_CHANNEL_ID", "NICEPAY_CLIENT_SECRET",
				"NICEPAY_PRIVATE_KEY", "NICEPAY_PUBLIC_KEY", "NICEPAY_CALLBACK_URL", "NICEPAY_RETURN_URL"} {
				t.Setenv(k, "")
			}
			cfg, err := Load()
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if err == nil && cfg.SimulatorAllowed() != c.wantAllowed {
				t.Fatalf("SimulatorAllowed=%v want %v", cfg.SimulatorAllowed(), c.wantAllowed)
			}
		})
	}
}
