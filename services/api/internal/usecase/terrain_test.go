package usecase_test

import (
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

// terrainEnv adds the relief store to the registry world: farm-a1 has two
// fields ("Talhão 1" and "Talhão 2"); farm-a2 and company B's farm have none.
type terrainEnv struct {
	*registryEnv
	terrain *fakes.Terrain
	wakeups *fakes.Wakeups
	rules   domain.TerrainRules
	f1, f2  domain.FieldID
}

func newTerrainEnv(t *testing.T) *terrainEnv {
	t.Helper()
	r := newRegistryEnv(t)
	r.withPerimeter("farm-a1")
	e := &terrainEnv{registryEnv: r, rules: domain.DefaultTerrainRules(), wakeups: &fakes.Wakeups{}}
	e.f1 = r.createField(t, "farm-a1", fieldIn("Talhão 1", rect(1, 1, 3, 3))).Field.ID
	e.f2 = r.createField(t, "farm-a1", fieldIn("Talhão 2", rect(4, 1, 6, 3))).Field.ID
	e.terrain = fakes.NewTerrain(r.reg)
	e.terrain.UserNames[r.adminA.ID] = r.adminA.Name
	return e
}

func (e *terrainEnv) getFarm() *usecase.GetFarmTerrain {
	return usecase.NewGetFarmTerrain(e.reg, e.terrain, e.rules)
}

func (e *terrainEnv) getField() *usecase.GetFieldTerrain {
	return usecase.NewGetFieldTerrain(e.terrain, e.rules)
}

func (e *terrainEnv) calculateDeps(wakeup usecase.TerrainWakeup) usecase.CalculateFarmTerrainDeps {
	return usecase.CalculateFarmTerrainDeps{Terrain: e.terrain, Tx: e.terrain, Wakeup: wakeup, Clock: e.clock, IDs: e.ids, Rules: e.rules}
}

func (e *terrainEnv) calculate() *usecase.CalculateFarmTerrain {
	return usecase.NewCalculateFarmTerrain(e.calculateDeps(e.wakeups))
}

func (e *terrainEnv) adjustDeps() usecase.AdjustFieldTerrainDeps {
	return usecase.AdjustFieldTerrainDeps{Terrain: e.terrain, Tx: e.terrain, Clock: e.clock, Rules: e.rules}
}

// seedReady stores a calculated relief whose whole area falls in one 1 % bin.
func (e *terrainEnv) seedReady(id domain.FieldID, meanPct float64) {
	var histogram [domain.SlopeHistogramBins]float64
	histogram[int(meanPct)] = 5
	p90 := meanPct
	aspect := domain.AspectEast
	e.terrain.Seed("co-a", "farm-a1", id, domain.TerrainReady, &domain.TerrainCalculation{
		Measurement: domain.TerrainMeasurement{
			SlopeMeanPct: meanPct, SlopeP90Pct: &p90, SlopeMaxPct: meanPct + 3,
			// Class is left empty on purpose: the Postgres adapter does not store it, so
			// the use case must derive it from the rules on every read.
			Position: domain.PositionSlope, Aspect: &aspect,
		},
		ElevationMinM: 700 + meanPct, ElevationMaxM: 720 + meanPct, SlopeHistogramHa: histogram,
		ElevationModel: domain.ElevationModel{Source: "Copernicus DEM GLO-30", ResolutionM: 30, Version: "2023_1"},
		CalculatedAt:   e.clock.T,
	})
}

func (e *terrainEnv) writerOutsideScope(t *testing.T) domain.Principal {
	t.Helper()
	writer := e.operatorOnly(t, "farm-a2")
	writer.Permissions = e.asAdminA().Permissions
	return writer
}

func adjustmentDetails(mean, max float64) domain.TerrainAdjustmentDetails {
	return domain.TerrainAdjustmentDetails{
		Origin: domain.OriginFieldAssessment, SlopeMeanPct: mean, SlopeMaxPct: max,
		Position: domain.PositionSlope, Justification: "Terraços não visíveis no modelo de 30 m",
	}
}

func (e *terrainEnv) adjust(t *testing.T, field domain.FieldID, details domain.TerrainAdjustmentDetails) usecase.FieldTerrainView {
	t.Helper()
	view, err := usecase.NewAdjustFieldTerrain(e.adjustDeps()).Execute(ctx(), usecase.AdjustFieldTerrainInput{
		Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: field, Details: details,
	})
	assertNoError(t, err)
	return view
}

func rowOf(t *testing.T, view usecase.FarmTerrainView, id domain.FieldID) usecase.FieldTerrainRowView {
	t.Helper()
	for _, row := range view.Fields {
		if row.FieldID == id {
			return row
		}
	}
	t.Fatalf("field %s not in the view", id)
	return usecase.FieldTerrainRowView{}
}

// ---- reading -------------------------------------------------------------------------

func TestGetFarmTerrain(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25) // steep: restricted for the slope-limited methods

	view, err := e.getFarm().Execute(ctx(), usecase.GetFarmTerrainInput{Actor: e.asOperatorA(), FarmID: "farm-a1"})
	assertNoError(t, err)

	if len(view.Fields) != 2 || view.Fields[0].FieldName != "Talhão 1" || view.Fields[1].FieldName != "Talhão 2" {
		t.Fatalf("fields = %+v", view.Fields)
	}
	first := rowOf(t, view, e.f1)
	if first.Status != domain.TerrainReady || first.Source == nil || *first.Source != domain.SourceCalculated ||
		first.Effective == nil || first.Effective.SlopeMeanPct != 25 || !first.HasRestrictedMethod ||
		first.ElevationMinM == nil || *first.ElevationMinM != 725 || first.FailureReason != nil {
		t.Fatalf("first row = %+v", first)
	}
	second := rowOf(t, view, e.f2)
	if second.Status != domain.TerrainNotCalculated || second.Effective != nil || second.Source != nil ||
		second.HasRestrictedMethod || second.ElevationMinM != nil {
		t.Fatalf("second row = %+v", second)
	}
	if view.Summary.FieldCount != 2 || view.Summary.CalculatedFieldCount != 1 || view.Summary.RestrictedFieldCount != 1 ||
		view.Summary.SlopeMeanPct == nil || *view.Summary.SlopeMeanPct != 25 {
		t.Fatalf("summary = %+v", view.Summary)
	}
	if view.Progress != nil {
		t.Fatalf("progress = %+v, want none without a run", view.Progress)
	}
}

