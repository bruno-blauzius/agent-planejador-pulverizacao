package platform

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func lookupFrom(values map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}

const testDatabaseURL = "postgres://api_app:s3cr3t-pass@agro-postgres:5432/api?sslmode=disable"

func TestLoadDatabaseConfigDefaultsAndOverrides(t *testing.T) {
	cfg, err := LoadDatabaseConfig(lookupFrom(map[string]string{"API_DATABASE_URL": testDatabaseURL}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != defaultDBMaxConns || cfg.StatementTimeout != defaultDBStatementTimeout || cfg.SessionTouchInterval != defaultSessionTouchEvery {
		t.Fatalf("defaults not applied: %+v", cfg)
	}

	cfg, err = LoadDatabaseConfig(lookupFrom(map[string]string{
		"API_DATABASE_URL": testDatabaseURL, "API_DB_MAX_CONNS": "25", "API_DB_MIN_CONNS": "5",
		"API_DB_STATEMENT_TIMEOUT": "3s", "API_SESSION_TOUCH_INTERVAL": "0s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 25 || cfg.MinConns != 5 || cfg.StatementTimeout != 3*time.Second || cfg.SessionTouchInterval != 0 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestLoadDatabaseConfigRejectsBadValuesWithoutEchoingThePassword(t *testing.T) {
	cases := map[string]map[string]string{
		"missing url":     {},
		"wrong scheme":    {"API_DATABASE_URL": "mysql://u:s3cr3t-pass@h/db"},
		"no host":         {"API_DATABASE_URL": "postgres:///db"},
		"max conns zero":  {"API_DATABASE_URL": testDatabaseURL, "API_DB_MAX_CONNS": "0"},
		"min above max":   {"API_DATABASE_URL": testDatabaseURL, "API_DB_MAX_CONNS": "2", "API_DB_MIN_CONNS": "3"},
		"bad duration":    {"API_DATABASE_URL": testDatabaseURL, "API_DB_CONNECT_TIMEOUT": "soon"},
		"zero timeout":    {"API_DATABASE_URL": testDatabaseURL, "API_DB_STATEMENT_TIMEOUT": "0s"},
		"negative number": {"API_DATABASE_URL": testDatabaseURL, "API_DB_MAX_CONNS": "-1"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := LoadDatabaseConfig(lookupFrom(env))
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), "s3cr3t-pass") {
				t.Fatalf("error leaks the password: %v", err)
			}
		})
	}
}

func TestDatabaseConfigNeverPrintsThePassword(t *testing.T) {
	cfg, err := LoadDatabaseConfig(lookupFrom(map[string]string{"API_DATABASE_URL": testDatabaseURL}))
	if err != nil {
		t.Fatal(err)
	}
	var logged strings.Builder
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("database", "config", cfg)
	outputs := []string{
		cfg.String(), cfg.RedactedURL(), fmt.Sprintf("%v", cfg), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg), logged.String(),
	}
	for _, out := range outputs {
		if strings.Contains(out, "s3cr3t-pass") {
			t.Fatalf("password leaked: %s", out)
		}
	}
	if !strings.Contains(cfg.RedactedURL(), "api_app") {
		t.Fatal("the redacted URL keeps the role name for diagnosis")
	}
}
