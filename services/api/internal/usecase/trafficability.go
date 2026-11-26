package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Projection horizon of GET .../trafficability (contract: days 1..7, default 5).
const (
	defaultTrafficabilityDays = 5
	maxTrafficabilityDays     = 7
	// pastRainDays is how many local days (today included) rain_daily shows before the forecast.
	pastRainDays = 7
)

// TrafficabilityDataIssue names data that was missing in the calculation.
type TrafficabilityDataIssue string

// Data issues of the contract (TrafficabilityDataIssue).
const (
	IssueNoRainSource         TrafficabilityDataIssue = "no_rain_source"
	IssueNoSoilTexture        TrafficabilityDataIssue = "no_soil_texture"
	IssueWeatherUnavailable   TrafficabilityDataIssue = "weather_unavailable"
	IssueTerrainNotCalculated TrafficabilityDataIssue = "terrain_not_calculated"
)

// RainKind tells a measured (past) daily rain from a forecast one.
type RainKind string

// Rain kinds of the contract (DailyRainEntry.kind).
const (
	RainMeasured RainKind = "measured"
	RainForecast RainKind = "forecast"
)

// DailyRainEntry is the rain of one local day and where it comes from.
type DailyRainEntry struct {
	Date   time.Time
	RainMm float64
	Kind   RainKind
	Source domain.RainSource
}

// SoilStatusView is the soil state of today and what supports it.
type SoilStatusView struct {
	State            domain.SoilState
	Basis            domain.SoilBasis
	Texture          *domain.SoilTexture
	ThresholdsSource string
	Rain             *domain.RainAccumulation
	// RainSourceName is the gauge name; nil for the model.
	RainSourceName *string
	ModelMoisture  *domain.ModelSoilMoisture
	// Inspection is the inspection that is still valid, nil otherwise.
	Inspection *SoilInspectionView
}

// FieldTrafficabilityView is the answer of GET .../trafficability.
type FieldTrafficabilityView struct {
	FieldID    domain.FieldID
	FarmID     domain.FarmID
	AsOf       time.Time
	Soil       SoilStatusView
	RainDaily  []DailyRainEntry
	Plan       domain.TrafficabilityPlan
	DataIssues []TrafficabilityDataIssue
}

// GetFieldTrafficabilityDeps are the dependencies of GetFieldTrafficability.
type GetFieldTrafficabilityDeps struct {
	Fields       FieldConditionsReader
	Gauges       RainGaugeReader
	Thresholds   TrafficabilityThresholdsReader
	Terrain      TerrainReader
	Weather      WeatherProvider
	Clock        Clock
	TerrainRules domain.TerrainRules
	Settings     TrafficabilitySettings
}

// GetFieldTrafficabilityInput is the request of GET .../fields/{fieldId}/trafficability.
type GetFieldTrafficabilityInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
	// Days of projection after today; 0 means the default (5).
	Days int
}

// GetFieldTrafficability crosses rain, soil and relief into the verdict of
// each spray method per day (ADR 0022). It is read-only: the screen and the
// planner agent consume the same answer.
type GetFieldTrafficability struct {
	evaluator *TrafficabilityEvaluator
}

// NewGetFieldTrafficability builds the use case.
func NewGetFieldTrafficability(deps GetFieldTrafficabilityDeps) *GetFieldTrafficability {
	return &GetFieldTrafficability{evaluator: NewTrafficabilityEvaluator(deps)}
}

// FieldRef locates a field of a company.
type FieldRef struct {
	CompanyID domain.CompanyID
	FarmID    domain.FarmID
	FieldID   domain.FieldID
}

// TrafficabilityEvaluator is the core of GetFieldTrafficability: it reads the
// data of one field and lets the domain decide. It checks no actor: the GET
// use case checks the scope of the caller first, and the soil history
// executor (no user) runs it for every field of every company. Both get the
// very same verdict.
type TrafficabilityEvaluator struct {
	deps GetFieldTrafficabilityDeps
}

// NewTrafficabilityEvaluator builds the core.
func NewTrafficabilityEvaluator(deps GetFieldTrafficabilityDeps) *TrafficabilityEvaluator {
	return &TrafficabilityEvaluator{deps: deps}
}

// Evaluate returns the trafficability of the field for today and `days`
// forecast days (1 to 7). ErrNotFound when the field is not in the farm of the company.
func (uc *TrafficabilityEvaluator) Evaluate(ctx context.Context, field FieldRef, days int) (FieldTrafficabilityView, error) {
	if days < 1 || days > maxTrafficabilityDays {
		return FieldTrafficabilityView{}, domain.Invalid("days", "deve estar entre 1 e 7")
	}
	inputs, err := uc.read(ctx, field)
	if err != nil {
		return FieldTrafficabilityView{}, err
	}
	return uc.decide(inputs, days)
}

// trafficabilityInputs is everything read before the domain decides.
type trafficabilityInputs struct {
	now, today time.Time
	field      FieldConditionsData
	readings   []domain.RainReading
	inspection *StoredSoilInspection
	weather    *WeatherReport
	fits       []domain.MethodFit
	calculator domain.SoilStateCalculator
	thresholds map[domain.SoilTexture]domain.TrafficabilityThresholds
}

