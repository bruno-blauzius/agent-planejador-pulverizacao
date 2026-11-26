package platform

import (
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Defaults of the relief executor and of the elevation source. They repeat the
// defaults of usecase.TerrainWorkerConfig and of the copernicus client because
// platform imports neither layer (the composition root converts); the
// copernicus package tests that the elevation values do not drift apart.
const (
	defaultTerrainWorkerEnabled       = true
	defaultTerrainWorkerConcurrency   = 1
	defaultTerrainWorkerSweepInterval = 30 * time.Second
	defaultTerrainWorkerTileTimeout   = 10 * time.Minute
	defaultTerrainWorkerFieldTimeout  = 2 * time.Minute
	defaultTerrainWorkerLeaseSlack    = 2 * time.Minute

	// maxPositionRadiusM caps API_TERRAIN_POSITION_RADIUS_M: the window grows with the
	// square of the radius and is read into memory, so a typo must not ask for a
	// continent. 5 km is far beyond a useful neighborhood for hilltop and lowland.
	maxPositionRadiusM = 5000.0

	defaultElevationBaseURL        = "https://copernicus-dem-30m.s3.amazonaws.com"
	defaultElevationRequestTimeout = 2 * time.Minute
	defaultElevationMaxBytes       = int64(120 << 20)
	defaultElevationRetries        = 2
	defaultElevationModelVersion   = "GLO-30 COG 2022-05"
)

// TerrainWorkerSettings configures the background relief executor.
type TerrainWorkerSettings struct {
	// Enabled starts the executor with the API (API_TERRAIN_WORKER_ENABLED).
	Enabled bool
	// Concurrency is the number of fields calculated at once (API_TERRAIN_WORKER_CONCURRENCY).
	Concurrency int
	// SweepInterval is how often idle goroutines look for pending fields (API_TERRAIN_WORKER_SWEEP_INTERVAL).
	SweepInterval time.Duration
	// TileTimeout bounds the download and ingestion of ONE elevation block (API_TERRAIN_WORKER_TILE_TIMEOUT).
	TileTimeout time.Duration
	// FieldTimeout bounds the bounds query, the window cut and the calculation of a field
	// (API_TERRAIN_WORKER_FIELD_TIMEOUT).
	FieldTimeout time.Duration
	// LeaseSlack is added to TileTimeout + FieldTimeout to form the lease after which a
	// calculating field counts as abandoned (API_TERRAIN_WORKER_LEASE_SLACK).
	LeaseSlack time.Duration
}

// ElevationSettings configures the elevation model source.
type ElevationSettings struct {
	BaseURL        string        // API_ELEVATION_BASE_URL
	RequestTimeout time.Duration // API_ELEVATION_REQUEST_TIMEOUT, per attempt
	MaxBytes       int64         // API_ELEVATION_MAX_BYTES, ceiling of one tile
	// Retries are the extra attempts after the first (API_ELEVATION_RETRIES).
	// Zero means no retry; the copernicus client reads 0 as "use the default",
	// so the composition root must pass a negative value for it.
	Retries      int
	ModelVersion string // API_ELEVATION_MODEL_VERSION
}

// TerrainConfig is the configuration of the relief feature (ADR 0021).
type TerrainConfig struct {
	Rules     domain.TerrainRules
	Worker    TerrainWorkerSettings
	Elevation ElevationSettings
}

// LoadTerrainConfig reads the relief configuration from the environment:
//
//	API_TERRAIN_CLASS_LIMITS_PCT         "3,8,20,45"   upper limits of the first four Embrapa classes
//	API_TERRAIN_FLAT_ASPECT_BELOW_PCT    3             mean slope below which there is no aspect
//	API_TERRAIN_POSITION_RADIUS_M        500           radius used to classify the position (at most 5000)
//	API_TERRAIN_POSITION_TOLERANCE_M     5             elevation gap for hilltop and lowland
//	API_TERRAIN_PROFILE_POINTS           24            strips of the elevation profile
//	API_TERRAIN_MIN_VALID_CELLS          20            cells with slope needed to accept a calculation
//	API_TERRAIN_ADJUSTMENT_DIVERGENCE_PCT 5            gap that flags an adjustment as outdated
//	API_TERRAIN_METHOD_SLOPE_LIMITS      "tractor_boom=20,self_propelled=15,airblast=20" (empty: no limits)
//	API_TERRAIN_RESTRICTED_FROM_PCT      10            area share above the limit that makes a method restricted
//	API_TERRAIN_UNSUITABLE_FROM_PCT      50            area share that makes it unsuitable
//	API_TERRAIN_WORKER_ENABLED           true
//	API_TERRAIN_WORKER_CONCURRENCY       1
//	API_TERRAIN_WORKER_SWEEP_INTERVAL    30s
//	API_TERRAIN_WORKER_TILE_TIMEOUT      10m
//	API_TERRAIN_WORKER_FIELD_TIMEOUT     2m
//	API_TERRAIN_WORKER_LEASE_SLACK       2m
//	API_ELEVATION_BASE_URL               https://copernicus-dem-30m.s3.amazonaws.com
//	API_ELEVATION_REQUEST_TIMEOUT        2m
//	API_ELEVATION_MAX_BYTES              125829120
//	API_ELEVATION_RETRIES                2
//	API_ELEVATION_MODEL_VERSION          "GLO-30 COG 2022-05"
func LoadTerrainConfig(lookup LookupFunc) (TerrainConfig, error) {
	cfg := TerrainConfig{
		Rules: domain.DefaultTerrainRules(),
		Worker: TerrainWorkerSettings{
			Enabled: defaultTerrainWorkerEnabled, Concurrency: defaultTerrainWorkerConcurrency,
			SweepInterval: defaultTerrainWorkerSweepInterval, TileTimeout: defaultTerrainWorkerTileTimeout,
			FieldTimeout: defaultTerrainWorkerFieldTimeout, LeaseSlack: defaultTerrainWorkerLeaseSlack,
		},
		Elevation: ElevationSettings{
			BaseURL: defaultElevationBaseURL, RequestTimeout: defaultElevationRequestTimeout,
			MaxBytes: defaultElevationMaxBytes, Retries: defaultElevationRetries, ModelVersion: defaultElevationModelVersion,
		},
	}
	if err := loadTerrainRules(lookup, &cfg.Rules); err != nil {
		return TerrainConfig{}, err
	}
	if err := cfg.Rules.Validate(); err != nil {
		return TerrainConfig{}, fmt.Errorf("invalid API_TERRAIN_* configuration: %w", err)
	}
	if err := loadTerrainWorker(lookup, &cfg.Worker); err != nil {
		return TerrainConfig{}, err
	}
	if err := loadElevation(lookup, &cfg.Elevation); err != nil {
		return TerrainConfig{}, err
	}
	if err := validateTerrainTimeouts(cfg.Worker, cfg.Elevation); err != nil {
		return TerrainConfig{}, err
	}
	return cfg, nil
}

// validateTerrainTimeouts checks that the time given to a block covers the
// download attempts the source may make: the request timeout is per attempt, and
// every retry starts a new one, all inside the single TileTimeout.
func validateTerrainTimeouts(worker TerrainWorkerSettings, elevation ElevationSettings) error {
	if worker.TileTimeout < elevation.RequestTimeout {
		return fmt.Errorf("invalid configuration: API_TERRAIN_WORKER_TILE_TIMEOUT (%s) must be at least API_ELEVATION_REQUEST_TIMEOUT (%s)",
			worker.TileTimeout, elevation.RequestTimeout)
	}
	if attempts := time.Duration(1 + elevation.Retries); elevation.RequestTimeout*attempts > worker.TileTimeout {
		return fmt.Errorf("invalid configuration: API_ELEVATION_REQUEST_TIMEOUT (%s) x %d attempts (API_ELEVATION_RETRIES=%d) exceeds API_TERRAIN_WORKER_TILE_TIMEOUT (%s); raise the tile timeout or lower the timeout or the retries",
			elevation.RequestTimeout, attempts, elevation.Retries, worker.TileTimeout)
	}
	return nil
}

func loadTerrainRules(lookup LookupFunc, rules *domain.TerrainRules) error {
	if raw, ok := lookup("API_TERRAIN_CLASS_LIMITS_PCT"); ok {
		limits, err := parseClassLimits(raw)
		if err != nil {
			return fmt.Errorf("invalid API_TERRAIN_CLASS_LIMITS_PCT %q: %w", raw, err)
		}
		rules.ClassUpperLimitsPct = limits
	}
	decimals := []struct {
		key    string
		target *float64
	}{
		{"API_TERRAIN_FLAT_ASPECT_BELOW_PCT", &rules.FlatAspectBelowPct},
		{"API_TERRAIN_POSITION_RADIUS_M", &rules.PositionRadiusM},
		{"API_TERRAIN_POSITION_TOLERANCE_M", &rules.PositionToleranceM},
		{"API_TERRAIN_ADJUSTMENT_DIVERGENCE_PCT", &rules.AdjustmentDivergencePct},
		{"API_TERRAIN_RESTRICTED_FROM_PCT", &rules.RestrictedFromSharePct},
		{"API_TERRAIN_UNSUITABLE_FROM_PCT", &rules.UnsuitableFromSharePct},
	}
	for _, decimal := range decimals {
		if err := readPositiveDecimal(lookup, decimal.key, decimal.target); err != nil {
			return err
		}
	}
	if rules.PositionRadiusM > maxPositionRadiusM {
		return fmt.Errorf("invalid API_TERRAIN_POSITION_RADIUS_M %v: must be at most %v meters", rules.PositionRadiusM, maxPositionRadiusM)
	}
	counts := []struct {
		key    string
		target *int
	}{
		{"API_TERRAIN_PROFILE_POINTS", &rules.ProfilePoints},
		{"API_TERRAIN_MIN_VALID_CELLS", &rules.MinValidCells},
	}
	for _, count := range counts {
		if err := readPositiveInt(lookup, count.key, count.target); err != nil {
			return err
		}
	}
	if raw, ok := lookup("API_TERRAIN_METHOD_SLOPE_LIMITS"); ok {
		limits, err := parseMethodSlopeLimits(raw)
		if err != nil {
			return fmt.Errorf("invalid API_TERRAIN_METHOD_SLOPE_LIMITS %q: %w", raw, err)
		}
		rules.MethodSlopeLimits = limits
	}
	return nil
}

func loadTerrainWorker(lookup LookupFunc, worker *TerrainWorkerSettings) error {
	if raw, ok := lookup("API_TERRAIN_WORKER_ENABLED"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("invalid API_TERRAIN_WORKER_ENABLED %q: must be true or false", raw)
		}
		worker.Enabled = enabled
	}
	if err := readPositiveInt(lookup, "API_TERRAIN_WORKER_CONCURRENCY", &worker.Concurrency); err != nil {
		return err
	}
	if err := readPositiveDuration(lookup, "API_TERRAIN_WORKER_SWEEP_INTERVAL", &worker.SweepInterval); err != nil {
		return err
	}
	if err := readPositiveDuration(lookup, "API_TERRAIN_WORKER_TILE_TIMEOUT", &worker.TileTimeout); err != nil {
		return err
	}
	if err := readPositiveDuration(lookup, "API_TERRAIN_WORKER_FIELD_TIMEOUT", &worker.FieldTimeout); err != nil {
		return err
	}
	return readPositiveDuration(lookup, "API_TERRAIN_WORKER_LEASE_SLACK", &worker.LeaseSlack)
}

