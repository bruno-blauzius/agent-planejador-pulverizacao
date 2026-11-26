//go:build integration

package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/platform"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

func nowMicro() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// drainTerrainQueue empties the open runs of every test: ClaimNextPending is global (it is
// the executor's queue), so a test that claims must not meet the leftovers of another one.
func drainTerrainQueue(t *testing.T) {
	t.Helper()
	for {
		claim, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
		must(t, err)
		if !ok {
			return
		}
		must(t, testDB.SaveFailure(ctx(), claim, "limpeza do teste", nowMicro()))
		_, err = testDB.FinishRun(ctx(), claim.CompanyID, claim.RunID, nowMicro())
		must(t, err)
	}
}

// sampleCalculation has values that survive the numeric(7,3), numeric(8,2) and numeric[] columns.
func sampleCalculation(meanPct float64) domain.TerrainCalculation {
	p90, aspect := meanPct+3.25, domain.AspectSoutheast
	var histogram [domain.SlopeHistogramBins]float64
	histogram[int(meanPct)] = 1.5
	histogram[domain.SlopeHistogramBins-1] = 0.25
	return domain.TerrainCalculation{
		Measurement: domain.TerrainMeasurement{
			SlopeMeanPct: meanPct, SlopeP90Pct: &p90, SlopeMaxPct: meanPct + 10.5,
			Position: domain.PositionSlope, Aspect: &aspect,
		},
		ElevationMinM: 801.25, ElevationMaxM: 845.5, SlopeHistogramHa: histogram,
		Profile:        []domain.ProfilePoint{{DistanceM: 10, ElevationM: 801.25}, {DistanceM: 30, ElevationM: 830.5}},
		ElevationModel: domain.ElevationModel{Source: "copernicus_glo30", ResolutionM: 30, Version: "2022_1"},
		CalculatedAt:   nowMicro(),
	}
}

type terrainFixture struct {
	company domain.Company
	farm    domain.Farm
	user    domain.User
	fields  []domain.Field
}

// newTerrainFarm creates a company with a farm and fields in the given blocks and names.
func newTerrainFarm(t *testing.T, fields ...struct{ block, name string }) terrainFixture {
	t.Helper()
	company := newCompany(t)
	farm := farmValue(company, "Fazenda Relevo")
	insertFarm(t, farm)
	fixture := terrainFixture{company: company, farm: farm, user: newUser(t, company)}
	for i, spec := range fields {
		left := float64(1 + 3*i)
		field := fieldValue(farm, spec.name, fieldRect(left, 1, left+2, 3))
		if spec.block == "" {
			field.Block = nil
		} else {
			block := spec.block
			field.Block = &block
		}
		insertField(t, field)
		fixture.fields = append(fixture.fields, field)
	}
	return fixture
}

func blockName(block, name string) struct{ block, name string } {
	return struct{ block, name string }{block, name}
}

// openRun creates a run covering the fields and marks them pending, like the use case does.
func (f terrainFixture) openRun(t *testing.T, scope usecase.TerrainRunScope, ids ...domain.FieldID) usecase.TerrainRun {
	t.Helper()
	run := usecase.TerrainRun{
		ID: newID(), CompanyID: f.company.ID, FarmID: f.farm.ID, Scope: scope,
		RequestedBy: f.user.ID, RequestedAt: nowMicro(),
	}
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		if err := tx.Terrain.LockFarmForTerrain(ctx, run.CompanyID, run.FarmID); err != nil {
			return err
		}
		if err := tx.Terrain.CreateRun(ctx, run); err != nil {
			return err
		}
		return tx.Terrain.MarkFieldsPending(ctx, run, ids, nowMicro())
	}))
	return run
}

func (f terrainFixture) fieldIDs() []domain.FieldID {
	ids := make([]domain.FieldID, len(f.fields))
	for i, field := range f.fields {
		ids[i] = field.ID
	}
	return ids
}

func (f terrainFixture) readField(t *testing.T, id domain.FieldID) usecase.FieldTerrainData {
	t.Helper()
	data, err := testDB.ReadFieldTerrain(ctx(), f.company.ID, f.farm.ID, id)
	must(t, err)
	return data
}

func rowVersion(t *testing.T, fieldID domain.FieldID) int {
	t.Helper()
	var version int
	must(t, testPool.QueryRow(ctx(), `SELECT version FROM field_terrain WHERE field_id = $1`, string(fieldID)).Scan(&version))
	return version
}

