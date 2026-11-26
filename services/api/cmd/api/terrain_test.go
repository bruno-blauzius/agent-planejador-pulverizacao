package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

func terrainConfig(t *testing.T, env map[string]string) platform.TerrainConfig {
	t.Helper()
	cfg, err := platform.LoadTerrainConfig(func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	})
	if err != nil {
		t.Fatalf("loading terrain config: %v", err)
	}
	return cfg
}

func terrainPorts() (*fakes.Terrain, *fakes.Elevation, *fakes.ElevationSource, *fakes.FixedClock) {
	return fakes.NewTerrain(fakes.NewRegistry()), fakes.NewElevation(), fakes.NewElevationSource(),
		&fakes.FixedClock{T: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)}
}

func TestTerrainRoutesAreMountedAndProtected(t *testing.T) {
	handler, _ := composed(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/farms/f1/terrain"},
		{http.MethodPost, "/farms/f1/terrain/calculations"},
		{http.MethodGet, "/farms/f1/fields/t1/terrain"},
		{http.MethodPut, "/farms/f1/fields/t1/terrain/adjustment"},
		{http.MethodDelete, "/farms/f1/fields/t1/terrain/adjustment"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			if status, _ := send(handler, "", route.method, route.path, nil); status != http.StatusUnauthorized {
				t.Errorf("anonymous: status %d, want 401 (a 404 would mean the route is not mounted)", status)
			}
		})
	}
}

func TestLoadAppConfigRejectsInvalidTerrainConfig(t *testing.T) {
	for _, tc := range []struct{ name, variable, value string }{
		{"class limits out of order", "API_TERRAIN_CLASS_LIMITS_PCT", "8,3"},
		{"negative retries", "API_ELEVATION_RETRIES", "-1"},
		{"invalid worker flag", "API_TERRAIN_WORKER_ENABLED", "maybe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := validAppEnv()
			env[tc.variable] = tc.value
			_, err := loadAppConfig(func(name string) (string, bool) {
				value, ok := env[name]
				return value, ok
			})
			if err == nil {
				t.Fatal("expected the startup to fail")
			}
			if !strings.Contains(err.Error(), "terrain") || !strings.Contains(err.Error(), tc.variable) {
				t.Errorf("error %q must name the relief config and %s", err, tc.variable)
			}
		})
	}
}

func TestCopernicusConfigMapsZeroRetriesToNone(t *testing.T) {
	for _, tc := range []struct{ retries, want int }{{0, -1}, {1, 1}, {3, 3}} {
		cfg := terrainConfig(t, map[string]string{"API_ELEVATION_RETRIES": strconv.Itoa(tc.retries)})
		if got := copernicusConfig(cfg.Elevation).Retries; got != tc.want {
			t.Errorf("retries %d -> %d, want %d", tc.retries, got, tc.want)
		}
	}
	cfg := terrainConfig(t, map[string]string{"API_ELEVATION_BASE_URL": "http://127.0.0.1:9000", "API_ELEVATION_MAX_BYTES": "1000"})
	got := copernicusConfig(cfg.Elevation)
	if got.BaseURL != "http://127.0.0.1:9000" || got.MaxBytes != 1000 {
		t.Errorf("config not carried over: %+v", got)
	}
}

func TestTerrainWorkerConfigCarriesTheSettings(t *testing.T) {
	cfg := terrainConfig(t, map[string]string{
		"API_TERRAIN_WORKER_CONCURRENCY": "2", "API_TERRAIN_WORKER_SWEEP_INTERVAL": "10s", "API_TERRAIN_WORKER_FIELD_TIMEOUT": "90s",
	})
	got := terrainWorkerConfig(cfg.Worker)
	if got.Concurrency != 2 || got.SweepInterval != 10*time.Second || got.FieldTimeout != 90*time.Second {
		t.Errorf("worker config: %+v", got)
	}
}

