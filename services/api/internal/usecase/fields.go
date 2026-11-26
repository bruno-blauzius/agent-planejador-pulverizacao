package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Fields follow the registry rules (see farms.go): reading needs no
// capability, writing needs registry.write, and a farm outside the scope of a
// planning operator is ErrNotFound.

// ListCrops returns the crop catalog.
type ListCrops struct {
	crops CropReader
}

// NewListCrops builds the use case.
func NewListCrops(crops CropReader) *ListCrops { return &ListCrops{crops: crops} }

// Execute returns the active crops. The catalog is global (no company).
func (uc *ListCrops) Execute(ctx context.Context, _ domain.Principal) ([]domain.Crop, error) {
	crops, err := uc.crops.ListCrops(ctx)
	if err != nil {
		return nil, fmt.Errorf("list crops: %w", err)
	}
	return crops, nil
}

// ListFieldsInput is the request of GET /farms/{farmId}/fields.
type ListFieldsInput struct {
	Actor  domain.Principal
	FarmID domain.FarmID
}

// ListFields returns the fields of a farm the caller sees.
type ListFields struct {
	farms  FarmReader
	fields FieldReader
}

// NewListFields builds the use case.
func NewListFields(farms FarmReader, fields FieldReader) *ListFields {
	return &ListFields{farms: farms, fields: fields}
}

// Execute returns every field of the farm.
func (uc *ListFields) Execute(ctx context.Context, in ListFieldsInput) ([]FieldListing, error) {
	if _, err := findVisibleFarm(ctx, uc.farms, in.Actor, in.FarmID); err != nil {
		return nil, err
	}
	fields, err := uc.fields.ListFields(ctx, in.Actor.CompanyID, in.FarmID)
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	return fields, nil
}

// FieldWriteDeps are the dependencies of CreateField and UpdateField.
type FieldWriteDeps struct {
	Farms    FarmReader
	Fields   FieldReader
	Crops    CropReader
	Measurer PerimeterMeasurer
	Tx       RegistryTxRunner
	Clock    Clock
	IDs      IDGenerator
	Areas    domain.AreaRules
}

// SaveFieldInput is the request of POST /farms/{farmId}/fields (FieldID empty)
// and PUT /farms/{farmId}/fields/{fieldId}.
type SaveFieldInput struct {
	Actor   domain.Principal
	FarmID  domain.FarmID
	FieldID domain.FieldID
	Details domain.FieldDetails
}

// CreateField registers a field in a farm.
type CreateField struct {
	deps FieldWriteDeps
}

// NewCreateField builds the use case.
func NewCreateField(deps FieldWriteDeps) *CreateField { return &CreateField{deps: deps} }

// Execute validates the field and its place in the farm, on a locked farm so
// two fields drawn at the same time cannot overlap.
func (uc *CreateField) Execute(ctx context.Context, in SaveFieldInput) (FieldListing, error) {
	if err := checkFieldWrite(in); err != nil {
		return FieldListing{}, err
	}
	crop, err := findCrop(ctx, uc.deps.Crops, in.Details.CropCode)
	if err != nil {
		return FieldListing{}, err
	}
	now := uc.deps.Clock.Now()
	id := domain.FieldID(uc.deps.IDs.NewID())
	err = uc.deps.Tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		farm, err := tx.Farms.LockFarm(ctx, in.Actor.CompanyID, in.FarmID)
		if err != nil {
			return fmt.Errorf("lock farm: %w", err)
		}
		field, err := domain.NewField(id, farm, in.Details, crop, now)
		if err != nil {
			return err
		}
		if err := placeField(ctx, uc.deps, tx.Fields, farm, field); err != nil {
			return err
		}
		if err := tx.Fields.InsertField(ctx, field); err != nil {
			return fmt.Errorf("insert field: %w", err)
		}
		return recordFieldEvent(ctx, tx.Audit, in.Actor, now, domain.AuditFieldCreated, field)
	})
	if err != nil {
		return FieldListing{}, err
	}
	return uc.deps.Fields.FindField(ctx, in.Actor.CompanyID, in.FarmID, id)
}

// UpdateField replaces the editable data of a field.
type UpdateField struct {
	deps FieldWriteDeps
}

// NewUpdateField builds the use case.
func NewUpdateField(deps FieldWriteDeps) *UpdateField { return &UpdateField{deps: deps} }

