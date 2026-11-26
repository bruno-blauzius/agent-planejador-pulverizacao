package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

const (
	workerGrid  = 30
	workerScale = 0.0003
)

// slopedWindow is a 30x30 grid descending to the east (about 10 %) whose field
// is the central block of insideSide x insideSide cells.
func slopedWindow(insideSide int) domain.ElevationWindow {
	w := domain.ElevationWindow{
		Rows: workerGrid, Cols: workerGrid,
		Values: make([]float64, workerGrid*workerGrid), Inside: make([]bool, workerGrid*workerGrid),
		OriginLon: -47, OriginLat: -21, ScaleXDeg: workerScale, ScaleYDeg: -workerScale,
	}
	cellM := workerScale * 111_320 * 0.93 // about the cell width at 21 S
	lo := (workerGrid - insideSide) / 2
	for r := 0; r < workerGrid; r++ {
		for c := 0; c < workerGrid; c++ {
			w.Values[r*workerGrid+c] = 800 - float64(c)*cellM*0.10
			w.Inside[r*workerGrid+c] = r >= lo && r < lo+insideSide && c >= lo && c < lo+insideSide
		}
	}
	return w
}

// workerEnv wires the executor to fakes. Both fields get a valid window by
// default; boxes lie in the single block S21W047 unless a test moves them.
type workerEnv struct {
	*terrainEnv
	elevation *fakes.Elevation
	source    *fakes.ElevationSource
}

var (
	boxS21W047 = fakes.BoundingBox{MinLon: -46.999, MinLat: -20.999, MaxLon: -46.99, MaxLat: -20.99}
	boxS21Both = fakes.BoundingBox{MinLon: -47.005, MinLat: -20.999, MaxLon: -46.99, MaxLat: -20.99} // W048 and W047
	boxS22W047 = fakes.BoundingBox{MinLon: -46.999, MinLat: -21.5, MaxLon: -46.99, MaxLat: -21.4}
)

func newWorkerEnv(t *testing.T) *workerEnv {
	t.Helper()
	e := &workerEnv{terrainEnv: newTerrainEnv(t), elevation: fakes.NewElevation(), source: fakes.NewElevationSource()}
	for _, id := range []domain.FieldID{e.f1, e.f2} {
		e.elevation.Boxes[id] = boxS21W047
		e.elevation.Windows[id] = slopedWindow(20)
	}
	e.source.Tiles["S21W047"] = []byte("geotiff S21W047")
	e.source.Tiles["S21W048"] = []byte("geotiff S21W048")
	return e
}

func (e *workerEnv) newWorker(t *testing.T, cfg usecase.TerrainWorkerConfig) *usecase.TerrainWorker {
	t.Helper()
	if cfg.SweepInterval == 0 {
		cfg.SweepInterval = 5 * time.Millisecond
	}
	if cfg.FieldTimeout == 0 {
		cfg.FieldTimeout = 5 * time.Second
	}
	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: e.terrain, Elevation: e.elevation, Source: e.source, Clock: e.clock, Rules: e.rules,
	}, cfg)
	assertNoError(t, err)
	t.Cleanup(worker.Stop)
	return worker
}

// request schedules the calculation of farm-a1 through the use case, waking the worker.
func (e *workerEnv) request(t *testing.T, wakeup usecase.TerrainWakeup, scope usecase.TerrainRunScope) {
	t.Helper()
	_, err := usecase.NewCalculateFarmTerrain(e.calculateDeps(wakeup)).Execute(ctx(), usecase.CalculateFarmTerrainInput{
		Actor: e.asAdminA(), FarmID: "farm-a1", Scope: scope,
	})
	assertNoError(t, err)
}

// eventually polls until cond holds; it fails the test after a generous deadline.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func (e *workerEnv) runFinished() bool {
	runs := e.terrain.Runs()
	return len(runs) > 0 && runs[len(runs)-1].FinishedAt != nil
}

func (e *workerEnv) status(id domain.FieldID) domain.TerrainStatus {
	stored, ok := e.terrain.Stored(id)
	if !ok {
		return domain.TerrainNotCalculated
	}
	return stored.Status
}

func (e *workerEnv) reason(id domain.FieldID) string {
	stored, _ := e.terrain.Stored(id)
	return stored.FailureReason
}

