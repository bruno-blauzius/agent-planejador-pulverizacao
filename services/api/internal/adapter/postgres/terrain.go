package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Field relief (ADR 0021; DER: terrain_runs, field_terrain, field_terrain_adjustments).
// The tables keep measures only; class, method fit and the "outdated adjustment" flag are
// derived in the domain from them. Every product read and write filters by company_id;
// only the background executor (ClaimNextPending, RecoverStale) looks across
// companies, because the open runs of all tenants share one queue.

// terrainRow is the shape of the joined field, relief and adjustment reads.
type terrainRow sqlcgen.GetFieldTerrainRow

// profilePointJSON is the stored shape of one elevation profile point.
type profilePointJSON struct {
	DistanceM  float64 `json:"distance_m"`
	ElevationM float64 `json:"elevation_m"`
}

// ---- TerrainReader -------------------------------------------------------------------

// ReadFarmTerrain implements usecase.TerrainReader.
func (r *Repository) ReadFarmTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) (usecase.FarmTerrainData, error) {
	data := usecase.FarmTerrainData{Fields: []usecase.FieldTerrainData{}}
	if !validUUID(string(companyID)) || !validUUID(string(farmID)) {
		return data, nil
	}
	rows, err := r.q.ListFarmTerrain(ctx, sqlcgen.ListFarmTerrainParams{CompanyID: string(companyID), FarmID: string(farmID)})
	if err != nil {
		return usecase.FarmTerrainData{}, fmt.Errorf("list farm terrain: %w", err)
	}
	for _, row := range rows {
		item, err := fieldTerrainOf(terrainRow(row))
		if err != nil {
			return usecase.FarmTerrainData{}, err
		}
		data.Fields = append(data.Fields, item)
	}
	run, err := r.q.GetLatestTerrainRun(ctx, sqlcgen.GetLatestTerrainRunParams{CompanyID: string(companyID), FarmID: string(farmID)})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return data, nil
	case err != nil:
		return usecase.FarmTerrainData{}, fmt.Errorf("get latest terrain run: %w", err)
	}
	latest := terrainRunOf(run.ID, run.CompanyID, run.FarmID, run.Scope, run.RequestedBy, run.RequestedAt, run.FinishedAt, run.Total, run.Done)
	data.LatestRun = &latest
	return data, nil
}

// ReadFieldTerrain implements usecase.TerrainReader.
func (r *Repository) ReadFieldTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (usecase.FieldTerrainData, error) {
	row, err := r.q.GetFieldTerrain(ctx, sqlcgen.GetFieldTerrainParams{CompanyID: string(companyID), FarmID: string(farmID), ID: string(fieldID)})
	if err != nil {
		return usecase.FieldTerrainData{}, translateRead("read field terrain", err)
	}
	return fieldTerrainOf(terrainRow(row))
}

func terrainRunOf(id, companyID, farmID, scope, requestedBy string, requestedAt time.Time, finishedAt *time.Time, total, done int32) usecase.TerrainRun {
	return usecase.TerrainRun{
		ID: id, CompanyID: domain.CompanyID(companyID), FarmID: domain.FarmID(farmID),
		Scope: usecase.TerrainRunScope(scope), RequestedBy: domain.UserID(requestedBy),
		RequestedAt: requestedAt, FinishedAt: finishedAt, Total: int(total), Done: int(done),
	}
}

func fieldTerrainOf(row terrainRow) (usecase.FieldTerrainData, error) {
	listing, err := fieldListingOf(fieldColumns{
		ID: row.ID, CompanyID: row.CompanyID, FarmID: row.FarmID, CropCode: row.CropCode, CropName: row.CropName,
		Name: row.Name, Block: row.Block, Variety: row.Variety, Season: row.Season, PlantingDate: row.PlantingDate,
		RowSpacingM: row.RowSpacingM, PlantSpacingM: row.PlantSpacingM, Polygon: row.Polygon, AreaHa: row.AreaHa,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: row.Version,
	})
	if err != nil {
		return usecase.FieldTerrainData{}, err
	}
	data := usecase.FieldTerrainData{Field: listing}
	if row.TerrainID != nil {
		terrain, err := storedTerrainOf(row)
		if err != nil {
			return usecase.FieldTerrainData{}, err
		}
		data.Terrain = &terrain
	}
	if row.AdjustmentID != nil {
		adjustment, err := storedAdjustmentOf(row)
		if err != nil {
			return usecase.FieldTerrainData{}, err
		}
		data.Adjustment = &adjustment
	}
	return data, nil
}

