// Package platform holds infrastructure concerns shared by the composition root:
// configuration loading and logger construction.
package platform

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Defaults are overridable through environment variables (see LoadConfig).
const (
	defaultPort              = 8080
	defaultLogLevel          = slog.LevelInfo
	defaultReadHeaderTimeout = 5 * time.Second
	defaultShutdownTimeout   = 15 * time.Second
	defaultProbeTimeout      = 2 * time.Second
	defaultDocsEnabled       = true
)

// Config is the immutable runtime configuration of the API.
type Config struct {
	Port              int
	LogLevel          slog.Level
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	ProbeTimeout      time.Duration
	DocsEnabled       bool
}

// LookupFunc matches os.LookupEnv, so tests can inject a fake environment.
type LookupFunc func(key string) (string, bool)

// LoadConfig reads the configuration from environment variables:
// API_PORT, API_LOG_LEVEL, API_READ_HEADER_TIMEOUT, API_SHUTDOWN_TIMEOUT,
// API_PROBE_TIMEOUT and API_DOCS_ENABLED.
func LoadConfig(lookup LookupFunc) (Config, error) {
	cfg := Config{
		Port:              defaultPort,
		LogLevel:          defaultLogLevel,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ShutdownTimeout:   defaultShutdownTimeout,
		ProbeTimeout:      defaultProbeTimeout,
		DocsEnabled:       defaultDocsEnabled,
	}

	if raw, ok := lookup("API_PORT"); ok {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("invalid API_PORT %q: must be an integer between 1 and 65535", raw)
		}
		cfg.Port = port
	}

	if raw, ok := lookup("API_LOG_LEVEL"); ok {
		if err := cfg.LogLevel.UnmarshalText([]byte(raw)); err != nil {
			return Config{}, fmt.Errorf("invalid API_LOG_LEVEL %q: %w", raw, err)
		}
	}

	durations := []struct {
		key    string
		target *time.Duration
	}{
		{"API_READ_HEADER_TIMEOUT", &cfg.ReadHeaderTimeout},
		{"API_SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout},
		{"API_PROBE_TIMEOUT", &cfg.ProbeTimeout},
	}
	for _, d := range durations {
		raw, ok := lookup(d.key)
		if !ok {
			continue
		}
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("invalid %s %q: must be a positive duration such as 5s", d.key, raw)
		}
		*d.target = value
	}

	if raw, ok := lookup("API_DOCS_ENABLED"); ok {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid API_DOCS_ENABLED %q: %w", raw, err)
		}
		cfg.DocsEnabled = enabled
	}

	return cfg, nil
}
