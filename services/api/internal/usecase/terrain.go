package usecase

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// The relief follows the registry rules (ADR 0021, decision 8): reading needs no
// capability, calculating and adjusting need registry.write, and a farm outside
// the scope of a planning operator is ErrNotFound.

// ---- views -------------------------------------------------------------------------

// TerrainProgress is the progress of a calculation that has not finished.
type TerrainProgress struct {
	Total int
	Done  int
}

// FieldTerrainRowView is one field of the farm relief listing.
type FieldTerrainRowView struct {
	FieldID   domain.FieldID
	FieldName string
	Block     *string
	AreaHa    float64
	Status    domain.TerrainStatus
	// FailureReason is nil unless Status is failed.
	FailureReason *string
	// Source and Effective are nil without a relief (calculated or adjusted).
	Source    *domain.TerrainSource
	Effective *domain.TerrainMeasurement
	// ElevationMinM and ElevationMaxM come from the calculation only, even when
	// there is a manual adjustment; nil without a calculation.
	ElevationMinM       *float64
	ElevationMaxM       *float64
	HasRestrictedMethod bool
}

// FarmTerrainView is the relief of a farm.
type FarmTerrainView struct {
	FarmID  domain.FarmID
	Summary domain.FarmTerrainSummary
	// Progress is present while the most recent run has not finished.
	Progress *TerrainProgress
	// Fields are ordered by block and name.
	Fields []FieldTerrainRowView
}

// TerrainAdjustmentView is a manual adjustment with its author.
type TerrainAdjustmentView struct {
	Adjustment     domain.TerrainAdjustment
	AdjustedByName string
}

// FieldTerrainView is the detailed relief of a field.
type FieldTerrainView struct {
	FieldID       domain.FieldID
	FarmID        domain.FarmID
	Status        domain.TerrainStatus // not_calculated without a row
	FailureReason *string
	Source        *domain.TerrainSource
	Effective     *domain.TerrainMeasurement
	Calculated    *domain.TerrainCalculation
	// ClassAreas has the area per slope class of Calculated (nil without it).
	ClassAreas []domain.SlopeClassArea
	Adjustment *TerrainAdjustmentView
	// AdjustmentOutdated: the calculation diverges from the adjustment beyond the configured gap.
	AdjustmentOutdated bool
	// MethodFits is empty without an effective relief.
	MethodFits []domain.MethodFit
}

// terrainEvaluation is the relief of a field resolved by the domain rules.
type terrainEvaluation struct {
	calculation *domain.TerrainCalculation
	adjustment  *domain.TerrainAdjustment
	effective   *domain.TerrainMeasurement
	source      domain.TerrainSource
	fits        []domain.MethodFit
}

func evaluateTerrain(rules domain.TerrainRules, data FieldTerrainData) terrainEvaluation {
	var ev terrainEvaluation
	// The class is not stored: it depends on the configured limits, so it is derived
	// here on every read, on copies (the stored values stay untouched).
	if data.Terrain != nil && data.Terrain.Calculation != nil {
		calculation := *data.Terrain.Calculation
		calculation.Measurement.Class = rules.ClassOf(calculation.Measurement.SlopeMeanPct)
		ev.calculation = &calculation
	}
	if data.Adjustment != nil {
		adjustment := data.Adjustment.Adjustment
		adjustment.Class = rules.ClassOf(adjustment.SlopeMeanPct)
		ev.adjustment = &adjustment
	}
	measurement, source, ok := domain.EffectiveTerrain(ev.calculation, ev.adjustment)
	if !ok {
		return ev
	}
	ev.effective, ev.source = &measurement, source
	var histogram *[domain.SlopeHistogramBins]float64
	if ev.calculation != nil {
		histogram = &ev.calculation.SlopeHistogramHa
	}
	ev.fits = domain.EvaluateMethodFits(rules, measurement, source, histogram)
	return ev
}

func statusOf(data FieldTerrainData) domain.TerrainStatus {
	if data.Terrain == nil {
		return domain.TerrainNotCalculated
	}
	return data.Terrain.Status
}

func failureReasonOf(data FieldTerrainData) *string {
	if data.Terrain == nil || data.Terrain.Status != domain.TerrainFailed {
		return nil
	}
	reason := data.Terrain.FailureReason
	return &reason
}

