//go:build integration

package postgres_test

import (
	"context"
	"io"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

type microClock struct{}

func (microClock) Now() time.Time { return nowMicro() }

// editingSource wraps the in-memory source: the first time the block of a field is requested, the
// user "redraws" that field (MarkStale), like an edit that lands while the executor works on it.
type editingSource struct {
	*fakes.ElevationSource
	editKey domain.ElevationTileKey
	edit    func()
	once    atomic.Bool
}

func (s *editingSource) FetchTile(ctx context.Context, key domain.ElevationTileKey) (io.ReadCloser, error) {
	if key == s.editKey && s.once.CompareAndSwap(false, true) {
		s.edit()
	}
	return s.ElevationSource.FetchTile(ctx, key)
}

// TestTerrainWorkerAgainstPostgres runs the real executor over the real adapter: claim, download
// (a fake source that serves a synthetic GeoTIFF), ingestion, window, calculation, save and the
// closing of the run, for three fields: one calculates, one has no block in the source and one is
// edited in the middle of its calculation (its first result must be discarded and recomputed).
func TestTerrainWorkerAgainstPostgres(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t)
	addField := func(name string, lon, lat float64) domain.Field {
		field := fieldValue(f.farm, name, fieldBox(lon, lat, 0.003, 0.003))
		field.Block = nil
		insertField(t, field)
		f.fields = append(f.fields, field)
		return field
	}
	alpha := addField("Alfa", 70.05, 61.05) // block N61E070: calculates
	beta := addField("Beta", 70.05, 63.05)  // block N63E070: not in the source
	gamma := addField("Gama", 70.05, 62.05) // block N62E070: edited meanwhile
	source := fakes.NewElevationSource()
	source.Tiles["N61E070"] = ramp{ulx: 70.0, uly: 61.12, cols: 400, rows: 400, scale: 0.0003, base: 500, slopePct: 8}.geotiff(t, 61.05)
	source.Tiles["N62E070"] = ramp{ulx: 70.0, uly: 62.12, cols: 400, rows: 400, scale: 0.0003, base: 500, slopePct: 12}.geotiff(t, 62.05)

	wrapped := &editingSource{ElevationSource: source, editKey: "N62E070"}
	wrapped.edit = func() { must(t, testDB.MarkStale(ctx(), f.company.ID, gamma.ID, nowMicro())) }
	f.openRun(t, usecase.TerrainScopeAll, alpha.ID, beta.ID, gamma.ID)

	worker, err := usecase.NewTerrainWorker(usecase.TerrainWorkerDeps{
		Tx: testDB, Elevation: testDB, Source: wrapped, Clock: microClock{}, Rules: domain.DefaultTerrainRules(),
	}, usecase.TerrainWorkerConfig{SweepInterval: 50 * time.Millisecond, TileTimeout: time.Minute, FieldTimeout: time.Minute})
	must(t, err)
	t.Cleanup(worker.Stop)
	must(t, worker.Start(ctx()))

	deadline := time.Now().Add(2 * time.Minute)
	var view usecase.FarmTerrainData
	for {
		view, err = testDB.ReadFarmTerrain(ctx(), f.company.ID, f.farm.ID)
		must(t, err)
		if view.LatestRun != nil && view.LatestRun.FinishedAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run did not finish: %+v", view.LatestRun)
		}
		time.Sleep(50 * time.Millisecond)
	}
	worker.Stop()

	if run := view.LatestRun; run.Total != 3 || run.Done != 3 {
		t.Fatalf("run = %+v, want 3 of 3 done", run)
	}
	byID := map[domain.FieldID]usecase.FieldTerrainData{}
	for _, row := range view.Fields {
		byID[row.Field.Field.ID] = row
	}
	for id, want := range map[domain.FieldID]float64{alpha.ID: 8, gamma.ID: 12} {
		stored := byID[id].Terrain
		if stored == nil || stored.Status != domain.TerrainReady || stored.Calculation == nil {
			t.Fatalf("field %s = %+v, want ready", id, stored)
		}
		if got := stored.Calculation.Measurement.SlopeMeanPct; math.Abs(got-want) > 0.5 {
			t.Fatalf("field %s mean slope = %v %%, want %v +/- 0.5", id, got, want)
		}
	}
	failed := byID[beta.ID].Terrain
	if failed == nil || failed.Status != domain.TerrainFailed || failed.FailureReason != usecase.TerrainFailureTileUnavailable {
		t.Fatalf("beta = %+v, want failed with the unavailable-block reason", failed)
	}
	// The edit discarded the first result of gamma, which was then computed again, but its block was
	// downloaded once: the second pass found it stored.
	fetched := map[domain.ElevationTileKey]int{}
	for _, key := range source.Fetched() {
		fetched[key]++
	}
	if fetched["N62E070"] != 1 || fetched["N61E070"] != 1 || fetched["N63E070"] != 1 {
		t.Fatalf("blocks requested from the source = %v, want each one once", fetched)
	}
	if version := rowVersion(t, gamma.ID); version < 3 {
		t.Fatalf("gamma version = %d: the edit and the recalculation must each have bumped it", version)
	}
	drainTerrainQueue(t)
}

