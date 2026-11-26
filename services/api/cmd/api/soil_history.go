package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// soilHistoryRuntime is the daily soil history executor as the composition
// root sees it; worker is nil when it is disabled.
type soilHistoryRuntime struct {
	worker *usecase.SoilHistoryWorker
	log    *slog.Logger
}

// soilHistoryConfig converts the settings to the executor configuration.
func soilHistoryConfig(s platform.SoilHistorySettings, location *time.Location) usecase.SoilHistoryConfig {
	return usecase.SoilHistoryConfig{
		Location: location, Hour: s.Hour, Concurrency: s.Concurrency,
		SweepInterval: s.SweepInterval, FieldTimeout: s.FieldTimeout,
	}
}

// buildSoilHistoryRuntime builds the executor from its ports, or nothing when
// API_SOIL_HISTORY_ENABLED is false.
func buildSoilHistoryRuntime(s platform.SoilHistorySettings, location *time.Location, store usecase.SoilHistoryStore,
	evaluator usecase.FieldEvaluator, clock usecase.Clock, logger *slog.Logger,
) (soilHistoryRuntime, error) {
	if !s.Enabled {
		logger.Warn("soil history disabled (API_SOIL_HISTORY_ENABLED=false): no daily soil state is recorded")
		return soilHistoryRuntime{log: logger}, nil
	}
	worker, err := usecase.NewSoilHistoryWorker(usecase.SoilHistoryDeps{
		Store: store, Evaluator: evaluator, Clock: clock, Log: logger,
	}, soilHistoryConfig(s, location))
	if err != nil {
		return soilHistoryRuntime{}, fmt.Errorf("building the soil history worker: %w", err)
	}
	return soilHistoryRuntime{worker: worker, log: logger}, nil
}

// Start launches the executor, if enabled.
func (r soilHistoryRuntime) Start(ctx context.Context) error {
	if r.worker == nil {
		return nil
	}
	if err := r.worker.Start(ctx); err != nil {
		return fmt.Errorf("starting the soil history worker: %w", err)
	}
	r.log.Info("soil history worker started")
	return nil
}

// stopSoilHistory waits for the executor, giving up when ctx ends.
func stopSoilHistory(ctx context.Context, logger *slog.Logger, history soilHistoryRuntime) {
	if history.worker == nil {
		return
	}
	stopped := make(chan struct{})
	go func() {
		history.worker.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		logger.Info("soil history worker stopped")
	case <-ctx.Done():
		logger.Warn("soil history worker did not stop within the shutdown timeout; the missing fields are recorded by the next run")
	}
}