func sourceOf(ev terrainEvaluation) *domain.TerrainSource {
	if ev.effective == nil {
		return nil
	}
	source := ev.source
	return &source
}

// buildFarmTerrainView assembles the view of a farm from what the reader returned.
func buildFarmTerrainView(rules domain.TerrainRules, farmID domain.FarmID, data FarmTerrainData) FarmTerrainView {
	view := FarmTerrainView{FarmID: farmID, Fields: make([]FieldTerrainRowView, 0, len(data.Fields))}
	rows := make([]domain.FieldTerrainRow, 0, len(data.Fields))
	for _, item := range data.Fields {
		ev := evaluateTerrain(rules, item)
		restricted := domain.HasRestrictedMethod(ev.fits)
		row := FieldTerrainRowView{
			FieldID:             item.Field.Field.ID,
			FieldName:           item.Field.Field.Name,
			Block:               item.Field.Field.Block,
			AreaHa:              item.Field.AreaHa,
			Status:              statusOf(item),
			FailureReason:       failureReasonOf(item),
			Source:              sourceOf(ev),
			Effective:           ev.effective,
			HasRestrictedMethod: restricted,
		}
		summaryRow := domain.FieldTerrainRow{AreaHa: item.Field.AreaHa, Measurement: ev.effective, Restricted: restricted}
		if ev.calculation != nil {
			minM, maxM, at := ev.calculation.ElevationMinM, ev.calculation.ElevationMaxM, ev.calculation.CalculatedAt
			row.ElevationMinM, row.ElevationMaxM = &minM, &maxM
			summaryRow.ElevationMinM, summaryRow.ElevationMaxM, summaryRow.CalculatedAt = &minM, &maxM, &at
		}
		view.Fields = append(view.Fields, row)
		rows = append(rows, summaryRow)
	}
	view.Summary = domain.SummarizeFarmTerrain(rows)
	if run := data.LatestRun; run != nil && run.FinishedAt == nil {
		view.Progress = &TerrainProgress{Total: run.Total, Done: run.Done}
	}
	return view
}

func buildFieldTerrainView(rules domain.TerrainRules, farmID domain.FarmID, data FieldTerrainData) FieldTerrainView {
	ev := evaluateTerrain(rules, data)
	view := FieldTerrainView{
		FieldID:       data.Field.Field.ID,
		FarmID:        farmID,
		Status:        statusOf(data),
		FailureReason: failureReasonOf(data),
		Source:        sourceOf(ev),
		Effective:     ev.effective,
		Calculated:    ev.calculation,
		MethodFits:    ev.fits,
	}
	if view.MethodFits == nil {
		view.MethodFits = []domain.MethodFit{}
	}
	if ev.calculation != nil {
		view.ClassAreas = ev.calculation.ClassAreas(rules)
	}
	if data.Adjustment != nil {
		view.Adjustment = &TerrainAdjustmentView{Adjustment: data.Adjustment.Adjustment, AdjustedByName: data.Adjustment.AdjustedByName}
	}
	view.AdjustmentOutdated = domain.AdjustmentOutdated(ev.calculation, ev.adjustment, rules)
	return view
}

// ---- reading -----------------------------------------------------------------------

// GetFarmTerrainInput is the request of GET /farms/{farmId}/terrain.
type GetFarmTerrainInput struct {
	Actor  domain.Principal
	FarmID domain.FarmID
}

// GetFarmTerrain returns the relief of the fields of a farm.
type GetFarmTerrain struct {
	farms   FarmReader
	terrain TerrainReader
	rules   domain.TerrainRules
}

// NewGetFarmTerrain builds the use case.
func NewGetFarmTerrain(farms FarmReader, terrain TerrainReader, rules domain.TerrainRules) *GetFarmTerrain {
	return &GetFarmTerrain{farms: farms, terrain: terrain, rules: rules}
}

