package platform

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Defaults of the trafficability feature (ADR 0022, addendum). The weather
// values repeat the defaults of the openmeteo client because platform does not
// import adapters; the openmeteo package tests that they do not drift apart.
const (
	defaultWeatherBaseURL        = "https://api.open-meteo.com"
	defaultWeatherRequestTimeout = 10 * time.Second
	defaultWeatherMaxBytes       = int64(1 << 20)
	defaultWeatherCacheTTL       = time.Hour

	defaultSoilGaugeMaxStaleness  = 36 * time.Hour
	defaultSoilInspectionValidity = 12 * time.Hour

	defaultSoilHistoryEnabled       = true
	defaultSoilHistoryHour          = 7
	defaultSoilHistoryConcurrency   = 4
	defaultSoilHistorySweepInterval = 5 * time.Minute
	defaultSoilHistoryFieldTimeout  = 30 * time.Second
	// maxSoilHistoryConcurrency keeps the daily executor from taking the
	// connections of the pool away from the screens.
	maxSoilHistoryConcurrency = 32

	// maxWeatherRequestTimeout keeps a slow provider from holding the request
	// of the screen (and a connection of the pool) for minutes.
	maxWeatherRequestTimeout = time.Minute
)

// WeatherSettings configures the weather model client (Open-Meteo).
type WeatherSettings struct {
	BaseURL        string        // API_WEATHER_BASE_URL
	RequestTimeout time.Duration // API_WEATHER_REQUEST_TIMEOUT
	MaxBytes       int64         // API_WEATHER_MAX_BYTES, ceiling of one answer
	CacheTTL       time.Duration // API_WEATHER_CACHE_TTL, per cell and local day
}

// SoilHistorySettings configures the daily soil history executor (ADR 0022, phase 4).
type SoilHistorySettings struct {
	Enabled       bool          // API_SOIL_HISTORY_ENABLED
	Hour          int           // API_SOIL_HISTORY_HOUR, local hour (0 to 23) from which the day is recorded
	Concurrency   int           // API_SOIL_HISTORY_CONCURRENCY, fields evaluated at once (1 to 32)
	SweepInterval time.Duration // API_SOIL_HISTORY_SWEEP_INTERVAL, how often the executor checks the schedule
	FieldTimeout  time.Duration // API_SOIL_HISTORY_FIELD_TIMEOUT, evaluation and save of one field
}

// TrafficabilityConfig is the configuration of rain, soil and trafficability.
type TrafficabilityConfig struct {
	Weather WeatherSettings
	History SoilHistorySettings
	// GaugeMaxStaleness: a gauge without a reading this recent does not count (API_SOIL_GAUGE_MAX_STALENESS).
	GaugeMaxStaleness time.Duration
	// InspectionValidity: how long a soil inspection replaces the calculation (API_SOIL_INSPECTION_VALIDITY).
	InspectionValidity time.Duration
}

