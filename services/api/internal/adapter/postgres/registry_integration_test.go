//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// square100ha is ~100 ha near Guaxupé (MG).
func square100ha() *domain.Perimeter {
	ring := domain.Ring{{-47.0, -21.0}, {-47.0, -20.991}, {-46.991, -20.991}, {-46.991, -21.0}, {-47.0, -21.0}}
	perimeter := domain.Perimeter{{ring}}
	return &perimeter
}

func newOwner(t *testing.T, company domain.Company, name string) domain.Owner {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	owner := domain.Owner{
		ID: domain.OwnerID(newID()), CompanyID: company.ID, PersonType: domain.PersonIndividual,
		Name: name, Document: domain.TaxID(randomDigits(11)), CreatedAt: now, UpdatedAt: now,
	}
	must(t, testDB.InsertOwner(ctx(), owner))
	return owner
}

func shareOf(percent float64) *domain.SharePercent {
	share := domain.SharePercent(math.Round(percent * 100))
	return &share
}

func farmValue(company domain.Company, name string, owners ...domain.Ownership) domain.Farm {
	now := time.Now().UTC().Truncate(time.Microsecond)
	id := domain.FarmID(newID())
	for i := range owners {
		owners[i].FarmID = id
	}
	return domain.Farm{
		ID: id, CompanyID: company.ID, Name: name, ProductionType: domain.ProductionCoffee,
		City: "Guaxupé", State: "MG", Status: domain.FarmActive, Perimeter: square100ha(),
		Owners: owners, CreatedAt: now, UpdatedAt: now,
	}
}

func insertFarm(t *testing.T, farm domain.Farm) {
	t.Helper()
	must(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Farms.InsertFarm(ctx, farm)
	}))
}

func TestFarmRoundTrip(t *testing.T) {
	company := newCompany(t)
	helena, marcos := newOwner(t, company, "Helena"), newOwner(t, company, "Marcos")
	farm := farmValue(company, "Santa Rita",
		domain.Ownership{OwnerID: helena.ID, Role: domain.LinkOwner, Share: shareOf(66.67)},
		domain.Ownership{OwnerID: marcos.ID, Role: domain.LinkOwner, Share: shareOf(33.33)},
	)
	code := "STR-01"
	farm.Code = &code
	insertFarm(t, farm)

	listing, err := testDB.FindFarm(ctx(), company.ID, farm.ID)
	must(t, err)
	got := listing.Farm
	if got.Name != "Santa Rita" || got.Code == nil || *got.Code != code || got.Version != 1 {
		t.Fatalf("stored farm differs: %+v", got)
	}
	if listing.AreaHa < 90 || listing.AreaHa > 110 {
		t.Fatalf("area = %v ha, want ~100", listing.AreaHa)
	}
	if got.ForecastPoint == nil || math.Abs(got.ForecastPoint.Lat-(-20.9955)) > 1e-4 || math.Abs(got.ForecastPoint.Lng-(-46.9955)) > 1e-4 {
		t.Fatalf("forecast point must default to the centroid, got %+v", got.ForecastPoint)
	}
	if got.Perimeter == nil || len(*got.Perimeter) != 1 || listing.Outline == nil {
		t.Fatalf("perimeter/outline missing: %+v / %+v", got.Perimeter, listing.Outline)
	}
	if len(got.Owners) != 2 || domain.TotalShare(got.Owners) != domain.FullOwnership {
		t.Fatalf("owners = %+v, want two summing 100%%", got.Owners)
	}
	if listing.OwnerNames[helena.ID] != "Helena" {
		t.Fatalf("owner names = %v", listing.OwnerNames)
	}
}

func TestFarmRejectsInvalidPerimeter(t *testing.T) {
	company := newCompany(t)
	farm := farmValue(company, "Gravata")
	bowtie := domain.Perimeter{{domain.Ring{{0, 0}, {1, 1}, {1, 0}, {0, 1}, {0, 0}}}}
	farm.Perimeter = &bowtie
	err := testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Farms.InsertFarm(ctx, farm)
	})
	var verr *domain.ValidationError
	if !errors.As(err, &verr) || verr.Fields[0].Field != "location.perimeter" {
		t.Fatalf("error = %v, want validation on location.perimeter", err)
	}
}