// Execute returns the summary, the progress of the running calculation and one
// row per field.
func (uc *GetFarmTerrain) Execute(ctx context.Context, in GetFarmTerrainInput) (FarmTerrainView, error) {
	if _, err := findVisibleFarm(ctx, uc.farms, in.Actor, in.FarmID); err != nil {
		return FarmTerrainView{}, err
	}
	return readFarmTerrain(ctx, uc.terrain, uc.rules, in.Actor.CompanyID, in.FarmID)
}

func readFarmTerrain(ctx context.Context, terrain TerrainReader, rules domain.TerrainRules, companyID domain.CompanyID, farmID domain.FarmID) (FarmTerrainView, error) {
	data, err := terrain.ReadFarmTerrain(ctx, companyID, farmID)
	if err != nil {
		return FarmTerrainView{}, fmt.Errorf("read farm terrain: %w", err)
	}
	return buildFarmTerrainView(rules, farmID, data), nil
}

// GetFieldTerrainInput is the request of GET /farms/{farmId}/fields/{fieldId}/terrain.
type GetFieldTerrainInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
}

// GetFieldTerrain returns the detailed relief of a field.
type GetFieldTerrain struct {
	terrain TerrainReader
	rules   domain.TerrainRules
}

// NewGetFieldTerrain builds the use case.
func NewGetFieldTerrain(terrain TerrainReader, rules domain.TerrainRules) *GetFieldTerrain {
	return &GetFieldTerrain{terrain: terrain, rules: rules}
}

// Execute returns ErrNotFound for a field that is missing, of another company
// or in a farm outside the scope of the caller.
func (uc *GetFieldTerrain) Execute(ctx context.Context, in GetFieldTerrainInput) (FieldTerrainView, error) {
	if !in.Actor.FarmScope.Allows(in.FarmID) {
		return FieldTerrainView{}, fmt.Errorf("get field terrain of farm %s: %w", in.FarmID, ErrNotFound)
	}
	return readFieldTerrain(ctx, uc.terrain, uc.rules, in.Actor.CompanyID, in.FarmID, in.FieldID)
}

func readFieldTerrain(ctx context.Context, terrain TerrainReader, rules domain.TerrainRules, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (FieldTerrainView, error) {
	data, err := terrain.ReadFieldTerrain(ctx, companyID, farmID, fieldID)
	if err != nil {
		return FieldTerrainView{}, fmt.Errorf("read field terrain: %w", err)
	}
	return buildFieldTerrainView(rules, farmID, data), nil
}

// ---- calculating -------------------------------------------------------------------

// CalculateFarmTerrainDeps are the dependencies of CalculateFarmTerrain.
type CalculateFarmTerrainDeps struct {
	Terrain TerrainReader
	Tx      TerrainTxRunner
	Wakeup  TerrainWakeup
	Clock   Clock
	IDs     IDGenerator
	Rules   domain.TerrainRules
}

// CalculateFarmTerrainInput is the request of POST /farms/{farmId}/terrain/calculations.
type CalculateFarmTerrainInput struct {
	Actor  domain.Principal
	FarmID domain.FarmID
	// Scope defaults to TerrainScopeMissing when empty.
	Scope TerrainRunScope
}

// CalculateFarmTerrain schedules the relief calculation of a farm. The work is
// done in the background by the TerrainWorker; the request does not wait.
type CalculateFarmTerrain struct {
	deps CalculateFarmTerrainDeps
}

// NewCalculateFarmTerrain builds the use case.
func NewCalculateFarmTerrain(deps CalculateFarmTerrainDeps) *CalculateFarmTerrain {
	return &CalculateFarmTerrain{deps: deps}
}

// Execute is idempotent: with a run already open it creates nothing and returns
// the current state; with nothing to calculate it creates no run either.
func (uc *CalculateFarmTerrain) Execute(ctx context.Context, in CalculateFarmTerrainInput) (FarmTerrainView, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return FarmTerrainView{}, err
	}
	if !in.Actor.FarmScope.Allows(in.FarmID) {
		return FarmTerrainView{}, fmt.Errorf("calculate terrain of farm %s: %w", in.FarmID, ErrNotFound)
	}
	scope := in.Scope
	if scope == "" {
		scope = TerrainScopeMissing
	}
	if !scope.Valid() {
		return FarmTerrainView{}, domain.Invalid("scope", "escopo deve ser missing ou all")
	}
	now := uc.deps.Clock.Now()
	scheduled := false
	err := uc.deps.Tx.InTerrainTx(ctx, func(ctx context.Context, tx TerrainStores) error {
		if err := tx.Terrain.LockFarmForTerrain(ctx, in.Actor.CompanyID, in.FarmID); err != nil {
			return fmt.Errorf("lock farm: %w", err)
		}
		if _, open, err := tx.Terrain.FindOpenRun(ctx, in.Actor.CompanyID, in.FarmID); err != nil {
			return fmt.Errorf("find open run: %w", err)
		} else if open {
			return nil
		}
		fieldIDs, err := tx.Terrain.SelectFieldsForRun(ctx, in.Actor.CompanyID, in.FarmID, scope)
		if err != nil {
			return fmt.Errorf("select fields: %w", err)
		}
		if len(fieldIDs) == 0 {
			return nil
		}
		run := TerrainRun{
			ID: uc.deps.IDs.NewID(), CompanyID: in.Actor.CompanyID, FarmID: in.FarmID,
			Scope: scope, RequestedBy: in.Actor.UserID, RequestedAt: now,
		}
		if err := tx.Terrain.CreateRun(ctx, run); err != nil {
			return fmt.Errorf("create run: %w", err)
		}
		if err := tx.Terrain.MarkFieldsPending(ctx, run, fieldIDs, now); err != nil {
			return fmt.Errorf("mark fields pending: %w", err)
		}
		event := actorEvent(in.Actor, now, in.Actor.CompanyID, domain.AuditTerrainCalculationRequested, domain.AuditEntityFarm, string(in.FarmID),
			map[string]string{"scope": string(scope), "total": strconv.Itoa(len(fieldIDs))})
		if err := tx.Audit.Record(ctx, event); err != nil {
			return fmt.Errorf("record audit: %w", err)
		}
		scheduled = true
		return nil
	})
	if err != nil {
		return FarmTerrainView{}, err
	}
	if scheduled {
		uc.deps.Wakeup.Wake()
	}
	return readFarmTerrain(ctx, uc.deps.Terrain, uc.deps.Rules, in.Actor.CompanyID, in.FarmID)
}

