package platform_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
)

func TestLoadTerrainConfigDefaults(t *testing.T) {
	cfg, err := platform.LoadTerrainConfig(fakeEnv(nil))
	if err != nil {
		t.Fatalf("LoadTerrainConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg.Rules, domain.DefaultTerrainRules()) {
		t.Errorf("rules = %+v, want the domain defaults", cfg.Rules)
	}
	wantWorker := platform.TerrainWorkerSettings{
		Enabled: true, Concurrency: 1, SweepInterval: 30 * time.Second, TileTimeout: 10 * time.Minute,
		FieldTimeout: 2 * time.Minute, LeaseSlack: 2 * time.Minute,
	}
	if cfg.Worker != wantWorker {
		t.Errorf("worker = %+v, want %+v", cfg.Worker, wantWorker)
	}
	wantElevation := platform.ElevationSettings{
		BaseURL: "https://copernicus-dem-30m.s3.amazonaws.com", RequestTimeout: 2 * time.Minute,
		MaxBytes: 120 << 20, Retries: 2, ModelVersion: "GLO-30 COG 2022-05",
	}
	if cfg.Elevation != wantElevation {
		t.Errorf("elevation = %+v, want %+v", cfg.Elevation, wantElevation)
	}
}

func TestLoadTerrainConfigOverrides(t *testing.T) {
	cfg, err := platform.LoadTerrainConfig(fakeEnv(map[string]string{
		"API_TERRAIN_CLASS_LIMITS_PCT":          " 2, 6, 15, 40 ",
		"API_TERRAIN_FLAT_ASPECT_BELOW_PCT":     "2.5",
		"API_TERRAIN_POSITION_RADIUS_M":         "300",
		"API_TERRAIN_POSITION_TOLERANCE_M":      "3",
		"API_TERRAIN_PROFILE_POINTS":            "12",
		"API_TERRAIN_MIN_VALID_CELLS":           "10",
		"API_TERRAIN_ADJUSTMENT_DIVERGENCE_PCT": "7.5",
		"API_TERRAIN_METHOD_SLOPE_LIMITS":       "tractor_boom=18, drone=30.5",
		"API_TERRAIN_RESTRICTED_FROM_PCT":       "15",
		"API_TERRAIN_UNSUITABLE_FROM_PCT":       "60",
		"API_TERRAIN_WORKER_ENABLED":            "false",
		"API_TERRAIN_WORKER_CONCURRENCY":        "3",
		"API_TERRAIN_WORKER_SWEEP_INTERVAL":     "10s",
		"API_TERRAIN_WORKER_TILE_TIMEOUT":       "20m",
		"API_TERRAIN_WORKER_FIELD_TIMEOUT":      "5m",
		"API_TERRAIN_WORKER_LEASE_SLACK":        "3m",
		"API_ELEVATION_BASE_URL":                "http://localhost:9000/tiles/",
		"API_ELEVATION_REQUEST_TIMEOUT":         "30s",
		"API_ELEVATION_MAX_BYTES":               "1048576",
		"API_ELEVATION_RETRIES":                 "0",
		"API_ELEVATION_MODEL_VERSION":           "test-1",
	}))
	if err != nil {
		t.Fatalf("LoadTerrainConfig: %v", err)
	}
	rules := cfg.Rules
	if rules.ClassUpperLimitsPct != [4]float64{2, 6, 15, 40} || rules.FlatAspectBelowPct != 2.5 || rules.PositionRadiusM != 300 ||
		rules.PositionToleranceM != 3 || rules.ProfilePoints != 12 || rules.MinValidCells != 10 || rules.AdjustmentDivergencePct != 7.5 ||
		rules.RestrictedFromSharePct != 15 || rules.UnsuitableFromSharePct != 60 {
		t.Errorf("rules = %+v", rules)
	}
	if len(rules.MethodSlopeLimits) != 2 || *rules.MethodSlopeLimits[domain.MethodTractorBoom] != 18 || *rules.MethodSlopeLimits[domain.MethodDrone] != 30.5 {
		t.Errorf("method limits = %v", rules.MethodSlopeLimits)
	}
	if rules.MethodSlopeLimits[domain.MethodSelfPropelled] != nil {
		t.Error("a method that is not listed has no limit")
	}
	wantWorker := platform.TerrainWorkerSettings{
		Enabled: false, Concurrency: 3, SweepInterval: 10 * time.Second, TileTimeout: 20 * time.Minute,
		FieldTimeout: 5 * time.Minute, LeaseSlack: 3 * time.Minute,
	}
	if cfg.Worker != wantWorker {
		t.Errorf("worker = %+v", cfg.Worker)
	}
	wantElevation := platform.ElevationSettings{BaseURL: "http://localhost:9000/tiles", RequestTimeout: 30 * time.Second, MaxBytes: 1 << 20, Retries: 0, ModelVersion: "test-1"}
	if cfg.Elevation != wantElevation {
		t.Errorf("elevation = %+v", cfg.Elevation)
	}
}

func TestLoadTerrainConfigEmptyMethodLimitsMeansNoLimits(t *testing.T) {
	cfg, err := platform.LoadTerrainConfig(fakeEnv(map[string]string{"API_TERRAIN_METHOD_SLOPE_LIMITS": ""}))
	if err != nil {
		t.Fatalf("LoadTerrainConfig: %v", err)
	}
	if len(cfg.Rules.MethodSlopeLimits) != 0 {
		t.Errorf("limits = %v, want none", cfg.Rules.MethodSlopeLimits)
	}
}

func TestLoadTerrainConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		key, value string
	}{
		{"API_TERRAIN_CLASS_LIMITS_PCT", "3,8,20"},
		{"API_TERRAIN_CLASS_LIMITS_PCT", "3,8,20,abc"},
		{"API_TERRAIN_CLASS_LIMITS_PCT", "3,8,8,45"},
		{"API_TERRAIN_CLASS_LIMITS_PCT", ""},
		{"API_TERRAIN_FLAT_ASPECT_BELOW_PCT", "0"},
		{"API_TERRAIN_FLAT_ASPECT_BELOW_PCT", "NaN"},
		{"API_TERRAIN_POSITION_RADIUS_M", "-1"},
		{"API_TERRAIN_POSITION_RADIUS_M", "5001"},
		{"API_TERRAIN_POSITION_TOLERANCE_M", "x"},
		{"API_TERRAIN_PROFILE_POINTS", "1"},
		{"API_TERRAIN_PROFILE_POINTS", "2.5"},
		{"API_TERRAIN_MIN_VALID_CELLS", "0"},
		{"API_TERRAIN_ADJUSTMENT_DIVERGENCE_PCT", "-5"},
		{"API_TERRAIN_METHOD_SLOPE_LIMITS", "tractor_boom"},
		{"API_TERRAIN_METHOD_SLOPE_LIMITS", "tractor=20"},
		{"API_TERRAIN_METHOD_SLOPE_LIMITS", "tractor_boom=0"},
		{"API_TERRAIN_METHOD_SLOPE_LIMITS", "tractor_boom=20,tractor_boom=15"},
		{"API_TERRAIN_RESTRICTED_FROM_PCT", "60"},
		{"API_TERRAIN_UNSUITABLE_FROM_PCT", "101"},
		{"API_TERRAIN_WORKER_ENABLED", "maybe"},
		{"API_TERRAIN_WORKER_CONCURRENCY", "0"},
		{"API_TERRAIN_WORKER_SWEEP_INTERVAL", "0s"},
		{"API_TERRAIN_WORKER_SWEEP_INTERVAL", "soon"},
		{"API_TERRAIN_WORKER_FIELD_TIMEOUT", "-1m"},
		{"API_TERRAIN_WORKER_TILE_TIMEOUT", "0s"},
		{"API_TERRAIN_WORKER_TILE_TIMEOUT", "1m"}, // below the request timeout of 2m
		{"API_TERRAIN_WORKER_TILE_TIMEOUT", "5m"}, // 2m x 3 attempts = 6m
		{"API_TERRAIN_WORKER_LEASE_SLACK", "-1s"},
		{"API_ELEVATION_BASE_URL", "http://dem.test"},
		{"API_ELEVATION_BASE_URL", "http://169.254.169.254/latest"},
		{"API_ELEVATION_BASE_URL", "ftp://dem.test"},
		{"API_ELEVATION_BASE_URL", "dem.test/tiles"},
		{"API_ELEVATION_BASE_URL", "https://dem.test?x=1"},
		{"API_ELEVATION_REQUEST_TIMEOUT", "0"},
		{"API_ELEVATION_MAX_BYTES", "0"},
		{"API_ELEVATION_RETRIES", "-1"},
		{"API_ELEVATION_MODEL_VERSION", "  "},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			_, err := platform.LoadTerrainConfig(fakeEnv(map[string]string{tt.key: tt.value}))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "API_TERRAIN_") && !strings.Contains(err.Error(), "API_ELEVATION_") {
				t.Errorf("error does not name the variable: %v", err)
			}
		})
	}
}

