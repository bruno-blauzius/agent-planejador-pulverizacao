package main

import (
	"fmt"
	"log/slog"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/keycloak"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/objectstorage"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/oidc"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// appConfig groups the configuration of the process and of each adapter. Every
// part reads its own variables: the process (platform.LoadConfig), the
// database, the Keycloak provisioner and the OIDC client. Values that more than
// one part needs (the invitation lifetime) are loaded once and shared.
type appConfig struct {
	platform.Config
	Database platform.DatabaseConfig
	Keycloak keycloak.Config
	OIDC     oidc.Config
	// Documents locates the S3 bucket of owner documents; DocumentsEnabled is
	// false without API_DOCUMENTS_BUCKET (the document routes then answer 503).
	Documents        objectstorage.Config
	DocumentsEnabled bool
	// Terrain is the relief configuration (ADR 0021): rules, worker and elevation source.
	Terrain platform.TerrainConfig
	// Trafficability is the rain and soil configuration (ADR 0022): weather source and validity windows.
	Trafficability platform.TrafficabilityConfig
}

// loadAppConfig reads and validates everything the API needs to start. Errors
// name the failing part and never echo a secret.
func loadAppConfig(lookup platform.LookupFunc) (appConfig, error) {
	base, err := platform.LoadConfig(lookup)
	if err != nil {
		return appConfig{}, fmt.Errorf("process: %w", err)
	}
	database, err := platform.LoadDatabaseConfig(lookup)
	if err != nil {
		return appConfig{}, fmt.Errorf("database: %w", err)
	}
	identity, err := keycloak.ConfigFromEnv(keycloak.LookupFunc(lookup))
	if err != nil {
		return appConfig{}, fmt.Errorf("keycloak provisioner: %w", err)
	}
	// One source of truth: the link of the action e-mail lives as long as the invitation.
	identity.ActionLifespan = base.InvitationTTL

	session, err := oidc.ConfigFromEnv(oidc.LookupFunc(lookup))
	if err != nil {
		return appConfig{}, fmt.Errorf("oidc client: %w", err)
	}
	documents, documentsEnabled, err := objectstorage.ConfigFromEnv(objectstorage.LookupFunc(lookup))
	if err != nil {
		return appConfig{}, fmt.Errorf("owner documents: %w", err)
	}
	terrain, err := platform.LoadTerrainConfig(lookup)
	if err != nil {
		return appConfig{}, fmt.Errorf("terrain: %w", err)
	}
	trafficability, err := platform.LoadTrafficabilityConfig(lookup)
	if err != nil {
		return appConfig{}, fmt.Errorf("trafficability: %w", err)
	}
	return appConfig{
		Config: base, Database: database, Keycloak: identity, OIDC: session,
		Documents: documents, DocumentsEnabled: documentsEnabled, Terrain: terrain, Trafficability: trafficability,
	}, nil
}

// settings builds the business values of the use cases.
func (c appConfig) settings() (usecase.Settings, error) {
	settings := usecase.Settings{
		InvitationTTL:      c.InvitationTTL,
		TrialDuration:      c.TrialDuration,
		LastAccessInterval: c.LastAccessInterval,
		DefaultPageSize:    c.PageSizeDefault,
		MaxPageSize:        c.PageSizeMax,
		Areas: domain.AreaRules{
			MinFarmAreaHa: c.FarmMinAreaHa, MinFieldAreaHa: c.FieldMinAreaHa, ToleranceM2: c.FieldToleranceM2,
		},
		MaxDocumentBytes: c.OwnerDocumentMaxBytes,
	}
	if err := settings.Validate(); err != nil {
		return usecase.Settings{}, fmt.Errorf("settings: %w", err)
	}
	return settings, nil
}

// logValue describes the configuration for the startup log: no secrets, no
// keys, no connection password (the database config redacts its URL itself).
func (c appConfig) logValue() slog.Value {
	return slog.GroupValue(
		slog.Int("port", c.Port),
		slog.Bool("docs_enabled", c.DocsEnabled),
		slog.Any("cors_allowed_origins", c.CORSAllowedOrigins),
		slog.Any("database", c.Database),
		slog.Group("keycloak",
			slog.String("url", c.Keycloak.BaseURL),
			slog.String("realm", c.Keycloak.Realm),
			slog.String("issuer", c.OIDC.IssuerURL),
		),
		slog.Group("session",
			slog.Duration("ttl", c.OIDC.SessionTTL),
		),
		slog.Duration("invitation_ttl", c.InvitationTTL),
		slog.Group("terrain",
			slog.Bool("worker_enabled", c.Terrain.Worker.Enabled),
			slog.Int("worker_concurrency", c.Terrain.Worker.Concurrency),
			slog.String("elevation_url", c.Terrain.Elevation.BaseURL),
		),
		slog.Group("trafficability",
			slog.String("weather_url", c.Trafficability.Weather.BaseURL),
			slog.Duration("weather_cache_ttl", c.Trafficability.Weather.CacheTTL),
			slog.Duration("gauge_max_staleness", c.Trafficability.GaugeMaxStaleness),
			slog.Duration("inspection_validity", c.Trafficability.InspectionValidity),
		),
	)
}
