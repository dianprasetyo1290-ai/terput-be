package security

import (
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)

func ptr(t time.Time) *time.Time { return &t }

func TestRegisterFailure(t *testing.T) {
	p := DefaultPolicy() // 5 attempts, 15 minutes
	cases := []struct {
		name       string
		count      int
		wantCount  int
		wantLocked bool
	}{
		{"first failure", 0, 1, false},
		{"fourth failure", 3, 4, false},
		{"fifth failure locks and resets", 4, 0, true},
		{"already above limit locks", 9, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RegisterFailure(c.count, p, now)
			if got.FailedCount != c.wantCount || (got.LockedUntil != nil) != c.wantLocked {
				t.Fatalf("got %+v", got)
			}
			if c.wantLocked && !got.LockedUntil.Equal(now.Add(15*time.Minute)) {
				t.Fatalf("locked until %v", got.LockedUntil)
			}
		})
	}
}

func TestIsLocked(t *testing.T) {
	cases := []struct {
		until *time.Time
		want  bool
	}{
		{nil, false},
		{ptr(now.Add(-time.Second)), false},
		{ptr(now), false},
		{ptr(now.Add(time.Minute)), true},
	}
	for i, c := range cases {
		if got := IsLocked(c.until, now); got != c.want {
			t.Errorf("case %d: got %v want %v", i, got, c.want)
		}
	}
}

