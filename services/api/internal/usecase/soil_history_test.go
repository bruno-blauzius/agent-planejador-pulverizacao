package usecase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

// historyEnv: field 1 (farm-a1) is clay with a fresh gauge (waterlogged);
// field 2 (farm-a1) has no texture; company B has one field, no texture.
type historyEnv struct {
	*conditionsEnv
	history *fakes.SoilHistory
	logs    *syncBuffer
	fieldB  domain.FieldID
}

// syncBuffer collects the JSON log lines of the concurrent executor.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// events returns the log records with the given message.
func (b *syncBuffer) events(t *testing.T, msg string) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var found []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if record["msg"] == msg {
			found = append(found, record)
		}
	}
	return found
}

func newHistoryEnv(t *testing.T) *historyEnv {
	t.Helper()
	e := &historyEnv{conditionsEnv: newConditionsEnv(t), logs: &syncBuffer{}}
	e.clayWithGauge(t, 1)
	e.weather.Report = e.dryWeather(0.2, 0)
	e.withPerimeter("farm-b1")
	listing, err := usecase.NewCreateField(e.fieldDeps()).Execute(ctx(), usecase.SaveFieldInput{
		Actor: e.asAdminB(), FarmID: "farm-b1", Details: fieldIn("Talhão B", rect(1, 1, 3, 3)),
	})
	assertNoError(t, err)
	e.fieldB = listing.Field.ID
	e.history = fakes.NewSoilHistory(e.reg, "co-a", "co-b")
	return e
}

func (e *historyEnv) config() usecase.SoilHistoryConfig {
	return usecase.SoilHistoryConfig{Location: e.brt, Hour: 7, Concurrency: 2, SweepInterval: time.Millisecond, FieldTimeout: time.Second}
}

func (e *historyEnv) worker(t *testing.T, evaluator usecase.FieldEvaluator) *usecase.SoilHistoryWorker {
	t.Helper()
	if evaluator == nil {
		evaluator = usecase.NewTrafficabilityEvaluator(usecase.GetFieldTrafficabilityDeps{
			Fields: e.conditions, Gauges: e.conditions, Thresholds: e.conditions, Terrain: e.terrain,
			Weather: e.weather, Clock: e.clock, TerrainRules: e.rules, Settings: e.settings,
		})
	}
	worker, err := usecase.NewSoilHistoryWorker(usecase.SoilHistoryDeps{
		Store: e.history, Evaluator: evaluator, Clock: e.clock,
		Log: slog.New(slog.NewJSONHandler(e.logs, nil)),
	}, e.config())
	assertNoError(t, err)
	return worker
}

