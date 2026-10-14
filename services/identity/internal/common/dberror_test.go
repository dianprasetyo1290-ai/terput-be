package common

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDBErrorStatus(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantField  string
	}{
		{"duplicate unit code", &pgconn.PgError{Code: "23505", ConstraintName: "schools_code_key"}, http.StatusConflict, "code"},
		{"duplicate email wrapped", fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}), http.StatusConflict, "email"},
		{"unknown unique", &pgconn.PgError{Code: "23505", ConstraintName: "x"}, http.StatusConflict, ""},
		{"fk", &pgconn.PgError{Code: "23503"}, http.StatusConflict, ""},
		{"check", &pgconn.PgError{Code: "23514", ColumnName: "level"}, http.StatusBadRequest, "level"},
		{"bad uuid", &pgconn.PgError{Code: "22P02"}, http.StatusBadRequest, ""},
		{"other pg", &pgconn.PgError{Code: "40001"}, http.StatusInternalServerError, ""},
		{"non pg", errors.New("conn refused"), http.StatusInternalServerError, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, msg, f := DBErrorStatus(c.err, "fallback")
			if st != c.wantStatus || f != c.wantField || msg == "" {
				t.Fatalf("got %d %q %q", st, msg, f)
			}
		})
	}
}

func TestWriteDBErrorHidesSQL(t *testing.T) {
	w := httptest.NewRecorder()
	WriteDBError(w, &pgconn.PgError{Code: "23505", ConstraintName: "schools_code_key", Message: `duplicate key value violates unique constraint "schools_code_key"`}, "gagal")
	if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "duplicate key") || strings.Contains(w.Body.String(), "SQLSTATE") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
