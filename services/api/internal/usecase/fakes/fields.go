package fakes

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// The geometry engine is the database's. The fake measures polygons by their
// bounding boxes, which is exact for the axis-aligned rectangles the tests
// draw: one degree is taken as 111.32 km on both axes.
const squareMetersPerSquareDegree = 111_320.0 * 111_320.0

type box struct{ minLng, minLat, maxLng, maxLat float64 }

func boxOf(polygon domain.Polygon) box {
	b := box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	if len(polygon) == 0 {
		return box{}
	}
	for _, position := range polygon[0] {
		b.minLng, b.maxLng = min(b.minLng, position[0]), max(b.maxLng, position[0])
		b.minLat, b.maxLat = min(b.minLat, position[1]), max(b.maxLat, position[1])
	}
	return b
}

func (b box) areaM2() float64 {
	return max(0, b.maxLng-b.minLng) * max(0, b.maxLat-b.minLat) * squareMetersPerSquareDegree
}

func (b box) intersect(o box) box {
	return box{max(b.minLng, o.minLng), max(b.minLat, o.minLat), min(b.maxLng, o.maxLng), min(b.maxLat, o.maxLat)}
}

// insideM2 is the area of b covered by the polygons of perimeter (which do not overlap).
func (b box) insideM2(perimeter domain.Perimeter) float64 {
	var inside float64
	for _, polygon := range perimeter {
		inside += b.intersect(boxOf(polygon)).areaM2()
	}
	return min(inside, b.areaM2())
}

func roundHa(m2 float64) float64 { return math.Round(m2) / 10000 }

// ---- crops ---------------------------------------------------------------------------

// ListCrops implements usecase.CropReader.
func (r *Registry) ListCrops(context.Context) ([]domain.Crop, error) {
	if err := r.check("ListCrops"); err != nil {
		return nil, err
	}
	crops := slices.Clone(r.Crops)
	slices.SortFunc(crops, func(a, b domain.Crop) int { return cmp.Compare(a.Name, b.Name) })
	return crops, nil
}

// FindCrop implements usecase.CropReader.
func (r *Registry) FindCrop(_ context.Context, code string) (domain.Crop, error) {
	for _, crop := range r.Crops {
		if crop.Code == code {
			return crop, nil
		}
	}
	return domain.Crop{}, fmt.Errorf("find crop %s: %w", code, usecase.ErrNotFound)
}

// ---- fields --------------------------------------------------------------------------

func (r *Registry) fieldsOf(companyID domain.CompanyID, farmID domain.FarmID) []domain.Field {
	var fields []domain.Field
	for _, field := range r.Fields {
		if field.CompanyID == companyID && field.FarmID == farmID {
			fields = append(fields, field)
		}
	}
	slices.SortFunc(fields, func(a, b domain.Field) int {
		return cmp.Or(cmp.Compare(textOf(a.Block), textOf(b.Block)), cmp.Compare(a.Name, b.Name))
	})
	return fields
}

func textOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func fieldListing(field domain.Field) usecase.FieldListing {
	return usecase.FieldListing{Field: field, AreaHa: roundHa(boxOf(field.Polygon).areaM2())}
}

// ListFields implements usecase.FieldReader.
func (r *Registry) ListFields(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID) ([]usecase.FieldListing, error) {
	if err := r.check("ListFields"); err != nil {
		return nil, err
	}
	listings := []usecase.FieldListing{}
	for _, field := range r.fieldsOf(companyID, farmID) {
		listings = append(listings, fieldListing(field))
	}
	return listings, nil
}

// FindField implements usecase.FieldReader.
func (r *Registry) FindField(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.FieldID) (usecase.FieldListing, error) {
	field, ok := r.Fields[id]
	if !ok || field.CompanyID != companyID || field.FarmID != farmID {
		return usecase.FieldListing{}, fmt.Errorf("find field: %w", usecase.ErrNotFound)
	}
	return fieldListing(field), nil
}

// LockField implements usecase.FieldStore.
func (r *Registry) LockField(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.FieldID) (domain.Field, error) {
	listing, err := r.FindField(ctx, companyID, farmID, id)
	if err != nil {
		return domain.Field{}, err
	}
	return listing.Field, nil
}