// ---- end to end ----------------------------------------------------------------------

func TestTerrainWorkerCalculatesTheRun(t *testing.T) {
	e := newWorkerEnv(t)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	worker.Stop()

	for _, id := range []domain.FieldID{e.f1, e.f2} {
		stored, _ := e.terrain.Stored(id)
		calc := stored.Calculation
		if stored.Status != domain.TerrainReady || calc == nil {
			t.Fatalf("field %s = %+v", id, stored)
		}
		if calc.Measurement.SlopeMeanPct < 8 || calc.Measurement.SlopeMeanPct > 12 || calc.ElevationModel != e.source.Model() || !calc.CalculatedAt.Equal(e.clock.T) {
			t.Fatalf("calculation of %s = %+v", id, calc)
		}
	}
	runs := e.terrain.Runs()
	if runs[0].Total != 2 || runs[0].Done != 2 {
		t.Fatalf("run = %+v", runs[0])
	}
	view, err := e.getFarm().Execute(ctx(), usecase.GetFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1"})
	assertNoError(t, err)
	if view.Progress != nil || view.Summary.CalculatedFieldCount != 2 {
		t.Fatalf("a finished run must not show progress: %+v", view)
	}
}

func TestTerrainWorkerIsWokenByTheUseCase(t *testing.T) {
	e := newWorkerEnv(t)
	// A sweep interval of one hour proves that only Wake can start the work.
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{SweepInterval: time.Hour})
	assertNoError(t, worker.Start(ctx()))

	e.request(t, worker, usecase.TerrainScopeMissing)
	eventually(t, "the run to finish after Wake", e.runFinished)
}

func TestTerrainWorkerDownloadsAMissingTileOnceAndReusesIt(t *testing.T) {
	e := newWorkerEnv(t)
	e.elevation.Boxes[e.f1] = boxS21Both // needs W048 and W047
	e.elevation.Boxes[e.f2] = boxS21W047 // needs W047 only
	e.elevation.AddTile("S21W048")       // already ingested before: must not be fetched
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)

	if got := e.source.Fetched(); len(got) != 1 || got[0] != "S21W047" {
		t.Fatalf("fetched = %v, want only S21W047 once", got)
	}
	if got := e.elevation.Ingested(); len(got) != 1 || got[0] != "S21W047" {
		t.Fatalf("ingested = %v, want S21W047 once", got)
	}
	if e.status(e.f1) != domain.TerrainReady || e.status(e.f2) != domain.TerrainReady {
		t.Fatal("both fields must be ready")
	}
}

func TestTerrainWorkerFailsAFieldWithoutItsTileAndGoesOn(t *testing.T) {
	e := newWorkerEnv(t)
	e.elevation.Boxes[e.f1] = boxS22W047 // the source has no S22W047 block
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)

	if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != usecase.TerrainFailureTileUnavailable {
		t.Fatalf("f1 = %s / %q", e.status(e.f1), e.reason(e.f1))
	}
	if e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f2 = %s, want ready despite the failure of f1", e.status(e.f2))
	}
	view, err := e.getFarm().Execute(ctx(), usecase.GetFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1"})
	assertNoError(t, err)
	if row := rowOf(t, view, e.f1); row.Status != domain.TerrainFailed || row.FailureReason == nil || row.Effective != nil {
		t.Fatalf("f1 row = %+v", row)
	}
	if e.runs() != 1 {
		t.Fatalf("runs = %d", e.runs())
	}
}

func (e *workerEnv) runs() int { return len(e.terrain.Runs()) }

func TestTerrainWorkerFailsWhenTheDownloadFails(t *testing.T) {
	e := newWorkerEnv(t)
	e.source.Tiles = map[domain.ElevationTileKey][]byte{}
	e.source.Errors["S21W047"] = context.DeadlineExceeded
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != usecase.TerrainFailureTimeout {
		t.Fatalf("f1 = %s / %q", e.status(e.f1), e.reason(e.f1))
	}
}

func TestTerrainWorkerFailsWithInsufficientData(t *testing.T) {
	e := newWorkerEnv(t)
	e.elevation.Windows[e.f1] = slopedWindow(2) // 4 cells inside, the rules ask for 20
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != usecase.TerrainFailureInsufficient {
		t.Fatalf("f1 = %s / %q", e.status(e.f1), e.reason(e.f1))
	}
	if e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f2 = %s", e.status(e.f2))
	}
}