// Execute returns ErrNotFound for a field that is missing or out of scope. A
// weather failure never becomes a value: with a fresh gauge the state comes
// from the measurement and the projection is dropped; without one the state
// is unknown (addendum item 8).
func (uc *GetFieldTrafficability) Execute(ctx context.Context, in GetFieldTrafficabilityInput) (FieldTrafficabilityView, error) {
	if err := checkConditionsRead(in.Actor, in.FarmID); err != nil {
		return FieldTrafficabilityView{}, err
	}
	days := in.Days
	if days == 0 {
		days = defaultTrafficabilityDays
	}
	return uc.evaluator.Evaluate(ctx, FieldRef{CompanyID: in.Actor.CompanyID, FarmID: in.FarmID, FieldID: in.FieldID}, days)
}

func (uc *TrafficabilityEvaluator) read(ctx context.Context, in FieldRef) (trafficabilityInputs, error) {
	companyID := in.CompanyID
	now := uc.deps.Clock.Now().In(uc.deps.Settings.Location)
	inputs := trafficabilityInputs{now: now, today: domain.LocalDay(now, uc.deps.Settings.Location)}

	field, err := uc.deps.Fields.ReadFieldConditions(ctx, companyID, in.FarmID, in.FieldID)
	if err != nil {
		return inputs, fmt.Errorf("read field conditions: %w", err)
	}
	inputs.field = field
	if inputs.inspection, err = uc.deps.Fields.LatestSoilInspection(ctx, companyID, in.FieldID); err != nil {
		return inputs, fmt.Errorf("read latest soil inspection: %w", err)
	}
	if field.Gauge != nil {
		stored, err := uc.deps.Gauges.ListRainReadings(ctx, companyID, field.Gauge.ID, now.Add(-7*24*time.Hour), now)
		if err != nil {
			return inputs, fmt.Errorf("list rain readings: %w", err)
		}
		for _, item := range stored {
			inputs.readings = append(inputs.readings, item.Reading)
		}
	}
	terrain, err := uc.deps.Terrain.ReadFieldTerrain(ctx, companyID, in.FarmID, in.FieldID)
	if err != nil {
		return inputs, fmt.Errorf("read field terrain: %w", err)
	}
	inputs.fits = evaluateTerrain(uc.deps.TerrainRules, terrain).fits

	if inputs.thresholds, err = uc.deps.Thresholds.CurrentTrafficabilityThresholds(ctx, inputs.today); err != nil {
		return inputs, fmt.Errorf("read trafficability thresholds: %w", err)
	}
	rules := domain.SoilTrafficabilityRules{Thresholds: inputs.thresholds, InspectionValidity: uc.deps.Settings.InspectionValidity}
	if inputs.calculator, err = domain.NewSoilStateCalculator(rules); err != nil {
		return inputs, fmt.Errorf("trafficability thresholds: %w", err)
	}

	report, err := uc.deps.Weather.Weather(ctx, domain.WeatherCellOf(field.Centroid))
	switch {
	case errors.Is(err, ErrWeatherUnavailable):
	case err != nil:
		return inputs, fmt.Errorf("read weather: %w", err)
	default:
		inputs.weather = &report
	}
	return inputs, nil
}

func (uc *TrafficabilityEvaluator) decide(in trafficabilityInputs, days int) (FieldTrafficabilityView, error) {
	view := FieldTrafficabilityView{FieldID: in.field.FieldID, FarmID: in.field.FarmID, AsOf: in.now, DataIssues: []TrafficabilityDataIssue{}}
	texture := in.field.Texture
	if texture == nil {
		view.DataIssues = append(view.DataIssues, IssueNoSoilTexture)
	}

	rain, sourceName := uc.rainSource(in)
	if rain == nil {
		view.DataIssues = append(view.DataIssues, IssueNoRainSource)
	}
	if in.weather == nil {
		view.DataIssues = append(view.DataIssues, IssueWeatherUnavailable)
	}
	if in.fits == nil {
		view.DataIssues = append(view.DataIssues, IssueTerrainNotCalculated)
	}

	soil, err := uc.soilStatus(in, rain, sourceName)
	if err != nil {
		return FieldTrafficabilityView{}, err
	}
	view.Soil = soil

	forecast := forecastDays(in.weather, in.today, days)
	projected := make([]domain.SoilState, len(forecast))
	for i := range projected {
		projected[i] = domain.SoilUnknown
	}
	if texture != nil {
		if projected, err = in.calculator.ProjectSoilStates(*texture, rain, forecast); err != nil {
			return FieldTrafficabilityView{}, fmt.Errorf("project soil states: %w", err)
		}
	}
	dates := make([]time.Time, len(forecast))
	for i, day := range forecast {
		dates[i] = day.Date
	}
	view.Plan = domain.PlanTrafficability(in.today, soil.State, dates, projected, in.fits)
	view.RainDaily = uc.rainDaily(in, rain, forecast)
	return view, nil
}