func TestReadFarmTerrainOrderStatesAndRun(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t,
		blockName("Área Norte", "Beta"), blockName("Área Norte", "Alfa"), blockName("", "Zeta"), blockName("Área Sul", "Gama"))
	alfa, zeta := f.fields[1], f.fields[2]

	before, err := testDB.ReadFarmTerrain(ctx(), f.company.ID, f.farm.ID)
	must(t, err)
	if len(before.Fields) != 4 || before.LatestRun != nil {
		t.Fatalf("a farm that never calculated: %d fields, run %+v", len(before.Fields), before.LatestRun)
	}
	// Same order as ListFields: block NULLS FIRST, then name.
	for i, name := range []string{"Zeta", "Alfa", "Beta", "Gama"} {
		if before.Fields[i].Field.Field.Name != name {
			t.Fatalf("position %d = %s, want %s", i, before.Fields[i].Field.Field.Name, name)
		}
		if before.Fields[i].Terrain != nil || before.Fields[i].Adjustment != nil {
			t.Fatalf("%s has relief before any run: %+v", name, before.Fields[i])
		}
	}

	var selected []domain.FieldID
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		var err error
		selected, err = tx.Terrain.SelectFieldsForRun(ctx, f.company.ID, f.farm.ID, usecase.TerrainScopeMissing)
		return err
	}))
	if len(selected) != 4 || selected[0] != zeta.ID || selected[1] != alfa.ID {
		t.Fatalf("missing scope selected %v", selected)
	}
	run := f.openRun(t, usecase.TerrainScopeMissing, selected...)

	pending, err := testDB.ReadFarmTerrain(ctx(), f.company.ID, f.farm.ID)
	must(t, err)
	if pending.LatestRun == nil || pending.LatestRun.ID != run.ID || pending.LatestRun.Total != 4 || pending.LatestRun.Done != 0 ||
		pending.LatestRun.FinishedAt != nil || pending.LatestRun.Scope != usecase.TerrainScopeMissing || pending.LatestRun.RequestedBy != f.user.ID {
		t.Fatalf("pending run = %+v", pending.LatestRun)
	}
	for _, item := range pending.Fields {
		if item.Terrain == nil || item.Terrain.Status != domain.TerrainPending || item.Terrain.Calculation != nil {
			t.Fatalf("%s should be pending without calculation: %+v", item.Field.Field.Name, item.Terrain)
		}
	}

	// The executor takes the fields in the order they were requested; ready, failed, still open.
	byField := map[domain.FieldID]usecase.TerrainClaim{}
	for range 3 {
		claim, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
		must(t, err)
		if !ok || claim.RunID != run.ID || claim.CompanyID != f.company.ID || claim.FarmID != f.farm.ID {
			t.Fatalf("claim = %+v, ok=%v", claim, ok)
		}
		byField[claim.FieldID] = claim
	}
	calc := sampleCalculation(12.5)
	var failed domain.FieldID
	ready := 0
	for id, claim := range byField {
		switch {
		case ready < 2:
			must(t, testDB.SaveCalculation(ctx(), claim, calc, nowMicro()))
			ready++
		default:
			failed = id
			must(t, testDB.SaveFailure(ctx(), claim, "Sem dados de elevação para esta área.", nowMicro()))
		}
	}

	mid, err := testDB.ReadFarmTerrain(ctx(), f.company.ID, f.farm.ID)
	must(t, err)
	if mid.LatestRun.Total != 4 || mid.LatestRun.Done != 3 || mid.LatestRun.FinishedAt != nil {
		t.Fatalf("run in progress = %+v", mid.LatestRun)
	}
	statuses := map[domain.TerrainStatus]int{}
	for _, item := range mid.Fields {
		statuses[item.Terrain.Status]++
		if item.Terrain.Status == domain.TerrainFailed && (item.Field.Field.ID != failed || item.Terrain.FailureReason != "Sem dados de elevação para esta área.") {
			t.Fatalf("failed row = %+v", item.Terrain)
		}
	}
	if statuses[domain.TerrainReady] != 2 || statuses[domain.TerrainFailed] != 1 || statuses[domain.TerrainPending] != 1 {
		t.Fatalf("statuses = %v", statuses)
	}

	// Round trip of the calculation through numeric(7,3), numeric[] and jsonb.
	var readyID domain.FieldID
	for id := range byField {
		if id != failed {
			readyID = id
			break
		}
	}
	got := f.readField(t, readyID).Terrain.Calculation
	if got == nil || got.Measurement.SlopeMeanPct != 12.5 || *got.Measurement.SlopeP90Pct != 15.75 || got.Measurement.SlopeMaxPct != 23 ||
		got.Measurement.Position != domain.PositionSlope || *got.Measurement.Aspect != domain.AspectSoutheast ||
		got.ElevationMinM != 801.25 || got.ElevationMaxM != 845.5 || got.SlopeHistogramHa != calc.SlopeHistogramHa ||
		len(got.Profile) != 2 || got.Profile[1] != (domain.ProfilePoint{DistanceM: 30, ElevationM: 830.5}) ||
		got.ElevationModel != calc.ElevationModel || !got.CalculatedAt.Equal(calc.CalculatedAt) {
		t.Fatalf("stored calculation differs:\n got %+v\nwant %+v", got, calc)
	}

	// Close the run: FinishRun refuses while a field is still pending, then accepts.
	finished, err := testDB.FinishRun(ctx(), f.company.ID, run.ID, nowMicro())
	must(t, err)
	if finished {
		t.Fatal("FinishRun finished a run that still has a pending field")
	}
	last, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok {
		t.Fatal("the fourth field was not claimed")
	}
	must(t, testDB.SaveCalculation(ctx(), last, sampleCalculation(4), nowMicro()))
	finished, err = testDB.FinishRun(ctx(), f.company.ID, run.ID, nowMicro())
	must(t, err)
	if !finished {
		t.Fatal("FinishRun did not close the run after every field was done")
	}
	done, err := testDB.ReadFarmTerrain(ctx(), f.company.ID, f.farm.ID)
	must(t, err)
	if done.LatestRun.FinishedAt == nil || done.LatestRun.Done != 4 {
		t.Fatalf("finished run = %+v", done.LatestRun)
	}
}

