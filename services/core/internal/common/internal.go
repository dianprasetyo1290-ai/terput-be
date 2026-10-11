package common

import (
	"context"
	"crypto/subtle"
	"net/http"
)

type internalCtxKey string

const internalKeyCtx internalCtxKey = "internal_call"

// InternalAuth protects the /internal/** group with the shared
// service-to-service key. It never accepts a browser JWT.
func InternalAuth(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Internal-Key")
			if key == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				WriteError(w, http.StatusUnauthorized, "invalid internal credentials")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), internalKeyCtx, true)))
		})
	}
}

// IsInternal reports whether the request was authenticated via X-Internal-Key.
func IsInternal(r *http.Request) bool {
	v, _ := r.Context().Value(internalKeyCtx).(bool)
	return v
}
