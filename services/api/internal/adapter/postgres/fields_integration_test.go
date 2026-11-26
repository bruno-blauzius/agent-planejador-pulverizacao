//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// fieldRect is a rectangle inside square100ha, in thousandths of a degree
// from its south-west corner (about 111 m each).
func fieldRect(west, south, east, north float64) domain.Polygon {
	x0, y0, x1, y1 := -47+west/1000, -21+south/1000, -47+east/1000, -21+north/1000
	return domain.Polygon{{{x0, y0}, {x0, y1}, {x1, y1}, {x1, y0}, {x0, y0}}}
}

func fieldValue(farm domain.Farm, name string, polygon domain.Polygon) domain.Field {
	now := time.Now().UTC().Truncate(time.Microsecond)
	row, plant := 3.5, 0.7
	block := "Área Norte"
	return domain.Field{
		ID: domain.FieldID(newID()), CompanyID: farm.CompanyID, FarmID: farm.ID, Name: name, Block: &block,
		Crop: domain.Crop{Code: "coffee_arabica"}, RowSpacingM: &row, PlantSpacingM: &plant,
		PlantingDate: &now, Polygon: polygon, CreatedAt: now, UpdatedAt: now,
	}
}

func insertField(t *testing.T, field domain.Field) {
	t.Helper()
	must(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Fields.InsertField(ctx, field)
	}))
}

func TestCrops(t *testing.T) {
	crops, err := testDB.ListCrops(ctx())
	must(t, err)
	if len(crops) != 6 || crops[0].Name != "Café arábica" {
		t.Fatalf("crops = %+v", crops)
	}
	crop, err := testDB.FindCrop(ctx(), "soybean")
	must(t, err)
	if crop.Name != "Soja" {
		t.Fatalf("crop = %+v", crop)
	}
	if _, err := testDB.FindCrop(ctx(), "banana"); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("unknown crop: %v", err)
	}
}

func TestFieldRoundTripAndFarmSummary(t *testing.T) {
	company := newCompany(t)
	farm := farmValue(company, "Santa Rita")
	insertFarm(t, farm)
	field := fieldValue(farm, "Talhão 1", fieldRect(1, 1, 3, 3))
	insertField(t, field)

	listing, err := testDB.FindField(ctx(), company.ID, farm.ID, field.ID)
	must(t, err)
	got := listing.Field
	if got.Name != "Talhão 1" || got.Crop.Name != "Café arábica" || got.Version != 1 ||
		got.RowSpacingM == nil || *got.RowSpacingM != 3.5 || got.PlantingDate == nil || len(got.Polygon) != 1 {
		t.Fatalf("stored field differs: %+v", got)
	}
	if listing.AreaHa < 4.5 || listing.AreaHa > 5.5 {
		t.Fatalf("area = %v ha, want ~4.8", listing.AreaHa)
	}

	summary, err := testDB.FindFarm(ctx(), company.ID, farm.ID)
	must(t, err)
	if summary.FieldCount != 1 || summary.ProductiveAreaHa != listing.AreaHa || len(summary.Crops) != 1 {
		t.Fatalf("farm summary = %d fields, %v ha, %+v", summary.FieldCount, summary.ProductiveAreaHa, summary.Crops)
	}

	other := newCompany(t)
	if _, err := testDB.FindField(ctx(), other.ID, farm.ID, field.ID); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("field visible to another company: %v", err)
	}
	fields, err := testDB.ListFields(ctx(), other.ID, farm.ID)
	must(t, err)
	if len(fields) != 0 {
		t.Fatalf("another company lists %d fields", len(fields))
	}
}