func TestTerrainWorkerSurvivesAPanic(t *testing.T) {
	e := newWorkerEnv(t)
	e.elevation.OnWindow = func(id domain.FieldID) {
		if id == e.f1 {
			panic("boom in the elevation adapter")
		}
	}
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != usecase.TerrainFailureUnexpected {
		t.Fatalf("f1 = %s / %q", e.status(e.f1), e.reason(e.f1))
	}
	if e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f2 = %s, the executor must go on after a panic", e.status(e.f2))
	}
}

func TestTerrainWorkerNeverCalculatesAFieldTwice(t *testing.T) {
	e := newWorkerEnv(t)
	var calls sync.Map // FieldID -> *atomic.Int32
	e.elevation.OnWindow = func(id domain.FieldID) {
		counter, _ := calls.LoadOrStore(id, new(atomic.Int32))
		counter.(*atomic.Int32).Add(1)
	}
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{Concurrency: 4})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	for _, id := range []domain.FieldID{e.f1, e.f2} {
		counter, ok := calls.Load(id)
		if !ok || counter.(*atomic.Int32).Load() != 1 {
			t.Fatalf("field %s was calculated %v times, want 1", id, counter)
		}
	}
}

func TestTerrainWorkerRecalculatesEverythingForScopeAll(t *testing.T) {
	e := newWorkerEnv(t)
	e.seedReady(e.f1, 60) // an old, very different result
	e.request(t, e.wakeups, usecase.TerrainScopeAll)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	stored, _ := e.terrain.Stored(e.f1)
	if stored.Status != domain.TerrainReady || stored.Calculation.Measurement.SlopeMeanPct > 15 {
		t.Fatalf("f1 = %+v", stored)
	}
}

// ---- stopping and resuming -----------------------------------------------------------

func TestTerrainWorkerStopsCleanlyAndResumes(t *testing.T) {
	e := newWorkerEnv(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	e.elevation.OnWindow = func(domain.FieldID) {
		once.Do(func() {
			close(entered)
			<-release
		})
	}
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	first := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, first.Start(ctx()))

	<-entered // the first field is being calculated
	stopped := make(chan struct{})
	go func() {
		first.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop must wait for the field in progress")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-stopped

	ready, pending := 0, 0
	for _, id := range []domain.FieldID{e.f1, e.f2} {
		switch e.status(id) {
		case domain.TerrainReady:
			ready++
		case domain.TerrainPending:
			pending++
		default:
			t.Fatalf("field %s = %s after a clean stop", id, e.status(id))
		}
	}
	if ready != 1 || pending != 1 || e.runFinished() {
		t.Fatalf("after stop: ready=%d pending=%d finished=%v, want 1, 1, false", ready, pending, e.runFinished())
	}

	second := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, second.Start(ctx()))
	eventually(t, "the resumed run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainReady || e.status(e.f2) != domain.TerrainReady {
		t.Fatal("both fields must be ready after the resume")
	}
}

func TestTerrainWorkerStopsWhenTheContextEnds(t *testing.T) {
	e := newWorkerEnv(t)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	runCtx, cancel := context.WithCancel(ctx())
	assertNoError(t, worker.Start(runCtx))
	cancel()
	worker.Stop() // returns: the goroutines left on the cancelled context
	worker.Stop() // and stopping again is harmless
	if err := worker.Start(ctx()); err == nil {
		t.Fatal("a stopped worker must not start again")
	}
}

func TestTerrainWorkerRecoversInterruptedFields(t *testing.T) {
	e := newWorkerEnv(t)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	e.terrain.ForceStatus(e.f1, domain.TerrainCalculating) // the previous process died in the middle of f1
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainReady || e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f1 = %s, f2 = %s", e.status(e.f1), e.status(e.f2))
	}
}

func TestTerrainWorkerFinishesARunThatWasAlreadyDrained(t *testing.T) {
	e := newWorkerEnv(t)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	// The previous process saved the last field but died before FinishRun.
	e.seedReady(e.f1, 5)
	e.seedReady(e.f2, 6)
	stored := e.terrain.Runs()[0]
	e.terrain.AttachToRun(stored.ID, e.f1, e.f2)

	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))
	if !e.runFinished() {
		t.Fatal("Start must finish a run with nothing left to do")
	}
}