func storedTerrainOf(row terrainRow) (usecase.StoredTerrain, error) {
	if row.TerrainRequestedAt == nil {
		return usecase.StoredTerrain{}, errors.New("decode terrain: row without requested_at")
	}
	terrain := usecase.StoredTerrain{
		Status:        domain.TerrainStatus(row.TerrainStatus.String),
		FailureReason: row.FailureReason.String,
		RequestedAt:   *row.TerrainRequestedAt,
	}
	// calculated_at is set together with every measure and cleared together by MarkStale;
	// a failed or pending row that kept it still carries the last successful result.
	if row.CalculatedAt == nil {
		return terrain, nil
	}
	calculation, err := calculationOf(row)
	if err != nil {
		return usecase.StoredTerrain{}, err
	}
	terrain.Calculation = &calculation
	return terrain, nil
}

func calculationOf(row terrainRow) (domain.TerrainCalculation, error) {
	var (
		mean, p90, maxSlope, low, high, resolution float64
		err                                        error
	)
	for _, field := range []struct {
		name  string
		value pgtype.Numeric
		into  *float64
	}{
		{"slope_mean_pct", row.SlopeMeanPct, &mean}, {"slope_p90_pct", row.SlopeP90Pct, &p90},
		{"slope_max_pct", row.SlopeMaxPct, &maxSlope}, {"elevation_min_m", row.ElevationMinM, &low},
		{"elevation_max_m", row.ElevationMaxM, &high}, {"elevation_resolution_m", row.ElevationResolutionM, &resolution},
	} {
		if *field.into, err = requiredNumeric(field.name, field.value); err != nil {
			return domain.TerrainCalculation{}, err
		}
	}
	if len(row.SlopeHistogramHa) != domain.SlopeHistogramBins {
		return domain.TerrainCalculation{}, fmt.Errorf("decode terrain: histogram has %d bins, want %d", len(row.SlopeHistogramHa), domain.SlopeHistogramBins)
	}
	var histogram [domain.SlopeHistogramBins]float64
	copy(histogram[:], row.SlopeHistogramHa)
	var points []profilePointJSON
	if err := json.Unmarshal(row.Profile, &points); err != nil {
		return domain.TerrainCalculation{}, fmt.Errorf("decode terrain profile: %w", err)
	}
	profile := make([]domain.ProfilePoint, 0, len(points))
	for _, point := range points {
		profile = append(profile, domain.ProfilePoint{DistanceM: point.DistanceM, ElevationM: point.ElevationM})
	}
	aspect := domain.TerrainAspect(row.Aspect.String)
	return domain.TerrainCalculation{
		Measurement: domain.TerrainMeasurement{
			SlopeMeanPct: mean, SlopeP90Pct: &p90, SlopeMaxPct: maxSlope,
			Position: domain.TerrainPosition(row.TerrainPosition.String), Aspect: &aspect,
		},
		ElevationMinM: low, ElevationMaxM: high, SlopeHistogramHa: histogram, Profile: profile,
		ElevationModel: domain.ElevationModel{
			Source: row.ElevationSource.String, ResolutionM: resolution, Version: row.ElevationVersion.String,
		},
		CalculatedAt: *row.CalculatedAt,
	}, nil
}

func requiredNumeric(name string, value pgtype.Numeric) (float64, error) {
	number, err := numericPtr(value)
	if err != nil {
		return 0, err
	}
	if number == nil {
		return 0, fmt.Errorf("decode terrain: %s is null on a calculated row", name)
	}
	return *number, nil
}