func loadElevation(lookup LookupFunc, elevation *ElevationSettings) error {
	if raw, ok := lookup("API_ELEVATION_BASE_URL"); ok {
		baseURL, err := parseBaseURL(raw)
		if err != nil {
			return fmt.Errorf("invalid API_ELEVATION_BASE_URL %q: %w", raw, err)
		}
		elevation.BaseURL = baseURL
	}
	if err := readPositiveDuration(lookup, "API_ELEVATION_REQUEST_TIMEOUT", &elevation.RequestTimeout); err != nil {
		return err
	}
	if raw, ok := lookup("API_ELEVATION_MAX_BYTES"); ok {
		value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || value < 1 {
			return fmt.Errorf("invalid API_ELEVATION_MAX_BYTES %q: must be a positive integer (bytes)", raw)
		}
		elevation.MaxBytes = value
	}
	if raw, ok := lookup("API_ELEVATION_RETRIES"); ok {
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || value < 0 {
			return fmt.Errorf("invalid API_ELEVATION_RETRIES %q: must be an integer greater than or equal to 0", raw)
		}
		elevation.Retries = value
	}
	if raw, ok := lookup("API_ELEVATION_MODEL_VERSION"); ok {
		version := strings.TrimSpace(raw)
		if version == "" {
			return fmt.Errorf("invalid API_ELEVATION_MODEL_VERSION %q: must not be empty", raw)
		}
		elevation.ModelVersion = version
	}
	return nil
}