// rainSource picks the rain of the field: a gauge with a recent reading
// decides alone; otherwise the weather model, when it answered.
func (uc *TrafficabilityEvaluator) rainSource(in trafficabilityInputs) (*domain.RainAccumulation, *string) {
	if gauge := in.field.Gauge; gauge != nil {
		if rain := domain.GaugeRainAccumulation(in.now, *gauge, in.readings, uc.deps.Settings.GaugeMaxStaleness); rain != nil {
			name := gauge.Name
			return rain, &name
		}
	}
	if in.weather == nil {
		return nil, nil
	}
	return domain.ModelRainAccumulation(in.now, in.weather.Hours), nil
}

func (uc *TrafficabilityEvaluator) soilStatus(in trafficabilityInputs, rain *domain.RainAccumulation, sourceName *string) (SoilStatusView, error) {
	status := SoilStatusView{
		State: domain.SoilUnknown, Basis: domain.BasisNone, Texture: in.field.Texture,
		ThresholdsSource: thresholdsSource(in.thresholds, in.field.Texture), Rain: rain, RainSourceName: sourceName,
	}
	var inspection *domain.SoilInspection
	if stored := in.inspection; stored != nil {
		assessment := stored.Inspection.Assessment()
		if in.calculator.InspectionValid(in.now, assessment) {
			inspection = &assessment
			status.Inspection = &SoilInspectionView{
				Inspection: stored.Inspection, InspectedByName: stored.InspectedByName,
				ValidUntil: in.calculator.InspectionValidUntil(assessment),
			}
		}
	}
	texture := in.field.Texture
	if texture == nil {
		// Without texture there are no thresholds to read the rain with, but a
		// person who saw the soil still says how it is: the inspection stands.
		if inspection != nil {
			status.State, status.Basis = inspection.State, domain.BasisInspection
		}
		return status, nil
	}
	thresholds := in.thresholds[*texture]
	if in.weather != nil {
		if moisture := domain.ModelMoistureAt(in.now, in.weather.Hours); moisture != nil {
			status.ModelMoisture = &domain.ModelSoilMoisture{VolumetricM3M3: *moisture, FieldCapacityM3M3: thresholds.FieldCapacityM3M3}
		}
	}
	assessment, err := in.calculator.Assess(in.now, domain.SoilConditions{
		Texture: *texture, Rain: rain, ModelMoisture: status.ModelMoisture, Inspection: inspection,
	})
	if err != nil {
		return SoilStatusView{}, fmt.Errorf("assess soil: %w", err)
	}
	status.State, status.Basis = assessment.State, assessment.Basis
	return status, nil
}

// thresholdsSource is the source of the thresholds of the texture; without a
// texture it is heuristic as long as any texture still is.
func thresholdsSource(thresholds map[domain.SoilTexture]domain.TrafficabilityThresholds, texture *domain.SoilTexture) string {
	if texture != nil {
		return thresholds[*texture].Source
	}
	for _, t := range thresholds {
		if t.Source == domain.ThresholdSourceHeuristic {
			return domain.ThresholdSourceHeuristic
		}
	}
	return domain.ThresholdSourceCalibrated
}

// forecastDays returns the forecast rain of the days after today, at most
// `days` of them. Without weather there is no projection.
func forecastDays(weather *WeatherReport, today time.Time, days int) []domain.DailyRain {
	if weather == nil {
		return nil
	}
	forecast := make([]domain.DailyRain, 0, days)
	for _, day := range weather.Days {
		if day.Date.After(today) && len(forecast) < days {
			forecast = append(forecast, day)
		}
	}
	return forecast
}

// rainDaily lists the past days (today included) from the same source as the
// soil state, then the forecast days of the model.
func (uc *TrafficabilityEvaluator) rainDaily(in trafficabilityInputs, rain *domain.RainAccumulation, forecast []domain.DailyRain) []DailyRainEntry {
	entries := make([]DailyRainEntry, 0, pastRainDays+len(forecast))
	first := in.today.AddDate(0, 0, -(pastRainDays - 1))
	switch {
	case rain != nil && rain.Source == domain.RainFromGauge:
		days := make([]time.Time, 0, pastRainDays)
		for day := first; !day.After(in.today); day = day.AddDate(0, 0, 1) {
			days = append(days, day)
		}
		for _, day := range domain.DailyGaugeRain(in.readings, days, uc.deps.Settings.Location) {
			entries = append(entries, DailyRainEntry{Date: day.Date, RainMm: day.Mm, Kind: RainMeasured, Source: domain.RainFromGauge})
		}
	case in.weather != nil:
		for _, day := range in.weather.Days {
			if !day.Date.Before(first) && !day.Date.After(in.today) {
				entries = append(entries, DailyRainEntry{Date: day.Date, RainMm: day.Mm, Kind: RainMeasured, Source: domain.RainFromModel})
			}
		}
	}
	for _, day := range forecast {
		entries = append(entries, DailyRainEntry{Date: day.Date, RainMm: day.Mm, Kind: RainForecast, Source: domain.RainFromModel})
	}
	return entries
}