func storedAdjustmentOf(row terrainRow) (usecase.StoredTerrainAdjustment, error) {
	mean, err := requiredNumeric("adjustment slope_mean_pct", row.AdjustmentSlopeMeanPct)
	if err != nil {
		return usecase.StoredTerrainAdjustment{}, err
	}
	maxSlope, err := requiredNumeric("adjustment slope_max_pct", row.AdjustmentSlopeMaxPct)
	if err != nil {
		return usecase.StoredTerrainAdjustment{}, err
	}
	if row.AdjustmentAdjustedBy == nil || row.AdjustmentAdjustedAt == nil {
		return usecase.StoredTerrainAdjustment{}, errors.New("decode terrain adjustment: row without author or date")
	}
	return usecase.StoredTerrainAdjustment{
		Adjustment: domain.TerrainAdjustment{
			Origin:       domain.TerrainAdjustmentOrigin(row.AdjustmentOrigin.String),
			SlopeMeanPct: mean, SlopeMaxPct: maxSlope,
			Position:      domain.TerrainPosition(row.AdjustmentPosition.String),
			Justification: row.AdjustmentJustification.String,
			AdjustedBy:    domain.UserID(*row.AdjustmentAdjustedBy),
			AdjustedAt:    *row.AdjustmentAdjustedAt,
		},
		AdjustedByName: row.AdjustmentAdjustedByName.String,
	}, nil
}

// ---- TerrainStore: runs --------------------------------------------------------------

// LockFarmForTerrain implements usecase.TerrainStore. The lock lasts as long as the
// transaction, so call it inside InTerrainTx.
func (r *Repository) LockFarmForTerrain(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) error {
	_, err := r.q.LockFarmForTerrain(ctx, sqlcgen.LockFarmForTerrainParams{CompanyID: string(companyID), ID: string(farmID)})
	return translateRead("lock farm for terrain", err)
}

// FindOpenRun implements usecase.TerrainStore.
func (r *Repository) FindOpenRun(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) (usecase.TerrainRun, bool, error) {
	row, err := r.q.GetOpenTerrainRun(ctx, sqlcgen.GetOpenTerrainRunParams{CompanyID: string(companyID), FarmID: string(farmID)})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return usecase.TerrainRun{}, false, nil
	case err != nil:
		return usecase.TerrainRun{}, false, fmt.Errorf("find open terrain run: %w", err)
	}
	return terrainRunOf(row.ID, row.CompanyID, row.FarmID, row.Scope, row.RequestedBy, row.RequestedAt, row.FinishedAt, row.Total, row.Done), true, nil
}

// SelectFieldsForRun implements usecase.TerrainStore.
func (r *Repository) SelectFieldsForRun(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, scope usecase.TerrainRunScope) ([]domain.FieldID, error) {
	var (
		ids []string
		err error
	)
	switch scope {
	case usecase.TerrainScopeAll:
		ids, err = r.q.SelectFieldsForRunAll(ctx, sqlcgen.SelectFieldsForRunAllParams{CompanyID: string(companyID), FarmID: string(farmID)})
	case usecase.TerrainScopeMissing:
		ids, err = r.q.SelectFieldsForRunMissing(ctx, sqlcgen.SelectFieldsForRunMissingParams{CompanyID: string(companyID), FarmID: string(farmID)})
	default:
		return nil, fmt.Errorf("select fields for run: unknown scope %q", scope)
	}
	if err != nil {
		return nil, fmt.Errorf("select fields for run: %w", err)
	}
	fields := make([]domain.FieldID, 0, len(ids))
	for _, id := range ids {
		fields = append(fields, domain.FieldID(id))
	}
	return fields, nil
}

// CreateRun implements usecase.TerrainStore.
func (r *Repository) CreateRun(ctx context.Context, run usecase.TerrainRun) error {
	return translateWrite("create terrain run", r.q.InsertTerrainRun(ctx, sqlcgen.InsertTerrainRunParams{
		ID: run.ID, CompanyID: string(run.CompanyID), FarmID: string(run.FarmID), Scope: string(run.Scope),
		RequestedBy: string(run.RequestedBy), RequestedAt: run.RequestedAt,
	}))
}

