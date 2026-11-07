// Package httpapi holds HTTP transport concerns only: routing, request decoding,
// response envelopes and status codes. Business rules live in internal/domain
// (`new_update.md` §4).
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// errorBody is the single error envelope for every API failure.
type errorBody struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON writes a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// The status line is already sent; log only (never leak internals).
		slog.Error("httpapi: encode response", "err", err)
	}
}

// WriteError writes the standard error envelope.
//
// Messages must be safe for clients: no SQL, no panel responses, no secrets
// (`new_update.md` §7).
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}})
}

// DecodeJSON strictly decodes a request body into dst.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
