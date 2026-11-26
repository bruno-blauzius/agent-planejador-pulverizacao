package domain

import "time"

// EligibilityVerdict says whether a method can be used on a field on a day.
type EligibilityVerdict string

// Eligibility verdicts, from best to worst.
const (
	VerdictApt       EligibilityVerdict = "apt"
	VerdictAttention EligibilityVerdict = "attention"
	VerdictUnfit     EligibilityVerdict = "unfit"
)

func (v EligibilityVerdict) rank() int {
	switch v {
	case VerdictApt:
		return 0
	case VerdictAttention:
		return 1
	default:
		return 2
	}
}

func worseVerdict(a, b EligibilityVerdict) EligibilityVerdict {
	if a.rank() >= b.rank() {
		return a
	}
	return b
}

// EligibilityReason is a machine-readable reason of a verdict. The screen
// and the agent turn it into text; numbers go in their own fields.
type EligibilityReason string

// Eligibility reasons.
const (
	ReasonSlopeRestricted          EligibilityReason = "slope_restricted"
	ReasonSlopeUnsuitable          EligibilityReason = "slope_unsuitable"
	ReasonTerrainUnknown           EligibilityReason = "terrain_unknown"
	ReasonSoilMoist                EligibilityReason = "soil_moist"
	ReasonSoilWaterlogged          EligibilityReason = "soil_waterlogged"
	ReasonSoilUnknown              EligibilityReason = "soil_unknown"
	ReasonSlopeRestrictedOnWetSoil EligibilityReason = "slope_restricted_on_wet_soil"
)

// MachineWeight groups the methods by how they load the soil.
type MachineWeight string

// Machine weights.
const (
	WeightHeavy  MachineWeight = "heavy"
	WeightLight  MachineWeight = "light"
	WeightAerial MachineWeight = "aerial"
)

// WeightOf returns the machine weight of a method. Until the farm equipment
// registry exists (ADR 0022, decision 7) it is fixed per method.
func WeightOf(method SprayMethod) MachineWeight {
	switch method {
	case MethodTractorBoom, MethodSelfPropelled, MethodAirblast:
		return WeightHeavy
	case MethodKnapsack:
		return WeightLight
	default:
		return WeightAerial
	}
}

// RequiredSoilState returns the worst state a method accepts without
// restriction; aerial methods do not depend on the soil.
func RequiredSoilState(method SprayMethod) (SoilState, bool) {
	switch WeightOf(method) {
	case WeightHeavy:
		return SoilIdeal, true
	case WeightLight:
		return SoilMoist, true
	default:
		return "", false
	}
}

// MethodEligibility is the verdict of one method on one day.
type MethodEligibility struct {
	Method            SprayMethod
	Verdict           EligibilityVerdict
	Reasons           []EligibilityReason
	AreaAboveLimitPct *float64
}

// EvaluateMethodEligibility crosses the relief fit with the soil state (ADR
// 0022, decision 6). fit is nil when the relief was not calculated. The worse
// verdict wins, except that a restricted slope on moist soil is unfit for a
// heavy machine: on a wet slope it slides.
func EvaluateMethodEligibility(method SprayMethod, fit *MethodFit, soil SoilState) MethodEligibility {
	result := MethodEligibility{Method: method, Verdict: VerdictApt, Reasons: []EligibilityReason{}}
	reliefVerdict, reliefReason := reliefVerdictOf(method, fit)
	soilVerdict, soilReason := soilVerdictOf(method, soil)

	result.Verdict = worseVerdict(reliefVerdict, soilVerdict)
	for _, reason := range []EligibilityReason{reliefReason, soilReason} {
		if reason != "" {
			result.Reasons = append(result.Reasons, reason)
		}
	}
	if fit != nil {
		result.AreaAboveLimitPct = fit.AreaAboveLimitPct
	}
	if reliefReason == ReasonSlopeRestricted && soilReason == ReasonSoilMoist {
		result.Verdict = VerdictUnfit
		result.Reasons = append(result.Reasons, ReasonSlopeRestrictedOnWetSoil)
	}
	return result
}

func reliefVerdictOf(method SprayMethod, fit *MethodFit) (EligibilityVerdict, EligibilityReason) {
	if fit == nil {
		if WeightOf(method) == WeightHeavy {
			return VerdictAttention, ReasonTerrainUnknown
		}
		return VerdictApt, ""
	}
	switch fit.Status {
	case FitRestricted:
		return VerdictAttention, ReasonSlopeRestricted
	case FitUnsuitable:
		return VerdictUnfit, ReasonSlopeUnsuitable
	default:
		return VerdictApt, ""
	}
}

func soilVerdictOf(method SprayMethod, soil SoilState) (EligibilityVerdict, EligibilityReason) {
	weight := WeightOf(method)
	if weight == WeightAerial {
		return VerdictApt, ""
	}
	switch {
	case !soil.Known():
		return VerdictAttention, ReasonSoilUnknown
	case soil == SoilWaterlogged && weight == WeightHeavy:
		return VerdictUnfit, ReasonSoilWaterlogged
	case soil == SoilWaterlogged:
		return VerdictAttention, ReasonSoilWaterlogged
	case soil == SoilMoist && weight == WeightHeavy:
		return VerdictAttention, ReasonSoilMoist
	default:
		return VerdictApt, ""
	}
}

// TrafficabilityDay is the soil state and the method verdicts of one day.
type TrafficabilityDay struct {
	Date      time.Time
	SoilState SoilState
	Methods   []MethodEligibility
}

// MethodRelease is the first day the soil meets the requirement of a method;
// Date is nil when it does not happen within the horizon.
type MethodRelease struct {
	Method        SprayMethod
	RequiredState SoilState
	Date          *time.Time
}

// TrafficabilityPlan is the verdict of a field for today and the forecast days.
type TrafficabilityPlan struct {
	Days     []TrafficabilityDay
	Releases []MethodRelease
}

// PlanTrafficability builds the verdict per day: today uses the assessed
// state and the following days the projected ones (dates and projected
// states must have the same length). fits is the relief fit of each method;
// a missing method or a nil slice means the relief was not calculated.
func PlanTrafficability(today time.Time, todayState SoilState, dates []time.Time, projected []SoilState, fits []MethodFit) TrafficabilityPlan {
	fitByMethod := make(map[SprayMethod]*MethodFit, len(fits))
	for i := range fits {
		fitByMethod[fits[i].Method] = &fits[i]
	}
	allDates := append([]time.Time{today}, dates...)
	allStates := append([]SoilState{todayState}, projected...)

	plan := TrafficabilityPlan{Days: make([]TrafficabilityDay, 0, len(allDates))}
	for i, date := range allDates {
		day := TrafficabilityDay{Date: date, SoilState: allStates[i]}
		for _, method := range SprayMethods() {
			day.Methods = append(day.Methods, EvaluateMethodEligibility(method, fitByMethod[method], allStates[i]))
		}
		plan.Days = append(plan.Days, day)
	}
	for _, method := range SprayMethods() {
		required, ok := RequiredSoilState(method)
		if !ok {
			continue
		}
		release := MethodRelease{Method: method, RequiredState: required}
		for i, state := range allStates {
			if state.Meets(required) {
				date := allDates[i]
				release.Date = &date
				break
			}
		}
		plan.Releases = append(plan.Releases, release)
	}
	return plan
}