// MarkFieldsPending implements usecase.TerrainStore. The statement only touches fields of
// the company and farm of the run, so an id from elsewhere creates no row.
func (r *Repository) MarkFieldsPending(ctx context.Context, run usecase.TerrainRun, fieldIDs []domain.FieldID, at time.Time) error {
	if len(fieldIDs) == 0 {
		return nil
	}
	ids := make([]string, len(fieldIDs))
	for i, id := range fieldIDs {
		ids[i] = string(id)
	}
	return translateWrite("mark fields pending", r.q.MarkFieldsPending(ctx, sqlcgen.MarkFieldsPendingParams{
		RunID: run.ID, At: at, CompanyID: string(run.CompanyID), FarmID: string(run.FarmID), FieldIds: ids,
	}))
}

// MarkStale implements usecase.TerrainStaleMarker.
func (r *Repository) MarkStale(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID, at time.Time) error {
	return translateWrite("mark terrain stale", r.q.MarkTerrainStale(ctx, sqlcgen.MarkTerrainStaleParams{
		At: at, CompanyID: string(companyID), FieldID: string(fieldID),
	}))
}

// ---- TerrainStore: executor ----------------------------------------------------------

// ClaimNextPending implements usecase.TerrainStore: one atomic statement
// (FOR UPDATE SKIP LOCKED), so concurrent executors never share a field.
func (r *Repository) ClaimNextPending(ctx context.Context, at time.Time) (usecase.TerrainClaim, bool, error) {
	row, err := r.q.ClaimNextPendingTerrain(ctx, at)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return usecase.TerrainClaim{}, false, nil
	case err != nil:
		return usecase.TerrainClaim{}, false, fmt.Errorf("claim pending terrain: %w", err)
	}
	if row.RunID == nil {
		return usecase.TerrainClaim{}, false, errors.New("claim pending terrain: claimed row without run")
	}
	return usecase.TerrainClaim{
		CompanyID: domain.CompanyID(row.CompanyID), FarmID: domain.FarmID(row.FarmID),
		FieldID: domain.FieldID(row.FieldID), RunID: *row.RunID, RowVersion: int(row.Version),
	}, true, nil
}

// RecoverStale implements usecase.TerrainStore.
func (r *Repository) RecoverStale(ctx context.Context, staleBefore, at time.Time) error {
	return r.atomic(ctx, func(q *sqlcgen.Queries) error {
		if err := q.RequeueStaleCalculatingTerrain(ctx, sqlcgen.RequeueStaleCalculatingTerrainParams{At: at, StaleBefore: staleBefore}); err != nil {
			return fmt.Errorf("requeue stale calculating terrain: %w", err)
		}
		if err := q.FinishIdleTerrainRuns(ctx, at); err != nil {
			return fmt.Errorf("finish idle terrain runs: %w", err)
		}
		return nil
	})
}

// SaveCalculation implements usecase.TerrainStore.
func (r *Repository) SaveCalculation(ctx context.Context, claim usecase.TerrainClaim, calculation domain.TerrainCalculation, at time.Time) error {
	measure := calculation.Measurement
	if measure.SlopeP90Pct == nil || measure.Aspect == nil {
		return errors.New("save terrain calculation: p90 slope and aspect are required")
	}
	points := make([]profilePointJSON, 0, len(calculation.Profile))
	for _, point := range calculation.Profile {
		points = append(points, profilePointJSON{DistanceM: point.DistanceM, ElevationM: point.ElevationM})
	}
	profile, err := json.Marshal(points)
	if err != nil {
		return fmt.Errorf("encode terrain profile: %w", err)
	}
	calculatedAt := calculation.CalculatedAt
	updated, err := r.q.SaveTerrainCalculation(ctx, sqlcgen.SaveTerrainCalculationParams{
		CalculatedAt: &calculatedAt, At: at,
		SlopeMeanPct: measure.SlopeMeanPct, SlopeP90Pct: *measure.SlopeP90Pct, SlopeMaxPct: measure.SlopeMaxPct,
		ElevationMinM: calculation.ElevationMinM, ElevationMaxM: calculation.ElevationMaxM,
		Position:         pgtype.Text{String: string(measure.Position), Valid: true},
		Aspect:           pgtype.Text{String: string(*measure.Aspect), Valid: true},
		SlopeHistogramHa: calculation.SlopeHistogramHa[:], Profile: profile,
		ElevationSource:      pgtype.Text{String: calculation.ElevationModel.Source, Valid: true},
		ElevationResolutionM: calculation.ElevationModel.ResolutionM,
		ElevationVersion:     pgtype.Text{String: calculation.ElevationModel.Version, Valid: true},
		CompanyID:            string(claim.CompanyID), FieldID: string(claim.FieldID), RowVersion: clampInt32(int64(claim.RowVersion)),
	})
	if err != nil {
		return translateWrite("save terrain calculation", err)
	}
	return versionConflictIfNone("save terrain calculation", updated)
}

