package domain

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Field conditions are the data people record about rain and soil (ADR 0022,
// addendum): rain gauges of a farm, their readings, soil inspections and the
// soil profile of a field. These limits are input sanity bounds of the
// contract, not agronomic rules.
const (
	MaxRainGaugeNameLength      = 100
	MaxSoilInspectionNoteLength = 500
	// MaxRainReadingMm is the largest rain a single reading may report. Above
	// it the value is a typing error, not rain.
	MaxRainReadingMm = 500.0
	// WeatherCellDegrees is the grid the field centroid is rounded to before it
	// leaves the company (addendum item 8): about 1 km, finer than the weather
	// model cell, and the key of the weather cache.
	WeatherCellDegrees = 0.01
)

// ---- rain gauge ----------------------------------------------------------------------

// RainGaugeDetails are the editable data of a rain gauge.
type RainGaugeDetails struct {
	Name string
	// Location is optional: the gauge is known by its name.
	Location *GeoPoint
}

func (d RainGaugeDetails) validated() (RainGaugeDetails, error) {
	var problems Violations
	d.Name = strings.TrimSpace(d.Name)
	switch {
	case d.Name == "":
		problems.Add("name", "é obrigatório")
	case utf8.RuneCountInString(d.Name) > MaxRainGaugeNameLength:
		problems.Add("name", "deve ter no máximo 100 caracteres")
	}
	if d.Location != nil {
		if _, err := NewGeoPoint("location", d.Location.Lat, d.Location.Lng); err != nil {
			problems.Add("location", "coordenada fora do intervalo")
		}
	}
	return d, problems.Err()
}

// RainGauge is a rain gauge of a farm. Its readings serve every field linked
// to it (addendum item 3).
type RainGauge struct {
	ID        RainGaugeID
	CompanyID CompanyID
	FarmID    FarmID
	Name      string
	Location  *GeoPoint
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// NewRainGauge validates the details and builds a gauge of the farm.
func NewRainGauge(id RainGaugeID, companyID CompanyID, farmID FarmID, details RainGaugeDetails, now time.Time) (RainGauge, error) {
	valid, err := details.validated()
	if err != nil {
		return RainGauge{}, err
	}
	return RainGauge{
		ID: id, CompanyID: companyID, FarmID: farmID, Name: valid.Name, Location: valid.Location,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Revise replaces the editable data of the gauge.
func (g RainGauge) Revise(details RainGaugeDetails, now time.Time) (RainGauge, error) {
	valid, err := details.validated()
	if err != nil {
		return RainGauge{}, err
	}
	g.Name, g.Location, g.UpdatedAt = valid.Name, valid.Location, now
	return g, nil
}

// ---- rain reading --------------------------------------------------------------------

// RainReading is the rain accumulated in a gauge since the previous reading,
// read at ObservedAt.
type RainReading struct {
	ID         RainReadingID
	CompanyID  CompanyID
	GaugeID    RainGaugeID
	RainMm     float64
	ObservedAt time.Time
	RecordedBy UserID
	RecordedAt time.Time
}

// NewRainReading validates a reading of the gauge: the rain is between 0 and
// MaxRainReadingMm and the reading is not in the future.
func NewRainReading(id RainReadingID, gauge RainGauge, rainMm float64, observedAt time.Time, recordedBy UserID, now time.Time) (RainReading, error) {
	var problems Violations
	if math.IsNaN(rainMm) || math.IsInf(rainMm, 0) || rainMm < 0 || rainMm > MaxRainReadingMm {
		problems.Add("rain_mm", "deve estar entre 0 e 500 mm")
	}
	switch {
	case observedAt.IsZero():
		problems.Add("observed_at", "é obrigatório")
	case observedAt.After(now):
		problems.Add("observed_at", "não pode estar no futuro")
	}
	if err := problems.Err(); err != nil {
		return RainReading{}, err
	}
	return RainReading{
		ID: id, CompanyID: gauge.CompanyID, GaugeID: gauge.ID, RainMm: rainMm,
		ObservedAt: observedAt, RecordedBy: recordedBy, RecordedAt: now,
	}, nil
}

// GaugeRainAccumulation sums the readings of a gauge over the last 24, 48 and
// 72 hours and 7 days before now (a reading counts in a window when its
// observed_at is inside it). A gauge is a source only while its latest reading
// is younger than maxStaleness: without a recent reading nobody can tell "it
// did not rain" from "nobody read the gauge" (addendum item 3). It returns nil
// in that case. Readings after now are ignored.
func GaugeRainAccumulation(now time.Time, gauge RainGauge, readings []RainReading, maxStaleness time.Duration) *RainAccumulation {
	var latest time.Time
	var sums [4]float64
	windows := [4]time.Duration{24 * time.Hour, 48 * time.Hour, 72 * time.Hour, 7 * 24 * time.Hour}
	for _, reading := range readings {
		if reading.ObservedAt.After(now) {
			continue
		}
		if reading.ObservedAt.After(latest) {
			latest = reading.ObservedAt
		}
		age := now.Sub(reading.ObservedAt)
		for i, window := range windows {
			if age < window {
				sums[i] += reading.RainMm
			}
		}
	}
	if latest.IsZero() || now.Sub(latest) >= maxStaleness {
		return nil
	}
	return &RainAccumulation{
		Last24hMm: sums[0], Last48hMm: sums[1], Last72hMm: sums[2], Last7dMm: sums[3],
		Source: RainFromGauge, SourceID: string(gauge.ID), ObservedAt: latest,
	}
}

// DailyGaugeRain sums the readings per local day (observed_at in loc) for each
// of the given days, which are local midnights.
func DailyGaugeRain(readings []RainReading, days []time.Time, loc *time.Location) []DailyRain {
	byDay := make(map[time.Time]float64, len(days))
	for _, reading := range readings {
		byDay[LocalDay(reading.ObservedAt, loc)] += reading.RainMm
	}
	daily := make([]DailyRain, 0, len(days))
	for _, day := range days {
		daily = append(daily, DailyRain{Date: day, Mm: byDay[LocalDay(day, loc)]})
	}
	return daily
}

// LocalDay returns the midnight of the local day of t.
func LocalDay(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// ---- weather model -------------------------------------------------------------------

// WeatherCell is the rounded coordinate sent to the weather provider.
type WeatherCell struct {
	Latitude  float64
	Longitude float64
}

// WeatherCellOf rounds a point to the WeatherCellDegrees grid.
func WeatherCellOf(point GeoPoint) WeatherCell {
	round := func(value float64) float64 {
		cells := math.Round(value / WeatherCellDegrees)
		// Dividing by the inverse avoids residues such as 0.30000000000000004.
		return cells / (1 / WeatherCellDegrees)
	}
	return WeatherCell{Latitude: round(point.Lat), Longitude: round(point.Lng)}
}

// HourlyWeather is one hour of the weather model. RainMm is the rain of the
// hour that ends at Time; nil values are missing in the model.
type HourlyWeather struct {
	Time             time.Time
	RainMm           *float64
	SoilMoistureM3M3 *float64
}

// ModelRainAccumulation sums the hourly rain of the model over the last 24,
// 48 and 72 hours and 7 days before now. Hours after now are forecast and do
// not count. It returns nil when the model has no rain up to now.
func ModelRainAccumulation(now time.Time, hours []HourlyWeather) *RainAccumulation {
	var latest time.Time
	var sums [4]float64
	windows := [4]time.Duration{24 * time.Hour, 48 * time.Hour, 72 * time.Hour, 7 * 24 * time.Hour}
	for _, hour := range hours {
		if hour.RainMm == nil || hour.Time.After(now) {
			continue
		}
		if hour.Time.After(latest) {
			latest = hour.Time
		}
		age := now.Sub(hour.Time)
		for i, window := range windows {
			if age < window {
				sums[i] += math.Max(0, *hour.RainMm)
			}
		}
	}
	if latest.IsZero() {
		return nil
	}
	return &RainAccumulation{
		Last24hMm: sums[0], Last48hMm: sums[1], Last72hMm: sums[2], Last7dMm: sums[3],
		Source: RainFromModel, ObservedAt: latest,
	}
}

// ModelMoistureAt returns the volumetric soil moisture of the latest hour up
// to now, or nil when the model has none.
func ModelMoistureAt(now time.Time, hours []HourlyWeather) *float64 {
	var latest time.Time
	var moisture *float64
	for _, hour := range hours {
		if hour.SoilMoistureM3M3 == nil || hour.Time.After(now) || !hour.Time.After(latest) {
			continue
		}
		latest = hour.Time
		value := *hour.SoilMoistureM3M3
		moisture = &value
	}
	return moisture
}

// ---- soil inspection -----------------------------------------------------------------

// FieldSoilInspection is a soil inspection recorded by a person on a field.
type FieldSoilInspection struct {
	ID          SoilInspectionID
	CompanyID   CompanyID
	FarmID      FarmID
	FieldID     FieldID
	State       SoilState
	InspectedAt time.Time
	InspectedBy UserID
	Note        *string
	RecordedAt  time.Time
}

// SoilInspectionDetails are what a person reports.
type SoilInspectionDetails struct {
	State SoilState
	// InspectedAt defaults to now when zero.
	InspectedAt time.Time
	Note        *string
}

// NewFieldSoilInspection validates an inspection: the state is one of the four
// known states, the time is not in the future and the note is short.
func NewFieldSoilInspection(id SoilInspectionID, field Field, details SoilInspectionDetails, inspectedBy UserID, now time.Time) (FieldSoilInspection, error) {
	var problems Violations
	if !details.State.Known() {
		problems.Add("state", "deve ser dry, ideal, moist ou waterlogged")
	}
	inspectedAt := details.InspectedAt
	if inspectedAt.IsZero() {
		inspectedAt = now
	}
	if inspectedAt.After(now) {
		problems.Add("inspected_at", "não pode estar no futuro")
	}
	note := details.Note
	if note != nil {
		trimmed := strings.TrimSpace(*note)
		switch {
		case trimmed == "":
			note = nil
		case utf8.RuneCountInString(trimmed) > MaxSoilInspectionNoteLength:
			problems.Add("note", "deve ter no máximo 500 caracteres")
		default:
			note = &trimmed
		}
	}
	if err := problems.Err(); err != nil {
		return FieldSoilInspection{}, err
	}
	return FieldSoilInspection{
		ID: id, CompanyID: field.CompanyID, FarmID: field.FarmID, FieldID: field.ID,
		State: details.State, InspectedAt: inspectedAt, InspectedBy: inspectedBy, Note: note, RecordedAt: now,
	}, nil
}

// Assessment is the inspection as an input of the soil state calculation.
func (i FieldSoilInspection) Assessment() SoilInspection {
	return SoilInspection{State: i.State, InspectedAt: i.InspectedAt}
}

// ---- soil profile --------------------------------------------------------------------

// SoilProfile is the soil texture of a field and the rain gauge that serves it.
// Both are optional: without texture the soil state is unknown; without a
// gauge the field uses the weather model.
type SoilProfile struct {
	Texture     *SoilTexture
	RainGaugeID *RainGaugeID
}

// NewSoilProfile validates the texture. Whether the gauge belongs to the farm
// of the field is checked against the store.
func NewSoilProfile(texture *SoilTexture, gaugeID *RainGaugeID) (SoilProfile, error) {
	if texture != nil && !texture.Valid() {
		return SoilProfile{}, Invalid("soil_texture", "deve ser sandy, medium ou clay")
	}
	if gaugeID != nil && strings.TrimSpace(string(*gaugeID)) == "" {
		return SoilProfile{}, Invalid("rain_gauge_id", "identificador inválido")
	}
	return SoilProfile{Texture: texture, RainGaugeID: gaugeID}, nil
}
