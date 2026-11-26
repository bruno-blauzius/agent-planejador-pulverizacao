package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

type noEvaluation struct{}

func (noEvaluation) Evaluate(context.Context, usecase.FieldRef, int) (usecase.FieldTrafficabilityView, error) {
	return usecase.FieldTrafficabilityView{}, nil
}

func TestSoilHistoryConfigCarriesTheSettings(t *testing.T) {
	settings := platform.SoilHistorySettings{Enabled: true, Hour: 6, Concurrency: 3, SweepInterval: time.Minute, FieldTimeout: 20 * time.Second}
	got := soilHistoryConfig(settings, time.UTC)
	want := usecase.SoilHistoryConfig{Location: time.UTC, Hour: 6, Concurrency: 3, SweepInterval: time.Minute, FieldTimeout: 20 * time.Second}
	if got != want {
		t.Errorf("config = %+v, want %+v", got, want)
	}
}

func TestDisabledSoilHistoryIsNotStartedAndWarns(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	runtime, err := buildSoilHistoryRuntime(platform.SoilHistorySettings{Enabled: false}, time.UTC,
		fakes.NewSoilHistory(fakes.NewRegistry()), noEvaluation{}, &fakes.FixedClock{T: time.Now()}, logger)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.worker != nil || runtime.Start(context.Background()) != nil {
		t.Fatal("a disabled executor must not start")
	}
	if !strings.Contains(logs.String(), "API_SOIL_HISTORY_ENABLED=false") {
		t.Errorf("missing warning: %q", logs.String())
	}
	stopSoilHistory(context.Background(), logger, runtime)
}

func TestEnabledSoilHistoryStartsAndStops(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg, err := platform.LoadTrafficabilityConfig(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := buildSoilHistoryRuntime(cfg.History, time.UTC, fakes.NewSoilHistory(fakes.NewRegistry()),
		noEvaluation{}, &fakes.FixedClock{T: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)}, logger)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatal(err)
	}
	stopSoilHistory(context.Background(), logger, runtime)
	if !strings.Contains(logs.String(), "soil history worker stopped") {
		t.Errorf("logs = %q", logs.String())
	}
}

func TestLoadAppConfigRejectsInvalidSoilHistoryConfig(t *testing.T) {
	env := validAppEnv()
	env["API_SOIL_HISTORY_HOUR"] = "25"
	_, err := loadAppConfig(func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	})
	if err == nil || !strings.Contains(err.Error(), "trafficability") || !strings.Contains(err.Error(), "API_SOIL_HISTORY_HOUR") {
		t.Errorf("error %v must name the trafficability config and API_SOIL_HISTORY_HOUR", err)
	}
}
