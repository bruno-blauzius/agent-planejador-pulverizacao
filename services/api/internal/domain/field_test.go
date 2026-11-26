package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

var coffee = domain.Crop{Code: "coffee_arabica", Name: "Café arábica"}

func fieldPolygon() domain.Polygon {
	return domain.Polygon{{{-47, -21}, {-47, -20.999}, {-46.999, -20.999}, {-46.999, -21}, {-47, -21}}}
}

func fieldDetails() domain.FieldDetails {
	return domain.FieldDetails{Name: "Talhão 1", CropCode: coffee.Code, Polygon: fieldPolygon()}
}

func farmWithPerimeter() domain.Farm {
	return domain.Farm{ID: "farm-1", CompanyID: "company-1", Perimeter: square()}
}

func TestNewField(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		edit       func(*domain.FieldDetails)
		crop       domain.Crop
		wantFields []string
	}{
		{"minimal", func(*domain.FieldDetails) {}, coffee, nil},
		{"complete", func(d *domain.FieldDetails) {
			d.Block, d.Variety, d.Season = "Área Norte", "Catuaí 144", "2026/27"
			d.RowSpacingM, d.PlantSpacingM = ptr(3.5), ptr(0.7)
		}, coffee, nil},
		{"season across the century", func(d *domain.FieldDetails) { d.Season = "2099/00" }, coffee, nil},
		{"blank name", func(d *domain.FieldDetails) { d.Name = "  " }, coffee, []string{"name"}},
		{"crop missing", func(d *domain.FieldDetails) { d.CropCode = "" }, domain.Crop{}, []string{"crop_code"}},
		{"crop unknown", func(d *domain.FieldDetails) { d.CropCode = "banana" }, domain.Crop{}, []string{"crop_code"}},
		{"season format", func(d *domain.FieldDetails) { d.Season = "2026-27" }, coffee, []string{"season"}},
		{"season years apart", func(d *domain.FieldDetails) { d.Season = "2026/28" }, coffee, []string{"season"}},
		{"zero spacing", func(d *domain.FieldDetails) { d.RowSpacingM = ptr(0.0) }, coffee, []string{"row_spacing_m"}},
		{"negative spacing", func(d *domain.FieldDetails) { d.PlantSpacingM = ptr(-1.0) }, coffee, []string{"plant_spacing_m"}},
		{"spacing beyond the column", func(d *domain.FieldDetails) { d.RowSpacingM = ptr(1000.0) }, coffee, []string{"row_spacing_m"}},
		{"no polygon", func(d *domain.FieldDetails) { d.Polygon = nil }, coffee, []string{"polygon"}},
		{"open ring", func(d *domain.FieldDetails) {
			d.Polygon = domain.Polygon{{{-47, -21}, {-47, -20.999}, {-46.999, -20.999}, {-46.999, -21}}}
		}, coffee, []string{"polygon"}},
		{"everything wrong", func(d *domain.FieldDetails) {
			d.Name, d.Season, d.RowSpacingM, d.Polygon = "", "x", ptr(-1.0), nil
		}, coffee, []string{"name", "season", "row_spacing_m", "polygon"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := fieldDetails()
			tt.edit(&details)
			field, err := domain.NewField("field-1", farmWithPerimeter(), details, tt.crop, now)
			if got := fieldsOf(t, err); !slices.Equal(got, tt.wantFields) {
				t.Fatalf("invalid fields = %v, want %v", got, tt.wantFields)
			}
			if err == nil && (field.FarmID != "farm-1" || field.CompanyID != "company-1" || field.CreatedAt != now) {
				t.Errorf("field not tied to the farm: %+v", field)
			}
		})
	}
}

func TestFieldPlantsPerHa(t *testing.T) {
	tests := []struct {
		name        string
		row, plant  *float64
		wantStand   *int
		wantSpacing *float64
	}{
		{"coffee 3.5 x 0.7", ptr(3.5), ptr(0.7), ptr(4082), ptr(3.5)},
		{"rounded spacing", ptr(0.4504), ptr(0.2), ptr(111111), ptr(0.45)},
		{"one spacing missing", ptr(3.5), nil, nil, ptr(3.5)},
		{"no spacing", nil, nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := fieldDetails()
			details.RowSpacingM, details.PlantSpacingM = tt.row, tt.plant
			field, err := domain.NewField("field-1", farmWithPerimeter(), details, coffee, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if got := field.PlantsPerHa(); !equalPtr(got, tt.wantStand) {
				t.Errorf("stand = %v, want %v", deref(got), deref(tt.wantStand))
			}
			if !equalPtr(field.RowSpacingM, tt.wantSpacing) {
				t.Errorf("row spacing = %v, want %v", deref(field.RowSpacingM), deref(tt.wantSpacing))
			}
		})
	}
}

