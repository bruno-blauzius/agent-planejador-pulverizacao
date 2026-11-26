package domain

import "time"

// SoilTexture is the texture class of a field soil (ADR 0022). It selects the
// trafficability thresholds.
type SoilTexture string

// Soil textures.
const (
	TextureSandy  SoilTexture = "sandy"
	TextureMedium SoilTexture = "medium"
	TextureClay   SoilTexture = "clay"
)

// SoilTextures lists every texture in enum order.
func SoilTextures() []SoilTexture {
	return []SoilTexture{TextureSandy, TextureMedium, TextureClay}
}

// Valid reports whether t is a known texture.
func (t SoilTexture) Valid() bool {
	return t == TextureSandy || t == TextureMedium || t == TextureClay
}

// SoilState is how the soil holds ground machines (ADR 0022, decision 1).
// The known states are ordered from best to worst for traffic; SoilUnknown
// means there was no rain source at all and is outside that order.
type SoilState string

// Soil states.
const (
	SoilDry         SoilState = "dry"
	SoilIdeal       SoilState = "ideal"
	SoilMoist       SoilState = "moist"
	SoilWaterlogged SoilState = "waterlogged"
	SoilUnknown     SoilState = "unknown"
)

// knownSoilStates is the traffic order, best first.
var knownSoilStates = [...]SoilState{SoilDry, SoilIdeal, SoilMoist, SoilWaterlogged}

// SoilStates lists every state in enum order.
func SoilStates() []SoilState {
	return []SoilState{SoilDry, SoilIdeal, SoilMoist, SoilWaterlogged, SoilUnknown}
}

// rank returns the position of s in the traffic order, or -1 for unknown.
func (s SoilState) rank() int {
	for i, known := range knownSoilStates {
		if s == known {
			return i
		}
	}
	return -1
}

// Known reports whether s is one of the four ordered states.
func (s SoilState) Known() bool { return s.rank() >= 0 }

// Meets reports whether s is at least as good as required for traffic. An
// unknown state never meets a requirement.
func (s SoilState) Meets(required SoilState) bool {
	return s.Known() && required.Known() && s.rank() <= required.rank()
}

// WorstSoilState returns the worse of two known states; if either is unknown
// it returns the other one.
func WorstSoilState(a, b SoilState) SoilState {
	switch {
	case !a.Known():
		return b
	case !b.Known():
		return a
	case a.rank() >= b.rank():
		return a
	default:
		return b
	}
}

// oneStateWorse returns the next state towards waterlogged.
func oneStateWorse(s SoilState) SoilState {
	r := s.rank()
	if r < 0 || r == len(knownSoilStates)-1 {
		return s
	}
	return knownSoilStates[r+1]
}

// RainSource tells where a rain amount comes from. A gauge or a farm station
// measures the field; the model is a weather model cell kilometres away.
type RainSource string

// Rain sources, in priority order.
const (
	RainFromGauge   RainSource = "gauge"
	RainFromStation RainSource = "station"
	RainFromModel   RainSource = "model"
)

// Measured reports whether the source measured the rain on the farm.
func (s RainSource) Measured() bool { return s == RainFromGauge || s == RainFromStation }

// RainAccumulation holds the recent rain on a field, in millimetres, summed
// over the last 24, 48 and 72 hours and 7 days.
type RainAccumulation struct {
	Last24hMm float64
	Last48hMm float64
	Last72hMm float64
	Last7dMm  float64
	Source    RainSource
	// SourceID identifies the gauge or station; empty for the model.
	SourceID   string
	ObservedAt time.Time
}

// ModelSoilMoisture is the volumetric soil moisture of the weather model over
// the top layer, next to the field capacity of the same soil.
type ModelSoilMoisture struct {
	VolumetricM3M3    float64
	FieldCapacityM3M3 float64
}

// FieldCapacityPct returns the moisture as a percentage of field capacity.
func (m ModelSoilMoisture) FieldCapacityPct() (float64, bool) {
	if m.FieldCapacityM3M3 <= 0 || m.VolumetricM3M3 < 0 {
		return 0, false
	}
	return m.VolumetricM3M3 / m.FieldCapacityM3M3 * 100, true
}

// SoilInspection is a person's assessment of the soil on the field ("soil ok
// to enter"). While valid it replaces the calculation.
type SoilInspection struct {
	State       SoilState
	InspectedAt time.Time
}

// DailyRain is the rain forecast for one local day.
type DailyRain struct {
	Date time.Time
	Mm   float64
}
