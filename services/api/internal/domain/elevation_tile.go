package domain

import (
	"fmt"
	"math"
)

// ElevationTileKey names a 1 degree by 1 degree block of the elevation model
// by its south-west corner: hemisphere and two-digit latitude, hemisphere and
// three-digit longitude, as in 'S22W047' (the Copernicus GLO-30 naming and the
// tile_key of elevation_sources, ADR 0021).
type ElevationTileKey string

// maxTilesCovering bounds the answer of TileKeysCovering, so a corrupt box
// cannot ask for the whole planet.
const maxTilesCovering = 64

// NewElevationTileKey builds the key of the block whose south-west corner is
// at the given whole degrees (south and west are negative).
func NewElevationTileKey(southLatDeg, westLonDeg int) ElevationTileKey {
	latHemisphere, lonHemisphere := 'N', 'E'
	if southLatDeg < 0 {
		latHemisphere, southLatDeg = 'S', -southLatDeg
	}
	if westLonDeg < 0 {
		lonHemisphere, westLonDeg = 'W', -westLonDeg
	}
	return ElevationTileKey(fmt.Sprintf("%c%02d%c%03d", latHemisphere, southLatDeg, lonHemisphere, westLonDeg))
}

// TileKeysCovering returns every block touched by the box, ordered by
// latitude and then longitude (south-west first). A block that the box only
// touches along an edge is not included, unless the box is a point or a line
// on that edge. It returns nil for an inverted box, a non-finite coordinate or
// a box that would need more than 64 blocks.
func TileKeysCovering(minLon, minLat, maxLon, maxLat float64) []ElevationTileKey {
	if !finite(minLon, minLat, maxLon, maxLat) || minLon > maxLon || minLat > maxLat {
		return nil
	}
	west, east := tileRange(minLon, maxLon)
	south, north := tileRange(minLat, maxLat)
	if (east-west+1)*(north-south+1) > maxTilesCovering {
		return nil
	}
	keys := make([]ElevationTileKey, 0, (east-west+1)*(north-south+1))
	for lat := south; lat <= north; lat++ {
		for lon := west; lon <= east; lon++ {
			keys = append(keys, NewElevationTileKey(lat, lon))
		}
	}
	return keys
}

// tileRange returns the first and last whole-degree block index of [lo, hi].
func tileRange(lo, hi float64) (first, last int) {
	first, last = int(math.Floor(lo)), int(math.Floor(hi))
	if hi == math.Floor(hi) && hi > lo {
		last-- // the interval ends exactly on a grid line: the next block is only touched
	}
	return first, last
}

func finite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}
