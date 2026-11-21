package httpadapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	requestIDHeader    = "X-Request-Id"
	maxRequestIDLength = 64
	healthPathPrefix   = "/health/"
)

type requestIDKey struct{}

// RequestIDFromContext returns the request id set by the RequestID middleware.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// recoverMiddleware turns a panic into a logged 500 so one bad request never
// takes the process down. It is outermost, so it also covers other middlewares.
func recoverMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer recoverPanic(r.Context(), logger, w, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// recoverPanic must be the deferred function itself: recover only works when
// called directly by it.
func recoverPanic(ctx context.Context, logger *slog.Logger, w http.ResponseWriter, method, path string) {
	recovered := recover()
	if recovered == nil {
		return
	}
	// The id comes from the response header because the context of this
	// layer does not see values added by inner middlewares.
	logger.ErrorContext(ctx, "panic recovered",
		"panic", recovered,
		"method", method,
		"path", path,
		"request_id", w.Header().Get(requestIDHeader),
	)
	writeError(w, logger, http.StatusInternalServerError, "internal_error", "internal server error")
}

// requestIDMiddleware propagates a valid incoming id or generates a new one.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !isValidRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isValidRequestID rejects anything that could be used for log injection.
func isValidRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for _, char := range id {
		if !isRequestIDChar(char) {
			return false
		}
	}
	return true
}

func isRequestIDChar(char rune) bool {
	return char >= 'a' && char <= 'z' ||
		char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' ||
		char == '-' || char == '_'
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		// crypto/rand failing means the OS entropy source is broken; an empty
		// id is harmless here, so the request still proceeds.
		return ""
	}
	return hex.EncodeToString(buffer)
}

// statusRecorder captures the status code written by the handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// accessLogMiddleware logs one line per request. Probes run every few seconds,
// so they log at Debug to keep Info output readable.
func accessLogMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		logger.Log(r.Context(), accessLogLevel(r.URL.Path), "request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
			"request_id", RequestIDFromContext(r.Context()),
		)
	})
}

func accessLogLevel(path string) slog.Level {
	if strings.HasPrefix(path, healthPathPrefix) {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
