package domain

import (
	"errors"
	"time"
)

// Terrain errors.
var (
	// ErrInsufficientElevationData means too few cells of the field have a
	// valid slope for the statistics to mean anything.
	ErrInsufficientElevationData = errors.New("insufficient elevation data")
	// ErrInvalidElevationWindow means the elevation grid is malformed.
	ErrInvalidElevationWindow = errors.New("invalid elevation window")
	// ErrInvalidTerrainRules means the terrain configuration is inconsistent.
	ErrInvalidTerrainRules = errors.New("invalid terrain rules")
)

// SlopeClass is the Embrapa relief class of a slope (ADR 0021).
type SlopeClass string

// Slope classes, from flattest to steepest.
const (
	SlopeFlat               SlopeClass = "flat"
	SlopeGentlyUndulating   SlopeClass = "gently_undulating"
	SlopeUndulating         SlopeClass = "undulating"
	SlopeStronglyUndulating SlopeClass = "strongly_undulating"
	SlopeMountainous        SlopeClass = "mountainous"
)

// SlopeClasses lists every class in enum order.
func SlopeClasses() []SlopeClass {
	return []SlopeClass{SlopeFlat, SlopeGentlyUndulating, SlopeUndulating, SlopeStronglyUndulating, SlopeMountainous}
}

// TerrainPosition is the position of a field in the relief around it.
type TerrainPosition string

// Terrain positions.
const (
	PositionHilltop TerrainPosition = "hilltop"
	PositionSlope   TerrainPosition = "slope"
	PositionLowland TerrainPosition = "lowland"
)

// Valid reports whether p is a known position.
func (p TerrainPosition) Valid() bool {
	return p == PositionHilltop || p == PositionSlope || p == PositionLowland
}

// TerrainAspect is the predominant direction a slope faces.
type TerrainAspect string

// Terrain aspects. AspectFlat means the slope defines no direction.
const (
	AspectNorth     TerrainAspect = "north"
	AspectNortheast TerrainAspect = "northeast"
	AspectEast      TerrainAspect = "east"
	AspectSoutheast TerrainAspect = "southeast"
	AspectSouth     TerrainAspect = "south"
	AspectSouthwest TerrainAspect = "southwest"
	AspectWest      TerrainAspect = "west"
	AspectNorthwest TerrainAspect = "northwest"
	AspectFlat      TerrainAspect = "flat"
)

// SprayMethod is a way of applying a product.
type SprayMethod string

// Spray methods, in enum order.
const (
	MethodKnapsack      SprayMethod = "knapsack"
	MethodTractorBoom   SprayMethod = "tractor_boom"
	MethodSelfPropelled SprayMethod = "self_propelled"
	MethodAirblast      SprayMethod = "airblast"
	MethodAircraft      SprayMethod = "aircraft"
	MethodDrone         SprayMethod = "drone"
)

// SprayMethods lists every method in enum order.
func SprayMethods() []SprayMethod {
	return []SprayMethod{MethodKnapsack, MethodTractorBoom, MethodSelfPropelled, MethodAirblast, MethodAircraft, MethodDrone}
}

// MethodFitStatus says how well a method fits the relief of a field.
type MethodFitStatus string

// Method fit statuses.
const (
	FitCompatible MethodFitStatus = "compatible"
	FitRestricted MethodFitStatus = "restricted"
	FitUnsuitable MethodFitStatus = "unsuitable"
)

// TerrainStatus is the state of the terrain calculation of a field. A field
// that was never calculated has no record; the API presents it as
// TerrainNotCalculated (the openapi enum includes it).
type TerrainStatus string

// Terrain statuses.
const (
	TerrainNotCalculated TerrainStatus = "not_calculated"
	TerrainPending       TerrainStatus = "pending"
	TerrainCalculating   TerrainStatus = "calculating"
	TerrainReady         TerrainStatus = "ready"
	TerrainFailed        TerrainStatus = "failed"
)

// TerrainSource tells where the effective terrain value comes from.
type TerrainSource string

// Terrain sources.
const (
	SourceCalculated TerrainSource = "calculated"
	SourceManual     TerrainSource = "manual"
)

// TerrainAdjustmentOrigin tells how a manual adjustment was obtained.
type TerrainAdjustmentOrigin string

// Adjustment origins.
const (
	OriginTopographicSurvey TerrainAdjustmentOrigin = "topographic_survey"
	OriginFieldAssessment   TerrainAdjustmentOrigin = "field_assessment"
)

// Valid reports whether o is a known origin.
func (o TerrainAdjustmentOrigin) Valid() bool {
	return o == OriginTopographicSurvey || o == OriginFieldAssessment
}

// LimitSourceReference marks a method slope limit that comes from the API
// configuration (as opposed to a registered equipment, in the future).
const LimitSourceReference = "reference"

// ElevationModel identifies the digital elevation model behind a calculation.
type ElevationModel struct {
	Source      string
	ResolutionM float64
	Version     string
}

// TerrainMeasurement holds the relief measures of a field. SlopeP90Pct and
// Aspect are nil in a manual adjustment.
type TerrainMeasurement struct {
	SlopeMeanPct float64
	SlopeP90Pct  *float64
	SlopeMaxPct  float64
	Class        SlopeClass
	Position     TerrainPosition
	Aspect       *TerrainAspect
}

// ProfilePoint is one point of the elevation profile along the steepest line.
type ProfilePoint struct {
	DistanceM  float64
	ElevationM float64
}

// SlopeHistogramBins is the size of the slope histogram: 1 % bins from 0 to
// 99 plus a last bin for slopes of 100 % or more.
const SlopeHistogramBins = 101

// SlopeClassArea is the area of a field in one slope class.
type SlopeClassArea struct {
	Class    SlopeClass
	AreaHa   float64
	SharePct float64
}

// TerrainCalculation is the result of the calculation from the elevation model.
type TerrainCalculation struct {
	Measurement      TerrainMeasurement
	ElevationMinM    float64
	ElevationMaxM    float64
	SlopeHistogramHa [SlopeHistogramBins]float64
	Profile          []ProfilePoint
	ElevationModel   ElevationModel
	CalculatedAt     time.Time
}

// ClassAreas derives the area per class from the calculation histogram.
func (c TerrainCalculation) ClassAreas(rules TerrainRules) []SlopeClassArea {
	return rules.ClassAreas(c.SlopeHistogramHa)
}
