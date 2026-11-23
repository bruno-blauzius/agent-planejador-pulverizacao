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
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/oidc"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

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

	groups := newRouteGroups(dependencies{
		Logger: logger, Store: database, Identity: identity, Clock: clock, IDs: ids, Settings: settings,
	})
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

	lifecycle.MarkStarted()
	logger.Info("api started", "config", cfg.logValue())

	select {
	case err := <-serveErr:
		return fmt.Errorf("serving http: %w", err)
	case <-ctx.Done():
	}

	return shutdown(logger, server, lifecycle, cfg.Config)
}

// shutdown drains the instance: readiness goes down first so Kubernetes stops
// routing traffic, then in-flight requests get ShutdownTimeout to finish. The
// pool closes afterwards (deferred in run), once nothing can use it.
func shutdown(logger *slog.Logger, server *http.Server, lifecycle *usecase.Lifecycle, cfg platform.Config) error {
	logger.Info("shutdown signal received, draining")
	lifecycle.BeginShutdown()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutting down http server: %w", err)
	}
	logger.Info("api stopped")
	return nil
}
