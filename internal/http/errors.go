package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"tonnet/internal/auth"
	"tonnet/internal/domain"
)

// errorBody is the single error envelope used by every endpoint.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps domain sentinels to HTTP statuses; anything else is a 500
// whose details stay in the logs.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal"
	msg := "internal error"
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code, msg = http.StatusNotFound, "not_found", err.Error()
	case errors.Is(err, domain.ErrInvalidInput):
		status, code, msg = http.StatusBadRequest, "invalid_input", err.Error()
	case errors.Is(err, domain.ErrForbidden):
		status, code, msg = http.StatusForbidden, "forbidden", err.Error()
	case errors.Is(err, domain.ErrConflict):
		status, code, msg = http.StatusConflict, "conflict", err.Error()
	case errors.Is(err, auth.ErrInvalidToken):
		status, code, msg = http.StatusUnauthorized, "unauthorized", "missing or invalid token"
	case errors.Is(err, auth.ErrInvalidProof):
		status, code, msg = http.StatusUnauthorized, "invalid_proof", err.Error()
	case errors.Is(err, errBadRequest):
		status, code, msg = http.StatusBadRequest, "bad_request", err.Error()
	default:
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: msg}})
}

var errBadRequest = errors.New("bad request")