func TestFinishRunReportsOnlyTheCallThatClosedIt(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t, blockName("", "Único"))
	run := f.openRun(t, usecase.TerrainScopeAll, f.fieldIDs()...)
	claim, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok {
		t.Fatal("nothing to claim")
	}
	if finished, err := testDB.FinishRun(ctx(), f.company.ID, run.ID, nowMicro()); err != nil || finished {
		t.Fatalf("FinishRun with a calculating field = %v, %v", finished, err)
	}
	must(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(8), nowMicro()))
	if finished, err := testDB.FinishRun(ctx(), f.company.ID, run.ID, nowMicro()); err != nil || !finished {
		t.Fatalf("first FinishRun = %v, %v", finished, err)
	}
	if finished, err := testDB.FinishRun(ctx(), f.company.ID, run.ID, nowMicro()); err != nil || finished {
		t.Fatalf("second FinishRun = %v, %v", finished, err)
	}
	if _, found, err := testDB.FindOpenRun(ctx(), f.company.ID, f.farm.ID); err != nil || found {
		t.Fatalf("FindOpenRun after finishing = %v, %v", found, err)
	}
}

func TestOpenRunAndScopes(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t, blockName("", "A"), blockName("", "B"), blockName("", "C"))
	a, b := f.fields[0], f.fields[1]
	run := f.openRun(t, usecase.TerrainScopeMissing, a.ID, b.ID)

	open, found, err := testDB.FindOpenRun(ctx(), f.company.ID, f.farm.ID)
	must(t, err)
	if !found || open.ID != run.ID || open.Total != 2 || open.Done != 0 {
		t.Fatalf("open run = %+v, found=%v", open, found)
	}
	// pending rows count as "missing" work; the field without a row does too.
	var missing, all []domain.FieldID
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		var err error
		if missing, err = tx.Terrain.SelectFieldsForRun(ctx, f.company.ID, f.farm.ID, usecase.TerrainScopeMissing); err != nil {
			return err
		}
		all, err = tx.Terrain.SelectFieldsForRun(ctx, f.company.ID, f.farm.ID, usecase.TerrainScopeAll)
		return err
	}))
	if len(missing) != 3 || len(all) != 3 {
		t.Fatalf("missing=%v all=%v", missing, all)
	}
	// Once A is ready it leaves the "missing" scope but stays in "all".
	claim, _, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	must(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(6), nowMicro()))
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		var err error
		missing, err = tx.Terrain.SelectFieldsForRun(ctx, f.company.ID, f.farm.ID, usecase.TerrainScopeMissing)
		return err
	}))
	if len(missing) != 2 {
		t.Fatalf("missing after one ready = %v", missing)
	}
	if err := testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		_, err := tx.Terrain.SelectFieldsForRun(ctx, f.company.ID, f.farm.ID, usecase.TerrainRunScope("everything"))
		return err
	}); err == nil {
		t.Fatal("an unknown scope must be refused")
	}
	drainTerrainQueue(t)
}

