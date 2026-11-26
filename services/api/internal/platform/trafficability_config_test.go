package platform_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
)

func TestLoadTrafficabilityConfigDefaults(t *testing.T) {
	cfg, err := platform.LoadTrafficabilityConfig(fakeEnv(nil))
	if err != nil {
		t.Fatalf("LoadTrafficabilityConfig: %v", err)
	}
	want := platform.TrafficabilityConfig{
		Weather: platform.WeatherSettings{
			BaseURL: "https://api.open-meteo.com", RequestTimeout: 10 * time.Second, MaxBytes: 1 << 20, CacheTTL: time.Hour,
		},
		GaugeMaxStaleness: 36 * time.Hour, InspectionValidity: 12 * time.Hour,
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadTrafficabilityConfigOverrides(t *testing.T) {
	cfg, err := platform.LoadTrafficabilityConfig(fakeEnv(map[string]string{
		"API_WEATHER_BASE_URL":         "http://127.0.0.1:8099/",
		"API_WEATHER_REQUEST_TIMEOUT":  "3s",
		"API_WEATHER_MAX_BYTES":        "2048",
		"API_WEATHER_CACHE_TTL":        "15m",
		"API_SOIL_GAUGE_MAX_STALENESS": "24h",
		"API_SOIL_INSPECTION_VALIDITY": "6h",
	}))
	if err != nil {
		t.Fatalf("LoadTrafficabilityConfig: %v", err)
	}
	want := platform.TrafficabilityConfig{
		Weather: platform.WeatherSettings{
			BaseURL: "http://127.0.0.1:8099", RequestTimeout: 3 * time.Second, MaxBytes: 2048, CacheTTL: 15 * time.Minute,
		},
		GaugeMaxStaleness: 24 * time.Hour, InspectionValidity: 6 * time.Hour,
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadTrafficabilityConfigRejects(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"plain http to a remote host", "API_WEATHER_BASE_URL", "http://api.open-meteo.com"},
		{"query in the base url", "API_WEATHER_BASE_URL", "https://api.open-meteo.com?x=1"},
		{"zero timeout", "API_WEATHER_REQUEST_TIMEOUT", "0s"},
		{"timeout too long", "API_WEATHER_REQUEST_TIMEOUT", "2m"},
		{"bytes not a number", "API_WEATHER_MAX_BYTES", "1MB"},
		{"zero bytes", "API_WEATHER_MAX_BYTES", "0"},
		{"negative cache ttl", "API_WEATHER_CACHE_TTL", "-1h"},
		{"staleness not a duration", "API_SOIL_GAUGE_MAX_STALENESS", "36"},
		{"zero inspection validity", "API_SOIL_INSPECTION_VALIDITY", "0h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := platform.LoadTrafficabilityConfig(fakeEnv(map[string]string{tt.key: tt.value}))
			if err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Errorf("err = %v, want an error naming %s", err, tt.key)
			}
		})
	}
}
