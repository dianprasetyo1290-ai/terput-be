package config

import (
	"strings"
	"testing"
	"time"
)

// TestLoadParticipantSecret pins item 1 of the admission-CBT work: the key
// that signs applicant participant tokens must come from config (never an empty
// string) and must never be the user-JWT secret. Before this, main.go sourced
// it from PARTICIPANT_TOKEN_SECRET with a fallback to SEKOCI_JWT_SECRET, i.e.
// the very key the applicant routes are supposed to be isolated from.
func TestLoadParticipantSecret(t *testing.T) {
	t.Run("falls back to the committed dev default, not the JWT secret", func(t *testing.T) {
		t.Setenv("EXAM_PARTICIPANT_SECRET", "")
		t.Setenv("PARTICIPANT_TOKEN_SECRET", "")
		t.Setenv("SEKOCI_JWT_SECRET", "change-me")
		c := Load()
		if c.ParticipantSecret == "" {
			t.Fatal("ParticipantSecret is empty: participant tokens would be signed with a zero-length HMAC key")
		}
		if c.ParticipantSecret != DefaultDevParticipantSecret {
			t.Errorf("ParticipantSecret = %q, want the dev default %q", c.ParticipantSecret, DefaultDevParticipantSecret)
		}
		if c.ParticipantSecret == c.JWTSecret {
			t.Error("ParticipantSecret must not reuse the user JWT secret: applicant tokens would then be interchangeable with user tokens")
		}
	})

	t.Run("EXAM_PARTICIPANT_SECRET wins", func(t *testing.T) {
		t.Setenv("EXAM_PARTICIPANT_SECRET", "prod-participant-secret-abc")
		t.Setenv("PARTICIPANT_TOKEN_SECRET", "legacy-participant-secret")
		if got := Load().ParticipantSecret; got != "prod-participant-secret-abc" {
			t.Errorf("ParticipantSecret = %q, want EXAM_PARTICIPANT_SECRET's value", got)
		}
	})

	t.Run("legacy PARTICIPANT_TOKEN_SECRET is still honoured", func(t *testing.T) {
		t.Setenv("EXAM_PARTICIPANT_SECRET", "")
		t.Setenv("PARTICIPANT_TOKEN_SECRET", "legacy-participant-secret")
		if got := Load().ParticipantSecret; got != "legacy-participant-secret" {
			t.Errorf("ParticipantSecret = %q, want the legacy PARTICIPANT_TOKEN_SECRET value", got)
		}
	})

	t.Run("JWT secret alone never becomes the participant secret", func(t *testing.T) {
		t.Setenv("EXAM_PARTICIPANT_SECRET", "")
		t.Setenv("PARTICIPANT_TOKEN_SECRET", "")
		t.Setenv("SEKOCI_JWT_SECRET", "only-the-jwt-secret-is-set")
		if got := Load().ParticipantSecret; got == "only-the-jwt-secret-is-set" {
			t.Error("fell back to SEKOCI_JWT_SECRET; the plain JWT fallback was removed on purpose")
		}
	})
}

// TestValidate is the production guard: an empty key is fatal everywhere, and
// the committed dev default (or any other "dev-" value from .env.example) is
// refused once the process is running in production.
func TestValidate(t *testing.T) {
	cases := []struct {
		name      string
		appEnv    string
		sentryEnv string
		secret    string
		wantErr   bool
		errSubstr string
	}{
		{name: "dev default is fine in development", appEnv: "development", secret: DefaultDevParticipantSecret},
		{name: "dev default is refused in production", appEnv: "production", secret: DefaultDevParticipantSecret, wantErr: true, errSubstr: "EXAM_PARTICIPANT_SECRET"},
		{name: "APP_ENV is case-insensitive", appEnv: "Production", secret: DefaultDevParticipantSecret, wantErr: true, errSubstr: "EXAM_PARTICIPANT_SECRET"},
		{name: "any dev- value is refused in production", appEnv: "production", secret: "secret-dev-rotate-me", wantErr: true, errSubstr: "production"},
		{
			// The shared root docker-compose.yml hands this service
			// SENTRY_ENVIRONMENT but not APP_ENV, so the guard has to fire on
			// that signal too or it would never apply to the real deployment.
			name: "SENTRY_ENVIRONMENT=production also counts as production", sentryEnv: "production",
			secret: DefaultDevParticipantSecret, wantErr: true, errSubstr: "EXAM_PARTICIPANT_SECRET",
		},
		{name: "explicit production secret passes", appEnv: "production", sentryEnv: "production", secret: "a-real-64-char-production-secret"},
		{name: "empty secret is always refused", appEnv: "development", secret: "", wantErr: true, errSubstr: "must not be empty"},
		{name: "whitespace-only secret is refused", appEnv: "development", secret: "   ", wantErr: true, errSubstr: "must not be empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := Config{AppEnv: c.appEnv, SentryEnvironment: c.sentryEnv, ParticipantSecret: c.secret}
			err := cfg.Validate()
			if c.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want an error (secret %q, app_env %q)", c.secret, c.appEnv)
				}
				if c.errSubstr != "" && !strings.Contains(err.Error(), c.errSubstr) {
					t.Errorf("Validate() = %q, want it to mention %q", err, c.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// TestParticipantTTL makes the now-wired PARTICIPANT_TOKEN_TTL_SECONDS env var
// safe: the default is 15 minutes in production terms, and the value is clamped
// to the same 3600s ceiling the per-request handler applies.
func TestParticipantTTL(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
	}{
		{"", 15 * time.Minute},
		{"0", 15 * time.Minute},
		{"-30", 15 * time.Minute},
		{"garbage", 15 * time.Minute},
		{"60", time.Minute},
		{"7200", 3600 * time.Second},
	}
	for _, c := range cases {
		t.Run("raw="+c.raw, func(t *testing.T) {
			t.Setenv("PARTICIPANT_TOKEN_TTL_SECONDS", c.raw)
			if got := Load().ParticipantTokenTTL; got != c.want {
				t.Errorf("ParticipantTokenTTL = %v, want %v", got, c.want)
			}
		})
	}
}