func TestClaimNextPendingNeverHandsOutTheSameField(t *testing.T) {
	drainTerrainQueue(t)
	const fieldCount = 8
	specs := make([]struct{ block, name string }, fieldCount)
	for i := range specs {
		specs[i] = blockName("", string(rune('A'+i)))
	}
	f := newTerrainFarm(t, specs...)
	f.openRun(t, usecase.TerrainScopeAll, f.fieldIDs()...)

	claims := make(chan usecase.TerrainClaim, fieldCount*2)
	errs := make(chan error, fieldCount*2)
	var wg sync.WaitGroup
	for range fieldCount * 2 { // more executors than fields
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
			if err != nil {
				errs <- err
				return
			}
			if ok {
				claims <- claim
			}
		}()
	}
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		t.Fatalf("claim failed: %v", err)
	}
	seen := map[domain.FieldID]bool{}
	for claim := range claims {
		if seen[claim.FieldID] {
			t.Fatalf("field %s claimed twice", claim.FieldID)
		}
		seen[claim.FieldID] = true
	}
	if len(seen) != fieldCount {
		t.Fatalf("%d fields claimed, want %d", len(seen), fieldCount)
	}
	if _, ok, err := testDB.ClaimNextPending(ctx(), nowMicro()); err != nil || ok {
		t.Fatalf("a ninth claim = %v, %v", ok, err)
	}
	// Leave nothing calculating behind.
	for _, field := range f.fields {
		if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainCalculating {
			t.Fatalf("%s is %s, want calculating", field.Name, stored.Status)
		}
	}
	must(t, testDB.RecoverStale(ctx(), nowMicro().Add(time.Hour), nowMicro()))
	drainTerrainQueue(t)
}

func TestClaimServesTheOldestRunFirstAndSkipsClosedRuns(t *testing.T) {
	drainTerrainQueue(t)
	older := newTerrainFarm(t, blockName("", "Velho"))
	newer := newTerrainFarm(t, blockName("", "Novo"))
	olderRun := older.openRun(t, usecase.TerrainScopeAll, older.fieldIDs()...)
	newer.openRun(t, usecase.TerrainScopeAll, newer.fieldIDs()...)

	claim, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok || claim.RunID != olderRun.ID {
		t.Fatalf("first claim = %+v, want the oldest run %s", claim, olderRun.ID)
	}
	must(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(3), nowMicro()))
	_, err = testDB.FinishRun(ctx(), claim.CompanyID, claim.RunID, nowMicro())
	must(t, err)

	// A pending row whose run is already closed (an edit after the run) waits for the next request.
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		return tx.Terrain.MarkStale(ctx, older.company.ID, older.fields[0].ID, nowMicro())
	}))
	next, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok || next.FieldID != newer.fields[0].ID {
		t.Fatalf("second claim = %+v, want the field of the newer run", next)
	}
	if again, ok, err := testDB.ClaimNextPending(ctx(), nowMicro()); err != nil || ok {
		t.Fatalf("the stale field of a closed run was claimed: %+v, %v, %v", again, ok, err)
	}
	if stored := older.readField(t, older.fields[0].ID).Terrain; stored.Status != domain.TerrainPending {
		t.Fatalf("stale field of a closed run = %s", stored.Status)
	}
	must(t, testDB.RecoverStale(ctx(), nowMicro().Add(time.Hour), nowMicro()))
	drainTerrainQueue(t)
}