// ---- manual adjustment -------------------------------------------------------------

// AdjustFieldTerrainDeps are the dependencies of the adjustment use cases.
type AdjustFieldTerrainDeps struct {
	Terrain TerrainReader
	Tx      TerrainTxRunner
	Clock   Clock
	Rules   domain.TerrainRules
}

// AdjustFieldTerrainInput is the request of PUT /farms/{farmId}/fields/{fieldId}/terrain/adjustment.
type AdjustFieldTerrainInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
	Details domain.TerrainAdjustmentDetails
}

// AdjustFieldTerrain creates or replaces the manual adjustment of a field.
type AdjustFieldTerrain struct {
	deps AdjustFieldTerrainDeps
}

// NewAdjustFieldTerrain builds the use case.
func NewAdjustFieldTerrain(deps AdjustFieldTerrainDeps) *AdjustFieldTerrain {
	return &AdjustFieldTerrain{deps: deps}
}

// Execute stores the adjustment; the calculated value is never touched. The
// audit event keeps origin and measures but not the free-text justification.
func (uc *AdjustFieldTerrain) Execute(ctx context.Context, in AdjustFieldTerrainInput) (FieldTerrainView, error) {
	if err := checkTerrainWrite(in.Actor, in.FarmID); err != nil {
		return FieldTerrainView{}, err
	}
	if _, err := uc.deps.Terrain.ReadFieldTerrain(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID); err != nil {
		return FieldTerrainView{}, fmt.Errorf("find field: %w", err)
	}
	now := uc.deps.Clock.Now()
	adjustment, err := domain.NewTerrainAdjustment(in.Details, in.Actor.UserID, uc.deps.Rules, now)
	if err != nil {
		return FieldTerrainView{}, err
	}
	err = uc.deps.Tx.InTerrainTx(ctx, func(ctx context.Context, tx TerrainStores) error {
		if err := tx.Terrain.LockFarmForTerrain(ctx, in.Actor.CompanyID, in.FarmID); err != nil {
			return fmt.Errorf("lock farm: %w", err)
		}
		stored := FieldAdjustment{CompanyID: in.Actor.CompanyID, FarmID: in.FarmID, FieldID: in.FieldID, Adjustment: adjustment}
		if err := tx.Terrain.UpsertAdjustment(ctx, stored); err != nil {
			return fmt.Errorf("upsert adjustment: %w", err)
		}
		return recordTerrainEvent(ctx, tx.Audit, in.Actor, now, domain.AuditTerrainAdjusted, in.FarmID, in.FieldID, map[string]string{
			"origin":         string(adjustment.Origin),
			"slope_mean_pct": formatPercent(adjustment.SlopeMeanPct),
			"slope_max_pct":  formatPercent(adjustment.SlopeMaxPct),
			"position":       string(adjustment.Position),
		})
	})
	if err != nil {
		return FieldTerrainView{}, err
	}
	return readFieldTerrain(ctx, uc.deps.Terrain, uc.deps.Rules, in.Actor.CompanyID, in.FarmID, in.FieldID)
}