func TestGetFarmTerrainFlatFieldIsNotRestricted(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 2)
	view, err := e.getFarm().Execute(ctx(), usecase.GetFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1"})
	assertNoError(t, err)
	if rowOf(t, view, e.f1).HasRestrictedMethod || view.Summary.RestrictedFieldCount != 0 {
		t.Fatalf("flat field flagged: %+v", view)
	}
}

func TestGetFarmTerrainShowsTheAdjustmentAsEffective(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 2)
	e.adjust(t, e.f1, adjustmentDetails(25, 30)) // the manual value wins and is steep
	e.adjust(t, e.f2, adjustmentDetails(10, 12)) // an adjustment alone is a relief too

	view, err := e.getFarm().Execute(ctx(), usecase.GetFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1"})
	assertNoError(t, err)

	first := rowOf(t, view, e.f1)
	if first.Source == nil || *first.Source != domain.SourceManual || first.Effective.SlopeMeanPct != 25 || !first.HasRestrictedMethod {
		t.Fatalf("first row = %+v", first)
	}
	if first.ElevationMinM == nil {
		t.Fatal("elevation must still come from the calculation")
	}
	second := rowOf(t, view, e.f2)
	if second.Status != domain.TerrainNotCalculated || second.Source == nil || *second.Source != domain.SourceManual || second.ElevationMinM != nil {
		t.Fatalf("second row = %+v", second)
	}
	if view.Summary.CalculatedFieldCount != 2 {
		t.Fatalf("summary = %+v", view.Summary)
	}
}

func TestGetFarmTerrainVisibility(t *testing.T) {
	tests := []struct {
		name    string
		actor   func(*terrainEnv, *testing.T) domain.Principal
		farm    domain.FarmID
		wantErr error
	}{
		{"operator inside the scope", func(e *terrainEnv, t *testing.T) domain.Principal { return e.operatorOnly(t, "farm-a1") }, "farm-a1", nil},
		{"operator outside the scope", func(e *terrainEnv, t *testing.T) domain.Principal { return e.operatorOnly(t, "farm-a2") }, "farm-a1", usecase.ErrNotFound},
		{"other company", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminB() }, "farm-a1", usecase.ErrNotFound},
		{"unknown farm", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminA() }, "ghost", usecase.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTerrainEnv(t)
			_, err := e.getFarm().Execute(ctx(), usecase.GetFarmTerrainInput{Actor: tt.actor(e, t), FarmID: tt.farm})
			if tt.wantErr == nil {
				assertNoError(t, err)
				return
			}
			assertIs(t, err, tt.wantErr)
		})
	}
}

