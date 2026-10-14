package common

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueConstraintMessages maps a unique constraint/index name to the field
// it guards and the Indonesian message shown to the user. A constraint that
// is not listed still answers 409, just with a generic message.
var uniqueConstraintMessages = map[string][2]string{
	"schools_code_key":                {"code", "Kode unit sudah dipakai"},
	"users_email_key":                 {"email", "Email sudah dipakai pengguna lain"},
	"idx_users_email":                 {"email", "Email sudah dipakai pengguna lain"},
	"idx_academic_years_school_label": {"label", "Label tahun ajaran sudah digunakan di unit ini"},
	"positions_school_id_name_key":    {"name", "Nama jabatan sudah dipakai di unit ini"},
	"idx_users_nisn_unique":           {"nisn", "NISN sudah dipakai siswa lain"},
	"idx_users_nik_unique":            {"nik", "NIK sudah dipakai pengguna lain"},
}

// DBErrorStatus classifies a database error into an HTTP status, a user-facing
// message and the offending field (may be ""). It never returns driver text:
// SQL messages leak schema details (BUG-045/BUG-069) and mean nothing to users.
func DBErrorStatus(err error, fallback string) (status int, msg, field string) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return http.StatusInternalServerError, fallback, ""
	}
	switch pgErr.Code {
	case "23505": // unique_violation
		if m, ok := uniqueConstraintMessages[pgErr.ConstraintName]; ok {
			return http.StatusConflict, m[1], m[0]
		}
		return http.StatusConflict, "Data yang sama sudah ada", ""
	case "23503": // foreign_key_violation
		return http.StatusConflict, "Data masih dirujuk data lain atau rujukan tidak ditemukan", ""
	case "23514", "23502": // check_violation, not_null_violation
		return http.StatusBadRequest, "Data tidak memenuhi aturan validasi", pgErr.ColumnName
	case "22P02", "22007", "22008", "22001": // invalid text repr / datetime / string too long
		return http.StatusBadRequest, "Format data tidak valid", ""
	}
	return http.StatusInternalServerError, fallback, ""
}

// WriteDBError writes the classified error. Unexpected (5xx) errors are sent
// to Sentry with the original driver error; the client only sees fallback.
func WriteDBError(w http.ResponseWriter, err error, fallback string) {
	status, msg, field := DBErrorStatus(err, fallback)
	if status >= 500 {
		CaptureError(err)
	}
	if field != "" {
		WriteValidationError(w, status, msg, map[string]string{field: msg, "message": msg})
		return
	}
	WriteError(w, status, msg)
}

// IsUniqueViolation reports whether err is a Postgres unique violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// SafeDBMessage is the user-facing text for err when it has to be embedded
// in a larger message (e.g. a per-row import report). Unexpected errors go
// to Sentry and are reported as fallback.
func SafeDBMessage(err error, fallback string) string {
	status, msg, _ := DBErrorStatus(err, fallback)
	if status >= 500 {
		CaptureError(err)
	}
	return msg
}
