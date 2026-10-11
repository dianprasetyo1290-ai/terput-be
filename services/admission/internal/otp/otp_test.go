package otp

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func fixedService(now time.Time) (*Service, *time.Time) {
	clock := now
	svc := New("test-pepper", 5*time.Minute, time.Minute, 3)
	svc.Now = func() time.Time { return clock }
	return svc, &clock
}

func TestGenerateAndHash(t *testing.T) {
	svc, _ := fixedService(time.Now())
	code, err := svc.Generate()
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("code %q is not six digits", code)
	}
	// The same challenge+code always hashes identically; a different challenge or
	// code does not.
	if svc.Hash("ch1", "123456") != svc.Hash("ch1", "123456") {
		t.Fatal("hash must be deterministic")
	}
	if svc.Hash("ch1", "123456") == svc.Hash("ch2", "123456") {
		t.Fatal("hash must bind the challenge id")
	}
	if svc.Hash("ch1", "123456") == svc.Hash("ch1", "654321") {
		t.Fatal("hash must bind the code")
	}
	if len(svc.Hash("ch1", "123456")) != 64 {
		t.Fatal("hash should be a hex sha256")
	}
}

func TestVerifyHappyPath(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	svc, _ := fixedService(now)
	code := "424242"
	challenge := &Challenge{
		Hash:        svc.Hash("ch-1", code),
		ExpiresAt:   now.Add(5 * time.Minute),
		ResendAfter: now.Add(time.Minute),
		MaxAttempts: 3,
	}
	if err := svc.Verify(challenge, "ch-1", code); err != nil {
		t.Fatalf("valid code rejected: %v", err)
	}
	if challenge.UsedAt == nil {
		t.Fatal("successful verify must mark the challenge used")
	}
	if challenge.Attempts != 1 {
		t.Fatalf("attempts=%d want 1", challenge.Attempts)
	}
	// Single use: a second verify with the same code fails.
	if err := svc.Verify(challenge, "ch-1", code); !errors.Is(err, ErrUsed) {
		t.Fatalf("reused challenge err=%v want ErrUsed", err)
	}
}

func TestVerifyAttempts(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	svc, _ := fixedService(now)
	challenge := &Challenge{
		Hash:        svc.Hash("ch-2", "111111"),
		ExpiresAt:   now.Add(time.Minute),
		MaxAttempts: 3,
	}
	for i := 0; i < 3; i++ {
		if err := svc.Verify(challenge, "ch-2", "000000"); !errors.Is(err, ErrInvalid) {
			t.Fatalf("attempt %d err=%v want ErrInvalid", i, err)
		}
	}
	if challenge.Attempts != 3 {
		t.Fatalf("attempts=%d want 3", challenge.Attempts)
	}
	// Fourth attempt is blocked before any comparison.
	if err := svc.Verify(challenge, "ch-2", "111111"); !errors.Is(err, ErrAttempts) {
		t.Fatalf("over-attempt err=%v want ErrAttempts", err)
	}
}

func TestVerifyExpiry(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	svc, clock := fixedService(now)
	challenge := &Challenge{
		Hash:        svc.Hash("ch-3", "999999"),
		ExpiresAt:   now.Add(5 * time.Minute),
		MaxAttempts: 3,
	}
	*clock = now.Add(6 * time.Minute)
	if err := svc.Verify(challenge, "ch-3", "999999"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired err=%v want ErrExpired", err)
	}
	if challenge.UsedAt != nil {
		t.Fatal("expired code must not be marked used")
	}
}

func TestCooldown(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	challenge := &Challenge{ResendAfter: now.Add(time.Minute)}
	if challenge.CanResend(now) {
		t.Fatal("resend must be blocked during the cooldown")
	}
	if got := challenge.CooldownRemaining(now); got != time.Minute {
		t.Fatalf("CooldownRemaining=%v want 1m", got)
	}
	later := now.Add(59 * time.Second)
	if challenge.CanResend(later) {
		t.Fatal("resend must still be blocked one second early")
	}
	if !challenge.CanResend(now.Add(time.Minute)) {
		t.Fatal("resend must be allowed once the cooldown elapses")
	}
	if got := challenge.CooldownRemaining(now.Add(2 * time.Minute)); got != 0 {
		t.Fatalf("CooldownRemaining after window=%v want 0", got)
	}
}

func TestExpired(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	c := &Challenge{ExpiresAt: now.Add(time.Minute)}
	if c.Expired(now) {
		t.Fatal("must not be expired before the TTL")
	}
	if !c.Expired(now.Add(time.Minute)) {
		t.Fatal("must be expired at the TTL boundary")
	}
}

func TestDevProvider(t *testing.T) {
	var buf bytes.Buffer
	disabled := DevProvider{Writer: &buf, Enabled: false}
	if err := disabled.Send(context.Background(), "a@b.c", "123456"); err == nil {
		t.Fatal("disabled dev provider must return an error")
	}
	enabled := DevProvider{Writer: &buf, Enabled: true}
	if err := enabled.Send(context.Background(), "a@b.c", "123456"); err != nil {
		t.Fatalf("enabled dev provider error: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("123456")) {
		t.Fatal("dev provider must write the code")
	}
}
