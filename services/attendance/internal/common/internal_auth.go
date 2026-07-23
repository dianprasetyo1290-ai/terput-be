package common

import "net/http"

// InternalOrJWTMiddleware wraps JWTMiddleware for the one route
// (GET /students/{id}/attendance-summary) that CONVENTIONS.md's cross-service
// section says report-service calls directly over internal HTTP using the
// shared X-Internal-Key header, while the rest of this service's routes are
// only ever hit by an end-user's browser/app with a real JWT. If the internal
// key header matches, the request proceeds without JWT claims in context
// (r.Context() will have no Claims — handlers on this route must not assume
// claims are present when called this way). Otherwise it falls through to
// normal JWT verification. See README "Assumptions" for the reasoning.
func InternalOrJWTMiddleware(jwtSecret, internalKey string) func(http.Handler) http.Handler {
	jwtMW := JWTMiddleware(jwtSecret)
	return func(next http.Handler) http.Handler {
		jwtWrapped := jwtMW(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key := r.Header.Get("X-Internal-Key"); key != "" && key == internalKey {
				next.ServeHTTP(w, r)
				return
			}
			jwtWrapped.ServeHTTP(w, r)
		})
	}
}