func TestFarmUpdateReplacesLinksWithVersion(t *testing.T) {
	company := newCompany(t)
	helena, marcos := newOwner(t, company, "Helena"), newOwner(t, company, "Marcos")
	farm := farmValue(company, "Boa Vista", domain.Ownership{OwnerID: helena.ID, Role: domain.LinkOwner, Share: shareOf(100)})
	insertFarm(t, farm)

	var locked domain.Farm
	must(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		var err error
		locked, err = tx.Farms.LockFarm(ctx, company.ID, farm.ID)
		if err != nil {
			return err
		}
		locked.Name = "Boa Vista II"
		locked.Owners = []domain.Ownership{
			{FarmID: farm.ID, OwnerID: marcos.ID, Role: domain.LinkOwner, Share: shareOf(100)},
			{FarmID: farm.ID, OwnerID: helena.ID, Role: domain.LinkTenant},
		}
		return tx.Farms.UpdateFarm(ctx, locked)
	}))
	listing, err := testDB.FindFarm(ctx(), company.ID, farm.ID)
	must(t, err)
	if listing.Farm.Name != "Boa Vista II" || listing.Farm.Version != 2 || len(listing.Farm.Owners) != 2 {
		t.Fatalf("unexpected farm after update: %+v", listing.Farm)
	}

	stale := locked // still version 1
	requireIs(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Farms.UpdateFarm(ctx, stale)
	}), usecase.ErrVersionConflict)
}

func TestRegistryTenantIsolation(t *testing.T) {
	companyA, companyB := newCompany(t), newCompany(t)
	ownerA, ownerB := newOwner(t, companyA, "Dono A"), newOwner(t, companyB, "Dono B")
	farmA := farmValue(companyA, "Fazenda A", domain.Ownership{OwnerID: ownerA.ID, Role: domain.LinkOwner, Share: shareOf(100)})
	insertFarm(t, farmA)

	_, err := testDB.FindFarm(ctx(), companyB.ID, farmA.ID)
	requireIs(t, err, usecase.ErrNotFound)
	_, err = testDB.FindOwner(ctx(), companyB.ID, ownerA.ID, domain.AllFarms())
	requireIs(t, err, usecase.ErrNotFound)

	page, err := testDB.ListFarms(ctx(), companyB.ID, usecase.FarmFilter{}, firstPage(50))
	must(t, err)
	if page.Total != 0 {
		t.Fatalf("company B sees %d farms of A", page.Total)
	}
	missing, err := testDB.MissingOwners(ctx(), companyA.ID, []domain.OwnerID{ownerA.ID, ownerB.ID, "not-a-uuid"})
	must(t, err)
	if len(missing) != 2 {
		t.Fatalf("missing = %v, want owner B and the invalid id", missing)
	}
	// The composite FK refuses a link across companies even if a use case forgot to check.
	crossed := farmValue(companyA, "Cruzada", domain.Ownership{OwnerID: ownerB.ID, Role: domain.LinkTenant})
	if err := testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		return tx.Farms.InsertFarm(ctx, crossed)
	}); err == nil {
		t.Fatal("a link to an owner of another company must fail")
	}
	missingFarms, err := testDB.LockFarms(ctx(), companyB.ID, []domain.FarmID{farmA.ID})
	must(t, err)
	if len(missingFarms) != 1 {
		t.Fatalf("farm of A must be missing for B, got %v", missingFarms)
	}
}

