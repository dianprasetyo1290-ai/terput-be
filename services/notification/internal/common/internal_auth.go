package common

import (
	"context"
	"crypto/subtle"
	"net/http"
)

type internalCtxKey struct{}

// InternalOnlyMiddleware guards the service-to-service routes (internal send,
// template CRUD, delivery retry). It compares X-Internal-Key in constant time
// and marks the context so handlers can tell an internal call apart from a
// user one.
func InternalOnlyMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Internal-Key")
			if got == "" || key == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				WriteError(w, http.StatusUnauthorized, "invalid internal credentials")
				return
			}
			ctx := context.WithValue(r.Context(), internalCtxKey{}, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsInternal reports whether the request passed InternalOnlyMiddleware.
func IsInternal(r *http.Request) bool {
	v, _ := r.Context().Value(internalCtxKey{}).(bool)
	return v
}