func TestFieldReviseKeepsIdentity(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	field, err := domain.NewField("field-1", farmWithPerimeter(), fieldDetails(), coffee, created)
	if err != nil {
		t.Fatal(err)
	}
	details := fieldDetails()
	details.Name = ""
	if err := field.Revise(details, coffee, created.Add(time.Hour)); err == nil {
		t.Fatal("blank name accepted on revise")
	}
	if field.Name != "Talhão 1" || field.UpdatedAt != created {
		t.Fatalf("failed revise changed the field: %+v", field)
	}
	details.Name = "Talhão 2"
	if err := field.Revise(details, coffee, created.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if field.ID != "field-1" || field.Name != "Talhão 2" || field.CreatedAt != created || field.UpdatedAt != created.Add(time.Hour) {
		t.Fatalf("revise = %+v", field)
	}
}

var rules = domain.AreaRules{MinFarmAreaHa: 0.1, MinFieldAreaHa: 0.01, ToleranceM2: 10}

func TestAreaRulesCheckField(t *testing.T) {
	good := domain.FieldFootprint{Valid: true, AreaHa: 12.5}
	tests := []struct {
		name       string
		farm       domain.Farm
		farmAreaHa float64
		footprint  domain.FieldFootprint
		wantFields []string
	}{
		{"fits", farmWithPerimeter(), 100, good, nil},
		{"farm without perimeter", domain.Farm{ID: "farm-1"}, 0, good, []string{"polygon"}},
		{"farm perimeter without area", farmWithPerimeter(), 0, good, []string{"polygon"}},
		{"invalid polygon", farmWithPerimeter(), 100, domain.FieldFootprint{}, []string{"polygon"}},
		{"smaller than the minimum", farmWithPerimeter(), 100, domain.FieldFootprint{Valid: true, AreaHa: 0.009}, []string{"polygon"}},
		{"exactly the minimum", farmWithPerimeter(), 100, domain.FieldFootprint{Valid: true, AreaHa: 0.01}, nil},
		{"outside within tolerance", farmWithPerimeter(), 100, domain.FieldFootprint{Valid: true, AreaHa: 1, OutsidePerimeterM2: 10}, nil},
		{"outside the perimeter", farmWithPerimeter(), 100, domain.FieldFootprint{Valid: true, AreaHa: 1, OutsidePerimeterM2: 10.5}, []string{"polygon"}},
		{"touching another field", farmWithPerimeter(), 100, domain.FieldFootprint{
			Valid: true, AreaHa: 1, Overlaps: []domain.FieldOverlap{{Name: "Talhão 2", AreaM2: 0.3}},
		}, nil},
		{"overlapping two fields", farmWithPerimeter(), 100, domain.FieldFootprint{
			Valid: true, AreaHa: 1, Overlaps: []domain.FieldOverlap{{Name: "Talhão 2", AreaM2: 400}, {Name: "Talhão 3", AreaM2: 50}},
		}, []string{"polygon", "polygon"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rules.CheckField(tt.farm, tt.farmAreaHa, tt.footprint)
			if got := fieldsOf(t, err); !slices.Equal(got, tt.wantFields) {
				t.Fatalf("invalid fields = %v, want %v (%v)", got, tt.wantFields, err)
			}
		})
	}
}

func TestAreaRulesCheckFarm(t *testing.T) {
	const perimeter = "location.perimeter"
	tests := []struct {
		name         string
		hasPerimeter bool
		areaHa       float64
		coverage     domain.FarmCoverage
		wantFields   []string
	}{
		{"draft without perimeter nor fields", false, 0, domain.FarmCoverage{}, nil},
		{"removing the perimeter of a farm with fields", false, 0, domain.FarmCoverage{FieldCount: 2, FieldsAreaHa: 10}, []string{perimeter}},
		{"perimeter without fields", true, 50, domain.FarmCoverage{}, nil},
		{"perimeter below the minimum", true, 0.05, domain.FarmCoverage{}, []string{perimeter}},
		{"fields fill the farm", true, 50, domain.FarmCoverage{FieldCount: 3, FieldsAreaHa: 50}, nil},
		{"fields larger than the farm", true, 50, domain.FarmCoverage{FieldCount: 3, FieldsAreaHa: 50.01}, []string{perimeter}},
		{"perimeter leaves a field out", true, 50, domain.FarmCoverage{FieldCount: 1, FieldsAreaHa: 5, OutsidePerimeterM2: 2000}, []string{perimeter}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rules.CheckFarm(tt.hasPerimeter, tt.areaHa, tt.coverage)
			if got := fieldsOf(t, err); !slices.Equal(got, tt.wantFields) {
				t.Fatalf("invalid fields = %v, want %v (%v)", got, tt.wantFields, err)
			}
		})
	}
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref[T any](v *T) any {
	if v == nil {
		return nil
	}
	return *v
}