// Execute revises the field with the rules of the registration. The contract
// has no version on FieldInput: the farm lock serializes concurrent edits.
func (uc *UpdateField) Execute(ctx context.Context, in SaveFieldInput) (FieldListing, error) {
	if err := checkFieldWrite(in); err != nil {
		return FieldListing{}, err
	}
	crop, err := findCrop(ctx, uc.deps.Crops, in.Details.CropCode)
	if err != nil {
		return FieldListing{}, err
	}
	now := uc.deps.Clock.Now()
	err = uc.deps.Tx.InRegistryTx(ctx, func(ctx context.Context, tx RegistryStores) error {
		farm, err := tx.Farms.LockFarm(ctx, in.Actor.CompanyID, in.FarmID)
		if err != nil {
			return fmt.Errorf("lock farm: %w", err)
		}
		field, err := tx.Fields.LockField(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID)
		if err != nil {
			return fmt.Errorf("lock field: %w", err)
		}
		polygonBefore := field.Polygon
		if err := field.Revise(in.Details, crop, now); err != nil {
			return err
		}
		if err := placeField(ctx, uc.deps, tx.Fields, farm, field); err != nil {
			return err
		}
		if err := tx.Fields.UpdateField(ctx, field); err != nil {
			return fmt.Errorf("update field: %w", err)
		}
		if err := invalidateTerrain(ctx, tx.Terrain, polygonBefore, field, now); err != nil {
			return err
		}
		return recordFieldEvent(ctx, tx.Audit, in.Actor, now, domain.AuditFieldUpdated, field)
	})
	if err != nil {
		return FieldListing{}, err
	}
	return uc.deps.Fields.FindField(ctx, in.Actor.CompanyID, in.FarmID, in.FieldID)
}

// invalidateTerrain marks the relief of the field stale when its polygon
// changed: the measures describe the old shape. Editing other data (name, crop)
// keeps the relief.
func invalidateTerrain(ctx context.Context, terrain TerrainStaleMarker, before domain.Polygon, field domain.Field, now time.Time) error {
	if samePolygon(before, field.Polygon) {
		return nil
	}
	if terrain == nil {
		return errors.New("invalidate terrain: registry transaction has no terrain store")
	}
	if err := terrain.MarkStale(ctx, field.CompanyID, field.ID, now); err != nil {
		return fmt.Errorf("mark terrain stale: %w", err)
	}
	return nil
}

func samePolygon(a, b domain.Polygon) bool {
	return slices.EqualFunc(a, b, func(ringA, ringB domain.Ring) bool { return slices.Equal(ringA, ringB) })
}

func checkFieldWrite(in SaveFieldInput) error {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return err
	}
	if !in.Actor.FarmScope.Allows(in.FarmID) {
		return fmt.Errorf("write field of farm %s: %w", in.FarmID, ErrNotFound)
	}
	return nil
}

// findCrop returns the catalog entry, or an empty crop for an unknown code
// (the domain reports it on crop_code with the other fields).
func findCrop(ctx context.Context, crops CropReader, code string) (domain.Crop, error) {
	if code == "" {
		return domain.Crop{}, nil
	}
	crop, err := crops.FindCrop(ctx, code)
	if errors.Is(err, ErrNotFound) {
		return domain.Crop{}, nil
	}
	if err != nil {
		return domain.Crop{}, fmt.Errorf("find crop: %w", err)
	}
	return crop, nil
}

// placeField enforces the area rules: the farm has a perimeter with area, and
// the field has its own area, inside the perimeter, without overlaps.
func placeField(ctx context.Context, deps FieldWriteDeps, fields FieldStore, farm domain.Farm, field domain.Field) error {
	var farmAreaHa float64
	if farm.Perimeter != nil {
		measure, err := deps.Measurer.MeasurePerimeter(ctx, *farm.Perimeter)
		if err != nil {
			return fmt.Errorf("measure farm perimeter: %w", err)
		}
		farmAreaHa = measure.AreaHa
	}
	if farm.Perimeter == nil || farmAreaHa <= 0 {
		return deps.Areas.CheckField(farm, farmAreaHa, domain.FieldFootprint{})
	}
	footprint, err := fields.FieldFootprint(ctx, farm.CompanyID, farm.ID, field.ID, field.Polygon)
	if err != nil {
		return fmt.Errorf("measure field: %w", err)
	}
	return deps.Areas.CheckField(farm, farmAreaHa, footprint)
}

func recordFieldEvent(ctx context.Context, audit AuditRecorder, actor domain.Principal, now time.Time, action string, field domain.Field) error {
	event := actorEvent(actor, now, field.CompanyID, action, domain.AuditEntityField, string(field.ID),
		map[string]string{"farm_id": string(field.FarmID), "crop": field.Crop.Code})
	if err := audit.Record(ctx, event); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	return nil
}