func TestGetFieldTerrain(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25)
	get := func(actor domain.Principal, farm domain.FarmID, field domain.FieldID) (usecase.FieldTerrainView, error) {
		return e.getField().Execute(ctx(), usecase.GetFieldTerrainInput{Actor: actor, FarmID: farm, FieldID: field})
	}

	t.Run("calculated", func(t *testing.T) {
		view, err := get(e.asOperatorA(), "farm-a1", e.f1)
		assertNoError(t, err)
		if view.Status != domain.TerrainReady || view.Calculated == nil || len(view.ClassAreas) != 5 || view.Adjustment != nil ||
			view.AdjustmentOutdated || len(view.MethodFits) != len(domain.SprayMethods()) {
			t.Fatalf("view = %+v", view)
		}
		if view.Effective == nil || view.Effective.Class != domain.SlopeStronglyUndulating {
			t.Fatalf("effective = %+v", view.Effective)
		}
		tractor := slices.IndexFunc(view.MethodFits, func(f domain.MethodFit) bool { return f.Method == domain.MethodTractorBoom })
		if view.MethodFits[tractor].Status != domain.FitUnsuitable {
			t.Fatalf("tractor fit = %+v", view.MethodFits[tractor])
		}
	})
	t.Run("not calculated", func(t *testing.T) {
		view, err := get(e.asOperatorA(), "farm-a1", e.f2)
		assertNoError(t, err)
		if view.Status != domain.TerrainNotCalculated || view.Effective != nil || view.Source != nil || view.Calculated != nil ||
			view.ClassAreas != nil || view.MethodFits == nil || len(view.MethodFits) != 0 {
			t.Fatalf("view = %+v", view)
		}
	})
	t.Run("failed shows the reason", func(t *testing.T) {
		env := newTerrainEnv(t)
		env.terrain.SeedFailed("co-a", "farm-a1", env.f2, usecase.TerrainFailureTileUnavailable)
		view, err := env.getField().Execute(ctx(), usecase.GetFieldTerrainInput{Actor: env.asAdminA(), FarmID: "farm-a1", FieldID: env.f2})
		assertNoError(t, err)
		if view.Status != domain.TerrainFailed || view.FailureReason == nil || *view.FailureReason != usecase.TerrainFailureTileUnavailable || view.Effective != nil {
			t.Fatalf("view = %+v", view)
		}
		farm, err := env.getFarm().Execute(ctx(), usecase.GetFarmTerrainInput{Actor: env.asAdminA(), FarmID: "farm-a1"})
		assertNoError(t, err)
		if row := rowOf(t, farm, env.f2); row.FailureReason == nil || *row.FailureReason != usecase.TerrainFailureTileUnavailable {
			t.Fatalf("row = %+v", row)
		}
	})
	t.Run("refusals", func(t *testing.T) {
		_, err := get(e.asAdminB(), "farm-a1", e.f1)
		assertIs(t, err, usecase.ErrNotFound)
		_, err = get(e.operatorOnly(t, "farm-a2"), "farm-a1", e.f1)
		assertIs(t, err, usecase.ErrNotFound)
		_, err = get(e.asAdminA(), "farm-a2", e.f1) // the field belongs to another farm
		assertIs(t, err, usecase.ErrNotFound)
		_, err = get(e.asAdminA(), "farm-a1", "ghost")
		assertIs(t, err, usecase.ErrNotFound)
	})
}

// ---- calculating ---------------------------------------------------------------------

