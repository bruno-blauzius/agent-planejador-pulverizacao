package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Rain gauges, readings, soil inspections and soil profiles (ADR 0022,
// addendum). Reading needs no capability; registering gauges and editing the
// soil profile need registry.write; recording readings and inspections needs
// field_conditions.record. A farm outside the scope of a planning operator is
// ErrNotFound, as in the registry.

// Limits of the readings listing (contract of GET .../readings).
const (
	defaultReadingsRange = 7 * 24 * time.Hour
	maxReadingsRange     = 31 * 24 * time.Hour
)

// FieldConditionsDeps are the dependencies of the field conditions use cases.
type FieldConditionsDeps struct {
	Farms    FarmReader
	Gauges   RainGaugeReader
	Fields   FieldConditionsReader
	Tx       FieldConditionsTxRunner
	Clock    Clock
	IDs      IDGenerator
	Settings TrafficabilitySettings
}

// checkConditionsWrite requires the permission and the farm in the scope.
func checkConditionsWrite(actor domain.Principal, permission domain.Permission, farmID domain.FarmID) error {
	if err := requireCan(actor, permission); err != nil {
		return err
	}
	if !actor.FarmScope.Allows(farmID) {
		return fmt.Errorf("write field conditions of farm %s: %w", farmID, ErrNotFound)
	}
	return nil
}

func checkConditionsRead(actor domain.Principal, farmID domain.FarmID) error {
	if !actor.FarmScope.Allows(farmID) {
		return fmt.Errorf("read field conditions of farm %s: %w", farmID, ErrNotFound)
	}
	return nil
}

func recordConditionsEvent(ctx context.Context, audit AuditRecorder, actor domain.Principal, now time.Time, action, entityType, entityID string, metadata map[string]string) error {
	if err := audit.Record(ctx, actorEvent(actor, now, actor.CompanyID, action, entityType, entityID, metadata)); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	return nil
}

func gaugeMetadata(gauge domain.RainGauge) map[string]string {
	return map[string]string{"farm_id": string(gauge.FarmID), "name": gauge.Name, "has_location": strconv.FormatBool(gauge.Location != nil)}
}

// ---- rain gauges -------------------------------------------------------------------

// ListRainGaugesInput is the request of GET /farms/{farmId}/rain-gauges.
type ListRainGaugesInput struct {
	Actor  domain.Principal
	FarmID domain.FarmID
}

// ListRainGauges returns the rain gauges of a farm.
type ListRainGauges struct {
	farms  FarmReader
	gauges RainGaugeReader
}

// NewListRainGauges builds the use case.
func NewListRainGauges(farms FarmReader, gauges RainGaugeReader) *ListRainGauges {
	return &ListRainGauges{farms: farms, gauges: gauges}
}

// Execute returns ErrNotFound for a farm that is missing or out of scope.
func (uc *ListRainGauges) Execute(ctx context.Context, in ListRainGaugesInput) ([]domain.RainGauge, error) {
	if _, err := findVisibleFarm(ctx, uc.farms, in.Actor, in.FarmID); err != nil {
		return nil, err
	}
	gauges, err := uc.gauges.ListRainGauges(ctx, in.Actor.CompanyID, in.FarmID)
	if err != nil {
		return nil, fmt.Errorf("list rain gauges: %w", err)
	}
	return gauges, nil
}

// CreateRainGaugeInput is the request of POST /farms/{farmId}/rain-gauges.
type CreateRainGaugeInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	Details domain.RainGaugeDetails
}

// CreateRainGauge registers a rain gauge in a farm.
type CreateRainGauge struct{ deps FieldConditionsDeps }

// NewCreateRainGauge builds the use case.
func NewCreateRainGauge(deps FieldConditionsDeps) *CreateRainGauge {
	return &CreateRainGauge{deps: deps}
}

// Execute stores the gauge on a locked farm, so the name check cannot race.
func (uc *CreateRainGauge) Execute(ctx context.Context, in CreateRainGaugeInput) (domain.RainGauge, error) {
	if err := checkConditionsWrite(in.Actor, domain.PermRegistryWrite, in.FarmID); err != nil {
		return domain.RainGauge{}, err
	}
	now := uc.deps.Clock.Now()
	gauge, err := domain.NewRainGauge(domain.RainGaugeID(uc.deps.IDs.NewID()), in.Actor.CompanyID, in.FarmID, in.Details, now)
	if err != nil {
		return domain.RainGauge{}, err
	}
	err = uc.deps.Tx.InFieldConditionsTx(ctx, func(ctx context.Context, tx FieldConditionsStores) error {
		if err := tx.Conditions.LockFarmForConditions(ctx, in.Actor.CompanyID, in.FarmID); err != nil {
			return fmt.Errorf("lock farm: %w", err)
		}
		if err := tx.Conditions.InsertRainGauge(ctx, gauge); err != nil {
			return fmt.Errorf("insert rain gauge: %w", err)
		}
		return recordConditionsEvent(ctx, tx.Audit, in.Actor, now, domain.AuditRainGaugeCreated, domain.AuditEntityRainGauge, string(gauge.ID), gaugeMetadata(gauge))
	})
	if err != nil {
		return domain.RainGauge{}, err
	}
	gauge.Version = 1
	return gauge, nil
}

