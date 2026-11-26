package fakes

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// FieldConditions is an in-memory store of rain gauges, readings, soil
// inspections and soil profiles. Fields and farms come from the Registry it
// was built with. It keeps the contracts of the Postgres adapter: company and
// farm isolation, unique gauge name per farm (case-insensitive), one reading
// per gauge and instant, optimistic versions and rollback.
type FieldConditions struct {
	reg *Registry
	// UserNames gives the author name of readings and inspections (the id when missing).
	UserNames map[domain.UserID]string
	// Centroids locates the fields; a field without one sits at (-21, -47).
	Centroids map[domain.FieldID]domain.GeoPoint
	// Thresholds is the global thresholds table (defaults: the heuristic values).
	Thresholds map[domain.SoilTexture]domain.TrafficabilityThresholds
	Audit      []domain.AuditEvent

	gauges      map[domain.RainGaugeID]domain.RainGauge
	readings    []domain.RainReading
	inspections []domain.FieldSoilInspection
	textures    map[domain.FieldID]domain.SoilTexture
	links       map[domain.FieldID]domain.RainGaugeID
	failures    map[string]error
}

// NewFieldConditions returns an empty store bound to the registry.
func NewFieldConditions(reg *Registry) *FieldConditions {
	return &FieldConditions{
		reg:        reg,
		UserNames:  map[domain.UserID]string{},
		Centroids:  map[domain.FieldID]domain.GeoPoint{},
		Thresholds: domain.DefaultSoilTrafficabilityRules().Thresholds,
		gauges:     map[domain.RainGaugeID]domain.RainGauge{},
		textures:   map[domain.FieldID]domain.SoilTexture{},
		links:      map[domain.FieldID]domain.RainGaugeID{},
		failures:   map[string]error{},
	}
}

// Fail makes every call of the named operation return err.
func (s *FieldConditions) Fail(operation string, err error) { s.failures[operation] = err }