func TestFieldFootprintAndCoverage(t *testing.T) {
	company := newCompany(t)
	farm := farmValue(company, "Boa Vista")
	insertFarm(t, farm)
	first := fieldValue(farm, "Talhão 1", fieldRect(1, 1, 3, 3))
	insertField(t, first)

	must(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		inside, err := tx.Fields.FieldFootprint(ctx, company.ID, farm.ID, domain.FieldID(newID()), fieldRect(4, 1, 6, 3))
		if err != nil {
			return err
		}
		if !inside.Valid || inside.OutsidePerimeterM2 > 1 || len(inside.Overlaps) != 0 {
			t.Errorf("field inside the farm: %+v", inside)
		}
		crossing, err := tx.Fields.FieldFootprint(ctx, company.ID, farm.ID, domain.FieldID(newID()), fieldRect(2, 2, 10, 4))
		if err != nil {
			return err
		}
		if crossing.OutsidePerimeterM2 < 10000 || len(crossing.Overlaps) != 1 || crossing.Overlaps[0].Name != "Talhão 1" {
			t.Errorf("field crossing the border and Talhão 1: %+v", crossing)
		}
		self, err := tx.Fields.FieldFootprint(ctx, company.ID, farm.ID, first.ID, fieldRect(1, 1, 3, 3))
		if err != nil {
			return err
		}
		if len(self.Overlaps) != 0 {
			t.Errorf("a field overlaps itself: %+v", self)
		}
		bowtie := domain.Polygon{{{-47, -21}, {-46.998, -20.998}, {-46.998, -21}, {-47, -20.998}, {-47, -21}}}
		invalid, err := tx.Fields.FieldFootprint(ctx, company.ID, farm.ID, domain.FieldID(newID()), bowtie)
		if err != nil {
			return err
		}
		if invalid.Valid {
			t.Errorf("bowtie must be invalid: %+v", invalid)
		}

		shrunk := domain.Perimeter{fieldRect(4, 4, 9, 9)}
		coverage, err := tx.Farms.FarmCoverage(ctx, company.ID, farm.ID, &shrunk)
		if err != nil {
			return err
		}
		if coverage.FieldCount != 1 || coverage.OutsidePerimeterM2 < 40000 {
			t.Errorf("coverage of a shrunk perimeter: %+v", coverage)
		}
		none, err := tx.Farms.FarmCoverage(ctx, company.ID, farm.ID, nil)
		if err != nil {
			return err
		}
		if none.FieldCount != 1 || none.FieldsAreaHa <= 0 || none.OutsidePerimeterM2 != 0 {
			t.Errorf("coverage without perimeter: %+v", none)
		}
		return nil
	}))
}

func TestFieldUpdateAndConstraints(t *testing.T) {
	company := newCompany(t)
	farm := farmValue(company, "Recanto")
	insertFarm(t, farm)
	field := fieldValue(farm, "Talhão 1", fieldRect(1, 1, 3, 3))
	insertField(t, field)
	insertField(t, fieldValue(farm, "Talhão 2", fieldRect(4, 1, 6, 3)))

	duplicate := fieldValue(farm, "Talhão 1", fieldRect(7, 1, 8, 2))
	err := testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Fields.InsertField(ctx, duplicate)
	})
	var verr *domain.ValidationError
	if !errors.As(err, &verr) || verr.Fields[0].Field != "name" {
		t.Fatalf("duplicate name: %v", err)
	}

	must(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		locked, err := tx.Fields.LockField(ctx, company.ID, farm.ID, field.ID)
		if err != nil {
			return err
		}
		locked.Name, locked.Crop, locked.RowSpacingM = "Talhão 1A", domain.Crop{Code: "soybean"}, nil
		return tx.Fields.UpdateField(ctx, locked)
	}))
	updated, err := testDB.FindField(ctx(), company.ID, farm.ID, field.ID)
	must(t, err)
	if updated.Field.Name != "Talhão 1A" || updated.Field.Crop.Name != "Soja" || updated.Field.Version != 2 || updated.Field.RowSpacingM != nil {
		t.Fatalf("updated field: %+v", updated.Field)
	}

	stale := field // version 1, already bumped to 2
	err = testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Fields.UpdateField(ctx, stale)
	})
	if !errors.Is(err, usecase.ErrVersionConflict) {
		t.Fatalf("stale update: %v", err)
	}

	other := newCompany(t)
	foreign := fieldValue(farm, "Intruso", fieldRect(7, 7, 8, 8))
	foreign.CompanyID = other.ID
	err = testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Fields.InsertField(ctx, foreign)
	})
	if err == nil {
		t.Fatal("a field of another company was stored in the farm")
	}
}