func TestSaveAfterAnEditIsAVersionConflict(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t, blockName("", "Editado"))
	field := f.fields[0]
	run := f.openRun(t, usecase.TerrainScopeAll, field.ID)

	// An earlier good result, so we can see MarkStale clear it.
	first, _, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	must(t, testDB.SaveCalculation(ctx(), first, sampleCalculation(9), nowMicro()))
	again := f.openRun(t, usecase.TerrainScopeAll, field.ID)
	if again.ID == run.ID {
		t.Fatal("expected a new run")
	}
	// Reopened: pending again, last calculation still there.
	if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainPending || stored.Calculation == nil {
		t.Fatalf("reopened row = %+v", stored)
	}

	claim, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok || claim.RunID != again.ID {
		t.Fatalf("claim = %+v, ok=%v", claim, ok)
	}
	// The field is edited between the claim and the save.
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		return tx.Terrain.MarkStale(ctx, f.company.ID, field.ID, nowMicro())
	}))
	requireIs(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(30), nowMicro()), usecase.ErrVersionConflict)
	requireIs(t, testDB.SaveFailure(ctx(), claim, "falha qualquer", nowMicro()), usecase.ErrVersionConflict)

	stale := f.readField(t, field.ID).Terrain
	if stale.Status != domain.TerrainPending || stale.Calculation != nil || stale.FailureReason != "" {
		t.Fatalf("stale row = %+v", stale)
	}
	// run_id is kept: the open run recomputes the field with the new polygon.
	retry, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok || retry.RunID != again.ID || retry.RowVersion <= claim.RowVersion {
		t.Fatalf("retry = %+v (ok=%v), want the same run and a newer version than %d", retry, ok, claim.RowVersion)
	}
	must(t, testDB.SaveCalculation(ctx(), retry, sampleCalculation(31), nowMicro()))
	if got := f.readField(t, field.ID).Terrain.Calculation.Measurement.SlopeMeanPct; got != 31 {
		t.Fatalf("recalculated slope = %v", got)
	}
}

func TestFailureKeepsTheLastCalculation(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t, blockName("", "Falha"))
	field := f.fields[0]
	f.openRun(t, usecase.TerrainScopeAll, field.ID)
	claim, _, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	must(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(14), nowMicro()))

	f.openRun(t, usecase.TerrainScopeAll, field.ID)
	if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainPending || stored.Calculation == nil || stored.Calculation.Measurement.SlopeMeanPct != 14 {
		t.Fatalf("MarkFieldsPending must keep the calculation: %+v", stored)
	}
	claim, _, err = testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainCalculating || stored.Calculation == nil {
		t.Fatalf("calculating row = %+v", stored)
	}
	must(t, testDB.SaveFailure(ctx(), claim, "Serviço de elevação fora do ar.", nowMicro()))
	stored := f.readField(t, field.ID).Terrain
	if stored.Status != domain.TerrainFailed || stored.FailureReason != "Serviço de elevação fora do ar." ||
		stored.Calculation == nil || stored.Calculation.Measurement.SlopeMeanPct != 14 {
		t.Fatalf("failed row = %+v", stored)
	}
}

func TestRecoverStaleRequeuesExpiredLeasesAndClosesIdleRuns(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t, blockName("", "Interrompido"))
	field := f.fields[0]
	run := f.openRun(t, usecase.TerrainScopeAll, field.ID)
	claim, _, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)

	// A run with nothing to do (all fields were removed, or none matched) is closed by recovery.
	empty := newTerrainFarm(t, blockName("", "Vazio"))
	emptyRun := empty.openRun(t, usecase.TerrainScopeMissing)

	// A recent claim is inside its lease: it may be the work of a live instance, so it stays calculating.
	must(t, testDB.RecoverStale(ctx(), nowMicro().Add(-time.Hour), nowMicro()))
	if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainCalculating {
		t.Fatalf("row inside its lease = %s, want calculating", stored.Status)
	}
	if _, found, err := testDB.FindOpenRun(ctx(), f.company.ID, f.farm.ID); err != nil || !found {
		t.Fatalf("run with a calculating row must stay open: found=%v err=%v", found, err)
	}

	// Once the lease is over (cutoff after the claim), the row goes back to pending with a higher version.
	must(t, testDB.RecoverStale(ctx(), nowMicro().Add(time.Hour), nowMicro()))

	if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainPending {
		t.Fatalf("interrupted row = %s, want pending", stored.Status)
	}
	// The process that died could still be alive elsewhere: its late save must not win.
	requireIs(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(5), nowMicro()), usecase.ErrVersionConflict)

	if _, found, err := testDB.FindOpenRun(ctx(), f.company.ID, f.farm.ID); err != nil || !found {
		t.Fatalf("run with pending work must stay open: found=%v err=%v", found, err)
	}
	if _, found, err := testDB.FindOpenRun(ctx(), empty.company.ID, empty.farm.ID); err != nil || found {
		t.Fatalf("idle run must be closed: found=%v err=%v", found, err)
	}
	if latest, err := testDB.ReadFarmTerrain(ctx(), empty.company.ID, empty.farm.ID); err != nil || latest.LatestRun == nil ||
		latest.LatestRun.ID != emptyRun.ID || latest.LatestRun.FinishedAt == nil {
		t.Fatalf("latest run of the idle farm = %+v, %v", latest.LatestRun, err)
	}

	again, ok, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	if !ok || again.RunID != run.ID || again.FieldID != field.ID {
		t.Fatalf("claim after recovery = %+v, ok=%v", again, ok)
	}
	drainTerrainQueue(t)
}

