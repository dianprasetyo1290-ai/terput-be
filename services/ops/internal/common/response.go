// Package common holds the HTTP plumbing every ops handler shares: the
// standard response envelope, request decoding, pagination and the identity
// JWT / internal-key middlewares (copied from billing/hr, not imported).
package common

import (
	"encoding/json"
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
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

// WriteJSON writes a success envelope.
func WriteJSON(w http.ResponseWriter, status int, data, meta any) {
	WriteEnvelope(w, status, Envelope{Data: data, Meta: meta})
}

// WriteError writes an error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	msg = SafeMessage(status, msg)
	WriteEnvelope(w, status, Envelope{Message: msg, Errors: map[string]string{"message": msg}})
}

// WriteEnvelope writes any envelope, e.g. a 409 that carries data.
func WriteEnvelope(w http.ResponseWriter, status int, env Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(env)
}
