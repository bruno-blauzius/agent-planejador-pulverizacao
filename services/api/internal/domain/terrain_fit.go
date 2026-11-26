package domain

import "math"

// MethodFit is the compatibility of one spray method with the field relief.
// It only considers slope; label, soil, crop size and buffer zones belong to
// the spray planner.
//
// AreaAboveLimitPct is only known when the effective value was calculated: a
// manual adjustment has no distribution, so there it is nil and Status comes
// from comparing the mean (above the limit: unsuitable) and the maximum (above
// the limit: restricted) with the limit.
type MethodFit struct {
	Method            SprayMethod
	SlopeLimitPct     *float64
	LimitSource       string
	Status            MethodFitStatus
	AreaAboveLimitPct *float64
}

// EvaluateMethodFits applies the slope rule to every method, in enum order.
//
// With a calculated value and its histogram, the share of the area above the
// limit decides: below RestrictedFromSharePct is compatible, below
// UnsuitableFromSharePct is restricted, otherwise unsuitable. A bin that
// straddles the limit counts proportionally (linear interpolation inside the
// bin). With a manual value there is no distribution, so the mean above the
// limit is unsuitable, the maximum above it is restricted, else compatible.
func EvaluateMethodFits(rules TerrainRules, effective TerrainMeasurement, source TerrainSource, histogramHa *[SlopeHistogramBins]float64) []MethodFit {
	methods := SprayMethods()
	fits := make([]MethodFit, 0, len(methods))
	for _, method := range methods {
		fits = append(fits, evaluateMethodFit(rules, method, effective, source, histogramHa))
	}
	return fits
}

func evaluateMethodFit(rules TerrainRules, method SprayMethod, effective TerrainMeasurement, source TerrainSource, histogramHa *[SlopeHistogramBins]float64) MethodFit {
	limit := rules.MethodSlopeLimits[method]
	fit := MethodFit{Method: method, LimitSource: LimitSourceReference, Status: FitCompatible}
	if limit == nil {
		return fit
	}
	l := *limit
	fit.SlopeLimitPct = &l

	if share, ok := shareAboveLimit(histogramHa, l); ok && source == SourceCalculated {
		fit.AreaAboveLimitPct = &share
		fit.Status = statusFromShare(rules, share)
		return fit
	}
	switch {
	case effective.SlopeMeanPct > l:
		fit.Status = FitUnsuitable
	case effective.SlopeMaxPct > l:
		fit.Status = FitRestricted
	}
	return fit
}

// shareAboveLimit returns the percentage of the histogram area above the
// limit. Bin i covers [i, i+1); the last bin is treated as [100, 101).
func shareAboveLimit(histogramHa *[SlopeHistogramBins]float64, limitPct float64) (float64, bool) {
	if histogramHa == nil {
		return 0, false
	}
	total, above := 0.0, 0.0
	for i, ha := range histogramHa {
		total += ha
		lower := float64(i)
		switch {
		case lower >= limitPct:
			above += ha
		case lower+1 > limitPct:
			above += ha * (lower + 1 - limitPct)
		}
	}
	if total <= 0 {
		return 0, false
	}
	return math.Min(above/total*100, 100), true
}

func statusFromShare(rules TerrainRules, sharePct float64) MethodFitStatus {
	switch {
	case sharePct < rules.RestrictedFromSharePct:
		return FitCompatible
	case sharePct < rules.UnsuitableFromSharePct:
		return FitRestricted
	default:
		return FitUnsuitable
	}
}

// HasRestrictedMethod reports whether any method limited by slope is not compatible.
func HasRestrictedMethod(fits []MethodFit) bool {
	for _, fit := range fits {
		if fit.SlopeLimitPct != nil && fit.Status != FitCompatible {
			return true
		}
	}
	return false
}
