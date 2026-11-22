package platform_test

import (
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
)

func fakeEnv(values map[string]string) platform.LookupFunc {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestLoadConfig(t *testing.T) {
	defaults := platform.Config{
		Port: 8080, LogLevel: slog.LevelInfo,
		ReadHeaderTimeout: 5 * time.Second, ShutdownTimeout: 15 * time.Second,
		ProbeTimeout: 2 * time.Second, DocsEnabled: true,
	}
	lowestPort := defaults
	lowestPort.Port = 1
	withOrigins := defaults
	withOrigins.CORSAllowedOrigins = []string{"http://localhost:3000", "https://app.example.com"}

	tests := []struct {
		name    string
		env     map[string]string
		want    platform.Config
		wantErr bool
	}{
		{name: "defaults when environment is empty", env: map[string]string{}, want: defaults},
		{
			name: "overrides every value",
			env: map[string]string{
				"API_PORT": "9000", "API_LOG_LEVEL": "debug",
				"API_READ_HEADER_TIMEOUT": "1s", "API_SHUTDOWN_TIMEOUT": "30s",
				"API_PROBE_TIMEOUT": "500ms", "API_DOCS_ENABLED": "false",
			},
			want: platform.Config{
				Port: 9000, LogLevel: slog.LevelDebug,
				ReadHeaderTimeout: time.Second, ShutdownTimeout: 30 * time.Second,
				ProbeTimeout: 500 * time.Millisecond, DocsEnabled: false,
			},
		},
		{name: "port at lower bound", env: map[string]string{"API_PORT": "1"}, want: lowestPort},
		{name: "port zero is rejected", env: map[string]string{"API_PORT": "0"}, wantErr: true},
		{name: "port above range is rejected", env: map[string]string{"API_PORT": "65536"}, wantErr: true},
		{name: "port not a number is rejected", env: map[string]string{"API_PORT": "abc"}, wantErr: true},
		{name: "unknown log level is rejected", env: map[string]string{"API_LOG_LEVEL": "loud"}, wantErr: true},
		{name: "negative duration is rejected", env: map[string]string{"API_SHUTDOWN_TIMEOUT": "-1s"}, wantErr: true},
		{name: "duration without unit is rejected", env: map[string]string{"API_PROBE_TIMEOUT": "5"}, wantErr: true},
		{name: "invalid docs flag is rejected", env: map[string]string{"API_DOCS_ENABLED": "maybe"}, wantErr: true},
		{
			name: "cors origins are trimmed and empty items dropped",
			env:  map[string]string{"API_CORS_ALLOWED_ORIGINS": " http://localhost:3000, ,https://app.example.com "},
			want: withOrigins,
		},
		{name: "empty cors list keeps cors disabled", env: map[string]string{"API_CORS_ALLOWED_ORIGINS": ""}, want: defaults},
		{name: "cors wildcard is rejected", env: map[string]string{"API_CORS_ALLOWED_ORIGINS": "*"}, wantErr: true},
		{name: "cors origin with path is rejected", env: map[string]string{"API_CORS_ALLOWED_ORIGINS": "http://localhost:3000/app"}, wantErr: true},
		{name: "cors origin without scheme is rejected", env: map[string]string{"API_CORS_ALLOWED_ORIGINS": "localhost:3000"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := platform.LoadConfig(fakeEnv(tt.env))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got config %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("config mismatch:\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}
