package openmeteo_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/openmeteo"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// The recorded answer is a trimmed real answer of /v1/forecast with
// timeformat=unixtime: 4 hours of 22/11/2026 (06:00 to 09:00 local) and 3 days.
var brt = time.FixedZone("America/Sao_Paulo", -3*3600)

var cell = domain.WeatherCell{Latitude: -21.12, Longitude: -47}

func fixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/forecast.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newClient(t *testing.T, server *httptest.Server, cfg openmeteo.Config) *openmeteo.Client {
	t.Helper()
	cfg.BaseURL = server.URL
	if cfg.Location == nil {
		cfg.Location = brt
	}
	client, err := openmeteo.NewClient(cfg, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestWeatherParsesTheRecordedAnswer(t *testing.T) {
	data := fixture(t)
	var gotPath string
	var gotQuery map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.Query()
		_, _ = w.Write(data)
	}))
	defer server.Close()

	report, err := newClient(t, server, openmeteo.Config{}).Weather(context.Background(), cell)
	if err != nil {
		t.Fatalf("Weather: %v", err)
	}

	if gotPath != "/v1/forecast" {
		t.Errorf("path = %s", gotPath)
	}
	wantQuery := map[string]string{
		"latitude": "-21.12", "longitude": "-47.00", "past_days": "7", "forecast_days": "8",
		"timezone": "America/Sao_Paulo", "timeformat": "unixtime", "daily": "precipitation_sum",
		"hourly": "precipitation,soil_moisture_0_to_1cm,soil_moisture_1_to_3cm,soil_moisture_3_to_9cm,soil_moisture_9_to_27cm",
	}
	for key, want := range wantQuery {
		if got := gotQuery[key]; len(got) != 1 || got[0] != want {
			t.Errorf("query %s = %v, want %s", key, got, want)
		}
	}

	if len(report.Hours) != 4 {
		t.Fatalf("hours = %d, want 4", len(report.Hours))
	}
	first := report.Hours[0]
	if !first.Time.Equal(time.Date(2026, 11, 22, 6, 0, 0, 0, brt)) || first.RainMm == nil || *first.RainMm != 0 {
		t.Errorf("first hour = %+v", first)
	}
	if second := report.Hours[1]; *second.RainMm != 2.4 || second.SoilMoistureM3M3 == nil || math.Abs(*second.SoilMoistureM3M3-0.33) > 1e-9 {
		t.Errorf("second hour = rain %v moisture %v", *second.RainMm, second.SoilMoistureM3M3)
	}
	if third := report.Hours[2]; third.RainMm != nil {
		t.Errorf("a null rain must stay missing, got %v", *third.RainMm)
	}
	if fourth := report.Hours[3]; fourth.SoilMoistureM3M3 != nil {
		t.Errorf("a missing layer must leave the moisture missing, got %v", *fourth.SoilMoistureM3M3)
	}

	if len(report.Days) != 2 {
		t.Fatalf("days = %+v, want the 2 days with a value", report.Days)
	}
	if day := report.Days[0]; !day.Date.Equal(time.Date(2026, 11, 21, 0, 0, 0, 0, brt)) || day.Mm != 18.2 {
		t.Errorf("first day = %+v", day)
	}
}

func TestWeatherWeightsTheSoilLayersByThickness(t *testing.T) {
	answer := `{"hourly":{"time":[1795338000],"precipitation":[0],
	  "soil_moisture_0_to_1cm":[0.10],"soil_moisture_1_to_3cm":[0.20],"soil_moisture_3_to_9cm":[0.30],"soil_moisture_9_to_27cm":[0.40]},
	  "daily":{"time":[],"precipitation_sum":[]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(answer)) }))
	defer server.Close()

	report, err := newClient(t, server, openmeteo.Config{}).Weather(context.Background(), cell)
	if err != nil {
		t.Fatal(err)
	}
	want := (0.10*1 + 0.20*2 + 0.30*6 + 0.40*18) / 27
	if got := report.Hours[0].SoilMoistureM3M3; got == nil || math.Abs(*got-want) > 1e-12 {
		t.Errorf("moisture = %v, want %v", got, want)
	}
}

func TestWeatherFailuresAreUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		cfg     openmeteo.Config
	}{
		{"server error", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }, openmeteo.Config{}},
		{"rate limited", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) }, openmeteo.Config{}},
		{"not JSON", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }, openmeteo.Config{}},
		{"too large", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(bytes.Repeat([]byte(" "), 2048)) }, openmeteo.Config{MaxBytes: 1024}},
		{"missing series", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"hourly":{"time":[1795338000],"precipitation":[1]},"daily":{"time":[],"precipitation_sum":[]}}`))
		}, openmeteo.Config{}},
		{"series of different lengths", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"hourly":{"time":[1795338000,1795341600],"precipitation":[1],
			  "soil_moisture_0_to_1cm":[0.1,0.1],"soil_moisture_1_to_3cm":[0.1,0.1],"soil_moisture_3_to_9cm":[0.1,0.1],"soil_moisture_9_to_27cm":[0.1,0.1]},
			  "daily":{"time":[],"precipitation_sum":[]}}`))
		}, openmeteo.Config{}},
		{"no hours", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"hourly":{"time":[],"precipitation":[],"soil_moisture_0_to_1cm":[],"soil_moisture_1_to_3cm":[],"soil_moisture_3_to_9cm":[],"soil_moisture_9_to_27cm":[]},"daily":{"time":[],"precipitation_sum":[]}}`))
		}, openmeteo.Config{}},
		{"timeout", func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}, openmeteo.Config{RequestTimeout: 20 * time.Millisecond}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()
			logs := &bytes.Buffer{}
			tt.cfg.Logger = slog.New(slog.NewTextHandler(logs, nil))
			_, err := newClient(t, server, tt.cfg).Weather(context.Background(), cell)
			if !errors.Is(err, usecase.ErrWeatherUnavailable) {
				t.Fatalf("err = %v, want ErrWeatherUnavailable", err)
			}
			for _, leak := range []string{"-21.12", "21.12", "latitude", server.URL} {
				if strings.Contains(err.Error(), leak) || strings.Contains(logs.String(), leak) {
					t.Errorf("the coordinate or the URL leaked (%q): err %q, logs %q", leak, err, logs)
				}
			}
			if !strings.Contains(logs.String(), "msg=weather.unavailable") {
				t.Errorf("the failure must be logged as weather.unavailable: %q", logs)
			}
		})
	}
}

