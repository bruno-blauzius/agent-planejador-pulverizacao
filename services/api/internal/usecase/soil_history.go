package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Daily soil state history (ADR 0022, phase 4 addendum, item 1). A background
// executor in the API, like the relief one, records once per local day the
// verdict of every field of every company, computed by the same
// TrafficabilityEvaluator as GET .../trafficability. It decides nothing itself.

// ---- ports -------------------------------------------------------------------------

// SoilHistoryField is a field to record, and whether the day is already recorded.
type SoilHistoryField struct {
	Field    FieldRef
	Recorded bool
}

// SoilStateRecord is one row of soil_state_history: a picture of the verdict.
type SoilStateRecord struct {
	Field            FieldRef
	LocalDay         time.Time
	State            domain.SoilState
	Basis            domain.SoilBasis
	RainSource       *domain.RainSource // nil without a rain source
	Last48hMm        *float64
	Last72hMm        *float64
	ModelMoisturePct *float64 // percentage of field capacity; nil without model or texture
	Texture          *domain.SoilTexture
	ThresholdsSource string
	RecordedAt       time.Time
}

// SoilHistoryStore is the persistence of the executor. It has no actor: it
// walks every company, and every query of business data still filters by
// the company it receives.
type SoilHistoryStore interface {
	// ListSoilHistoryCompanies returns the active, not deleted companies.
	ListSoilHistoryCompanies(ctx context.Context) ([]domain.CompanyID, error)
	// ListSoilHistoryFields returns the not deleted fields (of not deleted
	// farms) of the company, telling which already have a record for the day.
	ListSoilHistoryFields(ctx context.Context, companyID domain.CompanyID, day time.Time) ([]SoilHistoryField, error)
	// RecordSoilState inserts the record unless the field already has one for
	// the day (ON CONFLICT DO NOTHING); inserted tells which happened.
	RecordSoilState(ctx context.Context, record SoilStateRecord) (inserted bool, err error)
}

// FieldEvaluator computes the trafficability of a field without an actor
// (implemented by *TrafficabilityEvaluator).
type FieldEvaluator interface {
	Evaluate(ctx context.Context, field FieldRef, days int) (FieldTrafficabilityView, error)
}

var _ FieldEvaluator = (*TrafficabilityEvaluator)(nil)

// ---- configuration -----------------------------------------------------------------

// SoilHistoryConfig tunes the executor.
type SoilHistoryConfig struct {
	// Location is the zone of the local day (America/Sao_Paulo). Required.
	Location *time.Location
	// Hour is the local hour (0 to 23) from which the day is recorded (API_SOIL_HISTORY_HOUR).
	Hour int
	// Concurrency is the number of fields evaluated at once (API_SOIL_HISTORY_CONCURRENCY).
	Concurrency int
	// SweepInterval is how often the executor checks whether the day is due (API_SOIL_HISTORY_SWEEP_INTERVAL).
	SweepInterval time.Duration
	// FieldTimeout bounds the evaluation and the save of one field (API_SOIL_HISTORY_FIELD_TIMEOUT).
	FieldTimeout time.Duration
}

// Validate rejects a configuration the executor cannot run with.
func (c SoilHistoryConfig) Validate() error {
	switch {
	case c.Location == nil:
		return errors.New("soil history time zone is required")
	case c.Hour < 0 || c.Hour > 23:
		return errors.New("soil history hour must be between 0 and 23")
	case c.Concurrency < 1:
		return errors.New("soil history concurrency must be at least 1")
	case c.SweepInterval <= 0:
		return errors.New("soil history sweep interval must be positive")
	case c.FieldTimeout <= 0:
		return errors.New("soil history field timeout must be positive")
	}
	return nil
}

// historyDays is the projection asked to the evaluator: the record keeps only
// today, and one day is the smallest horizon the evaluator accepts.
const historyDays = 1

// maxSoilHistoryRunsPerDay bounds the retries of a day with failed fields:
// each new run only evaluates the fields still missing, but a field that keeps
// failing must not be retried every sweep until midnight.
const maxSoilHistoryRunsPerDay = 3

// SoilHistoryDue reports whether the executor should run at now (already in
// the farm zone): the local hour reached `hour` and the day is not done yet.
func SoilHistoryDue(now time.Time, hour int, doneDay time.Time) bool {
	day := domain.LocalDay(now, now.Location())
	return now.Hour() >= hour && !day.Equal(doneDay)
}

// ---- run summary -------------------------------------------------------------------

