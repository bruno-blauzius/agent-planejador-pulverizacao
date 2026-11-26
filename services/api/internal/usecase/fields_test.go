package usecase_test

import (
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// The perimeter of activeFarm is the square lng -47..-46.99, lat -21..-20.99
// (about 123 ha). rect draws an axis-aligned field inside it, in thousandths
// of a degree from its south-west corner (one thousandth is about 111 m).
func rect(west, south, east, north float64) domain.Polygon {
	x0, y0, x1, y1 := -47+west/1000, -21+south/1000, -47+east/1000, -21+north/1000
	return domain.Polygon{{{x0, y0}, {x0, y1}, {x1, y1}, {x1, y0}, {x0, y0}}}
}

func fieldIn(name string, polygon domain.Polygon) domain.FieldDetails {
	return domain.FieldDetails{Name: name, CropCode: "coffee_arabica", Polygon: polygon}
}

// withPerimeter gives farm-a1 the perimeter of activeFarm.
func (r *registryEnv) withPerimeter(id domain.FarmID) {
	farm := r.reg.Farms[id]
	farm.Perimeter = activeFarm("x").Perimeter
	r.reg.Farms[id] = farm
}

func (r *registryEnv) fieldDeps() usecase.FieldWriteDeps {
	return usecase.FieldWriteDeps{
		Farms: r.reg, Fields: r.reg, Crops: r.reg, Measurer: r.reg, Tx: r.reg,
		Clock: r.clock, IDs: r.ids, Areas: r.settings.Areas,
	}
}

func (r *registryEnv) createField(t *testing.T, farm domain.FarmID, details domain.FieldDetails) usecase.FieldListing {
	t.Helper()
	listing, err := usecase.NewCreateField(r.fieldDeps()).Execute(ctx(), usecase.SaveFieldInput{
		Actor: r.asAdminA(), FarmID: farm, Details: details,
	})
	assertNoError(t, err)
	return listing
}

func TestListCrops(t *testing.T) {
	r := newRegistryEnv(t)
	crops, err := usecase.NewListCrops(r.reg).Execute(ctx(), r.asOperatorA())
	assertNoError(t, err)
	names := make([]string, 0, len(crops))
	for _, crop := range crops {
		names = append(names, crop.Name)
	}
	if want := []string{"Café arábica", "Milho", "Soja"}; !slices.Equal(names, want) {
		t.Fatalf("crops = %v, want %v", names, want)
	}
}

func TestCreateField(t *testing.T) {
	r := newRegistryEnv(t)
	r.withPerimeter("farm-a1")
	details := fieldIn("Talhão 1", rect(1, 1, 3, 3))
	details.RowSpacingM, details.PlantSpacingM = percent(3.5), percent(0.7)

	listing := r.createField(t, "farm-a1", details)
	field := listing.Field
	if field.FarmID != "farm-a1" || field.CompanyID != r.companyA.ID || field.Crop.Name != "Café arábica" || field.Version != 1 {
		t.Fatalf("unexpected field: %+v", field)
	}
	if listing.AreaHa < 4.9 || listing.AreaHa > 5 {
		t.Fatalf("area = %v ha, want about 4.96", listing.AreaHa)
	}
	if stand := field.PlantsPerHa(); stand == nil || *stand != 4082 {
		t.Fatalf("stand = %v", stand)
	}
	r.assertRegistryAudit(t, domain.AuditFieldCreated)

	farm, err := usecase.NewGetFarm(r.reg).Execute(ctx(), usecase.GetFarmInput{Actor: r.asAdminA(), FarmID: "farm-a1"})
	assertNoError(t, err)
	if farm.FieldCount != 1 || farm.ProductiveAreaHa != listing.AreaHa || len(farm.Crops) != 1 {
		t.Fatalf("farm summary = %d fields, %v ha, %v", farm.FieldCount, farm.ProductiveAreaHa, farm.Crops)
	}
}

func TestCreateFieldRefusals(t *testing.T) {
	tests := []struct {
		name    string
		actor   func(*registryEnv) domain.Principal
		farm    domain.FarmID
		details domain.FieldDetails
		wantErr error
		field   string
	}{
		{"operator cannot write", (*registryEnv).asOperatorA, "farm-a1", fieldIn("T", rect(5, 5, 6, 6)), usecase.ErrForbidden, ""},
		{"farm of another company", (*registryEnv).asAdminB, "farm-a1", fieldIn("T", rect(5, 5, 6, 6)), usecase.ErrNotFound, ""},
		{"unknown farm", (*registryEnv).asAdminA, "ghost", fieldIn("T", rect(5, 5, 6, 6)), usecase.ErrNotFound, ""},
		{"farm without perimeter", (*registryEnv).asAdminA, "farm-a2", fieldIn("T", rect(5, 5, 6, 6)), nil, "polygon"},
		{"unknown crop", (*registryEnv).asAdminA, "farm-a1", domain.FieldDetails{Name: "T", CropCode: "banana", Polygon: rect(5, 5, 6, 6)}, nil, "crop_code"},
		{"too small", (*registryEnv).asAdminA, "farm-a1", fieldIn("T", rect(5, 5, 5.05, 5.05)), nil, "polygon"},
		{"outside the perimeter", (*registryEnv).asAdminA, "farm-a1", fieldIn("T", rect(9, 9, 11, 11)), nil, "polygon"},
		{"overlaps a field", (*registryEnv).asAdminA, "farm-a1", fieldIn("T", rect(2, 2, 4, 4)), nil, "polygon"},
		{"name taken in the farm", (*registryEnv).asAdminA, "farm-a1", fieldIn("Talhão 1", rect(5, 5, 6, 6)), nil, "name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistryEnv(t)
			r.withPerimeter("farm-a1")
			r.createField(t, "farm-a1", fieldIn("Talhão 1", rect(1, 1, 3, 3)))
			before, audit := len(r.reg.Fields), len(r.reg.Audit)

			_, err := usecase.NewCreateField(r.fieldDeps()).Execute(ctx(), usecase.SaveFieldInput{
				Actor: tt.actor(r), FarmID: tt.farm, Details: tt.details,
			})
			if tt.wantErr != nil {
				assertIs(t, err, tt.wantErr)
			} else {
				assertValidationOn(t, err, tt.field)
			}
			if len(r.reg.Fields) != before || len(r.reg.Audit) != audit {
				t.Fatal("a refused field must store nothing")
			}
		})
	}
}