func TestCheckSession(t *testing.T) {
	p := DefaultPolicy() // idle 30 minutes
	ok := SessionState{UserExists: true, UserActive: true, RoleHeld: true, HasSessionID: true, SessionFound: true,
		SessionExpiresAt: now.Add(time.Hour), SessionLastSeenAt: now.Add(-time.Minute)}
	legacy := SessionState{UserExists: true, UserActive: true, RoleHeld: true, IssuedAt: ptr(now.Add(-time.Hour))}
	mod := func(base SessionState, f func(*SessionState)) SessionState { f(&base); return base }

	cases := []struct {
		name string
		s    SessionState
		want string
	}{
		{"valid session", ok, ""},
		{"user deleted (BUG-022)", mod(ok, func(s *SessionState) { s.UserExists = false }), ReasonUserGone},
		{"user deactivated (BUG-022)", mod(ok, func(s *SessionState) { s.UserActive = false }), ReasonUserInactive},
		{"role removed (BUG-024)", mod(ok, func(s *SessionState) { s.RoleHeld = false }), ReasonRoleRevoked},
		{"session revoked (logout all)", mod(ok, func(s *SessionState) { s.SessionRevoked = true }), ReasonSessionRevoked},
		{"session row missing", mod(ok, func(s *SessionState) { s.SessionFound = false }), ReasonSessionRevoked},
		{"session expired", mod(ok, func(s *SessionState) { s.SessionExpiresAt = now }), ReasonSessionExpired},
		{"idle exactly 30 min still ok", mod(ok, func(s *SessionState) { s.SessionLastSeenAt = now.Add(-30 * time.Minute) }), ""},
		{"idle 31 min (BUG-025)", mod(ok, func(s *SessionState) { s.SessionLastSeenAt = now.Add(-31 * time.Minute) }), ReasonIdleTimeout},
		{"legacy token, never revoked", legacy, ""},
		{"legacy token issued before revoke-all", mod(legacy, func(s *SessionState) { s.TokensValidAfter = ptr(now.Add(-time.Minute)) }), ReasonTokenRevoked},
		{"legacy token issued after revoke-all", mod(legacy, func(s *SessionState) { s.TokensValidAfter = ptr(now.Add(-2 * time.Hour)) }), ""},
		{"legacy token without iat after revoke-all", mod(legacy, func(s *SessionState) { s.IssuedAt = nil; s.TokensValidAfter = ptr(now) }), ReasonTokenRevoked},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckSession(c.s, p, now); got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestRequiresMFA(t *testing.T) {
	mfa := []string{"superadmin", "finance_manager"}
	cases := []struct {
		held []string
		want bool
	}{
		{[]string{"guru"}, false},
		{[]string{"guru", "finance_manager"}, true},
		{[]string{"superadmin"}, true},
		{nil, false},
	}
	for i, c := range cases {
		if got := RequiresMFA(c.held, mfa); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
	if RequiresMFA([]string{"superadmin"}, nil) {
		t.Error("empty policy must not require MFA")
	}
}

func TestConflictsFor(t *testing.T) {
	conflicts := []RoleConflict{{RoleA: "admin", RoleB: "auditor"}, {RoleA: "finance_manager", RoleB: "payroll_officer"}}
	cases := []struct {
		name string
		held []string
		add  string
		want int
	}{
		{"no conflict", []string{"guru"}, "admin", 0},
		{"auditor onto admin", []string{"admin", "guru"}, "auditor", 1},
		{"admin onto auditor (order independent)", []string{"auditor"}, "admin", 1},
		{"same role again", []string{"auditor"}, "auditor", 0},
		{"payroll onto finance", []string{"finance_manager"}, "payroll_officer", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ConflictsFor(c.held, c.add, conflicts); len(got) != c.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestTOTPKnownVector(t *testing.T) {
	// RFC 6238 appendix B, SHA-1 secret "12345678901234567890", T=59 -> 94287082 (8 digits) -> 287082.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	code, err := TOTPCode(secret, time.Unix(59, 0))
	if err != nil || code != "287082" {
		t.Fatalf("got %q %v", code, err)
	}
	code, _ = TOTPCode(secret, time.Unix(1111111109, 0))
	if code != "081804" {
		t.Fatalf("got %q", code)
	}
}

func TestVerifyTOTP(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := TOTPCode(secret, now)
	prev, _ := TOTPCode(secret, now.Add(-30*time.Second))
	old, _ := TOTPCode(secret, now.Add(-90*time.Second))
	cases := []struct {
		code string
		want bool
	}{
		{cur, true},
		{prev, true},
		{old, old == cur || old == prev},
		{"12345", false},
		{"", false},
	}
	for i, c := range cases {
		if got := VerifyTOTP(secret, c.code, now); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

func TestEncryptRoundTrip(t *testing.T) {
	key := DeriveKey("k1")
	enc, err := Encrypt(key, "JBSWY3DPEHPK3PXP")
	if err != nil || strings.Contains(enc, "JBSWY3DP") {
		t.Fatalf("enc %q %v", enc, err)
	}
	if pt, err := Decrypt(key, enc); err != nil || pt != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("pt %q %v", pt, err)
	}
	if _, err := Decrypt(DeriveKey("k2"), enc); err == nil {
		t.Fatal("wrong key must fail")
	}
}

func TestOpaqueToken(t *testing.T) {
	tok, hash, err := NewOpaqueToken()
	if err != nil || len(tok) < 40 || hash != HashToken(tok) || hash == tok {
		t.Fatalf("tok %q hash %q %v", tok, hash, err)
	}
}

func TestValidPassword(t *testing.T) {
	cases := map[string]bool{"abc12": false, "abcdefgh": false, "12345678": false, "rahasia123": true}
	for p, ok := range cases {
		if got := ValidPassword(p) == ""; got != ok {
			t.Errorf("%q: got %v", p, got)
		}
	}
}

func TestValidatePolicy(t *testing.T) {
	valid := func(r string) bool { return r != "nope" }
	if errs := ValidatePolicy(DefaultPolicy(), valid); len(errs) != 0 {
		t.Fatalf("default policy invalid: %v", errs)
	}
	bad := DefaultPolicy()
	bad.MaxFailedLogins = 0
	bad.IdleTimeoutMinutes = 1
	bad.MFARoles = []string{"nope"}
	errs := ValidatePolicy(bad, valid)
	for _, f := range []string{"max_failed_logins", "idle_timeout_minutes", "mfa_roles"} {
		if errs[f] == "" {
			t.Errorf("expected error on %s", f)
		}
	}
}
