package common

import (
	"context"
	"net/http"
)

type internalCtxKey string

const internalCallCtxKey internalCtxKey = "internal_call"

// InternalOrJWTMiddleware lets a sibling microservice (report-service, for
// GET /exams/{id}/scores) call using the shared X-Internal-Key header
// instead of a user JWT. When the internal key matches, the request
// proceeds with no Claims in context, but a flag is set so handlers can
// detect this via IsInternal(r) and skip caller-scoped filtering in favor
// of an explicit ?school_id= or an unscoped-by-ID lookup. Falls through to
// normal JWT verification otherwise. See docs/CONVENTIONS.md.
//
// Added 2026-07-18: this service never had an internal-auth path at all
// (only ever sent X-Internal-Key when calling identity/academic, never
// verified it on its own routes) — found via a systematic endpoint sweep
// hitting GET /exams/{id}/scores with X-Internal-Key and getting 401,
// which would have broken report-service's real integration.
func InternalOrJWTMiddleware(jwtSecret, internalKey string) func(http.Handler) http.Handler {
	jwtMW := JWTMiddleware(jwtSecret)
	return func(next http.Handler) http.Handler {
		jwtWrapped := jwtMW(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key := r.Header.Get("X-Internal-Key"); key != "" && key == internalKey {
				ctx := context.WithValue(r.Context(), internalCallCtxKey, true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			jwtWrapped.ServeHTTP(w, r)
		})
	}
}

// IsInternal reports whether this request was authenticated via the
// service-to-service X-Internal-Key rather than a user JWT.
func IsInternal(r *http.Request) bool {
	v, _ := r.Context().Value(internalCallCtxKey).(bool)
	return v
}

// InternalOnlyMiddleware protects mutation/result contracts that must never
// fall through to a user JWT.
func InternalOnlyMiddleware(internalKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if internalKey == "" || r.Header.Get("X-Internal-Key") != internalKey {
				WriteError(w, http.StatusUnauthorized, "invalid internal key")
				return
			}
			ctx := context.WithValue(r.Context(), internalCallCtxKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
