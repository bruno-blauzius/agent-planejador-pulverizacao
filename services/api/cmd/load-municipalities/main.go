// Command load-municipalities refreshes the IBGE municipal registry of the API
// database (ADR 0019): it downloads names and boundaries from the public data
// service of IBGE and replaces the municipalities table in one transaction.
//
// It writes reference data, so it connects as api_migrator (the API runs as
// api_app, which only reads the table). Run it with make api-load-municipalities.
//
// Environment:
//
//	API_MIGRATOR_DATABASE_URL     connection of api_migrator (required; never logged)
//	API_IBGE_BASE_URL             default https://servicodados.ibge.gov.br
//	API_IBGE_MESH_QUALITY         minima, intermediaria (default) or maxima
//	API_IBGE_MIN_MUNICIPALITIES   smallest acceptable registry (default 5500)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/ibge"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	defaultMinMunicipalities = 5500
	downloadTimeout          = 10 * time.Minute
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "load-municipalities:", err)
		os.Exit(1)
	}
}

type config struct {
	database platform.DatabaseConfig
	baseURL  string
	quality  string
	minimum  int
}

func loadConfig(lookup platform.LookupFunc) (config, error) {
	databaseURL, ok := lookup("API_MIGRATOR_DATABASE_URL")
	if !ok || databaseURL == "" {
		return config{}, errors.New("API_MIGRATOR_DATABASE_URL is required (connection of api_migrator)")
	}
	database, err := platform.LoadDatabaseConfig(func(key string) (string, bool) {
		if key == "API_DATABASE_URL" {
			return databaseURL, true
		}
		return "", false
	})
	if err != nil {
		return config{}, fmt.Errorf("database: %w", err)
	}
	// Each municipality is one statement, so the default statement timeout bounds every upsert.
	cfg := config{database: database, baseURL: ibge.DefaultBaseURL, quality: ibge.QualityIntermediate, minimum: defaultMinMunicipalities}
	if value, ok := lookup("API_IBGE_BASE_URL"); ok && value != "" {
		cfg.baseURL = value
	}
	if value, ok := lookup("API_IBGE_MESH_QUALITY"); ok && value != "" {
		cfg.quality = value
	}
	if value, ok := lookup("API_IBGE_MIN_MUNICIPALITIES"); ok && value != "" {
		minimum, err := strconv.Atoi(value)
		if err != nil || minimum < 1 {
			return config{}, fmt.Errorf("invalid API_IBGE_MIN_MUNICIPALITIES %q: must be a positive integer", value)
		}
		cfg.minimum = minimum
	}
	return cfg, nil
}

func run() error {
	logger := platform.NewLogger(os.Stdout, slog.LevelInfo)
	cfg, err := loadConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	skip := func(code, name, reason string) {
		logger.Warn("municipality left out", slog.String("code", code), slog.String("name", name), slog.String("reason", reason))
	}
	source, err := ibge.NewClient(&http.Client{Timeout: downloadTimeout}, cfg.baseURL, cfg.quality, skip)
	if err != nil {
		return err
	}
	pool, err := postgres.NewPool(ctx, cfg.database)
	if err != nil {
		return fmt.Errorf("database pool: %w", err)
	}
	defer pool.Close()
	database := postgres.NewDatabase(pool, postgres.Options{})

	logger.Info("downloading the IBGE municipal registry", slog.String("source", cfg.baseURL), slog.String("quality", cfg.quality))
	started := time.Now()
	count, err := usecase.NewLoadMunicipalities(source, database, cfg.minimum).Execute(ctx)
	if err != nil {
		return err
	}
	logger.Info("municipal registry loaded", slog.Int("municipalities", count), slog.Duration("took", time.Since(started)))
	return nil
}
