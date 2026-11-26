package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/copernicus"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// terrainStorage is what the relief use cases and the worker consume from
// persistence: the Postgres database in production, the in-memory relief store
// in tests.
type terrainStorage interface {
	usecase.TerrainReader
	usecase.TerrainTxRunner
}

// noWakeup is the TerrainWakeup used when the worker is disabled: a calculation
// is still scheduled, but nobody is woken to process it.
type noWakeup struct{}

func (noWakeup) Wake() {}

// copernicusConfig converts the settings to the client configuration. The
// settings say 0 retries means "no retry", while copernicus.Config reads 0 as
// "use the default (2)" and a negative value as "none", hence the -1.
func copernicusConfig(s platform.ElevationSettings) copernicus.Config {
	retries := s.Retries
	if retries == 0 {
		retries = -1
	}
	return copernicus.Config{
		BaseURL: s.BaseURL, RequestTimeout: s.RequestTimeout, MaxBytes: s.MaxBytes,
		Retries: retries, ModelVersion: s.ModelVersion,
	}
}

// terrainWorkerConfig converts the worker settings. The concurrency is the
// number of fields calculated at once, and so of tile ingestions at once: an
// ingestion holds hundreds of MB of GeoTIFF in memory, so the default of 1
// serializes them; raise API_TERRAIN_WORKER_CONCURRENCY only with memory to match.
func terrainWorkerConfig(s platform.TerrainWorkerSettings) usecase.TerrainWorkerConfig {
	return usecase.TerrainWorkerConfig{
		Concurrency: s.Concurrency, SweepInterval: s.SweepInterval, TileTimeout: s.TileTimeout,
		FieldTimeout: s.FieldTimeout, LeaseSlack: s.LeaseSlack,
	}
}

// terrainRuntime is the relief executor as the composition root sees it: the
// wakeup the use cases call and the worker's lifecycle. Worker is nil when the
// executor is disabled.
type terrainRuntime struct {
	Wakeup usecase.TerrainWakeup
	worker *usecase.TerrainWorker
	log    *slog.Logger
}

// buildTerrainRuntime builds the executor from its ports. Without the worker it
// warns: calculations get scheduled but nobody processes them.
func buildTerrainRuntime(cfg platform.TerrainConfig, tx usecase.TerrainTxRunner, elevation usecase.ElevationStore,
	source usecase.ElevationSource, clock usecase.Clock, logger *slog.Logger,
) (terrainRuntime, error) {
	if !cfg.Worker.Enabled {
		logger.Warn("terrain worker disabled (API_TERRAIN_WORKER_ENABLED=false): calculations are scheduled but nothing processes them")
		return terrainRuntime{Wakeup: noWakeup{}}, nil
	}
	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: tx, Elevation: elevation, Source: source, Clock: clock, Rules: cfg.Rules, Log: logger,
	}, terrainWorkerConfig(cfg.Worker))
	if err != nil {
		return terrainRuntime{}, fmt.Errorf("building the terrain worker: %w", err)
	}
	return terrainRuntime{Wakeup: worker, worker: worker, log: logger}, nil
}

// newCopernicusSource builds the elevation source over the default HTTP client.
func newCopernicusSource(s platform.ElevationSettings) usecase.ElevationSource {
	return copernicus.NewClient(copernicusConfig(s), &http.Client{CheckRedirect: refuseUnsafeRedirect})
}

// maxRedirects is the number of redirects a tile download may follow.
const maxRedirects = 5

// refuseUnsafeRedirect keeps the https requirement of API_ELEVATION_BASE_URL from being
// undone by the server: a redirect may not downgrade https to http nor leave the host
// of the original request.
func refuseUnsafeRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errors.New("stopped after too many redirects")
	}
	first := via[0]
	if first.URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errors.New("redirect from https to http refused")
	}
	if req.URL.Host != first.URL.Host {
		return errors.New("redirect to another host refused")
	}
	return nil
}

// Start launches the worker, if enabled.
func (r terrainRuntime) Start(ctx context.Context) error {
	if r.worker == nil {
		return nil
	}
	if err := r.worker.Start(ctx); err != nil {
		return fmt.Errorf("starting the terrain worker: %w", err)
	}
	r.log.Info("terrain worker started")
	return nil
}

// Stop waits for the field in progress to be saved and ends the worker.
func (r terrainRuntime) Stop() {
	if r.worker != nil {
		r.worker.Stop()
	}
}

// Enabled reports whether a worker was built.
func (r terrainRuntime) Enabled() bool { return r.worker != nil }
