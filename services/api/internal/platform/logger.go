package platform

import (
	"io"
	"log/slog"
)

// NewLogger builds the structured JSON logger used across the API.
func NewLogger(out io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))
}