func TestTerrainAdjustments(t *testing.T) {
	f := newTerrainFarm(t, blockName("", "Ajustado"))
	field := f.fields[0]
	adjustment := func(mean float64, by domain.UserID) usecase.FieldAdjustment {
		return usecase.FieldAdjustment{
			CompanyID: f.company.ID, FarmID: f.farm.ID, FieldID: field.ID,
			Adjustment: domain.TerrainAdjustment{
				Origin: domain.OriginTopographicSurvey, SlopeMeanPct: mean, SlopeMaxPct: mean + 5,
				Position: domain.PositionHilltop, Justification: "Levantamento do topógrafo", AdjustedBy: by, AdjustedAt: nowMicro(),
			},
		}
	}
	if data := f.readField(t, field.ID); data.Adjustment != nil || data.Terrain != nil {
		t.Fatalf("fresh field = %+v", data)
	}
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		return tx.Terrain.UpsertAdjustment(ctx, adjustment(12.5, f.user.ID))
	}))
	data := f.readField(t, field.ID)
	if data.Terrain != nil || data.Adjustment == nil {
		t.Fatalf("adjustment without relief row: %+v", data)
	}
	stored := data.Adjustment
	if stored.AdjustedByName != f.user.Name || stored.Adjustment.SlopeMeanPct != 12.5 || stored.Adjustment.SlopeMaxPct != 17.5 ||
		stored.Adjustment.Origin != domain.OriginTopographicSurvey || stored.Adjustment.Position != domain.PositionHilltop ||
		stored.Adjustment.Justification != "Levantamento do topógrafo" || stored.Adjustment.AdjustedBy != f.user.ID {
		t.Fatalf("stored adjustment = %+v", stored)
	}

	// Replace by another author: one adjustment per field, version bumped.
	second := newUser(t, f.company)
	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		return tx.Terrain.UpsertAdjustment(ctx, adjustment(20, second.ID))
	}))
	replaced := f.readField(t, field.ID).Adjustment
	if replaced.Adjustment.SlopeMeanPct != 20 || replaced.AdjustedByName != second.Name {
		t.Fatalf("replaced adjustment = %+v", replaced)
	}
	var count, version int
	must(t, testPool.QueryRow(ctx(), `SELECT count(*)::int, max(version)::int FROM field_terrain_adjustments WHERE field_id = $1`, string(field.ID)).Scan(&count, &version))
	if count != 1 || version != 2 {
		t.Fatalf("adjustments = %d, version = %d, want 1 and 2", count, version)
	}

	must(t, testDB.InTerrainTx(ctx(), func(ctx context.Context, tx usecase.TerrainStores) error {
		return tx.Terrain.DeleteAdjustment(ctx, f.company.ID, field.ID)
	}))
	if f.readField(t, field.ID).Adjustment != nil {
		t.Fatal("adjustment still there after the delete")
	}
	err := testDB.DeleteAdjustment(ctx(), f.company.ID, field.ID)
	requireIs(t, err, usecase.ErrNotFound)

	// A field of another farm of the same company, or one that does not exist, is not found.
	elsewhere := adjustment(10, f.user.ID)
	elsewhere.FarmID = domain.FarmID(newID())
	requireIs(t, testDB.UpsertAdjustment(ctx(), elsewhere), usecase.ErrNotFound)
	unknown := adjustment(10, f.user.ID)
	unknown.FieldID = domain.FieldID(newID())
	requireIs(t, testDB.UpsertAdjustment(ctx(), unknown), usecase.ErrNotFound)
}

