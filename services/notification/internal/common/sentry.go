package common

import (
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

// InitSentry wires up error tracking when SENTRY_DSN is set; a no-op when it
// is empty (sentry.Init with an empty Dsn disables the SDK, it does not error),
// so every environment behaves identically until a real DSN is configured.
func InitSentry(dsn, environment, release string) error {
	return sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      environment,
		Release:          release,
		TracesSampleRate: 0.1,
	})
}

// FlushSentry blocks briefly to let pending events send before exit.
func FlushSentry() {
	sentry.Flush(2 * time.Second)
}

// SentryMiddleware captures panics (reporting them, then re-panicking) — mount
// it before chi's middleware.Recoverer so Recoverer still turns the panic into
// a 500 but Sentry sees it first.
func SentryMiddleware() func(http.Handler) http.Handler {
	h := sentryhttp.New(sentryhttp.Options{Repanic: true, Timeout: 3 * time.Second})
	return h.Handle
}

// CaptureError reports a handled (non-panic) error to Sentry.
func CaptureError(err error) {
	if err != nil {
		sentry.CaptureException(err)
	}
}
