package config

import "testing"

func TestDevelopmentDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SEKOCI_JWT_SECRET", "")
	t.Setenv("SEKOCI_INTERNAL_KEY", "")
	t.Setenv("NOTIFICATION_PROVIDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load (development): %v", err)
	}
	if cfg.Port != "8089" {
		t.Errorf("default port = %q, want 8089", cfg.Port)
	}
	if cfg.Production() {
		t.Error("development config reports Production() = true")
	}
	if cfg.DeliveryProvider != "dev" {
		t.Errorf("default delivery provider = %q, want dev", cfg.DeliveryProvider)
	}
	if cfg.JWTSecret == "" || cfg.InternalKey == "" {
		t.Error("development defaults did not supply dev secrets")
	}
}

func TestProductionRejectsDevDefaults(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "missing jwt secret",
			env:  map[string]string{"SEKOCI_INTERNAL_KEY": "prod-internal-key-1234"},
		},
		{
			name: "dev jwt secret",
			env: map[string]string{
				"SEKOCI_JWT_SECRET":   "teratai-dev-jwt-secret-change-me",
				"SEKOCI_INTERNAL_KEY": "prod-internal-key-1234",
			},
		},
		{
			name: "missing internal key",
			env:  map[string]string{"SEKOCI_JWT_SECRET": "prod-jwt-secret-1234"},
		},
		{
			name: "dev provider in production",
			env: map[string]string{
				"SEKOCI_JWT_SECRET":     "prod-jwt-secret-1234",
				"SEKOCI_INTERNAL_KEY":   "prod-internal-key-1234",
				"NOTIFICATION_PROVIDER": "dev",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("SEKOCI_JWT_SECRET", "")
			t.Setenv("SEKOCI_INTERNAL_KEY", "")
			t.Setenv("NOTIFICATION_PROVIDER", "")
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Fatal("Load succeeded with a production dev default; it must fail fast")
			}
		})
	}
}

func TestProductionAcceptsExplicitConfig(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SEKOCI_JWT_SECRET", "prod-jwt-secret-1234")
	t.Setenv("SEKOCI_INTERNAL_KEY", "prod-internal-key-1234")
	t.Setenv("NOTIFICATION_PROVIDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load (production, explicit secrets): %v", err)
	}
	if !cfg.Production() {
		t.Error("Production() = false for APP_ENV=production")
	}
	if cfg.DeliveryProvider != "unconfigured" {
		t.Errorf("production default provider = %q, want unconfigured (refuses sends until wired)", cfg.DeliveryProvider)
	}
}
