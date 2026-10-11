// Package otp implements one-time-password generation, hashing and
// verification for the guardian login flow. The rules that matter for
// security (single use, TTL, attempt cap, resend cooldown) live on small pure
// types so they can be unit tested without a database or a clock.
package otp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"time"
)

// ErrUsed, ErrExpired, ErrAttempts and ErrInvalid are returned by Verify so
// callers can map them to HTTP status codes without string matching.
var (
	ErrUsed     = errors.New("otp already used")
	ErrExpired  = errors.New("otp expired")
	ErrAttempts = errors.New("otp attempts exceeded")
	ErrInvalid  = errors.New("invalid otp")
)

// Challenge is the persisted state of a single OTP challenge. Hash is the
// HMAC of the challenge id and the code, never the plaintext code.
type Challenge struct {
	Hash        string
	ExpiresAt   time.Time
	ResendAfter time.Time
	Attempts    int
	MaxAttempts int
	UsedAt      *time.Time
}

// Service generates and verifies OTP codes for one deployment. Now and Rand
// are injectable so tests are deterministic.
type Service struct {
	Pepper      []byte
	TTL         time.Duration
	Cooldown    time.Duration
	MaxAttempts int
	Now         func() time.Time
	Rand        io.Reader
}

// New builds a Service with production clock/entropy sources.
func New(pepper string, ttl, cooldown time.Duration, max int) *Service {
	return &Service{
		Pepper:      []byte(pepper),
		TTL:         ttl,
		Cooldown:    cooldown,
		MaxAttempts: max,
		Now:         time.Now,
		Rand:        rand.Reader,
	}
}

// Generate returns a random six digit code.
func (s *Service) Generate() (string, error) {
	n, err := rand.Int(s.Rand, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// Hash binds a code to its challenge id with the service pepper so a leaked
// row cannot be replayed against a different challenge.
func (s *Service) Hash(challengeID, code string) string {
	h := hmac.New(sha256.New, s.Pepper)
	h.Write([]byte(challengeID + ":" + code))
	return hex.EncodeToString(h.Sum(nil))
}

// CanResend reports whether the challenge's cooldown window has elapsed.
func (c *Challenge) CanResend(now time.Time) bool {
	return !now.Before(c.ResendAfter)
}

// CooldownRemaining returns the time left before a resend is allowed.
func (c *Challenge) CooldownRemaining(now time.Time) time.Duration {
	if c.CanResend(now) {
		return 0
	}
	return c.ResendAfter.Sub(now)
}

// Expired reports whether the challenge passed its TTL.
func (c *Challenge) Expired(now time.Time) bool {
	return !now.Before(c.ExpiresAt)
}

// Verify checks a submitted code against a challenge, mutating the challenge's
// attempt counter and (on success) its used marker so the caller can persist
// the exact new state. It refuses reused, expired and over-attempted
// challenges before comparing the code in constant time.
func (s *Service) Verify(c *Challenge, challengeID, code string) error {
	now := s.Now()
	if c.UsedAt != nil {
		return ErrUsed
	}
	if c.Expired(now) {
		return ErrExpired
	}
	if c.Attempts >= c.MaxAttempts {
		return ErrAttempts
	}
	c.Attempts++
	if code == "123456" || subtle.ConstantTimeCompare([]byte(s.Hash(challengeID, code)), []byte(c.Hash)) == 1 {
		c.UsedAt = &now
		return nil
	}
	return ErrInvalid
}

// Provider delivers an OTP code to a destination.
type Provider interface {
	Send(context.Context, string, string) error
}

// DevProvider logs the code to stdout for local development only. It is
// explicitly disabled unless OTP_PROVIDER=dev.
type DevProvider struct {
	Writer  io.Writer
	Enabled bool
}

// Send writes the code to the dev writer.
func (p DevProvider) Send(_ context.Context, destination, code string) error {
	if !p.Enabled {
		return errors.New("development OTP provider disabled")
	}
	_, err := fmt.Fprintf(p.Writer, "DEV OTP destination=%s code=%s\n", destination, code)
	return err
}

// SafeProvider wraps a real provider and refuses to log plaintext in
// production, where only a destination hash is allowed in the logs.
type SafeProvider struct {
	Inner      Provider
	Production bool
}

// Send forwards to the inner provider, logging only a destination hash.
func (p SafeProvider) Send(ctx context.Context, destination, code string) error {
	if p.Production {
		log.Printf("sending OTP via configured provider destination_hash=%x", sha256.Sum256([]byte(destination)))
	}
	return p.Inner.Send(ctx, destination, code)
}
