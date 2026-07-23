package common

import (
	"encoding/json"
	"net/http"
)

// WriteJSONMsg is a small addition on top of the shared WriteJSON helper
// (see response.go, pasted verbatim from CONVENTIONS.md) for the common case
// of a successful write that wants a human-readable confirmation message
// (e.g. "Presensi berhasil disimpan") instead of the default empty string.
func WriteJSONMsg(w http.ResponseWriter, status int, data interface{}, meta interface{}, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{Data: normalizeNilSlice(data), Meta: meta, Message: msg, Errors: nil})
}
