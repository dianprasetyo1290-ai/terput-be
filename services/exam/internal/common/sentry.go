package common

import (
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

// InitSentry wires up error tracking if SENTRY_DSN is set; a no-op when
// it's empty (sentry.Init with an empty Dsn is documented as disabling the
// SDK, not erroring), so every environment works identically until a real
// DSN is configured — no separate on/off flag needed. See
// docs/CONVENTIONS.md "Error tracking".
func InitSentry(dsn, environment, release string) error {
	return sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      environment,
		Release:          release,
		TracesSampleRate: 0.1,
	})
}

// FlushSentry blocks briefly to let any pending events send before the
// process exits — call via `defer` right after InitSentry in main().
func FlushSentry() {
	sentry.Flush(2 * time.Second)
}

// SentryMiddleware captures panics (reporting them to Sentry, then
// re-panicking) — mount this BEFORE chi's middleware.Recoverer so
// Recoverer still gets the panic and turns it into a 500, but Sentry sees
// it first.
func SentryMiddleware() func(http.Handler) http.Handler {
	h := sentryhttp.New(sentryhttp.Options{Repanic: true, Timeout: 3 * time.Second})
	return h.Handle
}

// CaptureError reports a handled (non-panic) error to Sentry — use this in
// handlers at the same place you'd log an unexpected 500, not for expected
// 4xx validation failures.
func CaptureError(err error) {
	if err != nil {
		sentry.CaptureException(err)
	}
}
