// Package common holds the small pieces of HTTP plumbing every
// notification-service handler shares: the response envelope, request
// decoding/pagination, the JWT middleware (copied from CONVENTIONS.md), the
// service-to-service internal-key middleware, and Sentry wiring.
package common

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
)

// Envelope is the standard response shape every endpoint returns, success or
// failure (see docs/CONVENTIONS.md).
type Envelope struct {
	Data    interface{} `json:"data"`
	Meta    interface{} `json:"meta,omitempty"`
	Message string      `json:"message"`
	Errors  interface{} `json:"errors"`
}

// normalizeNilSlice guards against Go's nil slice marshaling to JSON null
// instead of [] — list endpoints must always return an array.
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

// WriteJSON writes a success envelope.
func WriteJSON(w http.ResponseWriter, status int, data interface{}, meta interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: normalizeNilSlice(data), Meta: meta, Message: "", Errors: nil})
}

// WriteJSONMsg is WriteJSON with a human-readable confirmation message.
func WriteJSONMsg(w http.ResponseWriter, status int, data interface{}, meta interface{}, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: normalizeNilSlice(data), Meta: meta, Message: msg, Errors: nil})
}

// WriteError writes an error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Data: nil, Message: msg, Errors: map[string]string{"message": msg}})
}

// Decode reads a bounded JSON body into dst. Unknown fields are ignored so
// clients round-tripping whole objects don't have requests rejected.
func Decode(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// Page reads page/per_page with defaults and a cap.
func Page(r *http.Request) (page, perPage int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 200 {
		perPage = 200
	}
	return page, perPage
}

// Offset is a convenience for LIMIT/OFFSET queries.
func Offset(page, perPage int) int {
	if page < 1 {
		page = 1
	}
	return (page - 1) * perPage
}

// PageMeta is the pagination block list endpoints return under "meta".
type PageMeta struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
}

// NewPageMeta builds the pagination meta block.
func NewPageMeta(page, perPage, total int) PageMeta {
	return PageMeta{Page: page, PerPage: perPage, Total: total}
}