func readPositiveDecimal(lookup LookupFunc, key string, target *float64) error {
	raw, ok := lookup(key)
	if !ok {
		return nil
	}
	value, err := parseDecimal(raw)
	if err != nil || value <= 0 {
		return fmt.Errorf("invalid %s %q: must be a positive decimal such as 5", key, raw)
	}
	*target = value
	return nil
}

func readPositiveInt(lookup LookupFunc, key string, target *int) error {
	raw, ok := lookup(key)
	if !ok {
		return nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return fmt.Errorf("invalid %s %q: must be a positive integer", key, raw)
	}
	*target = value
	return nil
}

func readPositiveDuration(lookup LookupFunc, key string, target *time.Duration) error {
	raw, ok := lookup(key)
	if !ok {
		return nil
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fmt.Errorf("invalid %s %q: must be a positive duration such as 30s", key, raw)
	}
	*target = value
	return nil
}

// parseDecimal reads a finite decimal.
func parseDecimal(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%q is not a finite number", raw)
	}
	return value, nil
}

// parseClassLimits reads exactly four limits such as "3,8,20,45"; whether they
// grow is checked by TerrainRules.Validate.
func parseClassLimits(raw string) ([4]float64, error) {
	var limits [4]float64
	parts := strings.Split(raw, ",")
	if len(parts) != len(limits) {
		return limits, fmt.Errorf("want 4 comma separated limits such as 3,8,20,45, got %d", len(parts))
	}
	for i, part := range parts {
		value, err := parseDecimal(part)
		if err != nil {
			return limits, fmt.Errorf("limit %d: %w", i+1, err)
		}
		limits[i] = value
	}
	return limits, nil
}

