// Package common holds the HTTP plumbing every hr handler shares: the
// standard response envelope, request decoding, pagination and the identity
// JWT / internal-key middlewares (docs/CONVENTIONS.md, copied not imported).
package common

import (
	"encoding/json"
	"log"
	"net/http"
)

// Envelope is the standard response shape.
type Envelope struct {
	Data    any    `json:"data"`
	Meta    any    `json:"meta,omitempty"`
	Message string `json:"message"`
	Errors  any    `json:"errors"`
}

// Meta is the pagination block of list responses.
type Meta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages,omitempty"`
}

// WriteJSON writes a success envelope.
func WriteJSON(w http.ResponseWriter, status int, data, meta any) {
	WriteEnvelope(w, status, Envelope{Data: data, Meta: meta})
}

// JSON writes a success envelope without meta.
func JSON(w http.ResponseWriter, status int, data any) {
	WriteJSON(w, status, data, nil)
}

// JSONWithMeta writes a success envelope with pagination meta.
func JSONWithMeta(w http.ResponseWriter, status int, data, meta any) {
	WriteJSON(w, status, data, meta)
}

// WriteError writes an error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteEnvelope(w, status, Envelope{Message: msg, Errors: map[string]string{"message": msg}})
}

// Error writes an error envelope.
func Error(w http.ResponseWriter, status int, msg string) {
	WriteError(w, status, msg)
}

// WriteEnvelope writes any envelope, e.g. a 409 that carries data.
func WriteEnvelope(w http.ResponseWriter, status int, env Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}

// InternalError logs the underlying error server-side (and reports it to
// Sentry) but writes only the user-facing msg. Raw driver/SQL messages such
// as `invalid input syntax for type uuid` must never reach the client.
func InternalError(w http.ResponseWriter, msg string, err error) {
	log.Printf("internal error: %s: %v", msg, err)
	CaptureError(err)
	WriteError(w, http.StatusInternalServerError, msg)
}
