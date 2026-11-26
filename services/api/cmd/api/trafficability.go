package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	// The farm time zone must load in minimal images without /usr/share/zoneinfo.
	_ "time/tzdata"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/openmeteo"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// farmTimeZone is the zone of the local days of rain and soil (ADR 0022,
// addendum item 8): the farms of the product are in Brazil's official time.
const farmTimeZone = "America/Sao_Paulo"

// conditionsStorage is what the rain and soil use cases consume from
// persistence: the Postgres database in production, the in-memory store in tests.
type conditionsStorage interface {
	usecase.RainGaugeReader
	usecase.FieldConditionsReader
	usecase.TrafficabilityThresholdsReader
	usecase.FieldConditionsTxRunner
}

// trafficabilitySettings converts the configuration into the use case settings.
func trafficabilitySettings(cfg platform.TrafficabilityConfig) (usecase.TrafficabilitySettings, error) {
	location, err := time.LoadLocation(farmTimeZone)
	if err != nil {
		return usecase.TrafficabilitySettings{}, fmt.Errorf("loading the time zone %s: %w", farmTimeZone, err)
	}
	settings := usecase.TrafficabilitySettings{
		Location: location, GaugeMaxStaleness: cfg.GaugeMaxStaleness, InspectionValidity: cfg.InspectionValidity,
	}
	if err := settings.Validate(); err != nil {
		return usecase.TrafficabilitySettings{}, fmt.Errorf("trafficability settings: %w", err)
	}
	return settings, nil
}

// newWeatherProvider builds the Open-Meteo client behind its in-memory cache.
// Redirects follow the same rule as the elevation source: never to http nor
// to another host.
func newWeatherProvider(cfg platform.WeatherSettings, location *time.Location, clock usecase.Clock, logger *slog.Logger) (usecase.WeatherProvider, error) {
	client, err := openmeteo.NewClient(openmeteo.Config{
		BaseURL: cfg.BaseURL, RequestTimeout: cfg.RequestTimeout, MaxBytes: cfg.MaxBytes,
		Location: location, Logger: logger,
	}, &http.Client{CheckRedirect: refuseUnsafeRedirect})
	if err != nil {
		return nil, fmt.Errorf("building the weather client: %w", err)
	}
	return openmeteo.NewCache(client, cfg.CacheTTL, clock, location), nil
}
