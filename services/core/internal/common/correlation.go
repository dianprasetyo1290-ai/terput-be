package common

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
)

// CorrelationHeader carries one cross-module flow id (BUG-117, TC-CC-013):
// bill -> transaction -> allocation -> journal share the same value.
const CorrelationHeader = "X-Correlation-ID"

type correlationKey struct{}

var correlationRe = regexp.MustCompile(`^[A-Za-z0-9._:\-]{1,128}$`)

// NewCorrelationID returns a random 32-hex id.
func NewCorrelationID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ValidCorrelationID accepts ids of safe characters, at most 128 long.
func ValidCorrelationID(s string) bool { return correlationRe.MatchString(s) }

// CorrelationMiddleware adopts the caller's X-Correlation-ID (falling back to
// X-Request-ID) or mints one, stores it in the context and echoes it in the
// response so the UI and logs can quote it.
func CorrelationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(CorrelationHeader)
		if !ValidCorrelationID(id) {
			id = r.Header.Get("X-Request-ID")
		}
		if !ValidCorrelationID(id) {
			id = NewCorrelationID()
		}
		w.Header().Set(CorrelationHeader, id)
		next.ServeHTTP(w, r.WithContext(WithCorrelationID(r.Context(), id)))
	})
}

// WithCorrelationID stores id in ctx.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, id)
}

// CorrelationID returns the id stored in ctx, or "".
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}

// CorrelationTransport stamps X-Correlation-ID from the request context on
// outbound calls that do not set one, so downstream services join the flow.
type CorrelationTransport struct{ Base http.RoundTripper }

func (t CorrelationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if id := CorrelationID(req.Context()); id != "" && req.Header.Get(CorrelationHeader) == "" {
		req = req.Clone(req.Context())
		req.Header.Set(CorrelationHeader, id)
	}
	return base.RoundTrip(req)
}