// parseMethodSlopeLimits reads "method=limit" pairs. A method that is not
// listed has no slope limit; an empty value removes every limit.
func parseMethodSlopeLimits(raw string) (map[domain.SprayMethod]*float64, error) {
	limits := make(map[domain.SprayMethod]*float64)
	if strings.TrimSpace(raw) == "" {
		return limits, nil
	}
	known := domain.SprayMethods()
	for _, pair := range strings.Split(raw, ",") {
		name, text, found := strings.Cut(pair, "=")
		method := domain.SprayMethod(strings.TrimSpace(name))
		if !found || !slices.Contains(known, method) {
			return nil, fmt.Errorf("%q must look like method=limit with a method among %v", strings.TrimSpace(pair), known)
		}
		if _, repeated := limits[method]; repeated {
			return nil, fmt.Errorf("method %s listed twice", method)
		}
		value, err := parseDecimal(text)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("limit of %s must be a positive decimal", method)
		}
		limits[method] = &value
	}
	return limits, nil
}

// parseBaseURL accepts an https address without query or fragment and drops the
// trailing slash. Plain http is accepted only for a loopback host, for local
// tests: the GeoTIFF that comes from this address is parsed by GDAL inside the
// database process (ADR 0021), so it must not travel over a network in the clear.
func parseBaseURL(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	parsed, err := url.Parse(text)
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return "", fmt.Errorf("must look like https://host/path (no query, fragment or credentials)")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return "", fmt.Errorf("must use https (http is accepted only for localhost, 127.0.0.1 or ::1)")
	}
	return strings.TrimRight(text, "/"), nil
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
