package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/academic/internal/common"
)

func TestInternalListEnrollmentsGuards(t *testing.T) {
	h := &Handler{}
	mw := common.InternalOrJWTMiddleware("jwt-secret", "internal-key")
	srv := mw(http.HandlerFunc(h.InternalListEnrollments))
	cases := []struct {
		name, path, key string
		want            int
	}{
		{"no credentials", "/internal/enrollments?school_id=s", "", http.StatusUnauthorized},
		{"internal without school", "/internal/enrollments", "internal-key", http.StatusBadRequest},
		{"internal unknown status", "/internal/enrollments?school_id=s&status=x", "internal-key", http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			if c.key != "" {
				req.Header.Set("X-Internal-Key", c.key)
			}
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}
