// Package common holds the HTTP plumbing every billing-service handler
// shares: the standard response envelope, request decoding, pagination, and
// the three auth middlewares (identity JWT, service-to-service internal key).
//
// This mirrors docs/CONVENTIONS.md — each service copies the small snippets
// into its own internal/common rather than sharing a Go package.
package common

import (
	"encoding/json"
	"net/http"
)

// Envelope is the standard response shape (see docs/CONVENTIONS.md).
type Envelope struct {
	Data    any    `json:"data"`
	Meta    any    `json:"meta,omitempty"`
	Message string `json:"message"`
	Errors  any    `json:"errors"`
}

// WriteJSON writes a success envelope.
func WriteJSON(w http.ResponseWriter, status int, data, meta any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: data, Meta: meta, Message: "", Errors: nil})
}

// WriteSuccess writes a 200 OK success envelope.
func WriteSuccess(w http.ResponseWriter, data, meta any) {
	WriteJSON(w, http.StatusOK, data, meta)
}

// WriteError writes an error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: map[string]string{"message": msg}})
}

// Meta is the pagination metadata in list envelopes.
type Meta struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}