// SoilHistorySummary counts one execution; it is logged as soil_history.run.
type SoilHistorySummary struct {
	LocalDay           time.Time
	FieldsTotal        int
	Recorded           int
	AlreadyRecorded    int
	NoRainSource       int
	NoSoilTexture      int
	WeatherUnavailable int
	Failed             int
	Duration           time.Duration
}

type historyCounters struct {
	recorded, already, noRain, noTexture, noWeather, failed atomic.Int64
}

// ---- executor ----------------------------------------------------------------------

// SoilHistoryDeps are the dependencies of the SoilHistoryWorker.
type SoilHistoryDeps struct {
	Store     SoilHistoryStore
	Evaluator FieldEvaluator
	Clock     Clock
	// Log receives soil_history.run and the failures (identifiers only). Nil discards it.
	Log *slog.Logger
}

// SoilHistoryWorker is the daily executor of the soil history.
type SoilHistoryWorker struct {
	deps SoilHistoryDeps
	cfg  SoilHistoryConfig
	log  *slog.Logger

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Only the loop goroutine touches the schedule below.
	doneDay     time.Time
	attemptDay  time.Time
	attemptsDay int
}

// NewSoilHistoryWorker validates the configuration and builds the executor.
func NewSoilHistoryWorker(deps SoilHistoryDeps, cfg SoilHistoryConfig) (*SoilHistoryWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("soil history worker: %w", err)
	}
	log := deps.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &SoilHistoryWorker{deps: deps, cfg: cfg, log: log}, nil
}

// Start launches the loop; it stops when ctx ends or on Stop. A worker starts once.
func (w *SoilHistoryWorker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return errors.New("soil history worker already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	w.started, w.cancel = true, cancel
	w.wg.Add(1)
	go w.loop(runCtx)
	return nil
}

// Stop ends the loop and waits for the run in progress to give up (its field
// contexts derive from the loop context). Safe without Start and more than once.
func (w *SoilHistoryWorker) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	w.wg.Wait()
}

func (w *SoilHistoryWorker) loop(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		w.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick runs the day when it is due. A run with failed fields is retried on
// the next ticks (only the missing fields are evaluated again), at most
// maxSoilHistoryRunsPerDay times; a run cut by the shutdown does not count.
func (w *SoilHistoryWorker) Tick(ctx context.Context) {
	now := w.deps.Clock.Now().In(w.cfg.Location)
	if !SoilHistoryDue(now, w.cfg.Hour, w.doneDay) {
		return
	}
	day := domain.LocalDay(now, w.cfg.Location)
	if !day.Equal(w.attemptDay) {
		w.attemptDay, w.attemptsDay = day, 0
	}
	if w.attemptsDay >= maxSoilHistoryRunsPerDay {
		return
	}
	summary, err := w.RunOnce(ctx, day)
	if ctx.Err() != nil {
		return
	}
	w.attemptsDay++
	if err == nil && summary.Failed == 0 {
		w.doneDay = day
	}
}

// RunOnce records the day for every field of every active company and logs
// soil_history.run. One failed field never stops the run: it is counted and
// logged. The error is returned only when the companies cannot be listed.
func (w *SoilHistoryWorker) RunOnce(ctx context.Context, day time.Time) (SoilHistorySummary, error) {
	started := w.deps.Clock.Now()
	summary := SoilHistorySummary{LocalDay: day}
	var counters historyCounters
	companies, err := w.deps.Store.ListSoilHistoryCompanies(ctx)
	if err != nil {
		err = fmt.Errorf("list companies for the soil history: %w", err)
		w.log.Error("soil_history.failed", "local_day", day.Format(time.DateOnly), "error", err)
		return summary, err
	}
	for _, companyID := range companies {
		if ctx.Err() != nil {
			break
		}
		fields, err := w.deps.Store.ListSoilHistoryFields(ctx, companyID, day)
		if err != nil {
			counters.failed.Add(1)
			w.log.Error("soil_history.company_failed", "company_id", companyID, "error", fmt.Errorf("list fields: %w", err))
			continue
		}
		summary.FieldsTotal += len(fields)
		pending := make([]FieldRef, 0, len(fields))
		for _, field := range fields {
			if field.Recorded {
				counters.already.Add(1)
				continue
			}
			pending = append(pending, field.Field)
		}
		w.recordAll(ctx, pending, &counters)
	}
	summary.Recorded = int(counters.recorded.Load())
	summary.AlreadyRecorded = int(counters.already.Load())
	summary.NoRainSource = int(counters.noRain.Load())
	summary.NoSoilTexture = int(counters.noTexture.Load())
	summary.WeatherUnavailable = int(counters.noWeather.Load())
	summary.Failed = int(counters.failed.Load())
	summary.Duration = w.deps.Clock.Now().Sub(started)
	w.log.Info("soil_history.run",
		"local_day", day.Format(time.DateOnly),
		"fields_total", summary.FieldsTotal,
		"recorded", summary.Recorded,
		"already_recorded", summary.AlreadyRecorded,
		"no_rain_source", summary.NoRainSource,
		"no_soil_texture", summary.NoSoilTexture,
		"weather_unavailable", summary.WeatherUnavailable,
		"failed", summary.Failed,
		"duration_ms", summary.Duration.Milliseconds(),
	)
	return summary, nil
}

// recordAll evaluates the fields with bounded concurrency.
func (w *SoilHistoryWorker) recordAll(ctx context.Context, fields []FieldRef, counters *historyCounters) {
	jobs := make(chan FieldRef)
	var wg sync.WaitGroup
	for range min(w.cfg.Concurrency, len(fields)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for field := range jobs {
				w.recordField(ctx, field, counters)
			}
		}()
	}
	for _, field := range fields {
		if ctx.Err() != nil {
			break
		}
		jobs <- field
	}
	close(jobs)
	wg.Wait()
}

