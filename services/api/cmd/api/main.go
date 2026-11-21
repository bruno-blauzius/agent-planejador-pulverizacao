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
	cfg, err := platform.LoadConfig(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	logger := platform.NewLogger(os.Stdout, cfg.LogLevel)

	lifecycle := usecase.NewLifecycle()

	deps := httpadapter.RouterDeps{
		Logger:       logger,
		Liveness:     usecase.CheckLiveness{},
		Startup:      usecase.NewCheckStartup(lifecycle),
		Readiness:    usecase.NewCheckReadiness(lifecycle, logger), // dependency probes are added in Phase 3
		ProbeTimeout: cfg.ProbeTimeout,
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", net.JoinHostPort("", strconv.Itoa(cfg.Port)))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", cfg.Port, err)
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	lifecycle.MarkStarted()
	logger.Info("api started", "port", cfg.Port, "docs_enabled", cfg.DocsEnabled)

	select {
	case err := <-serveErr:
		return fmt.Errorf("serving http: %w", err)
	case <-ctx.Done():
	}

	return shutdown(logger, server, lifecycle, cfg)
}

// shutdown drains the instance: readiness goes down first so Kubernetes stops
// routing traffic, then in-flight requests get ShutdownTimeout to finish.
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