func TestNewClientRequiresATimeZone(t *testing.T) {
	if _, err := openmeteo.NewClient(openmeteo.Config{}, nil); err == nil {
		t.Fatal("a client without time zone must be refused")
	}
}

// platform cannot import this adapter, so it repeats its defaults; this test
// keeps the two copies from drifting apart.
func TestPlatformWeatherDefaultsMatchTheClient(t *testing.T) {
	cfg, err := platform.LoadTrafficabilityConfig(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.Weather
	if got.BaseURL != openmeteo.DefaultBaseURL || got.RequestTimeout != openmeteo.DefaultRequestTimeout ||
		got.MaxBytes != openmeteo.DefaultMaxBytes || got.CacheTTL != openmeteo.DefaultCacheTTL {
		t.Errorf("platform defaults %+v differ from the openmeteo client defaults", got)
	}
}

// ---- cache ---------------------------------------------------------------------------

type countingProvider struct {
	calls atomic.Int32
	err   error
}

func (p *countingProvider) Weather(context.Context, domain.WeatherCell) (usecase.WeatherReport, error) {
	p.calls.Add(1)
	if p.err != nil {
		return usecase.WeatherReport{}, p.err
	}
	return usecase.WeatherReport{Days: []domain.DailyRain{{Mm: float64(p.calls.Load())}}}, nil
}

type manualClock struct{ t time.Time }

func (c *manualClock) Now() time.Time { return c.t }

func TestCacheKeepsAnAnswerPerCellAndDayForTheTTL(t *testing.T) {
	inner := &countingProvider{}
	clock := &manualClock{t: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)} // 09:00 local
	cache := openmeteo.NewCache(inner, time.Hour, clock, brt)
	ask := func(c domain.WeatherCell) float64 {
		t.Helper()
		report, err := cache.Weather(context.Background(), c)
		if err != nil {
			t.Fatal(err)
		}
		return report.Days[0].Mm
	}

	steps := []struct {
		name      string
		advance   time.Duration
		cell      domain.WeatherCell
		wantCalls int32
	}{
		{"first ask fetches", 0, cell, 1},
		{"same cell within the TTL hits", 30 * time.Minute, cell, 1},
		{"another cell fetches", 0, domain.WeatherCell{Latitude: -21.13, Longitude: -47}, 2},
		{"after the TTL fetches again", 31 * time.Minute, cell, 3},
		{"23:50 local, TTL expired, fetches", 13*time.Hour + 49*time.Minute, cell, 4},
		{"00:10 of the next local day fetches even within the TTL", 20 * time.Minute, cell, 5},
	}
	for _, step := range steps {
		clock.t = clock.t.Add(step.advance)
		ask(step.cell)
		if got := inner.calls.Load(); got != step.wantCalls {
			t.Fatalf("%s: calls = %d, want %d", step.name, got, step.wantCalls)
		}
	}
}

func TestCacheDoesNotKeepFailures(t *testing.T) {
	inner := &countingProvider{err: usecase.ErrWeatherUnavailable}
	cache := openmeteo.NewCache(inner, time.Hour, &manualClock{t: time.Now()}, brt)
	for range 2 {
		if _, err := cache.Weather(context.Background(), cell); !errors.Is(err, usecase.ErrWeatherUnavailable) {
			t.Fatalf("err = %v", err)
		}
	}
	if inner.calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 (failures are not cached)", inner.calls.Load())
	}
}
