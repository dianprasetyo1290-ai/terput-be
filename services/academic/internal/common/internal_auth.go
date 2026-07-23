package common

import (
	"context"
	"net/http"
)

type internalCtxKey string

const internalCallCtxKey internalCtxKey = "internal_call"

// InternalOrJWTMiddleware lets a sibling microservice call a handful of
// read-only cross-service endpoints (see main.go) using the shared
// X-Internal-Key header instead of a user JWT. When the internal key
// matches, the request proceeds with no Claims in context, but a flag is
// set so handlers can detect this via IsInternal(r) and skip
// caller-scoped filtering (school_id, etc.) in favor of an explicit
// ?school_id= query param, or an unscoped-by-ID lookup. Falls through to
// normal JWT verification otherwise. See docs/CONVENTIONS.md.
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