func TestTerrainIsolatesCompanies(t *testing.T) {
	drainTerrainQueue(t)
	a := newTerrainFarm(t, blockName("", "Talhão A"))
	b := newTerrainFarm(t, blockName("", "Talhão B"))
	fieldA := a.fields[0]

	runA := a.openRun(t, usecase.TerrainScopeAll, fieldA.ID)
	claim, _, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	must(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(11), nowMicro()))
	must(t, testDB.UpsertAdjustment(ctx(), usecase.FieldAdjustment{
		CompanyID: a.company.ID, FarmID: a.farm.ID, FieldID: fieldA.ID,
		Adjustment: domain.TerrainAdjustment{
			Origin: domain.OriginFieldAssessment, SlopeMeanPct: 7, SlopeMaxPct: 9, Position: domain.PositionSlope,
			Justification: "Avaliação de campo", AdjustedBy: a.user.ID, AdjustedAt: nowMicro(),
		},
	}))
	runB := b.openRun(t, usecase.TerrainScopeAll, b.fields[0].ID)
	versionBefore := rowVersion(t, fieldA.ID)

	// Reads: company B sees nothing of company A.
	farm, err := testDB.ReadFarmTerrain(ctx(), b.company.ID, a.farm.ID)
	must(t, err)
	if len(farm.Fields) != 0 || farm.LatestRun != nil {
		t.Fatalf("company B read the farm of A: %+v", farm)
	}
	_, err = testDB.ReadFieldTerrain(ctx(), b.company.ID, a.farm.ID, fieldA.ID)
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.ReadFieldTerrain(ctx(), b.company.ID, b.farm.ID, fieldA.ID)
	requireIs(t, err, usecase.ErrNotFound)
	// Same company, wrong farm.
	_, err = testDB.ReadFieldTerrain(ctx(), a.company.ID, b.farm.ID, fieldA.ID)
	requireIs(t, err, usecase.ErrNotFound)
	requireIs(t, testDB.LockFarmForTerrain(ctx(), b.company.ID, a.farm.ID), usecase.ErrNotFound)
	if _, found, err := testDB.FindOpenRun(ctx(), b.company.ID, a.farm.ID); err != nil || found {
		t.Fatalf("company B found a run of A: %v, %v", found, err)
	}
	if ids, err := testDB.SelectFieldsForRun(ctx(), b.company.ID, a.farm.ID, usecase.TerrainScopeAll); err != nil || len(ids) != 0 {
		t.Fatalf("company B selected fields of A: %v, %v", ids, err)
	}

	// Writes: nothing of company B reaches the rows of A.
	foreign := usecase.TerrainRun{ID: runB.ID, CompanyID: b.company.ID, FarmID: b.farm.ID}
	must(t, testDB.MarkFieldsPending(ctx(), foreign, []domain.FieldID{fieldA.ID}, nowMicro()))
	must(t, testDB.MarkStale(ctx(), b.company.ID, fieldA.ID, nowMicro()))
	requireIs(t, testDB.DeleteAdjustment(ctx(), b.company.ID, fieldA.ID), usecase.ErrNotFound)
	requireIs(t, testDB.UpsertAdjustment(ctx(), usecase.FieldAdjustment{
		CompanyID: b.company.ID, FarmID: a.farm.ID, FieldID: fieldA.ID,
		Adjustment: domain.TerrainAdjustment{
			Origin: domain.OriginFieldAssessment, SlopeMeanPct: 1, SlopeMaxPct: 2, Position: domain.PositionSlope,
			Justification: "x", AdjustedBy: b.user.ID, AdjustedAt: nowMicro(),
		},
	}), usecase.ErrNotFound)
	// A claim forged with the field of A under the company B finds no row.
	forged := usecase.TerrainClaim{CompanyID: b.company.ID, FarmID: a.farm.ID, FieldID: fieldA.ID, RunID: runA.ID, RowVersion: versionBefore}
	requireIs(t, testDB.SaveCalculation(ctx(), forged, sampleCalculation(50), nowMicro()), usecase.ErrVersionConflict)
	requireIs(t, testDB.SaveFailure(ctx(), forged, "x", nowMicro()), usecase.ErrVersionConflict)
	if finished, err := testDB.FinishRun(ctx(), b.company.ID, runA.ID, nowMicro()); err != nil || finished {
		t.Fatalf("company B finished the run of A: %v, %v", finished, err)
	}

	after := a.readField(t, fieldA.ID)
	if rowVersion(t, fieldA.ID) != versionBefore || after.Terrain.Status != domain.TerrainReady ||
		after.Terrain.Calculation == nil || after.Terrain.Calculation.Measurement.SlopeMeanPct != 11 ||
		after.Adjustment == nil || after.Adjustment.Adjustment.SlopeMeanPct != 7 {
		t.Fatalf("the relief of company A changed: version %d -> %d, %+v", versionBefore, rowVersion(t, fieldA.ID), after)
	}
	var rowsOfB int
	must(t, testPool.QueryRow(ctx(), `SELECT count(*)::int FROM field_terrain WHERE company_id = $1 AND field_id = $2`, string(b.company.ID), string(fieldA.ID)).Scan(&rowsOfB))
	if rowsOfB != 0 {
		t.Fatal("MarkFieldsPending created a row of company B for a field of A")
	}
	drainTerrainQueue(t)
}