func TestTerrainWorkerDiscardsAResultWhenTheFieldWasEditedMeanwhile(t *testing.T) {
	e := newWorkerEnv(t)
	var edited atomic.Bool
	var windows atomic.Int32
	e.elevation.OnWindow = func(id domain.FieldID) {
		windows.Add(1)
		if id == e.f1 && edited.CompareAndSwap(false, true) {
			// The user redraws the field while the executor works on it.
			_, err := usecase.NewUpdateField(e.fieldDeps()).Execute(ctx(), usecase.SaveFieldInput{
				Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Details: fieldIn("Talhão 1", rect(1, 1, 3.5, 3.5)),
			})
			if err != nil {
				panic(err)
			}
		}
	}
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	// f1 was calculated twice: the first result (old polygon) was discarded
	// by the version check, and the field went back to pending and was recomputed.
	if windows.Load() != 3 {
		t.Fatalf("windows cut = %d, want 3 (f1 twice, f2 once)", windows.Load())
	}
	if e.status(e.f1) != domain.TerrainReady || e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f1 = %s, f2 = %s", e.status(e.f1), e.status(e.f2))
	}
}

func TestNewTerrainWorkerRejectsInvalidRules(t *testing.T) {
	_, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{Rules: domain.TerrainRules{}}, usecase.TerrainWorkerConfig{})
	assertIs(t, err, domain.ErrInvalidTerrainRules)
}

// failingSave wraps the terrain transactions so that saving the calculation of
// one field fails with a database error that is not a version conflict.
type failingSave struct {
	usecase.TerrainTxRunner
	field       domain.FieldID
	failFailure bool
}

func (f failingSave) InTerrainTx(ctx context.Context, fn func(context.Context, usecase.TerrainStores) error) error {
	return f.TerrainTxRunner.InTerrainTx(ctx, func(ctx context.Context, tx usecase.TerrainStores) error {
		tx.Terrain = failingStore{TerrainStore: tx.Terrain, field: f.field, failFailure: f.failFailure}
		return fn(ctx, tx)
	})
}

type failingStore struct {
	usecase.TerrainStore
	field       domain.FieldID
	failFailure bool
}

func (s failingStore) SaveCalculation(ctx context.Context, claim usecase.TerrainClaim, c domain.TerrainCalculation, at time.Time) error {
	if claim.FieldID == s.field {
		return errors.New("numeric field overflow")
	}
	return s.TerrainStore.SaveCalculation(ctx, claim, c, at)
}

func (s failingStore) SaveFailure(ctx context.Context, claim usecase.TerrainClaim, reason string, at time.Time) error {
	if s.failFailure && claim.FieldID == s.field {
		return errors.New("database down")
	}
	return s.TerrainStore.SaveFailure(ctx, claim, reason, at)
}

func TestTerrainWorkerFailsAFieldWhenSavingTheCalculationFails(t *testing.T) {
	e := newWorkerEnv(t)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: failingSave{TerrainTxRunner: e.terrain, field: e.f1}, Elevation: e.elevation, Source: e.source,
		Clock: e.clock, Rules: e.rules,
	}, usecase.TerrainWorkerConfig{SweepInterval: 5 * time.Millisecond})
	assertNoError(t, err)
	t.Cleanup(worker.Stop)
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != usecase.TerrainFailureUnexpected {
		t.Fatalf("f1 = %s / %q, it must not stay calculating", e.status(e.f1), e.reason(e.f1))
	}
	if e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f2 = %s, the executor must go on", e.status(e.f2))
	}
}

func TestTerrainWorkerGoesOnWhenEvenTheFailureCannotBeSaved(t *testing.T) {
	e := newWorkerEnv(t)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: failingSave{TerrainTxRunner: e.terrain, field: e.f1, failFailure: true}, Elevation: e.elevation, Source: e.source,
		Clock: e.clock, Rules: e.rules,
	}, usecase.TerrainWorkerConfig{SweepInterval: 5 * time.Millisecond})
	assertNoError(t, err)
	t.Cleanup(worker.Stop)
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "f2 to be calculated", func() bool { return e.status(e.f2) == domain.TerrainReady })
	if e.status(e.f1) != domain.TerrainCalculating {
		t.Fatalf("f1 = %s, it waits for its lease to expire", e.status(e.f1))
	}
}

