package cache

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The cache key must change when the school's version counter changes --
// that is the whole invalidation mechanism. If these two ever collide, a
// write stops being visible to the next read for up to the TTL, which is
// the "dashboard says 3, bank soal shows 2" bug.
func TestCacheKeyVariesWithVersion(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/exams?folder_id=abc", nil)
	if cacheKey(r, "0") == cacheKey(r, "1") {
		t.Fatal("cache key ignores the version counter; writes stay invisible for a full TTL")
	}
	if cacheKey(r, "7") != cacheKey(r, "7") {
		t.Fatal("cache key is not stable for the same version")
	}
	other := httptest.NewRequest(http.MethodGet, "/exams?folder_id=xyz", nil)
	if cacheKey(r, "0") == cacheKey(other, "0") {
		t.Fatal("different queries share a cache key")
	}
}

func TestIsMutation(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !isMutation(m) {
			t.Errorf("%s must bump the version counter", m)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		if isMutation(m) {
			t.Errorf("%s must not bump the version counter", m)
		}
	}
}

// schoolScope falls back to "anon" rather than panicking when the request
// carries no claims (the middleware also runs before JWT on some paths).
func TestSchoolScopeAnon(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/exams", nil)
	if got := schoolScope(r); got != "anon" {
		t.Fatalf("schoolScope with no claims = %q, want anon", got)
	}
	if !strings.HasPrefix(versionKey("s1"), keyPrefix) {
		t.Fatal("version key must live under this service's prefix")
	}
}
