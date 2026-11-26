package domain

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Meters per degree used to size a cell from its angular scale. They are the
// usual WGS84 approximations near the equator, corrected by cos(latitude) for
// longitude; the error is far below the 30 m resolution of the model.
const (
	metersPerDegreeLon = 111_320.0
	metersPerDegreeLat = 110_574.0
	squareMetersPerHa  = 10_000.0
	slopeP90Rank       = 0.9
)

// ElevationWindow is a regular grid of elevations in degrees that covers the
// field plus its surroundings (radius PositionRadiusM). Values and Inside are
// row-major with Rows*Cols entries; Values is NaN where there is no data and
// Inside marks the cells whose center is inside the field polygon.
type ElevationWindow struct {
	Rows, Cols int
	Values     []float64
	Inside     []bool
	OriginLon  float64 // upper-left corner
	OriginLat  float64
	ScaleXDeg  float64
	ScaleYDeg  float64 // negative, as in PostGIS
}

func (w ElevationWindow) validate() error {
	switch {
	case w.Rows < 1 || w.Cols < 1:
		return fmt.Errorf("%w: empty grid", ErrInvalidElevationWindow)
	case len(w.Values) != w.Rows*w.Cols || len(w.Inside) != w.Rows*w.Cols:
		return fmt.Errorf("%w: values and mask must have rows*cols entries", ErrInvalidElevationWindow)
	case w.ScaleXDeg == 0 || w.ScaleYDeg == 0 || math.IsNaN(w.ScaleXDeg) || math.IsNaN(w.ScaleYDeg):
		return fmt.Errorf("%w: scale must be non-zero", ErrInvalidElevationWindow)
	}
	return nil
}

// cellSizesM returns the cell width and height in meters, using the latitude
// of the middle of the window.
func (w ElevationWindow) cellSizesM() (dx, dy float64) {
	lat := w.OriginLat + float64(w.Rows)/2*w.ScaleYDeg
	dx = math.Abs(w.ScaleXDeg) * metersPerDegreeLon * math.Cos(lat*math.Pi/180)
	dy = math.Abs(w.ScaleYDeg) * metersPerDegreeLat
	return dx, dy
}

// gradient is the descent direction and steepness of a cell.
type gradient struct {
	slopePct float64
	// east and north are the components of the downhill unit vector (zero on flat ground).
	east, north float64
}

// hornGradient computes slope and downhill direction with Horn's method over
// the 3x3 neighborhood. ok is false on grid borders and next to missing data.
func (w ElevationWindow) hornGradient(r, c int, dx, dy float64) (g gradient, ok bool) {
	if r < 1 || c < 1 || r >= w.Rows-1 || c >= w.Cols-1 {
		return gradient{}, false
	}
	at := func(dr, dc int) float64 { return w.Values[(r+dr)*w.Cols+c+dc] }
	z1, z2, z3 := at(-1, -1), at(-1, 0), at(-1, 1)
	z4, z6 := at(0, -1), at(0, 1)
	z7, z8, z9 := at(1, -1), at(1, 0), at(1, 1)
	if hasNaN(z1, z2, z3, z4, at(0, 0), z6, z7, z8, z9) {
		return gradient{}, false
	}
	// Rows grow southwards, so the northward gradient is (top - bottom).
	dzEast := ((z3 + 2*z6 + z9) - (z1 + 2*z4 + z7)) / (8 * dx)
	dzNorth := ((z1 + 2*z2 + z3) - (z7 + 2*z8 + z9)) / (8 * dy)
	magnitude := math.Hypot(dzEast, dzNorth)
	g = gradient{slopePct: magnitude * 100}
	if magnitude > 0 {
		g.east, g.north = -dzEast/magnitude, -dzNorth/magnitude
	}
	return g, true
}

func hasNaN(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) {
			return true
		}
	}
	return false
}

// terrainCell is a cell of the field with everything the statistics need.
type terrainCell struct {
	slopePct   float64
	hasSlope   bool
	elevationM float64
	hasElev    bool
	xM, yM     float64 // meters east and north of the grid origin
}

// CalculateTerrain derives the relief of a field from an elevation window.
// It is pure: no I/O, and the clock comes from now.
//
// Percentile 90 uses the nearest-rank method on the sorted slopes of the field.
// Aspect is the mean of the downhill unit vectors of the cells with slope.
// Position compares the mean elevation of the field with the mean of the
// window cells outside it.
func CalculateTerrain(window ElevationWindow, rules TerrainRules, model ElevationModel, now time.Time) (TerrainCalculation, error) {
	if err := window.validate(); err != nil {
		return TerrainCalculation{}, err
	}
	if err := rules.Validate(); err != nil {
		return TerrainCalculation{}, err
	}
	dx, dy := window.cellSizesM()
	cellHa := dx * dy / squareMetersPerHa

	var (
		cells                  []terrainCell
		slopes                 []float64
		eastSum, northSum      float64
		insideElevSum, ringSum float64
		insideElevN, ringN     int
	)
	for r := 0; r < window.Rows; r++ {
		for c := 0; c < window.Cols; c++ {
			i := r*window.Cols + c
			elevation := window.Values[i]
			if !window.Inside[i] {
				if !math.IsNaN(elevation) {
					ringSum += elevation
					ringN++
				}
				continue
			}
			cell := terrainCell{xM: float64(c) * dx, yM: -float64(r) * dy}
			if !math.IsNaN(elevation) {
				cell.elevationM, cell.hasElev = elevation, true
				insideElevSum += elevation
				insideElevN++
			}
			if g, ok := window.hornGradient(r, c, dx, dy); ok {
				cell.slopePct, cell.hasSlope = g.slopePct, true
				slopes = append(slopes, g.slopePct)
				eastSum += g.east
				northSum += g.north
			}
			cells = append(cells, cell)
		}
	}
	if len(slopes) < rules.MinValidCells || len(slopes) == 0 {
		return TerrainCalculation{}, fmt.Errorf("%w: %d cells with slope, need %d", ErrInsufficientElevationData, len(slopes), rules.MinValidCells)
	}

	meanPct := mean(slopes)
	sort.Float64s(slopes)
	p90 := slopes[nearestRankIndex(len(slopes), slopeP90Rank)]
	aspect := aspectOf(meanPct, eastSum, northSum, len(slopes), rules)
	position := positionOf(insideElevSum, insideElevN, ringSum, ringN, rules)

	calc := TerrainCalculation{
		Measurement: TerrainMeasurement{
			SlopeMeanPct: meanPct,
			SlopeP90Pct:  &p90,
			SlopeMaxPct:  slopes[len(slopes)-1],
			Class:        rules.ClassOf(meanPct),
			Position:     position,
			Aspect:       &aspect,
		},
		ElevationModel: model,
		CalculatedAt:   now,
	}
	for _, s := range slopes {
		calc.SlopeHistogramHa[histogramBin(s)] += cellHa
	}
	calc.ElevationMinM, calc.ElevationMaxM = elevationRange(cells)
	calc.Profile = elevationProfile(cells, eastSum, northSum, rules.ProfilePoints)
	return calc, nil
}

