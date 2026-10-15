package common

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type Envelope struct {
	Data  interface{} `json:"data,omitempty"`
	Error *ErrorBody  `json:"error,omitempty"`
	Meta  interface{} `json:"meta,omitempty"`
}

type ErrorBody struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}, meta ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	env := Envelope{Data: data}
	if len(meta) > 0 && meta[0] != nil {
		env.Meta = meta[0]
	}
	_ = json.NewEncoder(w).Encode(env)
}

func WriteError(w http.ResponseWriter, status int, message string, details ...interface{}) {
	message = SafeMessage(status, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	var d interface{}
	if len(details) > 0 {
		d = details[0]
	}

	env := Envelope{
		Error: &ErrorBody{
			Code:    status,
			Message: message,
			Details: d,
		},
	}
	_ = json.NewEncoder(w).Encode(env)
}

func ParsePagination(r *http.Request) (page int, perPage int) {
	page = 1
	perPage = 20

	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if pp := r.URL.Query().Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 && v <= 100 {
			perPage = v
		}
	}
	return
}