func (s *FieldConditions) check(operation string) error {
	if err := s.failures[operation]; err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

// AuditActions lists the recorded actions in order.
func (s *FieldConditions) AuditActions() []string {
	actions := make([]string, 0, len(s.Audit))
	for _, event := range s.Audit {
		actions = append(actions, event.Action)
	}
	return actions
}

// Readings returns every stored reading.
func (s *FieldConditions) Readings() []domain.RainReading { return slices.Clone(s.readings) }

// Inspections returns every stored inspection.
func (s *FieldConditions) Inspections() []domain.FieldSoilInspection {
	return slices.Clone(s.inspections)
}

// SeedGauge stores a gauge as it is (version 1 when zero).
func (s *FieldConditions) SeedGauge(gauge domain.RainGauge) {
	if gauge.Version == 0 {
		gauge.Version = 1
	}
	s.gauges[gauge.ID] = gauge
}

// SeedReading stores a reading without validation.
func (s *FieldConditions) SeedReading(reading domain.RainReading) {
	s.readings = append(s.readings, reading)
}

// SeedInspection stores an inspection without validation.
func (s *FieldConditions) SeedInspection(inspection domain.FieldSoilInspection) {
	s.inspections = append(s.inspections, inspection)
}

// SeedProfile sets the texture and the gauge link of a field.
func (s *FieldConditions) SeedProfile(fieldID domain.FieldID, texture *domain.SoilTexture, gaugeID *domain.RainGaugeID) {
	delete(s.textures, fieldID)
	delete(s.links, fieldID)
	if texture != nil {
		s.textures[fieldID] = *texture
	}
	if gaugeID != nil {
		s.links[fieldID] = *gaugeID
	}
}

func (s *FieldConditions) nameOf(id domain.UserID) string {
	if name, ok := s.UserNames[id]; ok {
		return name
	}
	return string(id)
}

// ---- transaction ---------------------------------------------------------------------

// InFieldConditionsTx implements usecase.FieldConditionsTxRunner with rollback on failure.
func (s *FieldConditions) InFieldConditionsTx(ctx context.Context, fn func(ctx context.Context, tx usecase.FieldConditionsStores) error) error {
	gauges, readings, inspections := maps.Clone(s.gauges), slices.Clone(s.readings), slices.Clone(s.inspections)
	textures, links, audit := maps.Clone(s.textures), maps.Clone(s.links), slices.Clone(s.Audit)
	err := fn(ctx, usecase.FieldConditionsStores{Conditions: s, Audit: conditionsAudit{s}})
	if err == nil {
		err = s.check("Commit")
	}
	if err != nil {
		s.gauges, s.readings, s.inspections, s.textures, s.links, s.Audit = gauges, readings, inspections, textures, links, audit
		return err
	}
	return nil
}

type conditionsAudit struct{ s *FieldConditions }

// Record implements usecase.AuditRecorder.
func (a conditionsAudit) Record(_ context.Context, event domain.AuditEvent) error {
	if err := a.s.check("Record"); err != nil {
		return err
	}
	a.s.Audit = append(a.s.Audit, event)
	return nil
}

// ---- reading -------------------------------------------------------------------------

func (s *FieldConditions) gaugeOf(companyID domain.CompanyID, farmID domain.FarmID, id domain.RainGaugeID) (domain.RainGauge, error) {
	gauge, ok := s.gauges[id]
	if !ok || gauge.CompanyID != companyID || gauge.FarmID != farmID {
		return domain.RainGauge{}, fmt.Errorf("find rain gauge: %w", usecase.ErrNotFound)
	}
	return gauge, nil
}

// ListRainGauges implements usecase.RainGaugeReader.
func (s *FieldConditions) ListRainGauges(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID) ([]domain.RainGauge, error) {
	if err := s.check("ListRainGauges"); err != nil {
		return nil, err
	}
	gauges := []domain.RainGauge{}
	for _, gauge := range s.gauges {
		if gauge.CompanyID == companyID && gauge.FarmID == farmID {
			gauges = append(gauges, gauge)
		}
	}
	slices.SortFunc(gauges, func(a, b domain.RainGauge) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return gauges, nil
}

// FindRainGauge implements usecase.RainGaugeReader.
func (s *FieldConditions) FindRainGauge(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.RainGaugeID) (domain.RainGauge, error) {
	return s.gaugeOf(companyID, farmID, id)
}

// ListRainReadings implements usecase.RainGaugeReader.
func (s *FieldConditions) ListRainReadings(_ context.Context, companyID domain.CompanyID, gaugeID domain.RainGaugeID, after, until time.Time) ([]usecase.StoredRainReading, error) {
	if err := s.check("ListRainReadings"); err != nil {
		return nil, err
	}
	stored := []usecase.StoredRainReading{}
	for _, reading := range s.readings {
		if reading.CompanyID == companyID && reading.GaugeID == gaugeID && reading.ObservedAt.After(after) && !reading.ObservedAt.After(until) {
			stored = append(stored, usecase.StoredRainReading{Reading: reading, RecordedByName: s.nameOf(reading.RecordedBy)})
		}
	}
	slices.SortFunc(stored, func(a, b usecase.StoredRainReading) int {
		return b.Reading.ObservedAt.Compare(a.Reading.ObservedAt)
	})
	return stored, nil
}

// ReadFieldConditions implements usecase.FieldConditionsReader.
func (s *FieldConditions) ReadFieldConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (usecase.FieldConditionsData, error) {
	if err := s.check("ReadFieldConditions"); err != nil {
		return usecase.FieldConditionsData{}, err
	}
	listing, err := s.reg.FindField(ctx, companyID, farmID, fieldID)
	if err != nil {
		return usecase.FieldConditionsData{}, err
	}
	centroid, ok := s.Centroids[fieldID]
	if !ok {
		centroid = domain.GeoPoint{Lat: -21, Lng: -47}
	}
	data := usecase.FieldConditionsData{FieldID: fieldID, FarmID: listing.Field.FarmID, Centroid: centroid}
	if texture, ok := s.textures[fieldID]; ok {
		data.Texture = &texture
	}
	if gaugeID, ok := s.links[fieldID]; ok {
		if gauge, err := s.gaugeOf(companyID, farmID, gaugeID); err == nil {
			data.Gauge = &gauge
		}
	}
	return data, nil
}

// LatestSoilInspection implements usecase.FieldConditionsReader.
func (s *FieldConditions) LatestSoilInspection(_ context.Context, companyID domain.CompanyID, fieldID domain.FieldID) (*usecase.StoredSoilInspection, error) {
	if err := s.check("LatestSoilInspection"); err != nil {
		return nil, err
	}
	var latest *usecase.StoredSoilInspection
	for _, inspection := range s.inspections {
		if inspection.CompanyID != companyID || inspection.FieldID != fieldID {
			continue
		}
		if latest == nil || inspection.InspectedAt.After(latest.Inspection.InspectedAt) {
			latest = &usecase.StoredSoilInspection{Inspection: inspection, InspectedByName: s.nameOf(inspection.InspectedBy)}
		}
	}
	return latest, nil
}

// CurrentTrafficabilityThresholds implements usecase.TrafficabilityThresholdsReader.
func (s *FieldConditions) CurrentTrafficabilityThresholds(_ context.Context, _ time.Time) (map[domain.SoilTexture]domain.TrafficabilityThresholds, error) {
	if err := s.check("CurrentTrafficabilityThresholds"); err != nil {
		return nil, err
	}
	return maps.Clone(s.Thresholds), nil
}

// ---- writing -------------------------------------------------------------------------

// LockFarmForConditions implements usecase.FieldConditionsStore.
func (s *FieldConditions) LockFarmForConditions(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID) error {
	if _, ok := s.reg.farmOf(companyID, farmID); !ok {
		return fmt.Errorf("lock farm: %w", usecase.ErrNotFound)
	}
	return nil
}

func (s *FieldConditions) nameTaken(gauge domain.RainGauge) bool {
	for _, other := range s.gauges {
		if other.ID != gauge.ID && other.CompanyID == gauge.CompanyID && other.FarmID == gauge.FarmID && strings.EqualFold(other.Name, gauge.Name) {
			return true
		}
	}
	return false
}

// InsertRainGauge implements usecase.FieldConditionsStore.
func (s *FieldConditions) InsertRainGauge(_ context.Context, gauge domain.RainGauge) error {
	if err := s.check("InsertRainGauge"); err != nil {
		return err
	}
	if s.nameTaken(gauge) {
		return domain.Invalid("name", "já existe um pluviômetro com esse nome na fazenda")
	}
	gauge.Version = 1
	s.gauges[gauge.ID] = gauge
	return nil
}

// LockRainGauge implements usecase.FieldConditionsStore.
func (s *FieldConditions) LockRainGauge(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.RainGaugeID) (domain.RainGauge, error) {
	return s.gaugeOf(companyID, farmID, id)
}

// UpdateRainGauge implements usecase.FieldConditionsStore.
func (s *FieldConditions) UpdateRainGauge(_ context.Context, gauge domain.RainGauge) error {
	if err := s.check("UpdateRainGauge"); err != nil {
		return err
	}
	current, ok := s.gauges[gauge.ID]
	if !ok || current.CompanyID != gauge.CompanyID || current.Version != gauge.Version {
		return fmt.Errorf("update rain gauge: %w", usecase.ErrVersionConflict)
	}
	if s.nameTaken(gauge) {
		return domain.Invalid("name", "já existe um pluviômetro com esse nome na fazenda")
	}
	gauge.Version++
	s.gauges[gauge.ID] = gauge
	return nil
}

// InsertRainReading implements usecase.FieldConditionsStore.
func (s *FieldConditions) InsertRainReading(_ context.Context, reading domain.RainReading) error {
	if err := s.check("InsertRainReading"); err != nil {
		return err
	}
	for _, other := range s.readings {
		if other.GaugeID == reading.GaugeID && other.ObservedAt.Equal(reading.ObservedAt) {
			return domain.Invalid("observed_at", "já existe uma leitura desse pluviômetro nesse horário")
		}
	}
	s.readings = append(s.readings, reading)
	return nil
}

// LockFieldForConditions implements usecase.FieldConditionsStore.
func (s *FieldConditions) LockFieldForConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (domain.Field, error) {
	listing, err := s.reg.FindField(ctx, companyID, farmID, fieldID)
	if err != nil {
		return domain.Field{}, err
	}
	return domain.Field{ID: listing.Field.ID, CompanyID: listing.Field.CompanyID, FarmID: listing.Field.FarmID}, nil
}

// SaveSoilProfile implements usecase.FieldConditionsStore.
func (s *FieldConditions) SaveSoilProfile(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID, profile domain.SoilProfile, _ time.Time) error {
	if err := s.check("SaveSoilProfile"); err != nil {
		return err
	}
	if profile.RainGaugeID != nil {
		if _, err := s.gaugeOf(companyID, farmID, *profile.RainGaugeID); err != nil {
			return fmt.Errorf("link rain gauge: foreign key: %w", err)
		}
	}
	s.SeedProfile(fieldID, profile.Texture, profile.RainGaugeID)
	return nil
}

// InsertSoilInspection implements usecase.FieldConditionsStore.
func (s *FieldConditions) InsertSoilInspection(_ context.Context, inspection domain.FieldSoilInspection) error {
	if err := s.check("InsertSoilInspection"); err != nil {
		return err
	}
	s.inspections = append(s.inspections, inspection)
	return nil
}

// ---- weather -------------------------------------------------------------------------

// Weather is a scripted weather provider: it answers Report, or Err when set,
// and remembers the cells it was asked for.
type Weather struct {
	Report usecase.WeatherReport
	Err    error
	Cells  []domain.WeatherCell
}

// Weather implements usecase.WeatherProvider.
func (w *Weather) Weather(_ context.Context, cell domain.WeatherCell) (usecase.WeatherReport, error) {
	w.Cells = append(w.Cells, cell)
	if w.Err != nil {
		return usecase.WeatherReport{}, w.Err
	}
	return w.Report, nil
}