// FieldFootprint implements usecase.FieldStore with bounding boxes.
func (r *Registry) FieldFootprint(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID, exclude domain.FieldID, polygon domain.Polygon) (domain.FieldFootprint, error) {
	if err := r.check("FieldFootprint"); err != nil {
		return domain.FieldFootprint{}, err
	}
	farm, ok := r.farmOf(companyID, farmID)
	if !ok {
		return domain.FieldFootprint{}, fmt.Errorf("field footprint: %w", usecase.ErrNotFound)
	}
	b := boxOf(polygon)
	footprint := domain.FieldFootprint{Valid: true, AreaHa: roundHa(b.areaM2())}
	if farm.Perimeter != nil {
		footprint.OutsidePerimeterM2 = b.areaM2() - b.insideM2(*farm.Perimeter)
	}
	for _, other := range r.fieldsOf(companyID, farmID) {
		if other.ID == exclude {
			continue
		}
		if overlap := b.intersect(boxOf(other.Polygon)).areaM2(); overlap > 0 {
			footprint.Overlaps = append(footprint.Overlaps, domain.FieldOverlap{FieldID: other.ID, Name: other.Name, AreaM2: overlap})
		}
	}
	return footprint, nil
}

// FarmCoverage implements usecase.FarmStore with bounding boxes.
func (r *Registry) FarmCoverage(_ context.Context, companyID domain.CompanyID, farmID domain.FarmID, perimeter *domain.Perimeter) (domain.FarmCoverage, error) {
	if err := r.check("FarmCoverage"); err != nil {
		return domain.FarmCoverage{}, err
	}
	var coverage domain.FarmCoverage
	for _, field := range r.fieldsOf(companyID, farmID) {
		b := boxOf(field.Polygon)
		coverage.FieldCount++
		coverage.FieldsAreaHa += roundHa(b.areaM2())
		if perimeter != nil {
			coverage.OutsidePerimeterM2 += b.areaM2() - b.insideM2(*perimeter)
		}
	}
	return coverage, nil
}

// InsertField implements usecase.FieldStore (unique name per farm).
func (r *Registry) InsertField(_ context.Context, field domain.Field) error {
	if err := r.check("InsertField"); err != nil {
		return err
	}
	if r.fieldNameTaken(field) {
		return domain.Invalid("name", "já existe um talhão com esse nome na fazenda")
	}
	field.Version = 1
	r.Fields[field.ID] = field
	return nil
}

// UpdateField implements usecase.FieldStore with optimistic concurrency.
func (r *Registry) UpdateField(_ context.Context, field domain.Field) error {
	if err := r.check("UpdateField"); err != nil {
		return err
	}
	stored, ok := r.Fields[field.ID]
	if !ok || stored.CompanyID != field.CompanyID {
		return fmt.Errorf("update field: %w", usecase.ErrNotFound)
	}
	if stored.Version != field.Version {
		return fmt.Errorf("update field: %w", usecase.ErrVersionConflict)
	}
	if r.fieldNameTaken(field) {
		return domain.Invalid("name", "já existe um talhão com esse nome na fazenda")
	}
	field.Version++
	r.Fields[field.ID] = field
	return nil
}

func (r *Registry) fieldNameTaken(field domain.Field) bool {
	for _, other := range r.Fields {
		if other.ID != field.ID && other.FarmID == field.FarmID && other.Name == field.Name {
			return true
		}
	}
	return false
}

// fieldSummary fills the field-derived values of a farm listing.
func (r *Registry) fieldSummary(listing *usecase.FarmListing) {
	listing.Crops = []domain.Crop{}
	seen := map[string]bool{}
	for _, field := range r.fieldsOf(listing.Farm.CompanyID, listing.Farm.ID) {
		listing.FieldCount++
		listing.ProductiveAreaHa += roundHa(boxOf(field.Polygon).areaM2())
		if !seen[field.Crop.Code] {
			seen[field.Crop.Code] = true
			listing.Crops = append(listing.Crops, field.Crop)
		}
	}
	slices.SortFunc(listing.Crops, func(a, b domain.Crop) int { return cmp.Compare(a.Name, b.Name) })
}
