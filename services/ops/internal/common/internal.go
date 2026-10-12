package common

import (
	"crypto/subtle"
	"net/http"
)

// InternalAuth guards service-to-service routes with X-Internal-Key. It never
// accepts a browser JWT.
func InternalAuth(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Internal-Key")
			if key == "" || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				WriteError(w, http.StatusUnauthorized, "invalid internal credentials")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
