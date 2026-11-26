// Command api is the composition root: it is the only place where
// dependencies are instantiated and wired together.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	apispec "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/api"
	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/keycloak"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/objectstorage"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/oidc"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/perimeter"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// documentStorage builds the S3 store of owner documents, or the disabled one
// when no bucket is configured (document routes answer 503; the rest works).
func documentStorage(ctx context.Context, cfg appConfig, logger *slog.Logger) (usecase.DocumentStorage, error) {
	if !cfg.DocumentsEnabled {
		logger.Warn("owner documents disabled: API_DOCUMENTS_BUCKET is not set")
		return objectstorage.Disabled{}, nil
	}
	store, err := objectstorage.NewS3Store(ctx, cfg.Documents)
	if err != nil {
		return nil, fmt.Errorf("owner document storage: %w", err)
	}
	return store, nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadAppConfig(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	settings, err := cfg.settings()
	if err != nil {
		return err
	}
	logger := platform.NewLogger(os.Stdout, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The pool uses the runtime role (api_app): DML only, never DDL.
	pool, err := postgres.NewPool(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}
	defer pool.Close()
	database := postgres.NewDatabase(pool, postgres.Options{SessionTouchInterval: cfg.Database.SessionTouchInterval})

	cfg.Keycloak.Logger = logger
	identity, err := keycloak.New(cfg.Keycloak)
	if err != nil {
		return fmt.Errorf("building the keycloak adapter: %w", err)
	}

	clock := platform.SystemClock{}
	ids := platform.UUIDGenerator{}

	oidcService, err := oidc.NewService(cfg.OIDC, oidc.Deps{
		Sessions:     database,
		Resolver:     usecase.NewResolvePrincipal(database, database, database, database, clock, settings),
		Identities:   database,
		UserSessions: database,
		Clock:        clock,
		IDs:          ids,
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("building the oidc service: %w", err)
	}
	sessionConfig := oidcService.Config()

	documents, err := documentStorage(ctx, cfg, logger)
	if err != nil {
		return err
	}
	// The relief executor reads and writes through the same pool and downloads
	// blocks from the elevation source; the use cases only get its wakeup.
	terrain, err := buildTerrainRuntime(cfg.Terrain, database, database, newCopernicusSource(cfg.Terrain.Elevation), clock, logger)
	if err != nil {
		return err
	}
	trafficability, err := trafficabilitySettings(cfg.Trafficability)
	if err != nil {
		return err
	}
	weather, err := newWeatherProvider(cfg.Trafficability.Weather, trafficability.Location, clock, logger)
	if err != nil {
		return err
	}
	wired := dependencies{
		Logger: logger, Store: database, Registry: database, PerimeterParser: perimeter.NewParser(),
		Documents: documents, Identity: identity, Clock: clock, IDs: ids, Settings: settings,
		Terrain: database, TerrainRules: cfg.Terrain.Rules, TerrainWakeup: terrain.Wakeup,
		Conditions: database, Weather: weather, Trafficability: trafficability,
	}
	groups := newRouteGroups(wired)
	// The daily soil history runs the same trafficability core as the GET route.
	history, err := buildSoilHistoryRuntime(cfg.Trafficability.History, trafficability.Location, database,
		usecase.NewTrafficabilityEvaluator(wired.trafficabilityDeps()), clock, logger)
	if err != nil {
		return err
	}
	groups = append(groups, httpadapter.NewAuthHandlers(httpadapter.AuthHandlerDeps{
		Logger: logger,
		Flow:   oidcService,
		Cookies: httpadapter.AuthCookies{
			Session: sessionConfig.SessionCookieName, Flow: sessionConfig.FlowCookieName, Reauth: sessionConfig.ReauthCookieName,
			SessionTTL: sessionConfig.SessionTTL, FlowTTL: sessionConfig.FlowTTL,
		},
		AllowedOrigins: cfg.CORSAllowedOrigins,
		WebURL:         sessionConfig.WebURL,
	}))

	lifecycle := usecase.NewLifecycle()
	deps := httpadapter.RouterDeps{
		Logger:    logger,
		Liveness:  usecase.CheckLiveness{}, // never checks dependencies
		Startup:   usecase.NewCheckStartup(lifecycle),
		Readiness: usecase.NewCheckReadiness(lifecycle, logger, postgres.NewProbe(database)),
		// ProbeTimeout bounds the context of every readiness check, including the database ping.
		ProbeTimeout: cfg.ProbeTimeout,

		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		SessionCookieName:  sessionConfig.SessionCookieName,
		Authenticator:      oidc.NewAuthenticator(oidcService),
		Groups:             groups,
	}
	if cfg.DocsEnabled {
		deps.Spec = apispec.Spec
	}

	server := &http.Server{
		Handler:           httpadapter.NewRouter(deps),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	// Bind the port before marking the process as started: the startup probe
	// must only pass when the API can really accept connections.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", net.JoinHostPort("", strconv.Itoa(cfg.Port)))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", cfg.Port, err)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	// The worker starts once the database answers and the port is bound; it
	// first puts fields left calculating by a previous run back to pending.
	if err := terrain.Start(ctx); err != nil {
		return err
	}
	// The soil history loop stops with ctx (the signal) and is awaited in shutdown.
	if err := history.Start(ctx); err != nil {
		return err
	}

	lifecycle.MarkStarted()
	logger.Info("api started", "config", cfg.logValue())

	select {
	case err := <-serveErr:
		return fmt.Errorf("serving http: %w", err)
	case <-ctx.Done():
	}

	return shutdown(logger, server, lifecycle, terrain, history, cfg.Config)
}

// shutdown drains the instance, in this order:
//  1. readiness goes down so Kubernetes stops routing traffic;
//  2. the HTTP server stops accepting and in-flight requests get ShutdownTimeout
//     to finish, so nothing can schedule new relief work any more;
//  3. the relief worker finishes the field it is calculating (it saves through
//     the pool, so it must end before the pool closes), bounded by the same
//     timeout; what stays pending or calculating is recovered by the next start;
//  4. the soil history executor gives up its run (a field not recorded today is
//     recorded by the next run or instance), bounded by the same timeout;
//  5. the pool closes afterwards (deferred in run), once nothing can use it.
func shutdown(logger *slog.Logger, server *http.Server, lifecycle *usecase.Lifecycle, terrain terrainRuntime, history soilHistoryRuntime, cfg platform.Config) error {
	logger.Info("shutdown signal received, draining")
	lifecycle.BeginShutdown()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	serverErr := server.Shutdown(ctx)
	stopTerrain(ctx, logger, terrain)
	stopSoilHistory(ctx, logger, history)
	if serverErr != nil && !errors.Is(serverErr, http.ErrServerClosed) {
		return fmt.Errorf("shutting down http server: %w", serverErr)
	}
	logger.Info("api stopped")
	return nil
}

// stopTerrain stops the relief worker, giving up when ctx ends so a field that
// takes longer than the shutdown budget cannot hold the process hostage.
func stopTerrain(ctx context.Context, logger *slog.Logger, terrain terrainRuntime) {
	if !terrain.Enabled() {
		return
	}
	stopped := make(chan struct{})
	go func() {
		terrain.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		logger.Info("terrain worker stopped")
	case <-ctx.Done():
		logger.Warn("terrain worker did not stop within the shutdown timeout; the field in progress is recovered on the next start")
	}
}
