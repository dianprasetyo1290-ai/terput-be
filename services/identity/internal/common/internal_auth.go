package common

import (
	"context"
	"crypto/subtle"
	"net/http"
)

type internalCtxKey string

const internalCallCtxKey internalCtxKey = "internal_call"

// InternalOrJWTMiddleware lets a sibling microservice call a handful of
// read-only cross-service endpoints (see main.go) using the shared
// X-Internal-Key header instead of a user JWT. When the internal key
// matches, the request proceeds with no Claims in context, but a flag is
// set so handlers can detect this via IsInternal(r) and skip
// caller-scoped authorization in favor of an unscoped-by-ID lookup. Falls
// through to normal JWT verification otherwise. See docs/CONVENTIONS.md.
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

// InternalOnlyMiddleware protects mutation endpoints that must never accept a
// browser JWT. Constant-time password comparison avoids leaking key prefixes.
func InternalOnlyMiddleware(internalKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Internal-Key")
			if internalKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(internalKey)) != 1 {
				WriteError(w, http.StatusUnauthorized, "invalid internal key")
				return
			}
			ctx := context.WithValue(r.Context(), internalCallCtxKey, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
