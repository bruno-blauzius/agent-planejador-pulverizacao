package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

func TestTrafficabilityRoutesAreMountedAndProtected(t *testing.T) {
	handler, _ := composed(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/farms/f1/rain-gauges"},
		{http.MethodPost, "/farms/f1/rain-gauges"},
		{http.MethodPatch, "/farms/f1/rain-gauges/g1"},
		{http.MethodGet, "/farms/f1/rain-gauges/g1/readings"},
		{http.MethodPost, "/farms/f1/rain-gauges/g1/readings"},
		{http.MethodPost, "/farms/f1/fields/t1/soil-inspections"},
		{http.MethodGet, "/farms/f1/fields/t1/soil-profile"},
		{http.MethodPut, "/farms/f1/fields/t1/soil-profile"},
		{http.MethodGet, "/farms/f1/fields/t1/trafficability"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			if status, _ := send(handler, "", route.method, route.path, nil); status != http.StatusUnauthorized {
				t.Errorf("anonymous: status %d, want 401 (a 404 would mean the route is not mounted)", status)
			}
		})
	}
}

func TestLoadAppConfigRejectsInvalidTrafficabilityConfig(t *testing.T) {
	for _, tc := range []struct{ name, variable, value string }{
		{"plain http weather url", "API_WEATHER_BASE_URL", "http://api.open-meteo.com"},
		{"zero cache ttl", "API_WEATHER_CACHE_TTL", "0s"},
		{"staleness not a duration", "API_SOIL_GAUGE_MAX_STALENESS", "36"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := validAppEnv()
			env[tc.variable] = tc.value
			_, err := loadAppConfig(func(name string) (string, bool) {
				value, ok := env[name]
				return value, ok
			})
			if err == nil || !strings.Contains(err.Error(), "trafficability") || !strings.Contains(err.Error(), tc.variable) {
				t.Errorf("error %v must name the trafficability config and %s", err, tc.variable)
			}
		})
	}
}

func TestTrafficabilitySettingsUseTheFarmTimeZone(t *testing.T) {
	cfg, err := platform.LoadTrafficabilityConfig(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	settings, err := trafficabilitySettings(cfg)
	if err != nil {
		t.Fatalf("trafficabilitySettings: %v", err)
	}
	if settings.Location.String() != "America/Sao_Paulo" || settings.GaugeMaxStaleness != 36*time.Hour || settings.InspectionValidity != 12*time.Hour {
		t.Errorf("settings = %+v", settings)
	}
	provider, err := newWeatherProvider(cfg.Weather, settings.Location, &fakes.FixedClock{T: time.Now()}, nil)
	if err != nil || provider == nil {
		t.Fatalf("weather provider: %v", err)
	}
}
