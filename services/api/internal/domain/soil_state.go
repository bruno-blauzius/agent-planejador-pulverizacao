package domain

import (
	"math"
	"time"
)

// SoilBasis tells what decided the soil state of a field.
type SoilBasis string

// Soil bases.
const (
	BasisInspection   SoilBasis = "inspection"
	BasisMeasuredRain SoilBasis = "measured_rain"
	BasisModel        SoilBasis = "model"
	BasisNone         SoilBasis = "none"
)

// SoilConditions are the inputs of the soil state of one field. Rain,
// ModelMoisture and Inspection are nil when there is no such data.
type SoilConditions struct {
	Texture       SoilTexture
	Rain          *RainAccumulation
	ModelMoisture *ModelSoilMoisture
	Inspection    *SoilInspection
}

// SoilAssessment is the soil state of a field now and what decided it.
type SoilAssessment struct {
	State SoilState
	Basis SoilBasis
}

// SoilStateCalculator applies the combination rules of ADR 0022 (decision 4)
// and projects the state over the forecast (decision 5).
type SoilStateCalculator struct {
	rules SoilTrafficabilityRules
}

// NewSoilStateCalculator validates the rules and returns the calculator.
func NewSoilStateCalculator(rules SoilTrafficabilityRules) (SoilStateCalculator, error) {
	if err := rules.Validate(); err != nil {
		return SoilStateCalculator{}, err
	}
	return SoilStateCalculator{rules: rules}, nil
}

// Assess returns the soil state at now. The rules, in order: a valid
// inspection replaces the calculation; measured rain decides alone; without a
// measurement the worse of model rain and model moisture wins; with no rain
// at all the state is unknown.
func (c SoilStateCalculator) Assess(now time.Time, conditions SoilConditions) (SoilAssessment, error) {
	thresholds, err := c.thresholdsFor(conditions.Texture)
	if err != nil {
		return SoilAssessment{}, err
	}
	if inspection := conditions.Inspection; inspection != nil && c.inspectionValid(now, *inspection) {
		return SoilAssessment{State: inspection.State, Basis: BasisInspection}, nil
	}
	rain := conditions.Rain
	if rain == nil {
		return SoilAssessment{State: SoilUnknown, Basis: BasisNone}, nil
	}
	rainState := thresholds.stateFromRain(rain.Last48hMm, rain.Last72hMm)
	if rain.Source.Measured() {
		return SoilAssessment{State: rainState, Basis: BasisMeasuredRain}, nil
	}
	state := rainState
	if moisture := conditions.ModelMoisture; moisture != nil {
		if pct, ok := moisture.FieldCapacityPct(); ok {
			state = WorstSoilState(state, stateFromBands(pct, thresholds.MoistureUpperPctOfFC))
		}
	}
	return SoilAssessment{State: state, Basis: BasisModel}, nil
}

// InspectionValid reports whether the inspection still replaces the
// calculation at now (see inspectionValid).
func (c SoilStateCalculator) InspectionValid(now time.Time, inspection SoilInspection) bool {
	return c.inspectionValid(now, inspection)
}

// InspectionValidUntil is the instant the inspection stops replacing the calculation.
func (c SoilStateCalculator) InspectionValidUntil(inspection SoilInspection) time.Time {
	return inspection.InspectedAt.Add(c.rules.InspectionValidity)
}

// inspectionValid reports whether the inspection still replaces the
// calculation: it must be known, not in the future and younger than the validity.
func (c SoilStateCalculator) inspectionValid(now time.Time, inspection SoilInspection) bool {
	age := now.Sub(inspection.InspectedAt)
	return inspection.State.Known() && age >= 0 && age < c.rules.InspectionValidity
}

// ProjectSoilStates returns the projected state of each forecast day, in the
// order given. Every day subtracts the drying rate of the texture from the
// accumulated rain (never below zero) and adds the forecast rain of the day.
// Without a rain accumulation every day is unknown.
func (c SoilStateCalculator) ProjectSoilStates(texture SoilTexture, rain *RainAccumulation, forecast []DailyRain) ([]SoilState, error) {
	thresholds, err := c.thresholdsFor(texture)
	if err != nil {
		return nil, err
	}
	states := make([]SoilState, len(forecast))
	if rain == nil {
		for i := range states {
			states[i] = SoilUnknown
		}
		return states, nil
	}
	accumulated := rain.Last48hMm
	for i, day := range forecast {
		accumulated = math.Max(0, accumulated-thresholds.DryingRateMmPerDay) + math.Max(0, day.Mm)
		states[i] = stateFromBands(accumulated, thresholds.Rain48hUpperMm)
	}
	return states, nil
}

// Thresholds returns the configured thresholds of a texture.
func (c SoilStateCalculator) Thresholds(texture SoilTexture) (TrafficabilityThresholds, error) {
	return c.thresholdsFor(texture)
}

func (c SoilStateCalculator) thresholdsFor(texture SoilTexture) (TrafficabilityThresholds, error) {
	if !texture.Valid() {
		return TrafficabilityThresholds{}, Invalid("soil_texture", "textura de solo desconhecida: %q", texture)
	}
	return c.rules.Thresholds[texture], nil
}