// A coherent set of values that the domain rejects as a whole is still an error.
func TestLoadTerrainConfigValidatesTheRulesAsAWhole(t *testing.T) {
	_, err := platform.LoadTerrainConfig(fakeEnv(map[string]string{
		"API_TERRAIN_RESTRICTED_FROM_PCT": "50", "API_TERRAIN_UNSUITABLE_FROM_PCT": "50",
	}))
	if err == nil {
		t.Fatal("restricted share equal to the unsuitable share must be rejected")
	}
}

func TestLoadTerrainConfigAcceptsPlainHTTPOnlyForLoopback(t *testing.T) {
	for _, base := range []string{"http://localhost:9000", "http://127.0.0.1:9000/x", "http://[::1]:9000", "https://dem.test/tiles"} {
		if _, err := platform.LoadTerrainConfig(fakeEnv(map[string]string{"API_ELEVATION_BASE_URL": base})); err != nil {
			t.Errorf("%s: %v", base, err)
		}
	}
}

func TestLoadTerrainConfigRelatesTheTileTimeoutToTheDownloadAttempts(t *testing.T) {
	// 2m x (1 + 2 retries) = 6m fits the 10m default; fewer retries or a shorter request fit a smaller tile timeout.
	ok := []map[string]string{
		{"API_TERRAIN_WORKER_TILE_TIMEOUT": "6m"},
		{"API_TERRAIN_WORKER_TILE_TIMEOUT": "2m", "API_ELEVATION_RETRIES": "0"},
		{"API_TERRAIN_WORKER_TILE_TIMEOUT": "3m", "API_ELEVATION_REQUEST_TIMEOUT": "1m"},
	}
	for _, env := range ok {
		if _, err := platform.LoadTerrainConfig(fakeEnv(env)); err != nil {
			t.Errorf("%v: %v", env, err)
		}
	}
	_, err := platform.LoadTerrainConfig(fakeEnv(map[string]string{"API_TERRAIN_WORKER_TILE_TIMEOUT": "5m"}))
	if err == nil || !strings.Contains(err.Error(), "API_ELEVATION_REQUEST_TIMEOUT") || !strings.Contains(err.Error(), "API_TERRAIN_WORKER_TILE_TIMEOUT") {
		t.Errorf("error must name both timeouts: %v", err)
	}
}
