// Package httpadapter exposes the use cases over HTTP. It is named httpadapter
// (not http) so it never collides with the standard library net/http.
package httpadapter

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []fieldDetail `json:"details,omitempty"`
}

// fieldDetail points at one invalid input field (only in validation_failed).
type fieldDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// writeJSON serializes body with the given status. Encoding failures can only
// happen after headers are sent, so they are logged instead of returned.
func writeJSON(w http.ResponseWriter, logger *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		logger.Error("failed to encode json response", "error", err)
	}
}

// writeError writes the standard error envelope used by every endpoint.
func writeError(w http.ResponseWriter, logger *slog.Logger, status int, code, message string) {
	writeJSON(w, logger, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}
