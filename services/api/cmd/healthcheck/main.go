// Command healthcheck probes the local API readiness endpoint and exits 0 when
// it is up. It exists because the distroless runtime image has no shell, curl
// or wget, so container orchestrators without HTTP probes (docker compose)
// need a binary to execute.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	defaultPort = "8080"
	timeout     = 2 * time.Second
	probePath   = "/health/ready"
)

func main() {
	if err := probe(); err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		os.Exit(1)
	}
}

func probe() error {
	port, ok := os.LookupEnv("API_PORT")
	if !ok || port == "" {
		port = defaultPort
	}
	url := "http://" + net.JoinHostPort("127.0.0.1", port) + probePath

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned status %d", url, response.StatusCode)
	}
	return nil
}