func TestCalculateFarmTerrain(t *testing.T) {
	e := newTerrainEnv(t)
	view, err := e.calculate().Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1"})
	assertNoError(t, err)

	runs := e.terrain.Runs()
	if len(runs) != 1 || runs[0].Scope != usecase.TerrainScopeMissing || runs[0].RequestedBy != e.adminA.ID || runs[0].Total != 2 || runs[0].Done != 0 {
		t.Fatalf("runs = %+v", runs)
	}
	if view.Progress == nil || view.Progress.Total != 2 || view.Progress.Done != 0 {
		t.Fatalf("progress = %+v", view.Progress)
	}
	for _, row := range view.Fields {
		if row.Status != domain.TerrainPending {
			t.Fatalf("row %s status = %s, want pending", row.FieldID, row.Status)
		}
	}
	if e.wakeups.Count() != 1 {
		t.Fatalf("wakeups = %d, want 1", e.wakeups.Count())
	}
	if got := e.terrain.AuditActions(); !slices.Equal(got, []string{domain.AuditTerrainCalculationRequested}) {
		t.Fatalf("audit = %v", got)
	}
	event := e.terrain.Audit[0]
	if event.Metadata["scope"] != "missing" || event.Metadata["total"] != "2" || event.EntityID != "farm-a1" || event.CompanyID != e.companyA.ID {
		t.Fatalf("audit event = %+v", event)
	}
}

func TestCalculateFarmTerrainIsIdempotent(t *testing.T) {
	e := newTerrainEnv(t)
	input := usecase.CalculateFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1"}
	_, err := e.calculate().Execute(ctx(), input)
	assertNoError(t, err)

	input.Scope = usecase.TerrainScopeAll // even another scope returns the current state
	view, err := e.calculate().Execute(ctx(), input)
	assertNoError(t, err)

	if len(e.terrain.Runs()) != 1 || len(e.terrain.Audit) != 1 || e.wakeups.Count() != 1 {
		t.Fatalf("runs = %d, audit = %d, wakeups = %d; want one of each", len(e.terrain.Runs()), len(e.terrain.Audit), e.wakeups.Count())
	}
	if view.Progress == nil || view.Progress.Total != 2 {
		t.Fatalf("progress = %+v", view.Progress)
	}
}

func TestCalculateFarmTerrainScopes(t *testing.T) {
	tests := []struct {
		name      string
		scope     usecase.TerrainRunScope
		prepare   func(*terrainEnv)
		wantTotal int // 0: no run
	}{
		{"missing skips what is ready", usecase.TerrainScopeMissing, func(e *terrainEnv) { e.seedReady(e.f1, 5) }, 1},
		{"missing includes failed", usecase.TerrainScopeMissing, func(e *terrainEnv) {
			e.seedReady(e.f1, 5)
			e.terrain.Seed("co-a", "farm-a1", e.f2, domain.TerrainFailed, nil)
		}, 1},
		{"missing includes pending", usecase.TerrainScopeMissing, func(e *terrainEnv) {
			e.seedReady(e.f1, 5)
			e.terrain.Seed("co-a", "farm-a1", e.f2, domain.TerrainPending, nil)
		}, 1},
		{"missing with everything ready creates no run", usecase.TerrainScopeMissing, func(e *terrainEnv) {
			e.seedReady(e.f1, 5)
			e.seedReady(e.f2, 6)
		}, 0},
		{"all recalculates what is ready", usecase.TerrainScopeAll, func(e *terrainEnv) {
			e.seedReady(e.f1, 5)
			e.seedReady(e.f2, 6)
		}, 2},
		{"empty scope means missing", "", func(e *terrainEnv) { e.seedReady(e.f1, 5) }, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTerrainEnv(t)
			tt.prepare(e)
			view, err := e.calculate().Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1", Scope: tt.scope})
			assertNoError(t, err)
			runs := e.terrain.Runs()
			if tt.wantTotal == 0 {
				if len(runs) != 0 || len(e.terrain.Audit) != 0 || e.wakeups.Count() != 0 || view.Progress != nil {
					t.Fatalf("expected no run: runs=%v audit=%v wakeups=%d progress=%v", runs, e.terrain.Audit, e.wakeups.Count(), view.Progress)
				}
				return
			}
			if len(runs) != 1 || runs[0].Total != tt.wantTotal {
				t.Fatalf("runs = %+v, want one with %d fields", runs, tt.wantTotal)
			}
		})
	}
}

func TestCalculateFarmTerrainAllKeepsTheLastCalculationVisible(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25)
	view, err := e.calculate().Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1", Scope: usecase.TerrainScopeAll})
	assertNoError(t, err)
	row := rowOf(t, view, e.f1)
	if row.Status != domain.TerrainPending || row.Effective == nil || row.Effective.SlopeMeanPct != 25 {
		t.Fatalf("row = %+v", row)
	}
}

