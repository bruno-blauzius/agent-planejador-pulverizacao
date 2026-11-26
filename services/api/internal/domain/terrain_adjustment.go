package domain

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxTerrainJustificationRunes is the limit of the adjustment justification.
const MaxTerrainJustificationRunes = 1000

const maxAdjustableSlopePct = 100

// TerrainAdjustment is a manual correction of the field relief. It wins over
// the calculated value but never replaces it (ADR 0021, decision 5).
type TerrainAdjustment struct {
	Origin        TerrainAdjustmentOrigin
	SlopeMeanPct  float64
	SlopeMaxPct   float64
	Class         SlopeClass
	Position      TerrainPosition
	Justification string
	AdjustedBy    UserID
	AdjustedAt    time.Time
}

// TerrainAdjustmentDetails is the user input of an adjustment.
type TerrainAdjustmentDetails struct {
	Origin        TerrainAdjustmentOrigin
	SlopeMeanPct  float64
	SlopeMaxPct   float64
	Position      TerrainPosition
	Justification string
}

// NewTerrainAdjustment validates the details and builds the adjustment. Errors
// are a *ValidationError with the fields origin, slope_mean_pct, slope_max_pct,
// position and justification.
func NewTerrainAdjustment(details TerrainAdjustmentDetails, by UserID, rules TerrainRules, now time.Time) (TerrainAdjustment, error) {
	var v Violations
	if !details.Origin.Valid() {
		v.Add("origin", "informe topographic_survey (levantamento topográfico) ou field_assessment (avaliação em campo)")
	}
	if !percentInRange(details.SlopeMeanPct) {
		v.Add("slope_mean_pct", "deve estar entre 0 e 100")
	}
	switch {
	case !percentInRange(details.SlopeMaxPct):
		v.Add("slope_max_pct", "deve estar entre 0 e 100")
	case percentInRange(details.SlopeMeanPct) && details.SlopeMaxPct < details.SlopeMeanPct:
		v.Add("slope_max_pct", "não pode ser menor que a declividade média")
	}
	if !details.Position.Valid() {
		v.Add("position", "informe hilltop (topo), slope (encosta) ou lowland (baixada)")
	}
	justification := strings.TrimSpace(details.Justification)
	switch {
	case justification == "":
		v.Add("justification", "a justificativa é obrigatória")
	case utf8.RuneCountInString(justification) > MaxTerrainJustificationRunes:
		v.Add("justification", "use no máximo 1000 caracteres")
	}
	if err := v.Err(); err != nil {
		return TerrainAdjustment{}, err
	}
	return TerrainAdjustment{
		Origin:        details.Origin,
		SlopeMeanPct:  details.SlopeMeanPct,
		SlopeMaxPct:   details.SlopeMaxPct,
		Class:         rules.ClassOf(details.SlopeMeanPct),
		Position:      details.Position,
		Justification: justification,
		AdjustedBy:    by,
		AdjustedAt:    now,
	}, nil
}

func percentInRange(v float64) bool {
	return !math.IsNaN(v) && v >= 0 && v <= maxAdjustableSlopePct
}

// Measurement presents the adjustment as a measurement; P90 and aspect are
// unknown for a manual value.
func (a TerrainAdjustment) Measurement() TerrainMeasurement {
	return TerrainMeasurement{
		SlopeMeanPct: a.SlopeMeanPct,
		SlopeMaxPct:  a.SlopeMaxPct,
		Class:        a.Class,
		Position:     a.Position,
	}
}

// EffectiveTerrain picks what counts for class and methods: the adjustment
// when there is one, otherwise the calculation. ok is false with neither.
func EffectiveTerrain(calc *TerrainCalculation, adj *TerrainAdjustment) (measurement TerrainMeasurement, source TerrainSource, ok bool) {
	switch {
	case adj != nil:
		return adj.Measurement(), SourceManual, true
	case calc != nil:
		return calc.Measurement, SourceCalculated, true
	default:
		return TerrainMeasurement{}, "", false
	}
}

// AdjustmentOutdated reports whether the calculated mean slope diverges from
// the adjusted one by more than the configured gap (strictly greater).
func AdjustmentOutdated(calc *TerrainCalculation, adj *TerrainAdjustment, rules TerrainRules) bool {
	if calc == nil || adj == nil {
		return false
	}
	return math.Abs(calc.Measurement.SlopeMeanPct-adj.SlopeMeanPct) > rules.AdjustmentDivergencePct
}
