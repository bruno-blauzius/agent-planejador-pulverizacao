package httpadapter

import (
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Bodies of the Terrain tag. They only copy what the use case already
// computed; no relief rule lives here.

type terrainMeasurementBody struct {
	SlopeMeanPct float64  `json:"slope_mean_pct"`
	SlopeP90Pct  *float64 `json:"slope_p90_pct"`
	SlopeMaxPct  float64  `json:"slope_max_pct"`
	SlopeClass   string   `json:"slope_class"`
	Position     string   `json:"position"`
	Aspect       *string  `json:"aspect"`
}

type slopeClassAreaBody struct {
	SlopeClass string  `json:"slope_class"`
	AreaHa     float64 `json:"area_ha"`
	SharePct   float64 `json:"share_pct"`
}

type elevationProfilePointBody struct {
	DistanceM  float64 `json:"distance_m"`
	ElevationM float64 `json:"elevation_m"`
}

type elevationModelBody struct {
	Source      string  `json:"source"`
	ResolutionM float64 `json:"resolution_m"`
	Version     string  `json:"version"`
}

type terrainCalculationBody struct {
	Measurement    terrainMeasurementBody      `json:"measurement"`
	ElevationMinM  float64                     `json:"elevation_min_m"`
	ElevationMaxM  float64                     `json:"elevation_max_m"`
	ClassAreas     []slopeClassAreaBody        `json:"class_areas"`
	Profile        []elevationProfilePointBody `json:"profile"`
	ElevationModel elevationModelBody          `json:"elevation_model"`
	CalculatedAt   time.Time                   `json:"calculated_at"`
}

type personRefBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type terrainAdjustmentBody struct {
	Origin        string                 `json:"origin"`
	Measurement   terrainMeasurementBody `json:"measurement"`
	Justification string                 `json:"justification"`
	AdjustedBy    personRefBody          `json:"adjusted_by"`
	AdjustedAt    time.Time              `json:"adjusted_at"`
}

type methodFitBody struct {
	Method            string   `json:"method"`
	SlopeLimitPct     *float64 `json:"slope_limit_pct"`
	LimitSource       string   `json:"limit_source"`
	Status            string   `json:"status"`
	AreaAboveLimitPct *float64 `json:"area_above_limit_pct"`
}

type fieldTerrainSummaryBody struct {
	FieldID             string                  `json:"field_id"`
	FieldName           string                  `json:"field_name"`
	Block               *string                 `json:"block"`
	AreaHa              float64                 `json:"area_ha"`
	Status              string                  `json:"status"`
	FailureReason       *string                 `json:"failure_reason"`
	Source              *string                 `json:"source"`
	Measurement         *terrainMeasurementBody `json:"measurement"`
	ElevationMinM       *float64                `json:"elevation_min_m"`
	ElevationMaxM       *float64                `json:"elevation_max_m"`
	HasRestrictedMethod bool                    `json:"has_restricted_method"`
}

type farmTerrainSummaryBody struct {
	FieldCount           int        `json:"field_count"`
	CalculatedFieldCount int        `json:"calculated_field_count"`
	SlopeMeanPct         *float64   `json:"slope_mean_pct"`
	RestrictedFieldCount int        `json:"restricted_field_count"`
	ElevationMinM        *float64   `json:"elevation_min_m"`
	ElevationMaxM        *float64   `json:"elevation_max_m"`
	CalculatedAt         *time.Time `json:"calculated_at"`
}

type terrainProgressBody struct {
	Total int `json:"total"`
	Done  int `json:"done"`
}

type farmTerrainBody struct {
	FarmID   string                    `json:"farm_id"`
	Summary  farmTerrainSummaryBody    `json:"summary"`
	Progress *terrainProgressBody      `json:"progress"`
	Items    []fieldTerrainSummaryBody `json:"items"`
}

type fieldTerrainBody struct {
	FieldID            string                  `json:"field_id"`
	FarmID             string                  `json:"farm_id"`
	Status             string                  `json:"status"`
	FailureReason      *string                 `json:"failure_reason"`
	Source             *string                 `json:"source"`
	Effective          *terrainMeasurementBody `json:"effective"`
	Calculated         *terrainCalculationBody `json:"calculated"`
	Adjustment         *terrainAdjustmentBody  `json:"adjustment"`
	AdjustmentOutdated bool                    `json:"adjustment_outdated"`
	MethodFits         []methodFitBody         `json:"method_fits"`
}

func enumPtr[T ~string](value *T) *string {
	if value == nil {
		return nil
	}
	text := string(*value)
	return &text
}

func terrainMeasurementOf(measurement domain.TerrainMeasurement) terrainMeasurementBody {
	return terrainMeasurementBody{
		SlopeMeanPct: measurement.SlopeMeanPct,
		SlopeP90Pct:  measurement.SlopeP90Pct,
		SlopeMaxPct:  measurement.SlopeMaxPct,
		SlopeClass:   string(measurement.Class),
		Position:     string(measurement.Position),
		Aspect:       enumPtr(measurement.Aspect),
	}
}

func terrainMeasurementPtrOf(measurement *domain.TerrainMeasurement) *terrainMeasurementBody {
	if measurement == nil {
		return nil
	}
	body := terrainMeasurementOf(*measurement)
	return &body
}

func classAreasOf(areas []domain.SlopeClassArea) []slopeClassAreaBody {
	body := make([]slopeClassAreaBody, 0, len(areas))
	for _, area := range areas {
		body = append(body, slopeClassAreaBody{SlopeClass: string(area.Class), AreaHa: area.AreaHa, SharePct: area.SharePct})
	}
	return body
}

func terrainCalculationOf(calculation *domain.TerrainCalculation, areas []domain.SlopeClassArea) *terrainCalculationBody {
	if calculation == nil {
		return nil
	}
	profile := make([]elevationProfilePointBody, 0, len(calculation.Profile))
	for _, point := range calculation.Profile {
		profile = append(profile, elevationProfilePointBody{DistanceM: point.DistanceM, ElevationM: point.ElevationM})
	}
	return &terrainCalculationBody{
		Measurement:   terrainMeasurementOf(calculation.Measurement),
		ElevationMinM: calculation.ElevationMinM,
		ElevationMaxM: calculation.ElevationMaxM,
		ClassAreas:    classAreasOf(areas),
		Profile:       profile,
		ElevationModel: elevationModelBody{
			Source:      calculation.ElevationModel.Source,
			ResolutionM: calculation.ElevationModel.ResolutionM,
			Version:     calculation.ElevationModel.Version,
		},
		CalculatedAt: calculation.CalculatedAt.UTC(),
	}
}

func terrainAdjustmentOf(view *usecase.TerrainAdjustmentView) *terrainAdjustmentBody {
	if view == nil {
		return nil
	}
	adjustment := view.Adjustment
	return &terrainAdjustmentBody{
		Origin:        string(adjustment.Origin),
		Measurement:   terrainMeasurementOf(adjustment.Measurement()),
		Justification: adjustment.Justification,
		AdjustedBy:    personRefBody{ID: string(adjustment.AdjustedBy), Name: view.AdjustedByName},
		AdjustedAt:    adjustment.AdjustedAt.UTC(),
	}
}

func methodFitsOf(fits []domain.MethodFit) []methodFitBody {
	body := make([]methodFitBody, 0, len(fits))
	for _, fit := range fits {
		body = append(body, methodFitBody{
			Method:            string(fit.Method),
			SlopeLimitPct:     fit.SlopeLimitPct,
			LimitSource:       fit.LimitSource,
			Status:            string(fit.Status),
			AreaAboveLimitPct: fit.AreaAboveLimitPct,
		})
	}
	return body
}

func fieldTerrainOf(view usecase.FieldTerrainView) fieldTerrainBody {
	return fieldTerrainBody{
		FieldID:            string(view.FieldID),
		FarmID:             string(view.FarmID),
		Status:             string(view.Status),
		FailureReason:      view.FailureReason,
		Source:             enumPtr(view.Source),
		Effective:          terrainMeasurementPtrOf(view.Effective),
		Calculated:         terrainCalculationOf(view.Calculated, view.ClassAreas),
		Adjustment:         terrainAdjustmentOf(view.Adjustment),
		AdjustmentOutdated: view.AdjustmentOutdated,
		MethodFits:         methodFitsOf(view.MethodFits),
	}
}

func farmTerrainOf(view usecase.FarmTerrainView) farmTerrainBody {
	items := make([]fieldTerrainSummaryBody, 0, len(view.Fields))
	for _, row := range view.Fields {
		items = append(items, fieldTerrainSummaryBody{
			FieldID:             string(row.FieldID),
			FieldName:           row.FieldName,
			Block:               row.Block,
			AreaHa:              row.AreaHa,
			Status:              string(row.Status),
			FailureReason:       row.FailureReason,
			Source:              enumPtr(row.Source),
			Measurement:         terrainMeasurementPtrOf(row.Effective),
			ElevationMinM:       row.ElevationMinM,
			ElevationMaxM:       row.ElevationMaxM,
			HasRestrictedMethod: row.HasRestrictedMethod,
		})
	}
	summary := view.Summary
	body := farmTerrainBody{
		FarmID: string(view.FarmID),
		Summary: farmTerrainSummaryBody{
			FieldCount:           summary.FieldCount,
			CalculatedFieldCount: summary.CalculatedFieldCount,
			SlopeMeanPct:         summary.SlopeMeanPct,
			RestrictedFieldCount: summary.RestrictedFieldCount,
			ElevationMinM:        summary.ElevationMinM,
			ElevationMaxM:        summary.ElevationMaxM,
			CalculatedAt:         utc(summary.CalculatedAt),
		},
		Items: items,
	}
	if view.Progress != nil {
		body.Progress = &terrainProgressBody{Total: view.Progress.Total, Done: view.Progress.Done}
	}
	return body
}