// SaveFailure implements usecase.TerrainStore. The last successful calculation stays.
func (r *Repository) SaveFailure(ctx context.Context, claim usecase.TerrainClaim, reason string, at time.Time) error {
	updated, err := r.q.SaveTerrainFailure(ctx, sqlcgen.SaveTerrainFailureParams{
		Reason: pgtype.Text{String: reason, Valid: true}, At: at,
		CompanyID: string(claim.CompanyID), FieldID: string(claim.FieldID), RowVersion: clampInt32(int64(claim.RowVersion)),
	})
	if err != nil {
		return translateWrite("save terrain failure", err)
	}
	return versionConflictIfNone("save terrain failure", updated)
}

func versionConflictIfNone(operation string, updated int64) error {
	if updated == 0 {
		return fmt.Errorf("%s: %w", operation, usecase.ErrVersionConflict)
	}
	return nil
}

// FinishRun implements usecase.TerrainStore.
func (r *Repository) FinishRun(ctx context.Context, companyID domain.CompanyID, runID string, at time.Time) (bool, error) {
	if !validUUID(runID) || !validUUID(string(companyID)) {
		return false, nil
	}
	finished, err := r.q.FinishTerrainRun(ctx, sqlcgen.FinishTerrainRunParams{At: at, CompanyID: string(companyID), ID: runID})
	if err != nil {
		return false, fmt.Errorf("finish terrain run: %w", err)
	}
	return finished > 0, nil
}

// ---- TerrainStore: adjustments -------------------------------------------------------

// UpsertAdjustment implements usecase.TerrainStore.
func (r *Repository) UpsertAdjustment(ctx context.Context, adjustment usecase.FieldAdjustment) error {
	value := adjustment.Adjustment
	written, err := r.q.UpsertTerrainAdjustment(ctx, sqlcgen.UpsertTerrainAdjustmentParams{
		Origin: string(value.Origin), SlopeMeanPct: value.SlopeMeanPct, SlopeMaxPct: value.SlopeMaxPct,
		Position: string(value.Position), Justification: value.Justification, AdjustedBy: string(value.AdjustedBy),
		AdjustedAt: value.AdjustedAt, CompanyID: string(adjustment.CompanyID), FarmID: string(adjustment.FarmID),
		FieldID: string(adjustment.FieldID),
	})
	if err != nil {
		return translateWrite("upsert terrain adjustment", err)
	}
	if written == 0 {
		return fmt.Errorf("upsert terrain adjustment: %w", usecase.ErrNotFound)
	}
	return nil
}

// DeleteAdjustment implements usecase.TerrainStore.
func (r *Repository) DeleteAdjustment(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID) error {
	if !validUUID(string(companyID)) || !validUUID(string(fieldID)) {
		return fmt.Errorf("delete terrain adjustment: %w", usecase.ErrNotFound)
	}
	deleted, err := r.q.DeleteTerrainAdjustment(ctx, sqlcgen.DeleteTerrainAdjustmentParams{CompanyID: string(companyID), FieldID: string(fieldID)})
	if err != nil {
		return fmt.Errorf("delete terrain adjustment: %w", err)
	}
	if deleted == 0 {
		return fmt.Errorf("delete terrain adjustment: %w", usecase.ErrNotFound)
	}
	return nil
}