// An edit of the polygon inside the registry transaction marks the relief pending, using the
// real use case: without InRegistryTx carrying the terrain store this fails at runtime.
func TestUpdateFieldWithANewPolygonMarksTerrainStale(t *testing.T) {
	drainTerrainQueue(t)
	f := newTerrainFarm(t, blockName("", "Editável"))
	field := f.fields[0]
	f.openRun(t, usecase.TerrainScopeAll, field.ID)
	claim, _, err := testDB.ClaimNextPending(ctx(), nowMicro())
	must(t, err)
	must(t, testDB.SaveCalculation(ctx(), claim, sampleCalculation(18), nowMicro()))
	if _, err := testDB.FinishRun(ctx(), f.company.ID, claim.RunID, nowMicro()); err != nil {
		t.Fatal(err)
	}
	versionBefore := rowVersion(t, field.ID)

	update := usecase.NewUpdateField(usecase.FieldWriteDeps{
		Farms: testDB, Fields: testDB, Crops: testDB, Measurer: testDB, Tx: testDB,
		Clock: platform.SystemClock{}, IDs: platform.UUIDGenerator{},
		Areas: domain.AreaRules{MinFarmAreaHa: 1, MinFieldAreaHa: 0.1, ToleranceM2: 1},
	})
	actor := domain.Principal{
		UserID: f.user.ID, CompanyID: f.company.ID, Name: f.user.Name, Email: f.user.Email, Role: domain.RoleAdmin,
		Permissions: []domain.Permission{domain.PermRegistryWrite}, FarmScope: domain.AllFarms(),
	}
	details := func(name string, polygon domain.Polygon) usecase.SaveFieldInput {
		return usecase.SaveFieldInput{Actor: actor, FarmID: f.farm.ID, FieldID: field.ID, Details: domain.FieldDetails{
			Name: name, CropCode: "coffee_arabica", Polygon: polygon,
		}}
	}

	// A rename keeps the relief.
	if _, err := update.Execute(ctx(), details("Renomeado", field.Polygon)); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if stored := f.readField(t, field.ID).Terrain; stored.Status != domain.TerrainReady || stored.Calculation == nil {
		t.Fatalf("a rename must keep the relief: %+v", stored)
	}

	// A new polygon invalidates it, in the same transaction.
	if _, err := update.Execute(ctx(), details("Renomeado", fieldRect(1, 1, 3.5, 3.5))); err != nil {
		t.Fatalf("new polygon: %v", err)
	}
	stale := f.readField(t, field.ID).Terrain
	if stale.Status != domain.TerrainPending || stale.Calculation != nil {
		t.Fatalf("a new polygon must mark the relief stale: %+v", stale)
	}
	if rowVersion(t, field.ID) <= versionBefore {
		t.Fatal("MarkStale must bump the version")
	}
	// The run was already finished, so the field waits for the next request.
	if again, ok, err := testDB.ClaimNextPending(ctx(), nowMicro()); err != nil || ok {
		t.Fatalf("stale field of a closed run was claimed: %+v, %v, %v", again, ok, err)
	}

	// A field that never had relief stays without a row.
	other := newTerrainFarm(t, blockName("", "Sem relevo"))
	otherActor := actor
	otherActor.CompanyID = other.company.ID
	_, err = update.Execute(ctx(), usecase.SaveFieldInput{Actor: otherActor, FarmID: other.farm.ID, FieldID: other.fields[0].ID,
		Details: domain.FieldDetails{Name: "Sem relevo", CropCode: "coffee_arabica", Polygon: fieldRect(1, 1, 3.5, 3.5)}})
	must(t, err)
	if data := other.readField(t, other.fields[0].ID); data.Terrain != nil {
		t.Fatalf("MarkStale must not create a row: %+v", data.Terrain)
	}
}
