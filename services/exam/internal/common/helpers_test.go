package common

import (
	"net/http/httptest"
	"testing"
)

// Regression check: over-max per_page must clamp to the max page size, not
// silently drop to the tiny default (same bug class as identity-service's
// ParsePage, which caused the guru Presensi UUID-name bug).
func TestPaginationPerPageClamp(t *testing.T) {
	cases := []struct {
		name    string
		perPage string
		want    int
	}{
		{"missing", "", 20},
		{"zero", "0", 20},
		{"negative", "-5", 20},
		{"within range", "150", 150},
		{"at max", "200", 200},
		{"over max clamps to max", "500", 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			url := "/"
			if c.perPage != "" {
				url = "/?per_page=" + c.perPage
			}
			r := httptest.NewRequest("GET", url, nil)
			_, perPage := Pagination(r)
			if perPage != c.want {
				t.Errorf("Pagination per_page=%q = %d, want %d", c.perPage, perPage, c.want)
			}
		})
	}
}
