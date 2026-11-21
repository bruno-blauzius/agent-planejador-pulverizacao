package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

type fakeProbe struct {
	report usecase.HealthReport
	panics bool
}

func (f fakeProbe) Execute() usecase.HealthReport {
	if f.panics {
		panic("boom")
	}
	return f.report
}

type fakeReadiness struct {
	report      usecase.HealthReport
	hasDeadline *bool
}

func (f fakeReadiness) Execute(ctx context.Context) usecase.HealthReport {
	if f.hasDeadline != nil {
		_, *f.hasDeadline = ctx.Deadline()
	}
	return f.report
}

var (
	up   = usecase.HealthReport{Status: usecase.StatusUp}
	down = usecase.HealthReport{Status: usecase.StatusDown}
)

func newDeps(logger *slog.Logger) httpadapter.RouterDeps {
	return httpadapter.RouterDeps{
		Logger:       logger,
		Liveness:     fakeProbe{report: up},
		Startup:      fakeProbe{report: up},
		Readiness:    fakeReadiness{report: up},
		ProbeTimeout: time.Second,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func do(handler http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestHealthProbes(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		mutate     func(*httpadapter.RouterDeps)
		wantStatus int
		wantBody   string
	}{
		{"liveness up", "/health/live", nil, 200, "up"},
		{"liveness down", "/health/live", func(d *httpadapter.RouterDeps) { d.Liveness = fakeProbe{report: down} }, 503, "down"},
		{"startup up", "/health/startup", nil, 200, "up"},
		{"startup down", "/health/startup", func(d *httpadapter.RouterDeps) { d.Startup = fakeProbe{report: down} }, 503, "down"},
		{"readiness up", "/health/ready", nil, 200, "up"},
		{"readiness down", "/health/ready", func(d *httpadapter.RouterDeps) { d.Readiness = fakeReadiness{report: down} }, 503, "down"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newDeps(discardLogger())
			if tt.mutate != nil {
				tt.mutate(&deps)
			}
			response := do(httpadapter.NewRouter(deps), http.MethodGet, tt.path, nil)

			if response.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
			if body["status"] != tt.wantBody {
				t.Errorf("body status = %v, want %s", body["status"], tt.wantBody)
			}
			if _, present := body["checks"]; present {
				t.Errorf("checks must be omitted when empty")
			}
		})
	}
}

func TestReadinessBodyAndTimeout(t *testing.T) {
	var hasDeadline bool
	deps := newDeps(discardLogger())
	deps.Readiness = fakeReadiness{
		hasDeadline: &hasDeadline,
		report: usecase.HealthReport{
			Status: usecase.StatusDown,
			Checks: []usecase.CheckResult{
				{Name: "postgres", Status: usecase.StatusUp},
				{Name: "agent", Status: usecase.StatusDown},
			},
		},
	}

	response := do(httpadapter.NewRouter(deps), http.MethodGet, "/health/ready", nil)

	want := `{"status":"down","checks":[{"name":"postgres","status":"up"},{"name":"agent","status":"down"}]}`
	if got := strings.TrimSpace(response.Body.String()); got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
	if !hasDeadline {
		t.Error("readiness context must carry the probe timeout")
	}
}

func TestDocsRoutes(t *testing.T) {
	tests := []struct {
		name            string
		spec            []byte
		path            string
		wantStatus      int
		wantContentType string
		wantBody        string
	}{
		{"openapi with spec", []byte("openapi: 3.0.3"), "/openapi.yaml", 200, "application/yaml", "openapi: 3.0.3"},
		{"docs with spec", []byte("openapi: 3.0.3"), "/docs", 200, "text/html; charset=utf-8", "swagger-ui-dist@5.17.14"},
		{"openapi without spec", nil, "/openapi.yaml", 404, "application/json", "not_found"},
		{"docs without spec", nil, "/docs", 404, "application/json", "not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newDeps(discardLogger())
			deps.Spec = tt.spec
			response := do(httpadapter.NewRouter(deps), http.MethodGet, tt.path, nil)

			if response.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if got := response.Header().Get("Content-Type"); got != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantContentType)
			}
			if !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Errorf("body %q does not contain %q", response.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestErrorEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
		wantAllow  string
	}{
		{"unknown path", http.MethodGet, "/nope", 404, "not_found", ""},
		{"wrong method", http.MethodPost, "/health/live", 405, "method_not_allowed", "GET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := do(httpadapter.NewRouter(newDeps(discardLogger())), tt.method, tt.path, nil)

			if response.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			var body struct {
				Error struct{ Code, Message string }
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid json: %v", err)
			}
			if body.Error.Code != tt.wantCode || body.Error.Message == "" {
				t.Errorf("error = %+v, want code %s with message", body.Error, tt.wantCode)
			}
			if got := response.Header().Get("Allow"); got != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
			}
		})
	}
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name      string
		incoming  string
		wantEcho  bool
		wantLenIs int
	}{
		{"valid is propagated", "abc-DEF_123", true, 0},
		{"max length is propagated", strings.Repeat("a", 64), true, 0},
		{"missing is generated", "", false, 32},
		{"too long is replaced", strings.Repeat("a", 65), false, 32},
		{"invalid chars are replaced", "bad id\r\ninjected", false, 32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{}
			if tt.incoming != "" {
				headers["X-Request-Id"] = tt.incoming
			}
			response := do(httpadapter.NewRouter(newDeps(discardLogger())), http.MethodGet, "/health/live", headers)

			got := response.Header().Get("X-Request-Id")
			if tt.wantEcho && got != tt.incoming {
				t.Errorf("request id = %q, want %q", got, tt.incoming)
			}
			if !tt.wantEcho && (got == tt.incoming || len(got) != tt.wantLenIs) {
				t.Errorf("request id = %q, want generated with length %d", got, tt.wantLenIs)
			}
		})
	}
}

func TestRecoverTurnsPanicInto500(t *testing.T) {
	deps := newDeps(discardLogger())
	deps.Liveness = fakeProbe{panics: true}
	handler := httpadapter.NewRouter(deps)

	response := do(handler, http.MethodGet, "/health/live", nil)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"internal_error"`) {
		t.Errorf("body = %s, want internal_error", response.Body.String())
	}
	if response.Header().Get("X-Request-Id") == "" {
		t.Error("panic response must still carry the request id")
	}
	if next := do(handler, http.MethodGet, "/health/startup", nil); next.Code != http.StatusOK {
		t.Errorf("server must keep serving after panic, got %d", next.Code)
	}
}

func TestAccessLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantLevel string
	}{
		{"health is debug", "/health/ready", "DEBUG"},
		{"other is info", "/nope", "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

			do(httpadapter.NewRouter(newDeps(logger)), http.MethodGet, tt.path, map[string]string{"X-Request-Id": "rid-1"})

			var entry map[string]any
			if err := json.Unmarshal(buffer.Bytes(), &entry); err != nil {
				t.Fatalf("invalid log line %q: %v", buffer.String(), err)
			}
			if entry["level"] != tt.wantLevel {
				t.Errorf("level = %v, want %s", entry["level"], tt.wantLevel)
			}
			if entry["method"] != "GET" || entry["path"] != tt.path || entry["request_id"] != "rid-1" {
				t.Errorf("unexpected fields: %v", entry)
			}
			if _, ok := entry["status"]; !ok {
				t.Error("missing status")
			}
			if _, ok := entry["duration_ms"]; !ok {
				t.Error("missing duration_ms")
			}
		})
	}
}