// LoadTrafficabilityConfig reads the trafficability configuration:
//
//	API_WEATHER_BASE_URL          https://api.open-meteo.com   (https; http only for localhost)
//	API_WEATHER_REQUEST_TIMEOUT   10s                          (at most 1m)
//	API_WEATHER_MAX_BYTES         1048576
//	API_WEATHER_CACHE_TTL         1h
//	API_SOIL_GAUGE_MAX_STALENESS  36h
//	API_SOIL_INSPECTION_VALIDITY  12h
//	API_SOIL_HISTORY_ENABLED      true
//	API_SOIL_HISTORY_HOUR         7                            (0 to 23, America/Sao_Paulo)
//	API_SOIL_HISTORY_CONCURRENCY  4                            (1 to 32)
//	API_SOIL_HISTORY_SWEEP_INTERVAL 5m
//	API_SOIL_HISTORY_FIELD_TIMEOUT  30s
func LoadTrafficabilityConfig(lookup LookupFunc) (TrafficabilityConfig, error) {
	cfg := TrafficabilityConfig{
		Weather: WeatherSettings{
			BaseURL: defaultWeatherBaseURL, RequestTimeout: defaultWeatherRequestTimeout,
			MaxBytes: defaultWeatherMaxBytes, CacheTTL: defaultWeatherCacheTTL,
		},
		GaugeMaxStaleness:  defaultSoilGaugeMaxStaleness,
		InspectionValidity: defaultSoilInspectionValidity,
		History: SoilHistorySettings{
			Enabled: defaultSoilHistoryEnabled, Hour: defaultSoilHistoryHour, Concurrency: defaultSoilHistoryConcurrency,
			SweepInterval: defaultSoilHistorySweepInterval, FieldTimeout: defaultSoilHistoryFieldTimeout,
		},
	}
	if raw, ok := lookup("API_WEATHER_BASE_URL"); ok {
		baseURL, err := parseBaseURL(raw)
		if err != nil {
			return TrafficabilityConfig{}, fmt.Errorf("invalid API_WEATHER_BASE_URL %q: %w", raw, err)
		}
		cfg.Weather.BaseURL = baseURL
	}
	durations := []struct {
		key    string
		target *time.Duration
	}{
		{"API_WEATHER_REQUEST_TIMEOUT", &cfg.Weather.RequestTimeout},
		{"API_WEATHER_CACHE_TTL", &cfg.Weather.CacheTTL},
		{"API_SOIL_GAUGE_MAX_STALENESS", &cfg.GaugeMaxStaleness},
		{"API_SOIL_INSPECTION_VALIDITY", &cfg.InspectionValidity},
		{"API_SOIL_HISTORY_SWEEP_INTERVAL", &cfg.History.SweepInterval},
		{"API_SOIL_HISTORY_FIELD_TIMEOUT", &cfg.History.FieldTimeout},
	}
	for _, duration := range durations {
		if err := readPositiveDuration(lookup, duration.key, duration.target); err != nil {
			return TrafficabilityConfig{}, err
		}
	}
	if cfg.Weather.RequestTimeout > maxWeatherRequestTimeout {
		return TrafficabilityConfig{}, fmt.Errorf("invalid API_WEATHER_REQUEST_TIMEOUT %s: must be at most %s", cfg.Weather.RequestTimeout, maxWeatherRequestTimeout)
	}
	if raw, ok := lookup("API_WEATHER_MAX_BYTES"); ok {
		value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || value < 1 {
			return TrafficabilityConfig{}, fmt.Errorf("invalid API_WEATHER_MAX_BYTES %q: must be a positive integer (bytes)", raw)
		}
		cfg.Weather.MaxBytes = value
	}
	if err := loadSoilHistory(lookup, &cfg.History); err != nil {
		return TrafficabilityConfig{}, err
	}
	return cfg, nil
}

func loadSoilHistory(lookup LookupFunc, history *SoilHistorySettings) error {
	if raw, ok := lookup("API_SOIL_HISTORY_ENABLED"); ok {
		enabled, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("invalid API_SOIL_HISTORY_ENABLED %q: must be true or false", raw)
		}
		history.Enabled = enabled
	}
	if raw, ok := lookup("API_SOIL_HISTORY_HOUR"); ok {
		hour, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || hour < 0 || hour > 23 {
			return fmt.Errorf("invalid API_SOIL_HISTORY_HOUR %q: must be an hour between 0 and 23", raw)
		}
		history.Hour = hour
	}
	if err := readPositiveInt(lookup, "API_SOIL_HISTORY_CONCURRENCY", &history.Concurrency); err != nil {
		return err
	}
	if history.Concurrency > maxSoilHistoryConcurrency {
		return fmt.Errorf("invalid API_SOIL_HISTORY_CONCURRENCY %d: must be at most %d", history.Concurrency, maxSoilHistoryConcurrency)
	}
	return nil
}
