package common

import (
	"net/http/httptest"
	"testing"
)

// Regression check: a caller asking for more than the max (e.g. web's
// listStudents per_page:500, meant to fetch "everyone") must get clamped to
// the max page size, not silently dropped to the tiny default — that
// silent drop is what caused students past position 20 to fall out of the
// name lookup and render as raw UUIDs in the guru Presensi roster.
func TestParsePagePerPageClamp(t *testing.T) {
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
			r := httptest.NewRequest("GET", "/?per_page="+c.perPage, nil)
			if c.perPage == "" {
				r = httptest.NewRequest("GET", "/", nil)
			}
			_, perPage := ParsePage(r)
			if perPage != c.want {
				t.Errorf("ParsePage per_page=%q = %d, want %d", c.perPage, perPage, c.want)
			}
		})
	}
}
