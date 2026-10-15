package common

import (
	"encoding/json"
	"net/http"
	"reflect"
)

type Envelope struct {
	Data    interface{} `json:"data"`
	Meta    interface{} `json:"meta,omitempty"`
	Message string      `json:"message"`
	Errors  interface{} `json:"errors"`
}

// normalizeNilSlice guards against Go's `nil` slice marshaling to JSON
// `null` instead of `[]` — a list handler that does `var out []T; ...;
// return out` (idiomatic Go, and how most list queries here are written)
// produces `null` for zero results, which breaks any frontend that assumes
// `data` is always an array on a list endpoint (e.g. `data.length`). Fixed
// once here, at the one place every response passes through, rather than
// auditing every handler — found via a real browser click-through
// (2026-07-18) hitting several "no rows yet" list screens across
// attendance and evaluation.
func normalizeNilSlice(v interface{}) interface{} {
	if v == nil {
		return v
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}, meta interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{Data: normalizeNilSlice(data), Meta: meta, Message: "", Errors: nil})
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	msg = SafeMessage(status, msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: map[string]string{"message": msg}})
}