func TestOwnersDocumentSearchAndVisibility(t *testing.T) {
	company, other := newCompany(t), newCompany(t)
	helena := newOwner(t, company, "Helena Prado")
	marcos := newOwner(t, company, "Marcos Prado")
	farm := farmValue(company, "Santa Rita", domain.Ownership{OwnerID: helena.ID, Role: domain.LinkOwner, Share: shareOf(100)})
	insertFarm(t, farm)

	dup := helena
	dup.ID = domain.OwnerID(newID())
	requireIs(t, testDB.InsertOwner(ctx(), dup), usecase.ErrOwnerDocumentTaken)
	dup.CompanyID = other.ID
	must(t, testDB.InsertOwner(ctx(), dup)) // the same CPF in another company

	doc := string(marcos.Document)
	masked := doc[:3] + "." + doc[3:6]
	page, err := testDB.ListOwners(ctx(), company.ID, usecase.OwnerFilter{Search: masked}, firstPage(50))
	must(t, err)
	if page.Total != 1 || page.Items[0].Owner.ID != marcos.ID {
		t.Fatalf("search by masked document = %+v", page.Items)
	}

	visible := selected(t, farm.ID)
	page, err = testDB.ListOwners(ctx(), company.ID, usecase.OwnerFilter{Visible: visible}, firstPage(50))
	must(t, err)
	if page.Total != 1 || page.Items[0].Owner.ID != helena.ID || len(page.Items[0].Links) != 1 || page.Items[0].Links[0].FarmName != "Santa Rita" {
		t.Fatalf("visible owners = %+v", page.Items)
	}
}

func TestOwnerUpdateAndLinks(t *testing.T) {
	company := newCompany(t)
	owner := newOwner(t, company, "Helena")
	farm1 := farmValue(company, "Um")
	farm2 := farmValue(company, "Dois")
	insertFarm(t, farm1)
	insertFarm(t, farm2)

	birth := time.Date(1979, 3, 12, 0, 0, 0, 0, time.UTC)
	phone, email := domain.Phone("35998124470"), domain.Email("helena@example.com")
	owner.Name, owner.BirthDate, owner.Phone, owner.Email, owner.WindowAlertsEnabled = "Helena P.", &birth, &phone, &email, true
	owner.Version = 1
	must(t, testDB.UpdateOwner(ctx(), owner))
	requireIs(t, testDB.UpdateOwner(ctx(), owner), usecase.ErrVersionConflict)

	must(t, testDB.InRegistryTx(ctx(), func(ctx context.Context, tx usecase.RegistryStores) error {
		if _, err := tx.Ownerships.LockFarms(ctx, company.ID, []domain.FarmID{farm1.ID, farm2.ID}); err != nil {
			return err
		}
		return tx.Ownerships.ReplaceOwnerLinks(ctx, company.ID, owner.ID, []domain.Ownership{
			{FarmID: farm1.ID, OwnerID: owner.ID, Role: domain.LinkOwner, Share: shareOf(12.5)},
			{FarmID: farm2.ID, OwnerID: owner.ID, Role: domain.LinkLegalRepresentative},
		})
	}))
	listing, err := testDB.FindOwner(ctx(), company.ID, owner.ID, domain.AllFarms())
	must(t, err)
	got := listing.Owner
	if got.Name != "Helena P." || got.BirthDate == nil || !got.BirthDate.Equal(birth) || got.Phone == nil || *got.Phone != phone || got.Version != 2 {
		t.Fatalf("stored owner differs: %+v", got)
	}
	if len(listing.Links) != 2 || listing.Links[1].Share == nil || *listing.Links[1].Share != 1250 {
		// links are ordered by farm name: "Dois" (representative) then "Um" (12.5%)
		t.Fatalf("links = %+v", listing.Links)
	}
}

func TestMeasurePerimeter(t *testing.T) {
	measure, err := testDB.MeasurePerimeter(ctx(), *square100ha())
	must(t, err)
	if !measure.Valid || measure.AreaHa < 90 || measure.AreaHa > 110 {
		t.Fatalf("measure = %+v", measure)
	}
	bowtie := domain.Perimeter{{domain.Ring{{0, 0}, {1, 1}, {1, 0}, {0, 1}, {0, 0}}}}
	measure, err = testDB.MeasurePerimeter(ctx(), bowtie)
	must(t, err)
	if measure.Valid {
		t.Fatal("a self-intersecting polygon must be invalid")
	}
}
