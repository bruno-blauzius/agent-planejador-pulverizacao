package main

import (
	"fmt"
	"log/slog"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/keycloak"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/oidc"
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
	return appConfig{Config: base, Database: database, Keycloak: identity, OIDC: session}, nil
}

// settings builds the business values of the use cases.
func (c appConfig) settings() (usecase.Settings, error) {
	settings := usecase.Settings{
		InvitationTTL:      c.InvitationTTL,
		TrialDuration:      c.TrialDuration,
		LastAccessInterval: c.LastAccessInterval,
		DefaultPageSize:    c.PageSizeDefault,
		MaxPageSize:        c.PageSizeMax,
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
	)
}
