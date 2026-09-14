package cache

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A bumped school version must change the cache key for an otherwise
// identical request -- that is what makes a write (PUT /formative)
// immediately visible to the next read instead of up to ttl later
// (SEKOCI V8: "nilai yang sudah diinput HILANG").
func TestCacheKeyChangesWithVersion(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/formative?term_id=t1&assessment_type_id=a1", nil)
	if cacheKey(r, "0") == cacheKey(r, "1") {
		t.Fatal("cache key ignored the school version -- a write would not invalidate cached GETs")
	}
	if cacheKey(r, "7") != cacheKey(r, "7") {
		t.Fatal("cache key is not stable for the same version")
	}
}

func TestIsMutation(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !isMutation(m) {
			t.Errorf("%s must bump the cache version", m)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodOptions, http.MethodHead} {
		if isMutation(m) {
			t.Errorf("%s must not bump the cache version", m)
		}
	}
}
