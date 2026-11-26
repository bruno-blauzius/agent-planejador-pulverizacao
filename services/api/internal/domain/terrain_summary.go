package domain

import (
	"math"
	"time"
)

// FieldTerrainRow is what the farm summary needs from one field.
type FieldTerrainRow struct {
	AreaHa        float64
	Measurement   *TerrainMeasurement // effective value; nil without relief
	ElevationMinM *float64
	ElevationMaxM *float64
	CalculatedAt  *time.Time
	Restricted    bool
}

// FarmTerrainSummary aggregates the relief of the fields of a farm.
type FarmTerrainSummary struct {
	FieldCount           int
	CalculatedFieldCount int // fields with an effective relief
	SlopeMeanPct         *float64
	RestrictedFieldCount int
	ElevationMinM        *float64
	ElevationMaxM        *float64
	CalculatedAt         *time.Time
}

// SummarizeFarmTerrain aggregates the rows. The mean slope is weighted by
// area over the fields with relief; if those fields have no area at all it
// falls back to the plain mean.
func SummarizeFarmTerrain(rows []FieldTerrainRow) FarmTerrainSummary {
	summary := FarmTerrainSummary{FieldCount: len(rows)}
	var weighted, weight, plain float64
	for _, row := range rows {
		if row.Restricted {
			summary.RestrictedFieldCount++
		}
		if row.Measurement != nil {
			summary.CalculatedFieldCount++
			weighted += row.Measurement.SlopeMeanPct * row.AreaHa
			weight += row.AreaHa
			plain += row.Measurement.SlopeMeanPct
		}
		summary.ElevationMinM = lowest(summary.ElevationMinM, row.ElevationMinM)
		summary.ElevationMaxM = highest(summary.ElevationMaxM, row.ElevationMaxM)
		if row.CalculatedAt != nil && (summary.CalculatedAt == nil || row.CalculatedAt.After(*summary.CalculatedAt)) {
			at := *row.CalculatedAt
			summary.CalculatedAt = &at
		}
	}
	switch {
	case summary.CalculatedFieldCount == 0:
	case weight > 0:
		m := weighted / weight
		summary.SlopeMeanPct = &m
	default:
		m := plain / float64(summary.CalculatedFieldCount)
		summary.SlopeMeanPct = &m
	}
	return summary
}

func lowest(current, candidate *float64) *float64 { return pick(current, candidate, math.Min) }

func highest(current, candidate *float64) *float64 { return pick(current, candidate, math.Max) }

func pick(current, candidate *float64, choose func(a, b float64) float64) *float64 {
	switch {
	case candidate == nil:
		return current
	case current == nil:
		v := *candidate
		return &v
	default:
		v := choose(*current, *candidate)
		return &v
	}
}