// steppedClock is a clock tests move by hand while the worker reads it from its goroutines.
type steppedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *steppedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *steppedClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// flakyTx makes both saves of one field fail while broken is set: the database
// is down exactly when the executor wants to record the result of that field.
type flakyTx struct {
	usecase.TerrainTxRunner
	field  domain.FieldID
	broken *atomic.Bool
}

func (f flakyTx) InTerrainTx(ctx context.Context, fn func(context.Context, usecase.TerrainStores) error) error {
	return f.TerrainTxRunner.InTerrainTx(ctx, func(ctx context.Context, tx usecase.TerrainStores) error {
		tx.Terrain = flakyStore{TerrainStore: tx.Terrain, field: f.field, broken: f.broken}
		return fn(ctx, tx)
	})
}

type flakyStore struct {
	usecase.TerrainStore
	field  domain.FieldID
	broken *atomic.Bool
}

func (s flakyStore) SaveCalculation(ctx context.Context, claim usecase.TerrainClaim, c domain.TerrainCalculation, at time.Time) error {
	if s.broken.Load() && claim.FieldID == s.field {
		return errors.New("database down")
	}
	return s.TerrainStore.SaveCalculation(ctx, claim, c, at)
}

func (s flakyStore) SaveFailure(ctx context.Context, claim usecase.TerrainClaim, reason string, at time.Time) error {
	if s.broken.Load() && claim.FieldID == s.field {
		return errors.New("database down")
	}
	return s.TerrainStore.SaveFailure(ctx, claim, reason, at)
}

func TestTerrainWorkerRecoversAFieldStuckCalculatingWhenItsLeaseExpires(t *testing.T) {
	e := newWorkerEnv(t)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	clock := &steppedClock{t: e.clock.T}
	var broken atomic.Bool
	broken.Store(true)
	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: flakyTx{TerrainTxRunner: e.terrain, field: e.f1, broken: &broken}, Elevation: e.elevation, Source: e.source,
		Clock: clock, Rules: e.rules,
	}, usecase.TerrainWorkerConfig{SweepInterval: 5 * time.Millisecond, TileTimeout: time.Second, FieldTimeout: time.Second, LeaseSlack: time.Second})
	assertNoError(t, err)
	t.Cleanup(worker.Stop)
	assertNoError(t, worker.Start(ctx()))

	// Neither the result nor the failure of f1 could be saved: it stays calculating, and
	// the run stays open. Many sweeps pass, but its lease (3s) is still running, so it is
	// not taken from a worker that may be alive.
	eventually(t, "f2 to be calculated", func() bool { return e.status(e.f2) == domain.TerrainReady })
	time.Sleep(50 * time.Millisecond)
	if e.status(e.f1) != domain.TerrainCalculating || e.runFinished() {
		t.Fatalf("f1 = %s, run finished = %v: a field inside its lease must be left alone", e.status(e.f1), e.runFinished())
	}

	// The database comes back and the lease expires: the sweep requeues f1, it is
	// calculated again and the run closes.
	broken.Store(false)
	clock.Advance(time.Hour)
	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainReady {
		t.Fatalf("f1 = %s, want ready after the recovery", e.status(e.f1))
	}
}

// panickingStore panics when saving the calculation of one field, as a buggy adapter would.
type panickingStore struct {
	usecase.TerrainStore
	field domain.FieldID
}

func (s panickingStore) SaveCalculation(ctx context.Context, claim usecase.TerrainClaim, c domain.TerrainCalculation, at time.Time) error {
	if claim.FieldID == s.field {
		panic("boom in the store")
	}
	return s.TerrainStore.SaveCalculation(ctx, claim, c, at)
}

type panickingTx struct {
	usecase.TerrainTxRunner
	field domain.FieldID
}

func (p panickingTx) InTerrainTx(ctx context.Context, fn func(context.Context, usecase.TerrainStores) error) error {
	return p.TerrainTxRunner.InTerrainTx(ctx, func(ctx context.Context, tx usecase.TerrainStores) error {
		tx.Terrain = panickingStore{TerrainStore: tx.Terrain, field: p.field}
		return fn(ctx, tx)
	})
}