func TestCalculateFarmTerrainRefusals(t *testing.T) {
	tests := []struct {
		name    string
		actor   func(*terrainEnv, *testing.T) domain.Principal
		farm    domain.FarmID
		scope   usecase.TerrainRunScope
		wantErr error
		field   string
	}{
		{"operator cannot write", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asOperatorA() }, "farm-a1", "", usecase.ErrForbidden, ""},
		{"writer outside the scope", func(e *terrainEnv, t *testing.T) domain.Principal { return e.writerOutsideScope(t) }, "farm-a1", "", usecase.ErrNotFound, ""},
		{"other company", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminB() }, "farm-a1", "", usecase.ErrNotFound, ""},
		{"unknown farm", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminA() }, "ghost", "", usecase.ErrNotFound, ""},
		{"unknown scope", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminA() }, "farm-a1", "everything", nil, "scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTerrainEnv(t)
			_, err := e.calculate().Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: tt.actor(e, t), FarmID: tt.farm, Scope: tt.scope})
			if tt.wantErr != nil {
				assertIs(t, err, tt.wantErr)
			} else {
				assertValidationOn(t, err, tt.field)
			}
			if len(e.terrain.Runs()) != 0 || len(e.terrain.Audit) != 0 || e.wakeups.Count() != 0 {
				t.Fatal("a refused request must schedule nothing")
			}
		})
	}
}

func TestCalculateFarmTerrainRollsBackOnAuditFailure(t *testing.T) {
	e := newTerrainEnv(t)
	e.terrain.Fail("Record", errBoom)
	_, err := e.calculate().Execute(ctx(), usecase.CalculateFarmTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1"})
	assertIs(t, err, errBoom)
	if _, ok := e.terrain.Stored(e.f1); ok || len(e.terrain.Runs()) != 0 || e.wakeups.Count() != 0 {
		t.Fatal("a failed transaction must leave no run, no row and no wake-up")
	}
}

// ---- manual adjustment ---------------------------------------------------------------

func TestAdjustFieldTerrain(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25)
	r := e.adjust(t, e.f1, adjustmentDetails(12, 20))

	if r.Source == nil || *r.Source != domain.SourceManual || r.Effective.SlopeMeanPct != 12 || r.Effective.Class != domain.SlopeUndulating {
		t.Fatalf("effective = %+v source = %v", r.Effective, r.Source)
	}
	if r.Calculated == nil || r.Calculated.Measurement.SlopeMeanPct != 25 {
		t.Fatal("the calculated value must stay next to the adjustment")
	}
	if r.Adjustment == nil || r.Adjustment.AdjustedByName != "Ana Admin" || r.Adjustment.Adjustment.AdjustedBy != e.adminA.ID ||
		r.Adjustment.Adjustment.Justification == "" || !r.Adjustment.Adjustment.AdjustedAt.Equal(e.clock.T) {
		t.Fatalf("adjustment = %+v", r.Adjustment)
	}
	if !r.AdjustmentOutdated { // |25 - 12| > 5
		t.Fatal("a 13 point gap must flag the adjustment as outdated")
	}
	event := e.terrain.Audit[0]
	if got := e.terrain.AuditActions(); !slices.Equal(got, []string{domain.AuditTerrainAdjusted}) ||
		event.EntityType != domain.AuditEntityField || event.EntityID != string(e.f1) || event.Metadata["farm_id"] != "farm-a1" ||
		event.Metadata["origin"] != "field_assessment" || event.Metadata["slope_mean_pct"] != "12.00" || event.Metadata["position"] != "slope" {
		t.Fatalf("audit = %+v", e.terrain.Audit)
	}
	for key, value := range event.Metadata {
		if value == r.Adjustment.Adjustment.Justification {
			t.Fatalf("the justification must not go to the audit trail (%s)", key)
		}
	}
}

func TestAdjustFieldTerrainReplacesAndTracksOutdated(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25)
	if view := e.adjust(t, e.f1, adjustmentDetails(12, 20)); !view.AdjustmentOutdated {
		t.Fatal("first adjustment should be outdated")
	}
	view := e.adjust(t, e.f1, adjustmentDetails(30, 40)) // |25 - 30| = 5, not strictly above the gap
	if view.Adjustment.Adjustment.SlopeMeanPct != 30 || view.AdjustmentOutdated {
		t.Fatalf("view = %+v", view)
	}
	if got := e.terrain.AuditActions(); len(got) != 2 {
		t.Fatalf("audit = %v, want two adjustments", got)
	}
}

func TestAdjustFieldTerrainValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.TerrainAdjustmentDetails)
		wantOn string
	}{
		{"max below mean", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMaxPct = 5 }, "slope_max_pct"},
		{"mean above 100", func(d *domain.TerrainAdjustmentDetails) { d.SlopeMeanPct = 101 }, "slope_mean_pct"},
		{"empty justification", func(d *domain.TerrainAdjustmentDetails) { d.Justification = "  " }, "justification"},
		{"unknown origin", func(d *domain.TerrainAdjustmentDetails) { d.Origin = "guess" }, "origin"},
		{"unknown position", func(d *domain.TerrainAdjustmentDetails) { d.Position = "valley" }, "position"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTerrainEnv(t)
			details := adjustmentDetails(12, 20)
			tt.mutate(&details)
			_, err := usecase.NewAdjustFieldTerrain(e.adjustDeps()).Execute(ctx(), usecase.AdjustFieldTerrainInput{
				Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Details: details,
			})
			assertValidationOn(t, err, tt.wantOn)
			if len(e.terrain.Audit) != 0 {
				t.Fatal("an invalid adjustment must store and record nothing")
			}
		})
	}
}

func TestAdjustFieldTerrainRefusals(t *testing.T) {
	tests := []struct {
		name    string
		actor   func(*terrainEnv, *testing.T) domain.Principal
		farm    domain.FarmID
		field   domain.FieldID
		wantErr error
	}{
		{"operator cannot write", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asOperatorA() }, "farm-a1", "", usecase.ErrForbidden},
		{"writer outside the scope", func(e *terrainEnv, t *testing.T) domain.Principal { return e.writerOutsideScope(t) }, "farm-a1", "", usecase.ErrNotFound},
		{"other company", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminB() }, "farm-a1", "", usecase.ErrNotFound},
		{"field of another farm", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminA() }, "farm-a2", "", usecase.ErrNotFound},
		{"unknown field", func(e *terrainEnv, _ *testing.T) domain.Principal { return e.asAdminA() }, "farm-a1", "ghost", usecase.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTerrainEnv(t)
			field := tt.field
			if field == "" {
				field = e.f1
			}
			input := usecase.AdjustFieldTerrainInput{Actor: tt.actor(e, t), FarmID: tt.farm, FieldID: field, Details: adjustmentDetails(12, 20)}
			_, err := usecase.NewAdjustFieldTerrain(e.adjustDeps()).Execute(ctx(), input)
			assertIs(t, err, tt.wantErr)
			_, err = usecase.NewRemoveFieldTerrainAdjustment(e.adjustDeps()).Execute(ctx(), usecase.RemoveFieldTerrainAdjustmentInput{
				Actor: input.Actor, FarmID: input.FarmID, FieldID: input.FieldID,
			})
			assertIs(t, err, tt.wantErr)
			if len(e.terrain.Audit) != 0 {
				t.Fatal("a refused request must record nothing")
			}
		})
	}
}

func TestRemoveFieldTerrainAdjustment(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25)
	e.adjust(t, e.f1, adjustmentDetails(12, 20))
	remove := usecase.NewRemoveFieldTerrainAdjustment(e.adjustDeps())
	input := usecase.RemoveFieldTerrainAdjustmentInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1}

	view, err := remove.Execute(ctx(), input)
	assertNoError(t, err)
	if view.Adjustment != nil || view.AdjustmentOutdated || view.Source == nil || *view.Source != domain.SourceCalculated || view.Effective.SlopeMeanPct != 25 {
		t.Fatalf("view = %+v", view)
	}
	if got := e.terrain.AuditActions(); !slices.Equal(got, []string{domain.AuditTerrainAdjusted, domain.AuditTerrainAdjustmentRemoved}) {
		t.Fatalf("audit = %v", got)
	}

	_, err = remove.Execute(ctx(), input) // nothing left to remove
	assertIs(t, err, usecase.ErrNotFound)
	if len(e.terrain.Audit) != 2 {
		t.Fatal("removing a missing adjustment must record nothing")
	}
}

