package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Ports of rain, soil and trafficability (ADR 0022, addendum). As with the
// registry, every method receives the company of the caller and the use case
// checks the farm scope; a gauge or field of another company or farm is
// ErrNotFound. The soil state is never stored: it is derived in the domain on
// every read from the stored data and the thresholds.

// ErrWeatherUnavailable means the weather provider is down, timed out or answered
// something unusable. The use case reports it in data_issues and never
// invents a value (addendum item 8).
var ErrWeatherUnavailable = errors.New("weather provider unavailable")

// ---- settings ----------------------------------------------------------------------

// TrafficabilitySettings are the business values of the soil state that do not
// live in the thresholds table.
type TrafficabilitySettings struct {
	// Location is the time zone of the farms; local days are counted in it
	// (America/Sao_Paulo).
	Location *time.Location
	// GaugeMaxStaleness is how old the latest reading of a gauge may be for it
	// to count as the rain source (API_SOIL_GAUGE_MAX_STALENESS, default 36h).
	GaugeMaxStaleness time.Duration
	// InspectionValidity is how long a soil inspection replaces the calculation
	// (API_SOIL_INSPECTION_VALIDITY, default 12h).
	InspectionValidity time.Duration
}

// Validate rejects settings that would break the use cases.
func (s TrafficabilitySettings) Validate() error {
	switch {
	case s.Location == nil:
		return errors.New("trafficability time zone is required")
	case s.GaugeMaxStaleness <= 0:
		return errors.New("rain gauge staleness must be positive")
	case s.InspectionValidity <= 0:
		return errors.New("soil inspection validity must be positive")
	}
	return nil
}

// ---- reading -----------------------------------------------------------------------

// StoredRainReading is a reading with the name of who recorded it.
type StoredRainReading struct {
	Reading        domain.RainReading
	RecordedByName string
}

// RainGaugeReader reads the rain gauges of a farm and their readings.
type RainGaugeReader interface {
	// ListRainGauges returns the gauges of the farm, by name.
	ListRainGauges(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) ([]domain.RainGauge, error)
	// FindRainGauge returns ErrNotFound when the gauge is not in the farm of the company.
	FindRainGauge(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.RainGaugeID) (domain.RainGauge, error)
	// ListRainReadings returns the readings observed in (after, until], newest first.
	ListRainReadings(ctx context.Context, companyID domain.CompanyID, gaugeID domain.RainGaugeID, after, until time.Time) ([]StoredRainReading, error)
}

// FieldConditionsData is what the store knows about the soil of a field.
type FieldConditionsData struct {
	FieldID domain.FieldID
	FarmID  domain.FarmID
	// Centroid locates the weather model cell of the field.
	Centroid domain.GeoPoint
	Texture  *domain.SoilTexture
	// Gauge is the rain gauge linked to the field, nil without a link.
	Gauge *domain.RainGauge
}

// StoredSoilInspection is an inspection with the name of who made it.
type StoredSoilInspection struct {
	Inspection      domain.FieldSoilInspection
	InspectedByName string
}

// FieldConditionsReader reads the soil data of a field.
type FieldConditionsReader interface {
	// ReadFieldConditions returns ErrNotFound when the field is not in the farm of the company.
	ReadFieldConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (FieldConditionsData, error)
	// LatestSoilInspection returns the most recent inspection of the field, or nil.
	LatestSoilInspection(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID) (*StoredSoilInspection, error)
}

// TrafficabilityThresholdsReader reads the global thresholds table.
type TrafficabilityThresholdsReader interface {
	// CurrentTrafficabilityThresholds returns, per texture, the most recent
	// thresholds effective on the local day `on`.
	CurrentTrafficabilityThresholds(ctx context.Context, on time.Time) (map[domain.SoilTexture]domain.TrafficabilityThresholds, error)
}

// ---- writing -----------------------------------------------------------------------

// FieldConditionsStore is the transactional side of rain gauges, readings,
// inspections and soil profiles.
type FieldConditionsStore interface {
	// LockFarmForConditions holds a row lock on the farm (ErrNotFound when it is
	// not in the company), so two gauges with the same name cannot race.
	LockFarmForConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) error
	// InsertRainGauge returns a validation error on "name" when the farm already has it.
	InsertRainGauge(ctx context.Context, gauge domain.RainGauge) error
	// LockRainGauge reads the gauge and holds a row lock (ErrNotFound when absent).
	LockRainGauge(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.RainGaugeID) (domain.RainGauge, error)
	// UpdateRainGauge uses optimistic concurrency (ErrVersionConflict) and
	// returns a validation error on "name" when the farm already has it.
	UpdateRainGauge(ctx context.Context, gauge domain.RainGauge) error
	// InsertRainReading returns a validation error on "observed_at" when the
	// gauge already has a reading at that instant.
	InsertRainReading(ctx context.Context, reading domain.RainReading) error
	// LockFieldForConditions reads the field (id, company and farm only) and
	// holds a row lock (ErrNotFound when absent).
	LockFieldForConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (domain.Field, error)
	// SaveSoilProfile stores the texture and replaces the gauge link of the field.
	SaveSoilProfile(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID, profile domain.SoilProfile, at time.Time) error
	InsertSoilInspection(ctx context.Context, inspection domain.FieldSoilInspection) error
}

// FieldConditionsStores are the transaction-bound stores of the field conditions.
type FieldConditionsStores struct {
	Conditions FieldConditionsStore
	Audit      AuditRecorder
}

// FieldConditionsTxRunner runs fn in one database transaction: commit when fn
// returns nil, roll back otherwise.
type FieldConditionsTxRunner interface {
	InFieldConditionsTx(ctx context.Context, fn func(ctx context.Context, tx FieldConditionsStores) error) error
}

// ---- weather model -----------------------------------------------------------------

// WeatherReport is what the weather model says about a cell: hourly rain and
// soil moisture of the last 7 days and the forecast, and the daily rain per
// local day (past and forecast).
type WeatherReport struct {
	Hours []domain.HourlyWeather
	Days  []domain.DailyRain
}

// WeatherProvider reads the weather model (Open-Meteo). Any failure wraps
// ErrWeatherUnavailable.
type WeatherProvider interface {
	Weather(ctx context.Context, cell domain.WeatherCell) (WeatherReport, error)
}