// RainGaugePatch is a partial change of a gauge: nil Name keeps the name;
// LocationSet false keeps the location, true replaces it (nil clears it).
type RainGaugePatch struct {
	Name        *string
	LocationSet bool
	Location    *domain.GeoPoint
}

// UpdateRainGaugeInput is the request of PATCH /farms/{farmId}/rain-gauges/{gaugeId}.
type UpdateRainGaugeInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	GaugeID domain.RainGaugeID
	Patch   RainGaugePatch
}

// UpdateRainGauge changes the name or the location of a gauge.
type UpdateRainGauge struct{ deps FieldConditionsDeps }

// NewUpdateRainGauge builds the use case.
func NewUpdateRainGauge(deps FieldConditionsDeps) *UpdateRainGauge {
	return &UpdateRainGauge{deps: deps}
}

// Execute applies the patch on the locked gauge.
func (uc *UpdateRainGauge) Execute(ctx context.Context, in UpdateRainGaugeInput) (domain.RainGauge, error) {
	if err := checkConditionsWrite(in.Actor, domain.PermRegistryWrite, in.FarmID); err != nil {
		return domain.RainGauge{}, err
	}
	now := uc.deps.Clock.Now()
	var revised domain.RainGauge
	err := uc.deps.Tx.InFieldConditionsTx(ctx, func(ctx context.Context, tx FieldConditionsStores) error {
		current, err := tx.Conditions.LockRainGauge(ctx, in.Actor.CompanyID, in.FarmID, in.GaugeID)
		if err != nil {
			return fmt.Errorf("lock rain gauge: %w", err)
		}
		details := domain.RainGaugeDetails{Name: current.Name, Location: current.Location}
		if in.Patch.Name != nil {
			details.Name = *in.Patch.Name
		}
		if in.Patch.LocationSet {
			details.Location = in.Patch.Location
		}
		revised, err = current.Revise(details, now)
		if err != nil {
			return err
		}
		if err := tx.Conditions.UpdateRainGauge(ctx, revised); err != nil {
			return fmt.Errorf("update rain gauge: %w", err)
		}
		return recordConditionsEvent(ctx, tx.Audit, in.Actor, now, domain.AuditRainGaugeUpdated, domain.AuditEntityRainGauge, string(revised.ID), gaugeMetadata(revised))
	})
	if err != nil {
		return domain.RainGauge{}, err
	}
	revised.Version++
	return revised, nil
}

// ---- readings ----------------------------------------------------------------------

// ListRainReadingsInput is the request of GET .../rain-gauges/{gaugeId}/readings.
// Zero times take the defaults: Until is now and After is 7 days before it.
type ListRainReadingsInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	GaugeID domain.RainGaugeID
	After   time.Time
	Until   time.Time
}

// ListRainReadings returns the readings of a gauge in a bounded interval.
type ListRainReadings struct{ deps FieldConditionsDeps }

// NewListRainReadings builds the use case.
func NewListRainReadings(deps FieldConditionsDeps) *ListRainReadings {
	return &ListRainReadings{deps: deps}
}

// Execute refuses an interval that is reversed or longer than 31 days.
func (uc *ListRainReadings) Execute(ctx context.Context, in ListRainReadingsInput) ([]StoredRainReading, error) {
	if err := checkConditionsRead(in.Actor, in.FarmID); err != nil {
		return nil, err
	}
	until := in.Until
	if until.IsZero() {
		until = uc.deps.Clock.Now()
	}
	after := in.After
	if after.IsZero() {
		after = until.Add(-defaultReadingsRange)
	}
	switch {
	case !after.Before(until):
		return nil, domain.Invalid("from", "deve ser anterior a to")
	case until.Sub(after) > maxReadingsRange:
		return nil, domain.Invalid("from", "o intervalo pode ter no máximo 31 dias")
	}
	gauge, err := uc.deps.Gauges.FindRainGauge(ctx, in.Actor.CompanyID, in.FarmID, in.GaugeID)
	if err != nil {
		return nil, fmt.Errorf("find rain gauge: %w", err)
	}
	readings, err := uc.deps.Gauges.ListRainReadings(ctx, in.Actor.CompanyID, gauge.ID, after, until)
	if err != nil {
		return nil, fmt.Errorf("list rain readings: %w", err)
	}
	return readings, nil
}

