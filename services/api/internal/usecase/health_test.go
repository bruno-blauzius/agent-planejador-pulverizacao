package usecase_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

type fakeProbe struct {
	name string
	err  error
}

func (f fakeProbe) Name() string                  { return f.name }
func (f fakeProbe) Check(_ context.Context) error { return f.err }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestCheckLiveness_AlwaysUp(t *testing.T) {
	if report := (usecase.CheckLiveness{}).Execute(); !report.Healthy() {
		t.Fatalf("liveness must be up, got %q", report.Status)
	}
}

func TestCheckStartup(t *testing.T) {
	tests := []struct {
		name    string
		started bool
		want    usecase.Status
	}{
		{"down before start", false, usecase.StatusDown},
		{"up after start", true, usecase.StatusUp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := usecase.NewLifecycle()
			if tt.started {
				lifecycle.MarkStarted()
			}
			if got := usecase.NewCheckStartup(lifecycle).Execute().Status; got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckReadiness(t *testing.T) {
	boom := errors.New("connection refused")
	tests := []struct {
		name         string
		started      bool
		shuttingDown bool
		probes       []usecase.DependencyProbe
		want         usecase.Status
		wantChecks   int
	}{
		{name: "down while starting", started: false, want: usecase.StatusDown},
		{name: "down while draining", started: true, shuttingDown: true, want: usecase.StatusDown},
		{name: "up without dependencies", started: true, want: usecase.StatusUp},
		{
			name: "up when all dependencies are up", started: true,
			probes: []usecase.DependencyProbe{fakeProbe{name: "postgres"}, fakeProbe{name: "agent"}},
			want:   usecase.StatusUp, wantChecks: 2,
		},
		{
			name: "down when one dependency fails", started: true,
			probes: []usecase.DependencyProbe{fakeProbe{name: "postgres", err: boom}, fakeProbe{name: "agent"}},
			want:   usecase.StatusDown, wantChecks: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := usecase.NewLifecycle()
			if tt.started {
				lifecycle.MarkStarted()
			}
			if tt.shuttingDown {
				lifecycle.BeginShutdown()
			}
			report := usecase.NewCheckReadiness(lifecycle, discardLogger(), tt.probes...).Execute(context.Background())
			if report.Status != tt.want {
				t.Fatalf("status: got %q, want %q", report.Status, tt.want)
			}
			if len(report.Checks) != tt.wantChecks {
				t.Fatalf("checks: got %d, want %d", len(report.Checks), tt.wantChecks)
			}
		})
	}
}
