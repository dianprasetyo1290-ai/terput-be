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

// WriteError writes an error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: map[string]string{"message": msg}})
}
