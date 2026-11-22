// Package platform holds infrastructure concerns shared by the composition root:
// configuration loading and logger construction.
package platform

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
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
	Port               int
	LogLevel           slog.Level
	ReadHeaderTimeout  time.Duration
	ShutdownTimeout    time.Duration
	ProbeTimeout       time.Duration
	DocsEnabled        bool
	CORSAllowedOrigins []string
}

// LookupFunc matches os.LookupEnv, so tests can inject a fake environment.
type LookupFunc func(key string) (string, bool)

// LoadConfig reads the configuration from environment variables:
// API_PORT, API_LOG_LEVEL, API_READ_HEADER_TIMEOUT, API_SHUTDOWN_TIMEOUT,
// API_PROBE_TIMEOUT, API_DOCS_ENABLED and API_CORS_ALLOWED_ORIGINS (comma
// separated origins such as http://localhost:3000; unset or empty disables CORS).
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

	if raw, ok := lookup("API_CORS_ALLOWED_ORIGINS"); ok {
		origins, err := parseOrigins(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid API_CORS_ALLOWED_ORIGINS %q: %w", raw, err)
		}
		cfg.CORSAllowedOrigins = origins
	}

	return cfg, nil
}

// parseOrigins accepts only bare origins (scheme://host[:port]). Wildcards and
// paths are rejected so a typo cannot silently open the API to every site.
func parseOrigins(raw string) ([]string, error) {
	var origins []string
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		if origin == "" {
			continue
		}
		parsed, err := url.Parse(origin)
		if err != nil {
			return nil, fmt.Errorf("origin %q: %w", origin, err)
		}
		isBareOrigin := (parsed.Scheme == "http" || parsed.Scheme == "https") &&
			parsed.Host != "" && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.User == nil
		if !isBareOrigin {
			return nil, fmt.Errorf("origin %q must look like https://app.example.com (no wildcard, path or query)", origin)
		}
		origins = append(origins, origin)
	}
	return origins, nil
}
