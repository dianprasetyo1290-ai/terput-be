package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
)

// param reads a chi path parameter.
func param(r *http.Request, key string) string { return chi.URLParam(r, key) }

// httpError couples a message with the status a handler should return.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// writeErr renders an httpError, or a 500 for anything unexpected.
func writeErr(w http.ResponseWriter, err error) {
	var he *httpError
	if errors.As(err, &he) {
		common.WriteError(w, he.status, he.msg)
		return
	}
	log.Printf("error response 500 (sanitized): %v", err)
	common.CaptureError(err)
	common.WriteError(w, http.StatusInternalServerError, "internal error")
}

// nullTime turns a zero time into a NULL-friendly value.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

// nullString turns an empty string into a NULL-friendly value.
func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// parsePositive parses a positive integer amount, rejecting signs/junk.
func parsePositive(v string) (int64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// parseTime accepts the shapes an upload form sends: RFC3339,
// datetime-local ("2006-01-02T15:04") and a plain date.
func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid timestamp")
}

// randomKey returns a hex object key segment.
func randomKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isUniqueViolation reports whether an error is a Postgres unique conflict.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "SQLSTATE 23505")
}

// isLockNotAvailable reports a NOWAIT lock conflict (SQLSTATE 55P03).
func isLockNotAvailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 55P03") || strings.Contains(msg, "could not obtain lock")
}

// jsonRaw marshals a value for a jsonb column, returning nil for nil input.
func jsonRaw(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// idempotencyKey reads the header, falling back to a generated key.
func idempotencyKey(r *http.Request) string {
	if k := strings.TrimSpace(r.Header.Get("Idempotency-Key")); k != "" {
		return k
	}
	return randomKey()
}

// audit writes one append-only billing audit event.
func (a *App) audit(ctx context.Context, schoolID, aggregateType, aggregateID, action, actorType, actorID, actorName, reason string, before, after any) {
	_, _ = a.DB.ExecContext(ctx,
		`INSERT INTO billing_audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,before_data,after_data,reason)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''))`,
		schoolID, aggregateType, aggregateID, action, actorType, actorID, actorName, jsonRaw(before), jsonRaw(after), reason)
}

// streamObject streams a private object to the response with its stored MIME
// type and an ETag built from the sha256 checksum. Objects are never served
// by a static path — only through authorized handlers that call this.
func (a *App) streamObject(w http.ResponseWriter, r *http.Request, key, mime, checksum string, size int64) {
	f, err := a.Storage.Open(r.Context(), key)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "object not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	if checksum != "" {
		w.Header().Set("ETag", `"`+checksum+`"`)
	}
	w.Header().Set("Content-Disposition", `inline; filename="proof`+strings.ReplaceAll(mime, "/", ".")+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, f)
}