// recordField evaluates and stores one field within FieldTimeout. A panic is
// turned into a failure of that field only.
func (w *SoilHistoryWorker) recordField(ctx context.Context, field FieldRef, counters *historyCounters) {
	fieldCtx, cancel := context.WithTimeout(ctx, w.cfg.FieldTimeout)
	defer cancel()
	err := w.evaluateAndRecord(fieldCtx, field, counters)
	if err == nil {
		return
	}
	counters.failed.Add(1)
	if ctx.Err() == nil {
		w.log.Error("soil_history.field_failed",
			"company_id", field.CompanyID, "farm_id", field.FarmID, "field_id", field.FieldID, "error", err)
	}
}

func (w *SoilHistoryWorker) evaluateAndRecord(ctx context.Context, field FieldRef, counters *historyCounters) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.log.Error("soil_history.panic", "field_id", field.FieldID, "panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
			err = fmt.Errorf("panic while recording the soil history: %v", recovered)
		}
	}()
	view, err := w.deps.Evaluator.Evaluate(ctx, field, historyDays)
	if err != nil {
		return fmt.Errorf("evaluate trafficability: %w", err)
	}
	inserted, err := w.deps.Store.RecordSoilState(ctx, soilStateRecordOf(field, view, w.cfg.Location, w.deps.Clock.Now()))
	if err != nil {
		return fmt.Errorf("record soil state: %w", err)
	}
	if inserted {
		counters.recorded.Add(1)
	} else {
		counters.already.Add(1)
	}
	countIssues(view.DataIssues, counters)
	return nil
}

func countIssues(issues []TrafficabilityDataIssue, counters *historyCounters) {
	if slices.Contains(issues, IssueNoRainSource) {
		counters.noRain.Add(1)
	}
	if slices.Contains(issues, IssueNoSoilTexture) {
		counters.noTexture.Add(1)
	}
	if slices.Contains(issues, IssueWeatherUnavailable) {
		counters.noWeather.Add(1)
	}
}

// soilStateRecordOf copies today's verdict; the day is the local day of the
// evaluation itself, so a run that crosses midnight stores each verdict on
// the day it describes.
func soilStateRecordOf(field FieldRef, view FieldTrafficabilityView, loc *time.Location, at time.Time) SoilStateRecord {
	soil := view.Soil
	record := SoilStateRecord{
		Field: field, LocalDay: domain.LocalDay(view.AsOf, loc), State: soil.State, Basis: soil.Basis,
		Texture: soil.Texture, ThresholdsSource: soil.ThresholdsSource, RecordedAt: at,
	}
	if rain := soil.Rain; rain != nil {
		source, last48h, last72h := rain.Source, rain.Last48hMm, rain.Last72hMm
		record.RainSource, record.Last48hMm, record.Last72hMm = &source, &last48h, &last72h
	}
	if moisture := soil.ModelMoisture; moisture != nil {
		if pct, ok := moisture.FieldCapacityPct(); ok {
			record.ModelMoisturePct = &pct
		}
	}
	return record
}