// RemoveFieldTerrainAdjustmentInput is the request of DELETE /farms/{farmId}/fields/{fieldId}/terrain/adjustment.
type RemoveFieldTerrainAdjustmentInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
}

// RemoveFieldTerrainAdjustment drops the manual adjustment of a field.
type RemoveFieldTerrainAdjustment struct {
	deps AdjustFieldTerrainDeps
}

// NewRemoveFieldTerrainAdjustment builds the use case.
func NewRemoveFieldTerrainAdjustment(deps AdjustFieldTerrainDeps) *RemoveFieldTerrainAdjustment {
	return &RemoveFieldTerrainAdjustment{deps: deps}
}

// Execute returns ErrNotFound when the field has no adjustment.
func (uc *RemoveFieldTerrainAdjustment) Execute(ctx context.Context, in RemoveFieldTerrainAdjustmentInput) (FieldTerrainView, error) {
	if err := checkTerrainWrite(in.Actor, in.FarmID); err != nil {
		return FieldTerrainView{}, err
	}
	if _, err := uc.deps.Terrain.ReadFieldTerrain(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID); err != nil {
		return FieldTerrainView{}, fmt.Errorf("find field: %w", err)
	}
	now := uc.deps.Clock.Now()
	err := uc.deps.Tx.InTerrainTx(ctx, func(ctx context.Context, tx TerrainStores) error {
		if err := tx.Terrain.LockFarmForTerrain(ctx, in.Actor.CompanyID, in.FarmID); err != nil {
			return fmt.Errorf("lock farm: %w", err)
		}
		if err := tx.Terrain.DeleteAdjustment(ctx, in.Actor.CompanyID, in.FieldID); err != nil {
			return fmt.Errorf("delete adjustment: %w", err)
		}
		return recordTerrainEvent(ctx, tx.Audit, in.Actor, now, domain.AuditTerrainAdjustmentRemoved, in.FarmID, in.FieldID, nil)
	})
	if err != nil {
		return FieldTerrainView{}, err
	}
	return readFieldTerrain(ctx, uc.deps.Terrain, uc.deps.Rules, in.Actor.CompanyID, in.FarmID, in.FieldID)
}

func checkTerrainWrite(actor domain.Principal, farmID domain.FarmID) error {
	if err := requireCan(actor, domain.PermRegistryWrite); err != nil {
		return err
	}
	if !actor.FarmScope.Allows(farmID) {
		return fmt.Errorf("write terrain of farm %s: %w", farmID, ErrNotFound)
	}
	return nil
}

func recordTerrainEvent(ctx context.Context, audit AuditRecorder, actor domain.Principal, now time.Time, action string, farmID domain.FarmID, fieldID domain.FieldID, extra map[string]string) error {
	metadata := map[string]string{"farm_id": string(farmID)}
	for key, value := range extra {
		metadata[key] = value
	}
	event := actorEvent(actor, now, actor.CompanyID, action, domain.AuditEntityField, string(fieldID), metadata)
	if err := audit.Record(ctx, event); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	return nil
}

func formatPercent(value float64) string { return strconv.FormatFloat(value, 'f', 2, 64) }