// RecordRainReadingInput is the request of POST .../rain-gauges/{gaugeId}/readings.
type RecordRainReadingInput struct {
	Actor      domain.Principal
	FarmID     domain.FarmID
	GaugeID    domain.RainGaugeID
	RainMm     float64
	ObservedAt time.Time
}

// RecordRainReading stores a reading of a gauge. It serves every field linked
// to the gauge (addendum item 3).
type RecordRainReading struct{ deps FieldConditionsDeps }

// NewRecordRainReading builds the use case.
func NewRecordRainReading(deps FieldConditionsDeps) *RecordRainReading {
	return &RecordRainReading{deps: deps}
}

// Execute validates and stores the reading on the locked gauge.
func (uc *RecordRainReading) Execute(ctx context.Context, in RecordRainReadingInput) (StoredRainReading, error) {
	if err := checkConditionsWrite(in.Actor, domain.PermFieldConditionsRecord, in.FarmID); err != nil {
		return StoredRainReading{}, err
	}
	now := uc.deps.Clock.Now()
	var reading domain.RainReading
	err := uc.deps.Tx.InFieldConditionsTx(ctx, func(ctx context.Context, tx FieldConditionsStores) error {
		gauge, err := tx.Conditions.LockRainGauge(ctx, in.Actor.CompanyID, in.FarmID, in.GaugeID)
		if err != nil {
			return fmt.Errorf("lock rain gauge: %w", err)
		}
		reading, err = domain.NewRainReading(domain.RainReadingID(uc.deps.IDs.NewID()), gauge, in.RainMm, in.ObservedAt, in.Actor.UserID, now)
		if err != nil {
			return err
		}
		if err := tx.Conditions.InsertRainReading(ctx, reading); err != nil {
			return fmt.Errorf("insert rain reading: %w", err)
		}
		return recordConditionsEvent(ctx, tx.Audit, in.Actor, now, domain.AuditRainReadingRecorded, domain.AuditEntityRainGauge, string(gauge.ID), map[string]string{
			"farm_id":     string(gauge.FarmID),
			"reading_id":  string(reading.ID),
			"rain_mm":     strconv.FormatFloat(reading.RainMm, 'f', -1, 64),
			"observed_at": reading.ObservedAt.UTC().Format(time.RFC3339),
		})
	})
	if err != nil {
		return StoredRainReading{}, err
	}
	return StoredRainReading{Reading: reading, RecordedByName: in.Actor.Name}, nil
}

// ---- soil inspections --------------------------------------------------------------

// SoilInspectionView is an inspection with its author and the instant it
// stops replacing the calculation.
type SoilInspectionView struct {
	Inspection      domain.FieldSoilInspection
	InspectedByName string
	ValidUntil      time.Time
}

// RecordSoilInspectionInput is the request of POST .../fields/{fieldId}/soil-inspections.
type RecordSoilInspectionInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
	Details domain.SoilInspectionDetails
}

// RecordSoilInspection stores a person's assessment of the soil of a field.
type RecordSoilInspection struct{ deps FieldConditionsDeps }

// NewRecordSoilInspection builds the use case.
func NewRecordSoilInspection(deps FieldConditionsDeps) *RecordSoilInspection {
	return &RecordSoilInspection{deps: deps}
}

// Execute validates and stores the inspection. The audit event keeps the
// state and the time but not the free-text note.
func (uc *RecordSoilInspection) Execute(ctx context.Context, in RecordSoilInspectionInput) (SoilInspectionView, error) {
	if err := checkConditionsWrite(in.Actor, domain.PermFieldConditionsRecord, in.FarmID); err != nil {
		return SoilInspectionView{}, err
	}
	now := uc.deps.Clock.Now()
	var inspection domain.FieldSoilInspection
	err := uc.deps.Tx.InFieldConditionsTx(ctx, func(ctx context.Context, tx FieldConditionsStores) error {
		field, err := tx.Conditions.LockFieldForConditions(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID)
		if err != nil {
			return fmt.Errorf("lock field: %w", err)
		}
		inspection, err = domain.NewFieldSoilInspection(domain.SoilInspectionID(uc.deps.IDs.NewID()), field, in.Details, in.Actor.UserID, now)
		if err != nil {
			return err
		}
		if err := tx.Conditions.InsertSoilInspection(ctx, inspection); err != nil {
			return fmt.Errorf("insert soil inspection: %w", err)
		}
		return recordConditionsEvent(ctx, tx.Audit, in.Actor, now, domain.AuditSoilInspectionRecorded, domain.AuditEntityField, string(field.ID), map[string]string{
			"farm_id":       string(field.FarmID),
			"inspection_id": string(inspection.ID),
			"state":         string(inspection.State),
			"inspected_at":  inspection.InspectedAt.UTC().Format(time.RFC3339),
		})
	})
	if err != nil {
		return SoilInspectionView{}, err
	}
	return SoilInspectionView{
		Inspection: inspection, InspectedByName: in.Actor.Name,
		ValidUntil: inspection.InspectedAt.Add(uc.deps.Settings.InspectionValidity),
	}, nil
}