func TestCreateFieldOutsideOperatorScope(t *testing.T) {
	r := newRegistryEnv(t)
	r.withPerimeter("farm-a1")
	writer := r.operatorOnly(t, "farm-a2")
	writer.Permissions = r.asAdminA().Permissions
	_, err := usecase.NewCreateField(r.fieldDeps()).Execute(ctx(), usecase.SaveFieldInput{
		Actor: writer, FarmID: "farm-a1", Details: fieldIn("T", rect(1, 1, 2, 2)),
	})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestUpdateField(t *testing.T) {
	r := newRegistryEnv(t)
	r.withPerimeter("farm-a1")
	first := r.createField(t, "farm-a1", fieldIn("Talhão 1", rect(1, 1, 3, 3)))
	r.createField(t, "farm-a1", fieldIn("Talhão 2", rect(4, 1, 6, 3)))
	update := usecase.NewUpdateField(r.fieldDeps())
	r.clock.T = r.clock.T.Add(time.Hour)

	// Redrawing a field over its own old polygon is not an overlap.
	details := fieldIn("Talhão 1A", rect(1, 1, 3.5, 3.5))
	details.CropCode = "soybean"
	listing, err := update.Execute(ctx(), usecase.SaveFieldInput{Actor: r.asAdminA(), FarmID: "farm-a1", FieldID: first.Field.ID, Details: details})
	assertNoError(t, err)
	if listing.Field.Name != "Talhão 1A" || listing.Field.Crop.Code != "soybean" || listing.Field.Version != 2 || !listing.Field.UpdatedAt.Equal(r.clock.T) {
		t.Fatalf("unexpected field: %+v", listing.Field)
	}
	r.assertRegistryAudit(t, domain.AuditFieldCreated, domain.AuditFieldCreated, domain.AuditFieldUpdated)

	_, err = update.Execute(ctx(), usecase.SaveFieldInput{
		Actor: r.asAdminA(), FarmID: "farm-a1", FieldID: first.Field.ID, Details: fieldIn("Talhão 1A", rect(1, 1, 5, 3)),
	})
	assertValidationOn(t, err, "polygon")

	_, err = update.Execute(ctx(), usecase.SaveFieldInput{
		Actor: r.asAdminA(), FarmID: "farm-a2", FieldID: first.Field.ID, Details: fieldIn("X", rect(1, 1, 2, 2)),
	})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestListFields(t *testing.T) {
	r := newRegistryEnv(t)
	r.withPerimeter("farm-a1")
	r.createField(t, "farm-a1", fieldIn("Talhão 2", rect(4, 1, 6, 3)))
	r.createField(t, "farm-a1", fieldIn("Talhão 1", rect(1, 1, 3, 3)))
	list := usecase.NewListFields(r.reg, r.reg)

	fields, err := list.Execute(ctx(), usecase.ListFieldsInput{Actor: r.operatorOnly(t, "farm-a1"), FarmID: "farm-a1"})
	assertNoError(t, err)
	if len(fields) != 2 || fields[0].Field.Name != "Talhão 1" {
		t.Fatalf("fields = %+v", fields)
	}
	_, err = list.Execute(ctx(), usecase.ListFieldsInput{Actor: r.operatorOnly(t, "farm-a2"), FarmID: "farm-a1"})
	assertIs(t, err, usecase.ErrNotFound)
	_, err = list.Execute(ctx(), usecase.ListFieldsInput{Actor: r.asAdminB(), FarmID: "farm-a1"})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestUpdateFarmKeepsItsFields(t *testing.T) {
	r := newRegistryEnv(t)
	r.withPerimeter("farm-a1")
	r.createField(t, "farm-a1", fieldIn("Talhão 1", rect(1, 1, 3, 3)))
	update := usecase.NewUpdateFarm(r.farmDeps())

	_, err := update.Execute(ctx(), usecase.UpdateFarmInput{Actor: r.asAdminA(), FarmID: "farm-a1", Details: draftFarm("Santa Rita")})
	assertValidationOn(t, err, "location.perimeter")

	shrunk := domain.Perimeter{rect(5, 5, 10, 10)}
	details := draftFarm("Santa Rita")
	details.Perimeter = &shrunk
	_, err = update.Execute(ctx(), usecase.UpdateFarmInput{Actor: r.asAdminA(), FarmID: "farm-a1", Details: details})
	assertValidationOn(t, err, "location.perimeter")

	details.Perimeter = activeFarm("x").Perimeter
	_, err = update.Execute(ctx(), usecase.UpdateFarmInput{Actor: r.asAdminA(), FarmID: "farm-a1", Details: details})
	assertNoError(t, err)
}