func TestSoilHistoryDue(t *testing.T) {
	brt := time.FixedZone("BRT", -3*3600)
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, brt) }
	midnight := func(day int) time.Time { return time.Date(2026, 11, day, 0, 0, 0, 0, brt) }
	tests := []struct {
		name    string
		now     time.Time
		hour    int
		doneDay time.Time
		want    bool
	}{
		{"before the hour", at(22, 6, 59), 7, time.Time{}, false},
		{"at the hour", at(22, 7, 0), 7, time.Time{}, true},
		{"later the same day", at(22, 18, 0), 7, time.Time{}, true},
		{"day already done", at(22, 18, 0), 7, midnight(22), false},
		{"next day after the hour", at(23, 7, 30), 7, midnight(22), true},
		{"next day before the hour", at(23, 2, 0), 7, midnight(22), false},
		{"hour zero runs right after midnight", at(23, 0, 1), 0, midnight(22), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usecase.SoilHistoryDue(tt.now, tt.hour, tt.doneDay); got != tt.want {
				t.Errorf("due = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSoilHistoryConfigValidate(t *testing.T) {
	valid := usecase.SoilHistoryConfig{Location: time.UTC, Hour: 7, Concurrency: 4, SweepInterval: time.Minute, FieldTimeout: time.Second}
	assertNoError(t, valid.Validate())
	broken := []func(*usecase.SoilHistoryConfig){
		func(c *usecase.SoilHistoryConfig) { c.Location = nil },
		func(c *usecase.SoilHistoryConfig) { c.Hour = 24 },
		func(c *usecase.SoilHistoryConfig) { c.Hour = -1 },
		func(c *usecase.SoilHistoryConfig) { c.Concurrency = 0 },
		func(c *usecase.SoilHistoryConfig) { c.SweepInterval = 0 },
		func(c *usecase.SoilHistoryConfig) { c.FieldTimeout = 0 },
	}
	for i, mutate := range broken {
		cfg := valid
		mutate(&cfg)
		if cfg.Validate() == nil {
			t.Errorf("case %d must be refused: %+v", i, cfg)
		}
	}
}

func TestSoilHistoryRunRecordsEveryFieldOnceWithTheSameVerdict(t *testing.T) {
	e := newHistoryEnv(t)
	worker := e.worker(t, nil)
	day := e.today()

	summary, err := worker.RunOnce(ctx(), day)
	assertNoError(t, err)
	want := usecase.SoilHistorySummary{LocalDay: day, FieldsTotal: 3, Recorded: 3, NoSoilTexture: 2}
	summary.Duration = 0
	if summary != want {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}

	// The record of field 1 is exactly what GET .../trafficability answers.
	view, err := e.getTrafficability().Execute(ctx(), usecase.GetFieldTrafficabilityInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1})
	assertNoError(t, err)
	records := e.history.Records()
	var first *usecase.SoilStateRecord
	companies := map[domain.CompanyID]int{}
	for i := range records {
		companies[records[i].Field.CompanyID]++
		if records[i].Field.FieldID == e.f1 {
			first = &records[i]
		}
	}
	if companies["co-a"] != 2 || companies["co-b"] != 1 {
		t.Fatalf("records per company = %v", companies)
	}
	if first == nil || first.State != view.Soil.State || first.Basis != view.Soil.Basis || first.State != domain.SoilWaterlogged ||
		first.RainSource == nil || *first.RainSource != domain.RainFromGauge || *first.Last48hMm != 24 || *first.Last72hMm != 24 ||
		first.Texture == nil || *first.Texture != domain.TextureClay || first.ThresholdsSource != domain.ThresholdSourceHeuristic ||
		!first.LocalDay.Equal(day) || first.ModelMoisturePct == nil || first.Field.FarmID != "farm-a1" {
		t.Fatalf("record of field 1 = %+v", first)
	}

	again, err := worker.RunOnce(ctx(), day)
	assertNoError(t, err)
	if again.FieldsTotal != 3 || again.Recorded != 0 || again.AlreadyRecorded != 3 || again.Failed != 0 {
		t.Fatalf("second run = %+v, want everything already recorded", again)
	}
	if len(e.history.Records()) != 3 {
		t.Errorf("records = %d, the second run must not duplicate", len(e.history.Records()))
	}

	runs := e.logs.events(t, "soil_history.run")
	if len(runs) != 2 {
		t.Fatalf("soil_history.run events = %d, want 2", len(runs))
	}
	for _, key := range []string{"fields_total", "recorded", "already_recorded", "no_rain_source", "no_soil_texture", "weather_unavailable", "failed", "duration_ms", "local_day"} {
		if _, ok := runs[0][key]; !ok {
			t.Errorf("soil_history.run lacks %s: %v", key, runs[0])
		}
	}
	if runs[0]["recorded"] != float64(3) || runs[1]["already_recorded"] != float64(3) {
		t.Errorf("run events = %v", runs)
	}
}

func TestSoilHistoryCountsTheWeatherAndRainIssues(t *testing.T) {
	e := newHistoryEnv(t)
	e.weather.Err = errors.Join(usecase.ErrWeatherUnavailable, errBoom)

	summary, err := e.worker(t, nil).RunOnce(ctx(), e.today())
	assertNoError(t, err)
	// Field 1 keeps its fresh gauge; the other two have no rain source at all.
	if summary.Recorded != 3 || summary.WeatherUnavailable != 3 || summary.NoRainSource != 2 || summary.NoSoilTexture != 2 || summary.Failed != 0 {
		t.Fatalf("summary = %+v", summary)
	}
	for _, record := range e.history.Records() {
		if record.Field.FieldID != e.f1 && (record.State != domain.SoilUnknown || record.RainSource != nil || record.Last48hMm != nil) {
			t.Errorf("without weather nor gauge the record must be unknown without rain: %+v", record)
		}
	}
}

// panickingEvaluator breaks on one field and delegates the others.
type panickingEvaluator struct {
	inner usecase.FieldEvaluator
	field domain.FieldID
}

func (p panickingEvaluator) Evaluate(ctx context.Context, field usecase.FieldRef, days int) (usecase.FieldTrafficabilityView, error) {
	if field.FieldID == p.field {
		panic("bug in an adapter")
	}
	return p.inner.Evaluate(ctx, field, days)
}

func TestSoilHistoryIsolatesFailedFields(t *testing.T) {
	e := newHistoryEnv(t)
	e.history.FailRecord[e.f1] = errBoom
	real := usecase.NewTrafficabilityEvaluator(usecase.GetFieldTrafficabilityDeps{
		Fields: e.conditions, Gauges: e.conditions, Thresholds: e.conditions, Terrain: e.terrain,
		Weather: e.weather, Clock: e.clock, TerrainRules: e.rules, Settings: e.settings,
	})

	summary, err := e.worker(t, panickingEvaluator{inner: real, field: e.f2}).RunOnce(ctx(), e.today())
	assertNoError(t, err)
	if summary.FieldsTotal != 3 || summary.Recorded != 1 || summary.Failed != 2 {
		t.Fatalf("summary = %+v, want one recorded and two failed", summary)
	}
	failures := e.logs.events(t, "soil_history.field_failed")
	if len(failures) != 2 {
		t.Fatalf("field_failed events = %d", len(failures))
	}
	for _, failure := range failures {
		if failure["field_id"] == nil || failure["company_id"] == nil || failure["error"] == nil {
			t.Errorf("failure event lacks identifiers or error: %v", failure)
		}
	}
	if len(e.logs.events(t, "soil_history.panic")) != 1 {
		t.Error("the panic must be logged")
	}
}

func TestSoilHistoryCompanyFailures(t *testing.T) {
	e := newHistoryEnv(t)
	e.history.FailFields["co-a"] = errBoom
	summary, err := e.worker(t, nil).RunOnce(ctx(), e.today())
	assertNoError(t, err)
	if summary.Failed != 1 || summary.Recorded != 1 {
		t.Fatalf("one company failing must not stop the other: %+v", summary)
	}

	e.history.FailCompanies = errBoom
	_, err = e.worker(t, nil).RunOnce(ctx(), e.today())
	assertIs(t, err, errBoom)
}

func TestSoilHistoryTickFollowsTheLocalSchedule(t *testing.T) {
	e := newHistoryEnv(t)
	worker := e.worker(t, nil)
	e.clock.T = time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC) // 06:00 local
	runs := func() int { return len(e.logs.events(t, "soil_history.run")) }

	worker.Tick(ctx())
	if runs() != 0 || len(e.history.Records()) != 0 {
		t.Fatal("must not run before the local hour")
	}
	e.clock.T = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC) // 07:00 local
	worker.Tick(ctx())
	worker.Tick(ctx())
	if runs() != 1 || len(e.history.Records()) != 3 {
		t.Fatalf("runs = %d, records = %d: one run per day", runs(), len(e.history.Records()))
	}

	// The next day a failing field is retried a bounded number of times.
	e.clock.T = e.clock.T.Add(24 * time.Hour)
	e.history.FailRecord[e.f1] = errBoom
	for range 5 {
		worker.Tick(ctx())
	}
	if runs() != 4 {
		t.Fatalf("runs = %d, want 1 + 3 attempts on the day with a failure", runs())
	}
	if got := len(e.history.Records()); got != 5 {
		t.Errorf("records = %d, want 3 + 2 (field 1 keeps failing)", got)
	}
}

func TestSoilHistoryWorkerStartsAndStops(t *testing.T) {
	e := newHistoryEnv(t)
	worker := e.worker(t, nil)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assertNoError(t, worker.Start(runCtx))
	if err := worker.Start(runCtx); err == nil {
		t.Error("a second Start must fail")
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(e.history.Records()) < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	worker.Stop()
	worker.Stop()
	if len(e.history.Records()) != 3 {
		t.Fatalf("records = %d after the worker ran", len(e.history.Records()))
	}
}