// A slow field of a crashed instance is recovered by the lease sweep, with a closed run and a version bump.
func TestStaleCalculatingFieldIsRecoveredAndTheRunCloses(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t, blockName("", "Preso"))
	field := f.fields[0]
	run := f.openRun(t, usecase.TerrainScopeAll, field.ID)
	claim, ok, err := testDB.ClaimNextPending(ctx(), nowMicro().Add(-time.Hour)) // claimed an hour ago, never finished
	must(t, err)
	if !ok || claim.FieldID != field.ID {
		t.Fatalf("claim = %+v, %v", claim, ok)
	}
	before := rowVersion(t, field.ID)

	// A claim made just now is outside the expiry: the sweep leaves it alone (same status and version).
	other := newTerrainFarm(t, blockName("", "Recente"))
	other.openRun(t, usecase.TerrainScopeAll, other.fieldIDs()...)
	recent, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok || recent.FieldID != other.fields[0].ID {
		t.Fatalf("claim of the recent field = %+v, %v", recent, ok)
	}
	recentVersion := rowVersion(t, recent.FieldID)

	// A lease of 30 minutes has expired for a row claimed an hour ago, but not for one claimed now.
	must(t, testDB.RecoverStale(ctx(), nowMicro().Add(-30*time.Minute), nowMicro()))
	if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainPending {
		t.Fatalf("row claimed an hour ago = %s, want pending", stored.Status)
	}
	if after := rowVersion(t, field.ID); after != before+1 {
		t.Fatalf("version = %d, want %d", after, before+1)
	}
	if _, found, err := testDB.FindOpenRun(ctx(), f.company.ID, f.farm.ID); err != nil || !found {
		t.Fatalf("run %s must stay open while the field is pending: found=%v err=%v", run.ID, found, err)
	}

	if stored := other.readField(t, recent.FieldID).Terrain; stored.Status != domain.TerrainCalculating || rowVersion(t, recent.FieldID) != recentVersion {
		t.Fatalf("recent claim = %s v%d, want it untouched (calculating v%d)", stored.Status, rowVersion(t, recent.FieldID), recentVersion)
	}

	again, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok || again.FieldID != field.ID {
		t.Fatalf("second claim = %+v, %v", again, ok)
	}
	must(t, testDB.SaveFailure(ctx(), again, "teste", nowMicro()))
	if finished, err := testDB.FinishRun(ctx(), f.company.ID, run.ID, nowMicro()); err != nil || !finished {
		t.Fatalf("FinishRun = %v, %v", finished, err)
	}
	// The late save of the first claimant cannot overwrite anything.
	requireIs(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(5), nowMicro()), usecase.ErrVersionConflict)

	// Leave nothing calculating behind for the other tests.
	must(t, testDB.RecoverStale(ctx(), nowMicro().Add(time.Hour), nowMicro()))
	drainTerrainQueue(t)
}