// ---- soil profile ------------------------------------------------------------------

// SoilProfileView is the soil texture of a field and its rain gauge.
type SoilProfileView struct {
	FieldID domain.FieldID
	FarmID  domain.FarmID
	Texture *domain.SoilTexture
	Gauge   *domain.RainGauge
}

func soilProfileViewOf(data FieldConditionsData) SoilProfileView {
	return SoilProfileView{FieldID: data.FieldID, FarmID: data.FarmID, Texture: data.Texture, Gauge: data.Gauge}
}

// GetSoilProfileInput is the request of GET .../fields/{fieldId}/soil-profile.
type GetSoilProfileInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
}

// GetSoilProfile reads the soil profile of a field.
type GetSoilProfile struct{ fields FieldConditionsReader }

// NewGetSoilProfile builds the use case.
func NewGetSoilProfile(fields FieldConditionsReader) *GetSoilProfile {
	return &GetSoilProfile{fields: fields}
}

// Execute returns ErrNotFound for a field that is missing or out of scope.
func (uc *GetSoilProfile) Execute(ctx context.Context, in GetSoilProfileInput) (SoilProfileView, error) {
	if err := checkConditionsRead(in.Actor, in.FarmID); err != nil {
		return SoilProfileView{}, err
	}
	data, err := uc.fields.ReadFieldConditions(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID)
	if err != nil {
		return SoilProfileView{}, fmt.Errorf("read field conditions: %w", err)
	}
	return soilProfileViewOf(data), nil
}

// UpdateSoilProfileInput is the request of PUT .../fields/{fieldId}/soil-profile.
// Nil values clear the texture or the gauge link.
type UpdateSoilProfileInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
	Texture *domain.SoilTexture
	GaugeID *domain.RainGaugeID
}

// UpdateSoilProfile replaces the texture and the gauge link of a field.
type UpdateSoilProfile struct{ deps FieldConditionsDeps }

// NewUpdateSoilProfile builds the use case.
func NewUpdateSoilProfile(deps FieldConditionsDeps) *UpdateSoilProfile {
	return &UpdateSoilProfile{deps: deps}
}

// Execute stores the profile. A gauge of another farm (or company) is a
// validation error on rain_gauge_id, not a 404: the field exists.
func (uc *UpdateSoilProfile) Execute(ctx context.Context, in UpdateSoilProfileInput) (SoilProfileView, error) {
	if err := checkConditionsWrite(in.Actor, domain.PermRegistryWrite, in.FarmID); err != nil {
		return SoilProfileView{}, err
	}
	profile, err := domain.NewSoilProfile(in.Texture, in.GaugeID)
	if err != nil {
		return SoilProfileView{}, err
	}
	now := uc.deps.Clock.Now()
	err = uc.deps.Tx.InFieldConditionsTx(ctx, func(ctx context.Context, tx FieldConditionsStores) error {
		field, err := tx.Conditions.LockFieldForConditions(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID)
		if err != nil {
			return fmt.Errorf("lock field: %w", err)
		}
		if profile.RainGaugeID != nil {
			_, err := tx.Conditions.LockRainGauge(ctx, in.Actor.CompanyID, in.FarmID, *profile.RainGaugeID)
			if errors.Is(err, ErrNotFound) {
				return domain.Invalid("rain_gauge_id", "pluviômetro não encontrado nesta fazenda")
			}
			if err != nil {
				return fmt.Errorf("lock rain gauge: %w", err)
			}
		}
		if err := tx.Conditions.SaveSoilProfile(ctx, in.Actor.CompanyID, in.FarmID, field.ID, profile, now); err != nil {
			return fmt.Errorf("save soil profile: %w", err)
		}
		metadata := map[string]string{"farm_id": string(field.FarmID), "soil_texture": "", "rain_gauge_id": ""}
		if profile.Texture != nil {
			metadata["soil_texture"] = string(*profile.Texture)
		}
		if profile.RainGaugeID != nil {
			metadata["rain_gauge_id"] = string(*profile.RainGaugeID)
		}
		return recordConditionsEvent(ctx, tx.Audit, in.Actor, now, domain.AuditSoilProfileUpdated, domain.AuditEntityField, string(field.ID), metadata)
	})
	if err != nil {
		return SoilProfileView{}, err
	}
	data, err := uc.deps.Fields.ReadFieldConditions(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID)
	if err != nil {
		return SoilProfileView{}, fmt.Errorf("read field conditions: %w", err)
	}
	return soilProfileViewOf(data), nil
}
