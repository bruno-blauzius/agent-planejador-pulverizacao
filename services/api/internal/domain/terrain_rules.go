package domain

import (
	"fmt"
	"math"
)

// TerrainRules is the business configuration of the terrain calculation
// (ADR 0021, decision 4). It is injected; nothing here is fixed in the code.
type TerrainRules struct {
	// ClassUpperLimitsPct are the upper limits (exclusive) of the first four
	// classes. A slope equal to a limit belongs to the class above, so
	// [0,3) is flat, [3,8) gently undulating and so on; anything from the
	// last limit up is mountainous, even above 75 %.
	ClassUpperLimitsPct [4]float64
	// FlatAspectBelowPct: a mean slope below this has no aspect ("flat").
	FlatAspectBelowPct float64
	// PositionRadiusM is the radius around the field used to classify its position.
	PositionRadiusM float64
	// PositionToleranceM: the field is a hilltop (lowland) when its mean
	// elevation exceeds (falls below) the surroundings by more than this.
	PositionToleranceM float64
	// ProfilePoints is the number of strips of the elevation profile.
	ProfilePoints int
	// MinValidCells is the minimum of cells with a slope to accept a calculation.
	MinValidCells int
	// AdjustmentDivergencePct is the gap, in percentage points, between the
	// calculated and the adjusted mean slope that flags the adjustment as outdated.
	AdjustmentDivergencePct float64
	// MethodSlopeLimits holds the reference slope limit per method; nil or
	// missing means the method is not limited by slope.
	MethodSlopeLimits map[SprayMethod]*float64
	// RestrictedFromSharePct: share of the area above the limit from which a
	// method is restricted.
	RestrictedFromSharePct float64
	// UnsuitableFromSharePct: share from which a method is unsuitable.
	UnsuitableFromSharePct float64
}

// DefaultTerrainRules returns the documented default values.
func DefaultTerrainRules() TerrainRules {
	limit := func(v float64) *float64 { return &v }
	return TerrainRules{
		ClassUpperLimitsPct:     [4]float64{3, 8, 20, 45},
		FlatAspectBelowPct:      3,
		PositionRadiusM:         500,
		PositionToleranceM:      5,
		ProfilePoints:           24,
		MinValidCells:           20,
		AdjustmentDivergencePct: 5,
		MethodSlopeLimits: map[SprayMethod]*float64{
			MethodTractorBoom:   limit(20),
			MethodSelfPropelled: limit(15),
			MethodAirblast:      limit(20),
		},
		RestrictedFromSharePct: 10,
		UnsuitableFromSharePct: 50,
	}
}

// Validate checks that the configuration is coherent.
func (r TerrainRules) Validate() error {
	prev := 0.0
	for i, limit := range r.ClassUpperLimitsPct {
		if math.IsNaN(limit) || limit <= prev {
			return fmt.Errorf("%w: class limit %d (%v) must be greater than the previous one", ErrInvalidTerrainRules, i, limit)
		}
		prev = limit
	}
	positives := []struct {
		name  string
		value float64
	}{
		{"FlatAspectBelowPct", r.FlatAspectBelowPct},
		{"PositionRadiusM", r.PositionRadiusM},
		{"PositionToleranceM", r.PositionToleranceM},
		{"AdjustmentDivergencePct", r.AdjustmentDivergencePct},
	}
	for _, p := range positives {
		if math.IsNaN(p.value) || p.value <= 0 {
			return fmt.Errorf("%w: %s must be positive", ErrInvalidTerrainRules, p.name)
		}
	}
	if r.ProfilePoints < 2 {
		return fmt.Errorf("%w: ProfilePoints must be at least 2", ErrInvalidTerrainRules)
	}
	if r.MinValidCells < 1 {
		return fmt.Errorf("%w: MinValidCells must be at least 1", ErrInvalidTerrainRules)
	}
	for method, limit := range r.MethodSlopeLimits {
		if limit != nil && (math.IsNaN(*limit) || *limit <= 0) {
			return fmt.Errorf("%w: slope limit of %s must be positive", ErrInvalidTerrainRules, method)
		}
	}
	share := func(v float64) bool { return !math.IsNaN(v) && v > 0 && v <= 100 }
	if !share(r.RestrictedFromSharePct) || !share(r.UnsuitableFromSharePct) ||
		r.RestrictedFromSharePct >= r.UnsuitableFromSharePct {
		return fmt.Errorf("%w: need 0 < RestrictedFromSharePct < UnsuitableFromSharePct <= 100", ErrInvalidTerrainRules)
	}
	return nil
}

// ClassOf returns the relief class of a slope in %. A slope on a limit
// belongs to the class above it.
func (r TerrainRules) ClassOf(slopePct float64) SlopeClass {
	classes := SlopeClasses()
	for i, limit := range r.ClassUpperLimitsPct {
		if slopePct < limit {
			return classes[i]
		}
	}
	return SlopeMountainous
}

// ClassAreas derives the area per class from a slope histogram. Bin i covers
// [i, i+1) and goes entirely to the class of its lower bound; this is exact
// when the class limits are whole numbers (the defaults) and an approximation
// of at most one bin otherwise. The last bin (100 % or more) is always
// mountainous. All five classes are returned, in enum order, including empty ones.
func (r TerrainRules) ClassAreas(histogramHa [SlopeHistogramBins]float64) []SlopeClassArea {
	classes := SlopeClasses()
	areas := make(map[SlopeClass]float64, len(classes))
	total := 0.0
	for i, ha := range histogramHa {
		areas[r.ClassOf(float64(i))] += ha
		total += ha
	}
	out := make([]SlopeClassArea, 0, len(classes))
	for _, class := range classes {
		share := 0.0
		if total > 0 {
			share = areas[class] / total * 100
		}
		out = append(out, SlopeClassArea{Class: class, AreaHa: areas[class], SharePct: share})
	}
	return out
}
