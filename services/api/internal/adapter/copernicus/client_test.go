package copernicus_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/copernicus"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const tilePath = "/Copernicus_DSM_COG_10_S22_00_W047_00_DEM/Copernicus_DSM_COG_10_S22_00_W047_00_DEM.tif"

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/tile.bin")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newClient(server *httptest.Server, cfg copernicus.Config) *copernicus.Client {
	cfg.BaseURL = server.URL
	if cfg.RetryBackoff == 0 {
		cfg.RetryBackoff = time.Millisecond
	}
	return copernicus.NewClient(cfg, server.Client())
}

func TestFetchTileSuccess(t *testing.T) {
	data := fixture(t)
	var gotPath, gotAgent, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAgent, gotMethod = r.URL.Path, r.UserAgent(), r.Method
		_, _ = w.Write(data)
	}))
	defer server.Close()

	body, err := newClient(server, copernicus.Config{UserAgent: "test-agent"}).FetchTile(context.Background(), "S22W047")
	if err != nil {
		t.Fatalf("FetchTile: %v", err)
	}
	got, err := io.ReadAll(body)
	if err != nil || string(got) != string(data) {
		t.Fatalf("body mismatch: %v (%d bytes)", err, len(got))
	}
	if err := body.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if gotPath != tilePath || gotMethod != http.MethodGet {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotAgent != "test-agent" {
		t.Errorf("User-Agent = %q", gotAgent)
	}
}

func TestDefaultUserAgent(t *testing.T) {
	var agent string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { agent = r.UserAgent() }))
	defer server.Close()
	body, err := newClient(server, copernicus.Config{}).FetchTile(context.Background(), "N05E010")
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	if agent != copernicus.DefaultUserAgent {
		t.Errorf("User-Agent = %q", agent)
	}
}

func TestModel(t *testing.T) {
	model := copernicus.NewClient(copernicus.Config{ModelVersion: "v1"}, nil).Model()
	want := domain.ElevationModel{Source: "Copernicus DEM GLO-30", ResolutionM: 30, Version: "v1"}
	if model != want {
		t.Errorf("Model() = %+v, want %+v", model, want)
	}
	if copernicus.NewClient(copernicus.Config{}, nil).Model().Version != copernicus.DefaultModelVersion {
		t.Error("default version not applied")
	}
}

func TestMissingTile(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
			}))
			defer server.Close()
			_, err := newClient(server, copernicus.Config{}).FetchTile(context.Background(), "S22W047")
			if !errors.Is(err, usecase.ErrElevationUnavailable) {
				t.Fatalf("err = %v", err)
			}
			if calls.Load() != 1 {
				t.Errorf("calls = %d, want no retry", calls.Load())
			}
		})
	}
}

func TestRetryThenSuccess(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(status)
					return
				}
				_, _ = w.Write([]byte("ok"))
			}))
			defer server.Close()
			body, err := newClient(server, copernicus.Config{}).FetchTile(context.Background(), "S22W047")
			if err != nil {
				t.Fatalf("FetchTile: %v", err)
			}
			_ = body.Close()
			if calls.Load() != 2 {
				t.Errorf("calls = %d, want 2", calls.Load())
			}
		})
	}
}

func TestPersistentServerError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	_, err := newClient(server, copernicus.Config{Retries: 2}).FetchTile(context.Background(), "S22W047")
	if !errors.Is(err, usecase.ErrElevationUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestClientErrorIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	_, err := newClient(server, copernicus.Config{}).FetchTile(context.Background(), "S22W047")
	if !errors.Is(err, usecase.ErrElevationUnavailable) || calls.Load() != 1 {
		t.Errorf("err = %v, calls = %d", err, calls.Load())
	}
}

func TestNetworkErrorKeepsCauseAndHidesURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close() // nothing listens any more
	client := copernicus.NewClient(copernicus.Config{BaseURL: baseURL, Retries: 1, RetryBackoff: time.Millisecond}, nil)
	_, err := client.FetchTile(context.Background(), "S22W047")
	if !errors.Is(err, usecase.ErrElevationUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), baseURL) {
		t.Errorf("error leaks the URL: %v", err)
	}
}

func TestRequestTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)
	client := newClient(server, copernicus.Config{RequestTimeout: 20 * time.Millisecond, Retries: -1})
	_, err := client.FetchTile(context.Background(), "S22W047")
	if !errors.Is(err, usecase.ErrElevationUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestCancelledContext(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newClient(server, copernicus.Config{}).FetchTile(ctx, "S22W047")
	if !errors.Is(err, context.Canceled) || !errors.Is(err, usecase.ErrElevationUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 0 {
		t.Errorf("calls = %d, want 0", calls.Load())
	}
}

func TestBodyAboveLimitWithoutContentLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 600))
		w.(http.Flusher).Flush()
		_, _ = w.Write(make([]byte, 600))
	}))
	defer server.Close()
	body, err := newClient(server, copernicus.Config{MaxBytes: 1000}).FetchTile(context.Background(), "S22W047")
	if err != nil {
		t.Fatalf("FetchTile: %v", err)
	}
	defer func() { _ = body.Close() }()
	if _, err := io.ReadAll(body); !errors.Is(err, copernicus.ErrTileTooLarge) {
		t.Fatalf("ReadAll err = %v", err)
	}
}

func TestBodyExactlyAtLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 500))
		w.(http.Flusher).Flush()
		_, _ = w.Write(make([]byte, 500))
	}))
	defer server.Close()
	body, err := newClient(server, copernicus.Config{MaxBytes: 1000}).FetchTile(context.Background(), "S22W047")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	got, err := io.ReadAll(body)
	if err != nil || len(got) != 1000 {
		t.Fatalf("read %d bytes, err %v", len(got), err)
	}
}

func TestContentLengthAboveLimit(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write(make([]byte, 2048))
	}))
	defer server.Close()
	_, err := newClient(server, copernicus.Config{MaxBytes: 1024}).FetchTile(context.Background(), "S22W047")
	if !errors.Is(err, copernicus.ErrTileTooLarge) || !errors.Is(err, usecase.ErrElevationUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want no retry", calls.Load())
	}
}

func TestInvalidKey(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := newClient(server, copernicus.Config{})
	keys := []domain.ElevationTileKey{"", "../x", "S22W47", "s22w047", "S22W047/..", "S22W047 ", "X22W047", "S2W047", "S22W0471", "S22W047\n"}
	for _, key := range keys {
		t.Run(strconv.Quote(string(key)), func(t *testing.T) {
			_, err := client.FetchTile(context.Background(), key)
			if err == nil {
				t.Fatal("expected error")
			}
			if errors.Is(err, usecase.ErrElevationUnavailable) {
				t.Errorf("invalid key must not be reported as unavailable data: %v", err)
			}
		})
	}
	if calls.Load() != 0 {
		t.Errorf("server was called %d times", calls.Load())
	}
}
