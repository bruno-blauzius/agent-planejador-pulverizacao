package httpadapter

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// LivenessChecker answers "is the process alive?".
type LivenessChecker interface{ Execute() usecase.HealthReport }

// StartupChecker answers "has initialization finished?".
type StartupChecker interface{ Execute() usecase.HealthReport }

// ReadinessChecker answers "can this instance receive traffic?".
type ReadinessChecker interface {
	Execute(ctx context.Context) usecase.HealthReport
}

type healthResponse struct {
	Status string        `json:"status"`
	Checks []checkResult `json:"checks,omitempty"`
}

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func livenessHandler(logger *slog.Logger, checker LivenessChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, logger, checker.Execute())
	}
}

func startupHandler(logger *slog.Logger, checker StartupChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, logger, checker.Execute())
	}
}

func readinessHandler(logger *slog.Logger, checker ReadinessChecker, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Bounded so a hanging dependency cannot make the probe itself hang.
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		writeHealth(w, logger, checker.Execute(ctx))
	}
}

func writeHealth(w http.ResponseWriter, logger *slog.Logger, report usecase.HealthReport) {
	// Probes must reflect the current state; intermediaries must not cache them.
	w.Header().Set("Cache-Control", "no-store")

	status := http.StatusOK
	if !report.Healthy() {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, logger, status, toHealthResponse(report))
}

func toHealthResponse(report usecase.HealthReport) healthResponse {
	response := healthResponse{Status: string(report.Status)}
	for _, check := range report.Checks {
		response.Checks = append(response.Checks, checkResult{
			Name:   check.Name,
			Status: string(check.Status),
		})
	}
	return response
}