func TestAdjustmentIsIsolatedBetweenCompanies(t *testing.T) {
	e := newTerrainEnv(t)
	e.adjust(t, e.f1, adjustmentDetails(12, 20))
	_, err := usecase.NewRemoveFieldTerrainAdjustment(e.adjustDeps()).Execute(ctx(), usecase.RemoveFieldTerrainAdjustmentInput{
		Actor: e.asAdminB(), FarmID: "farm-a1", FieldID: e.f1,
	})
	assertIs(t, err, usecase.ErrNotFound)
	view, err := e.getField().Execute(ctx(), usecase.GetFieldTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1})
	assertNoError(t, err)
	if view.Adjustment == nil {
		t.Fatal("company B must not touch the adjustment of company A")
	}
}

// ---- invalidation by editing the field -------------------------------------------------

func TestUpdateFieldInvalidatesTerrainOnlyWhenThePolygonChanges(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25)
	e.adjust(t, e.f1, adjustmentDetails(12, 20))
	update := usecase.NewUpdateField(e.fieldDeps())
	save := func(details domain.FieldDetails) {
		t.Helper()
		_, err := update.Execute(ctx(), usecase.SaveFieldInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Details: details})
		assertNoError(t, err)
	}

	renamed := fieldIn("Talhão Um", rect(1, 1, 3, 3))
	renamed.CropCode = "soybean"
	save(renamed)
	stored, _ := e.terrain.Stored(e.f1)
	if stored.Status != domain.TerrainReady || stored.Calculation == nil {
		t.Fatalf("a rename must keep the relief, got %+v", stored)
	}

	save(fieldIn("Talhão Um", rect(1, 1, 3.5, 3.5)))
	stored, _ = e.terrain.Stored(e.f1)
	if stored.Status != domain.TerrainPending || stored.Calculation != nil {
		t.Fatalf("a new polygon must mark the relief stale, got %+v", stored)
	}
	view, err := e.getField().Execute(ctx(), usecase.GetFieldTerrainInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1})
	assertNoError(t, err)
	if view.Adjustment == nil || view.Calculated != nil || view.Status != domain.TerrainPending {
		t.Fatalf("the adjustment stays, the calculation goes: %+v", view)
	}

	// A field that never had relief stays without it.
	_, err = update.Execute(ctx(), usecase.SaveFieldInput{Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f2, Details: fieldIn("Talhão 2", rect(4, 1, 7, 3))})
	assertNoError(t, err)
	if _, ok := e.terrain.Stored(e.f2); ok {
		t.Fatal("marking stale must not create a row")
	}
}

func TestUpdateFieldRollsBackTheInvalidation(t *testing.T) {
	e := newTerrainEnv(t)
	e.seedReady(e.f1, 25)
	e.reg.Fail("Commit", errBoom)
	_, err := usecase.NewUpdateField(e.fieldDeps()).Execute(ctx(), usecase.SaveFieldInput{
		Actor: e.asAdminA(), FarmID: "farm-a1", FieldID: e.f1, Details: fieldIn("Talhão 1", rect(1, 1, 3.5, 3.5)),
	})
	assertIs(t, err, errBoom)
	if stored, _ := e.terrain.Stored(e.f1); stored.Status != domain.TerrainReady {
		t.Fatalf("a rolled-back edit must keep the relief, got %+v", stored)
	}
}

func TestUpdateFieldWithoutTerrainStoreRefusesAPolygonChange(t *testing.T) {
	r := newRegistryEnv(t)
	r.reg.DetachTerrain() // the transaction carries no terrain store
	r.withPerimeter("farm-a1")
	field := r.createField(t, "farm-a1", fieldIn("Talhão 1", rect(1, 1, 3, 3)))
	update := usecase.NewUpdateField(r.fieldDeps())

	_, err := update.Execute(ctx(), usecase.SaveFieldInput{Actor: r.asAdminA(), FarmID: "farm-a1", FieldID: field.Field.ID, Details: fieldIn("Talhão 1", rect(1, 1, 3.5, 3.5))})
	if err == nil {
		t.Fatal("a polygon change without a terrain store must fail loudly, not skip the invalidation")
	}
	_, err = update.Execute(ctx(), usecase.SaveFieldInput{Actor: r.asAdminA(), FarmID: "farm-a1", FieldID: field.Field.ID, Details: fieldIn("Novo nome", rect(1, 1, 3, 3))})
	assertNoError(t, err)
}