func TestTerrainWorkerSurvivesAPanicOutsideTheCalculation(t *testing.T) {
	e := newWorkerEnv(t)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: panickingTx{TerrainTxRunner: e.terrain, field: e.f1}, Elevation: e.elevation, Source: e.source,
		Clock: e.clock, Rules: e.rules,
	}, usecase.TerrainWorkerConfig{SweepInterval: 5 * time.Millisecond})
	assertNoError(t, err)
	t.Cleanup(worker.Stop)
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != usecase.TerrainFailureUnexpected {
		t.Fatalf("f1 = %s / %q, a panic in the store must fail the field", e.status(e.f1), e.reason(e.f1))
	}
	if e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f2 = %s, the executor must go on", e.status(e.f2))
	}
}

// slowIngestion waits for d, or for the end of the context, like a database busy decoding a block.
func slowIngestion(d time.Duration) func(context.Context, domain.ElevationTileKey) error {
	return func(ctx context.Context, _ domain.ElevationTileKey) error {
		select {
		case <-time.After(d):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func TestTerrainWorkerGivesTheIngestionItsOwnTimeBudget(t *testing.T) {
	e := newWorkerEnv(t)
	// The ingestion takes four times the FieldTimeout, but the block has its own, larger budget.
	e.elevation.OnIngest = slowIngestion(400 * time.Millisecond)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{TileTimeout: 10 * time.Second, FieldTimeout: 100 * time.Millisecond})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainReady || e.status(e.f2) != domain.TerrainReady {
		t.Fatalf("f1 = %s / %q, f2 = %s: a slow ingestion within the tile timeout must not fail the field", e.status(e.f1), e.reason(e.f1), e.status(e.f2))
	}
}

func TestTerrainWorkerFailsWithATimeoutWhenTheBlockExceedsItsBudget(t *testing.T) {
	e := newWorkerEnv(t)
	e.elevation.OnIngest = slowIngestion(10 * time.Second)
	e.request(t, e.wakeups, usecase.TerrainScopeMissing)
	worker := e.newWorker(t, usecase.TerrainWorkerConfig{TileTimeout: 30 * time.Millisecond, FieldTimeout: 10 * time.Second})
	assertNoError(t, worker.Start(ctx()))

	eventually(t, "the run to finish", e.runFinished)
	if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != usecase.TerrainFailureTimeout {
		t.Fatalf("f1 = %s / %q", e.status(e.f1), e.reason(e.f1))
	}
}

func TestTerrainWorkerMapsTheFailureReasons(t *testing.T) {
	tests := []struct {
		name   string
		ingest error
		want   string
	}{
		{"database statement timeout", fmt.Errorf("ingest: %w", usecase.ErrElevationTimeout), usecase.TerrainFailureTimeout},
		{"download deadline wrapped as unavailable", fmt.Errorf("%w: %w", usecase.ErrElevationUnavailable, context.DeadlineExceeded), usecase.TerrainFailureTimeout},
		{"block refused without a deadline (404)", fmt.Errorf("%w: %w", usecase.ErrElevationUnavailable, errors.New("status 404")), usecase.TerrainFailureTileUnavailable},
		{"download cut in the middle", fmt.Errorf("read geotiff: %w", usecase.ErrElevationUnavailable), usecase.TerrainFailureTileUnavailable},
		{"too few cells", fmt.Errorf("window: %w", domain.ErrInsufficientElevationData), usecase.TerrainFailureInsufficient},
		{"anything else", errors.New("disk full"), usecase.TerrainFailureUnexpected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newWorkerEnv(t)
			e.elevation.OnIngest = func(context.Context, domain.ElevationTileKey) error { return tt.ingest }
			e.request(t, e.wakeups, usecase.TerrainScopeMissing)
			worker := e.newWorker(t, usecase.TerrainWorkerConfig{})
			assertNoError(t, worker.Start(ctx()))

			eventually(t, "the run to finish", e.runFinished)
			if e.status(e.f1) != domain.TerrainFailed || e.reason(e.f1) != tt.want {
				t.Fatalf("f1 = %s / %q, want %q", e.status(e.f1), e.reason(e.f1), tt.want)
			}
		})
	}
}
