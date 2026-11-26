package domain

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ErrInvalidSoilRules means the trafficability configuration is inconsistent.
var ErrInvalidSoilRules = errors.New("invalid soil trafficability rules")

// Sources of the thresholds (ADR 0022, decision 2 and addendum item 7).
const (
	// ThresholdSourceHeuristic marks thresholds that were not calibrated yet.
	ThresholdSourceHeuristic = "heuristic"
	// ThresholdSourceCalibrated marks thresholds reviewed by an agronomist.
	ThresholdSourceCalibrated = "calibrated"
)

// TrafficabilityThresholds are the limits of one soil texture. Each array
// holds the upper limits of dry, ideal and moist; above the last one the soil
// is waterlogged. A value below the first limit is dry; a value up to the
// second (inclusive) is ideal; up to the third (inclusive) is moist.
type TrafficabilityThresholds struct {
	Rain48hUpperMm       [3]float64
	MoistureUpperPctOfFC [3]float64
	Rain72hFactor        float64 // 72 h rain above this times the band limit worsens one state
	DryingRateMmPerDay   float64 // rain "removed" per dry day in the projection
	// FieldCapacityM3M3 is the reference field capacity of the texture, in m³/m³.
	// The weather model gives the volumetric moisture but not the capacity of
	// the soil, so the percentage of field capacity needs this reference.
	FieldCapacityM3M3 float64
	EffectiveFrom     time.Time
	Source            string
}

// SoilTrafficabilityRules is the business configuration of the soil state
// calculation. It is injected; nothing here is fixed in the code.
type SoilTrafficabilityRules struct {
	Thresholds map[SoilTexture]TrafficabilityThresholds
	// InspectionValidity is how long a person's inspection replaces the calculation.
	InspectionValidity time.Duration
}

// DefaultSoilTrafficabilityRules returns the initial heuristic values of the
// crossing document (ADR 0022). They must be calibrated with an agronomist.
func DefaultSoilTrafficabilityRules() SoilTrafficabilityRules {
	effective := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	moisture := [3]float64{60, 90, 100}
	thresholds := func(rain [3]float64, dryingRate, fieldCapacity float64) TrafficabilityThresholds {
		return TrafficabilityThresholds{
			Rain48hUpperMm:       rain,
			MoistureUpperPctOfFC: moisture,
			Rain72hFactor:        1.5,
			DryingRateMmPerDay:   dryingRate,
			FieldCapacityM3M3:    fieldCapacity,
			EffectiveFrom:        effective,
			Source:               ThresholdSourceHeuristic,
		}
	}
	return SoilTrafficabilityRules{
		Thresholds: map[SoilTexture]TrafficabilityThresholds{
			TextureSandy:  thresholds([3]float64{2, 15, 30}, 15, 0.15),
			TextureMedium: thresholds([3]float64{2, 10, 20}, 10, 0.25),
			TextureClay:   thresholds([3]float64{2, 8, 15}, 6, 0.36),
		},
		InspectionValidity: 12 * time.Hour,
	}
}

// Validate checks that every texture has coherent thresholds.
func (r SoilTrafficabilityRules) Validate() error {
	if r.InspectionValidity <= 0 {
		return fmt.Errorf("%w: inspection validity must be positive", ErrInvalidSoilRules)
	}
	for _, texture := range SoilTextures() {
		t, ok := r.Thresholds[texture]
		if !ok {
			return fmt.Errorf("%w: missing thresholds for %s", ErrInvalidSoilRules, texture)
		}
		if err := t.validate(); err != nil {
			return fmt.Errorf("%w: %s: %s", ErrInvalidSoilRules, texture, err.Error())
		}
	}
	return nil
}

func (t TrafficabilityThresholds) validate() error {
	if !increasingPositive(t.Rain48hUpperMm) {
		return errors.New("rain limits must be positive and increasing")
	}
	if !increasingPositive(t.MoistureUpperPctOfFC) {
		return errors.New("moisture limits must be positive and increasing")
	}
	if math.IsNaN(t.Rain72hFactor) || t.Rain72hFactor < 1 {
		return errors.New("72 h factor must be at least 1")
	}
	if math.IsNaN(t.DryingRateMmPerDay) || t.DryingRateMmPerDay <= 0 {
		return errors.New("drying rate must be positive")
	}
	if math.IsNaN(t.FieldCapacityM3M3) || t.FieldCapacityM3M3 <= 0 || t.FieldCapacityM3M3 > 1 {
		return errors.New("field capacity must be above 0 and at most 1 m³/m³")
	}
	return nil
}

func increasingPositive(limits [3]float64) bool {
	prev := 0.0
	for _, limit := range limits {
		if math.IsNaN(limit) || limit <= prev {
			return false
		}
		prev = limit
	}
	return true
}

// stateFromBands classifies a value with three upper limits (see
// TrafficabilityThresholds).
func stateFromBands(value float64, upper [3]float64) SoilState {
	switch {
	case value < upper[0]:
		return SoilDry
	case value <= upper[1]:
		return SoilIdeal
	case value <= upper[2]:
		return SoilMoist
	default:
		return SoilWaterlogged
	}
}

// bandUpperLimit returns the upper limit of the band of state s, if any.
func bandUpperLimit(s SoilState, upper [3]float64) (float64, bool) {
	r := s.rank()
	if r < 0 || r >= len(upper) {
		return 0, false
	}
	return upper[r], true
}

// stateFromRain applies the 48 h bands and the 72 h rule (ADR 0022, decision 4.4).
func (t TrafficabilityThresholds) stateFromRain(last48hMm, last72hMm float64) SoilState {
	state := stateFromBands(last48hMm, t.Rain48hUpperMm)
	if limit, ok := bandUpperLimit(state, t.Rain48hUpperMm); ok && last72hMm > t.Rain72hFactor*limit {
		return oneStateWorse(state)
	}
	return state
}