func mean(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// nearestRankIndex returns the zero-based index of the nearest-rank percentile.
func nearestRankIndex(n int, fraction float64) int {
	idx := int(math.Ceil(fraction*float64(n))) - 1
	return min(max(idx, 0), n-1)
}

func histogramBin(slopePct float64) int {
	return min(int(math.Floor(slopePct)), SlopeHistogramBins-1)
}

var aspectSectors = [8]TerrainAspect{
	AspectNorth, AspectNortheast, AspectEast, AspectSoutheast,
	AspectSouth, AspectSouthwest, AspectWest, AspectNorthwest,
}

// minAspectResultantLength is the length of the MEAN downhill unit vector (0 when
// the cells face every direction equally, 1 when they all face the same way)
// below which there is no predominant direction. A ridge or a bowl has cells
// facing opposite ways, so the vectors almost cancel and what is left only
// reflects which side has a few more cells; naming a direction would be noise.
const minAspectResultantLength = 0.1

// aspectOf names the predominant downhill direction in 8 sectors of 45 degrees
// centered on the compass points; "flat" when the mean slope is below the
// configured threshold or the mean downhill vector (the sums divided by the
// number of cells with slope) is shorter than minAspectResultantLength.
func aspectOf(meanSlopePct, eastSum, northSum float64, cells int, rules TerrainRules) TerrainAspect {
	if meanSlopePct < rules.FlatAspectBelowPct || cells == 0 || math.Hypot(eastSum, northSum)/float64(cells) < minAspectResultantLength {
		return AspectFlat
	}
	azimuth := math.Atan2(eastSum, northSum) * 180 / math.Pi
	if azimuth < 0 {
		azimuth += 360
	}
	return aspectSectors[int(math.Floor((azimuth+22.5)/45))%8]
}

func positionOf(insideSum float64, insideN int, ringSum float64, ringN int, rules TerrainRules) TerrainPosition {
	if insideN == 0 || ringN == 0 {
		return PositionSlope
	}
	diff := insideSum/float64(insideN) - ringSum/float64(ringN)
	switch {
	case diff > rules.PositionToleranceM:
		return PositionHilltop
	case diff < -rules.PositionToleranceM:
		return PositionLowland
	default:
		return PositionSlope
	}
}

func elevationRange(cells []terrainCell) (lowest, highest float64) {
	lowest, highest = math.Inf(1), math.Inf(-1)
	for _, c := range cells {
		if !c.hasElev {
			continue
		}
		lowest, highest = math.Min(lowest, c.elevationM), math.Max(highest, c.elevationM)
	}
	return lowest, highest
}

// elevationProfile projects the cells on the mean downhill direction, splits
// the distance from the highest point into equal strips and averages the
// elevation per strip. Empty strips are omitted; distance is the strip center.
// On flat ground (no direction) the axis points east.
func elevationProfile(cells []terrainCell, eastSum, northSum float64, points int) []ProfilePoint {
	axisEast, axisNorth := 1.0, 0.0
	if norm := math.Hypot(eastSum, northSum); norm > 1e-9 {
		axisEast, axisNorth = eastSum/norm, northSum/norm
	}
	type projected struct{ t, elevation float64 }
	var pts []projected
	tMin, tMax := math.Inf(1), math.Inf(-1)
	for _, c := range cells {
		if !c.hasElev {
			continue
		}
		t := c.xM*axisEast + c.yM*axisNorth
		pts = append(pts, projected{t, c.elevationM})
		tMin, tMax = math.Min(tMin, t), math.Max(tMax, t)
	}
	if len(pts) == 0 {
		return nil
	}
	width := (tMax - tMin) / float64(points)
	sums, counts := make([]float64, points), make([]int, points)
	for _, p := range pts {
		strip := 0
		if width > 0 {
			strip = min(int((p.t-tMin)/width), points-1)
		}
		sums[strip] += p.elevation
		counts[strip]++
	}
	profile := make([]ProfilePoint, 0, points)
	for i := range points {
		if counts[i] == 0 {
			continue
		}
		profile = append(profile, ProfilePoint{
			DistanceM:  (float64(i) + 0.5) * width,
			ElevationM: sums[i] / float64(counts[i]),
		})
	}
	return profile
}
