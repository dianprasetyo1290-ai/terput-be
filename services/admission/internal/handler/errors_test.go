package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// BUG-099: database errors never reach the client verbatim and validation
// failures are 4xx, not 500.
func TestErrorResponse(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"http error kept", &httpError{http.StatusConflict, "bentrok"}, http.StatusConflict},
		{"unique", &pgconn.PgError{Code: "23505", Message: `duplicate key value violates unique constraint "x"`}, http.StatusConflict},
		{"check", &pgconn.PgError{Code: "23514", Message: `new row violates check constraint "admission_tracks_quota_check"`}, http.StatusUnprocessableEntity},
		{"not null", &pgconn.PgError{Code: "23502"}, http.StatusUnprocessableEntity},
		{"fk", &pgconn.PgError{Code: "23503"}, http.StatusUnprocessableEntity},
		{"bad uuid", fmt.Errorf("wrap: %w", &pgconn.PgError{Code: "22P02", Message: "invalid input syntax for type uuid"}), http.StatusUnprocessableEntity},
		{"other", errors.New("pq: relation foo does not exist"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, msg := errorResponse(tc.err)
			if status != tc.status {
				t.Fatalf("status=%d want %d", status, tc.status)
			}
			low := strings.ToLower(msg)
			for _, leak := range []string{"constraint", "duplicate key", "relation", "syntax", "sqlstate"} {
				if strings.Contains(low, leak) {
					t.Fatalf("message leaks SQL detail: %q", msg)
				}
			}
		})
	}
}
