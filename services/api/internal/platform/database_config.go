package platform

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"
)

// Database pool defaults, overridable through the variables listed on LoadDatabaseConfig.
const (
	defaultDBMaxConns         = 10
	defaultDBMinConns         = 2
	defaultDBMaxConnLifetime  = 30 * time.Minute
	defaultDBMaxConnIdleTime  = 5 * time.Minute
	defaultDBHealthCheck      = time.Minute
	defaultDBConnectTimeout   = 5 * time.Second
	defaultDBStatementTimeout = 15 * time.Second
	defaultSessionTouchEvery  = time.Minute

	redactedPassword = "xxxxx"

	databaseURLVariable       = "API_DATABASE_URL"
	maxConnsVariable          = "API_DB_MAX_CONNS"
	minConnsVariable          = "API_DB_MIN_CONNS"
	maxConnLifetimeVariable   = "API_DB_MAX_CONN_LIFETIME"
	maxConnIdleTimeVariable   = "API_DB_MAX_CONN_IDLE_TIME"
	healthCheckVariable       = "API_DB_HEALTH_CHECK_PERIOD"
	connectTimeoutVariable    = "API_DB_CONNECT_TIMEOUT"
	statementTimeoutVariable  = "API_DB_STATEMENT_TIMEOUT"
	sessionTouchEveryVariable = "API_SESSION_TOUCH_INTERVAL"
)

// DatabaseConfig is the connection of the API to its own database. The URL
// carries the password of the runtime role (api_app): it never reaches a log,
// see LogValue and String.
type DatabaseConfig struct {
	URL               string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
	// StatementTimeout is sent as a runtime parameter so no query runs forever.
	StatementTimeout time.Duration
	// SessionTouchInterval limits how often auth_sessions.last_seen_at is written.
	SessionTouchInterval time.Duration
}

// LoadDatabaseConfig reads the database settings from the environment:
// API_DATABASE_URL (required, postgres:// URL of the api_app role),
// API_DB_MAX_CONNS, API_DB_MIN_CONNS, API_DB_MAX_CONN_LIFETIME,
// API_DB_MAX_CONN_IDLE_TIME, API_DB_HEALTH_CHECK_PERIOD, API_DB_CONNECT_TIMEOUT,
// API_DB_STATEMENT_TIMEOUT and API_SESSION_TOUCH_INTERVAL.
//
// Errors never echo the URL, since it holds the password.
func LoadDatabaseConfig(lookup LookupFunc) (DatabaseConfig, error) {
	cfg := DatabaseConfig{
		MaxConns:             defaultDBMaxConns,
		MinConns:             defaultDBMinConns,
		MaxConnLifetime:      defaultDBMaxConnLifetime,
		MaxConnIdleTime:      defaultDBMaxConnIdleTime,
		HealthCheckPeriod:    defaultDBHealthCheck,
		ConnectTimeout:       defaultDBConnectTimeout,
		StatementTimeout:     defaultDBStatementTimeout,
		SessionTouchInterval: defaultSessionTouchEvery,
	}

	raw, ok := lookup(databaseURLVariable)
	if !ok || raw == "" {
		return DatabaseConfig{}, fmt.Errorf("%s is required", databaseURLVariable)
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return DatabaseConfig{}, fmt.Errorf("invalid %s: expected postgres://user:password@host:port/database", databaseURLVariable)
	}
	cfg.URL = raw

	counts := []struct {
		key    string
		target *int32
	}{
		{maxConnsVariable, &cfg.MaxConns},
		{minConnsVariable, &cfg.MinConns},
	}
	for _, c := range counts {
		value, present := lookup(c.key)
		if !present {
			continue
		}
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n < 0 {
			return DatabaseConfig{}, fmt.Errorf("invalid %s %q: must be a non-negative integer", c.key, value)
		}
		*c.target = int32(n)
	}
	if cfg.MaxConns < 1 {
		return DatabaseConfig{}, fmt.Errorf("invalid %s: must be at least 1", maxConnsVariable)
	}
	if cfg.MinConns > cfg.MaxConns {
		return DatabaseConfig{}, errors.New("API_DB_MIN_CONNS must not exceed API_DB_MAX_CONNS")
	}

	durations := []struct {
		key    string
		target *time.Duration
	}{
		{maxConnLifetimeVariable, &cfg.MaxConnLifetime},
		{maxConnIdleTimeVariable, &cfg.MaxConnIdleTime},
		{healthCheckVariable, &cfg.HealthCheckPeriod},
		{connectTimeoutVariable, &cfg.ConnectTimeout},
		{statementTimeoutVariable, &cfg.StatementTimeout},
		{sessionTouchEveryVariable, &cfg.SessionTouchInterval},
	}
	for _, d := range durations {
		value, present := lookup(d.key)
		if !present {
			continue
		}
		parsedDuration, err := time.ParseDuration(value)
		if err != nil || parsedDuration < 0 || (parsedDuration == 0 && d.key != sessionTouchEveryVariable) {
			return DatabaseConfig{}, fmt.Errorf("invalid %s %q: must be a positive duration such as 30s", d.key, value)
		}
		*d.target = parsedDuration
	}
	return cfg, nil
}

// RedactedURL returns the connection URL with the password masked.
func (c DatabaseConfig) RedactedURL() string {
	parsed, err := url.Parse(c.URL)
	if err != nil {
		return "<invalid url>"
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), redactedPassword)
		}
	}
	return parsed.String()
}

// String implements fmt.Stringer without the password.
func (c DatabaseConfig) String() string {
	return fmt.Sprintf("DatabaseConfig{url=%s max_conns=%d min_conns=%d}", c.RedactedURL(), c.MaxConns, c.MinConns)
}

// GoString keeps %#v from printing the password.
func (c DatabaseConfig) GoString() string { return c.String() }

// LogValue implements slog.LogValuer: structured logs get the redacted URL.
func (c DatabaseConfig) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("url", c.RedactedURL()),
		slog.Int("max_conns", int(c.MaxConns)),
		slog.Int("min_conns", int(c.MinConns)),
		slog.Duration("statement_timeout", c.StatementTimeout),
	)
}
