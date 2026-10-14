package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"skci/admission/internal/common"
)

// param reads a chi path parameter.
func param(r *http.Request, key string) string { return chi.URLParam(r, key) }

// ipHash stores a stable, non-reversible fingerprint of the requester's IP for
// OTP rate limiting.
func ipHash(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	sum := sha256.Sum256([]byte(host))
	return hex.EncodeToString(sum[:])
}

// normalizeContact validates and canonicalizes an email or phone number.
func normalizeContact(kind, value string) (string, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	switch kind {
	case "email":
		return value, strings.Contains(value, "@") && len(value) <= 254
	case "phone":
		digits := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "", "+", "").Replace(value)
		return digits, len(digits) >= 9 && len(digits) <= 16
	default:
		return "", false
	}
}

// channelKind maps the frontend's delivery channel to a stored contact kind.
func channelKind(channel, contact string) string {
	switch strings.ToLower(strings.TrimSpace(channel)) {
	case "whatsapp", "phone", "sms":
		return "phone"
	case "email":
		return "email"
	}
	if strings.Contains(contact, "@") {
		return "email"
	}
	return "phone"
}

// guardianToken issues the admission guardian session JWT.
func (a *App) guardianToken(guardianID, schoolID string) (string, time.Time, error) {
	now := time.Now()
	expires := now.Add(a.Config.GuardianTTL)
	claims := common.GuardianClaims{
		GuardianID: guardianID,
		SchoolID:   schoolID,
		Scope:      common.GuardianScope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "admission",
			Audience:  jwt.ClaimStrings{"admission-guardian"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(a.Config.GuardianJWTSecret))
	return token, expires, err
}

// parseTime accepts the shapes the SPA sends: RFC3339, datetime-local
// ("2006-01-02T15:04"), and a plain date.
func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid timestamp")
}

// nullTime turns a zero time into a NULL-friendly value.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// nullString turns an empty string into a NULL-friendly value.
func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// parsePositive parses a positive integer amount, rejecting signs/junk.
func parsePositive(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// randomKey returns a hex object key segment.
func randomKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isUniqueViolation reports whether an error is a Postgres unique conflict,
// which the handlers translate into a 409.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// jsonRaw marshals a value for a jsonb column, returning nil for nil input.
func jsonRaw(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// audit writes one append-only audit event.
func (a *App) audit(ctx context.Context, schoolID, aggregateType, aggregateID, action, actorType, actorID, actorName, reason string, before, after any) {
	_, _ = a.DB.ExecContext(ctx,
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,before_data,after_data,reason)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''))`,
		schoolID, aggregateType, aggregateID, action, actorType, actorID, actorName, jsonRaw(before), jsonRaw(after), reason)
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