func TestDisabledTerrainWorkerIsNotStartedAndWarns(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))
	tx, elevation, source, clock := terrainPorts()
	cfg := terrainConfig(t, map[string]string{"API_TERRAIN_WORKER_ENABLED": "false"})

	runtime, err := buildTerrainRuntime(cfg, tx, elevation, source, clock, logger)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Enabled() {
		t.Error("a disabled worker must not be built")
	}
	if _, ok := runtime.Wakeup.(noWakeup); !ok {
		t.Errorf("wakeup is %T, want the null one", runtime.Wakeup)
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Errorf("start must be a no-op: %v", err)
	}
	runtime.Wakeup.Wake() // must not panic or block
	runtime.Stop()
	if !strings.Contains(out.String(), "level=WARN") || !strings.Contains(out.String(), "API_TERRAIN_WORKER_ENABLED") {
		t.Errorf("expected a startup warning, got %q", out.String())
	}
}

func TestEnabledTerrainWorkerStartsAndStopsCleanly(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))
	tx, elevation, source, clock := terrainPorts()

	runtime, err := buildTerrainRuntime(terrainConfig(t, nil), tx, elevation, source, clock, logger)
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.Enabled() {
		t.Fatal("the worker is enabled by default")
	}
	if strings.Contains(out.String(), "level=WARN") {
		t.Errorf("an enabled worker must not warn: %q", out.String())
	}
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := runtime.Start(context.Background()); err == nil {
		t.Error("a second start must fail")
	}
	if !strings.Contains(out.String(), "terrain worker started") {
		t.Errorf("expected the start to be logged: %q", out.String())
	}
	runtime.Wakeup.Wake()

	var stopped atomic.Bool
	go func() { runtime.Stop(); stopped.Store(true) }()
	deadline := time.Now().Add(5 * time.Second)
	for !stopped.Load() {
		if time.Now().After(deadline) {
			t.Fatal("the worker did not stop")
		}
		time.Sleep(time.Millisecond)
	}
	runtime.Stop() // idempotent
}

func TestStopTerrainGivesUpWhenTheBudgetEnds(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&out, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tx, elevation, source, clock := terrainPorts()
	runtime, err := buildTerrainRuntime(terrainConfig(t, nil), tx, elevation, source, clock, logger)
	if err != nil {
		t.Fatal(err)
	}
	stopTerrain(ctx, logger, runtime) // returns even with an expired budget; either branch is fine
	stopTerrain(ctx, logger, terrainRuntime{Wakeup: noWakeup{}})
}

var _ usecase.TerrainWakeup = noWakeup{}

func TestTerrainWorkerConfigCarriesTheTileTimeoutAndLease(t *testing.T) {
	cfg := terrainConfig(t, map[string]string{
		"API_TERRAIN_WORKER_TILE_TIMEOUT": "15m", "API_TERRAIN_WORKER_LEASE_SLACK": "3m",
	})
	got := terrainWorkerConfig(cfg.Worker)
	if got.TileTimeout != 15*time.Minute || got.LeaseSlack != 3*time.Minute {
		t.Errorf("worker config: %+v", got)
	}
}

func TestRefuseUnsafeRedirect(t *testing.T) {
	mustReq := func(url string) *http.Request {
		r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := mustReq("https://dem.test/a.tif")
	tests := []struct {
		name    string
		target  string
		via     []*http.Request
		wantErr bool
	}{
		{"same host https", "https://dem.test/b.tif", []*http.Request{first}, false},
		{"downgrade to http", "http://dem.test/b.tif", []*http.Request{first}, true},
		{"other host", "https://evil.test/b.tif", []*http.Request{first}, true},
		{"too many", "https://dem.test/b.tif", []*http.Request{first, first, first, first, first}, true},
		{"http origin to same http host", "http://127.0.0.1:9/b", []*http.Request{mustReq("http://127.0.0.1:9/a")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := refuseUnsafeRedirect(mustReq(tt.target), tt.via)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
