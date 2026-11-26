package usecase_test

import (
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase/fakes"
)

// registryEnv adds a registry to the small world of env: company A has the
// farms "Santa Rita" and "Boa Vista" and two owners; company B has one farm.
type registryEnv struct {
	*env
	reg *fakes.Registry
}

func newRegistryEnv(t *testing.T) *registryEnv {
	t.Helper()
	r := &registryEnv{env: newEnv(t), reg: fakes.NewRegistry()}
	r.putOwner("own-a1", r.companyA.ID, "Helena Prado", "52998224725")
	r.putOwner("own-a2", r.companyA.ID, "Marcos Prado", "11144477735")
	r.putOwner("own-b1", r.companyB.ID, "Bruna Lima", "52998224725")
	r.putFarm("farm-a1", r.companyA.ID, "Santa Rita", share("own-a1", 60))
	r.putFarm("farm-a2", r.companyA.ID, "Boa Vista")
	r.putFarm("farm-b1", r.companyB.ID, "Boa Esperança", share("own-b1", 100))
	return r
}

func (r *registryEnv) putOwner(id string, companyID domain.CompanyID, name, document string) {
	r.reg.Owners[domain.OwnerID(id)] = domain.Owner{
		ID: domain.OwnerID(id), CompanyID: companyID, PersonType: domain.PersonIndividual,
		Name: name, Document: domain.TaxID(document), Version: 1,
	}
}

func (r *registryEnv) putFarm(id string, companyID domain.CompanyID, name string, owners ...domain.OwnershipInput) {
	details := draftFarm(name)
	details.Owners = owners
	farm, err := domain.NewFarm(domain.FarmID(id), companyID, details, r.clock.T)
	if err != nil {
		panic(err)
	}
	farm.Version = 1
	r.reg.Links = append(r.reg.Links, farm.Owners...)
	r.reg.Farms[farm.ID] = withoutOwners(farm)
}

func (r *registryEnv) farmDeps() usecase.FarmWriteDeps {
	return usecase.FarmWriteDeps{Farms: r.reg, Measurer: r.reg, Tx: r.reg, Clock: r.clock, IDs: r.ids, Areas: r.settings.Areas}
}

func withoutOwners(farm domain.Farm) domain.Farm {
	farm.Owners = nil
	return farm
}

func percent(v float64) *float64 { return &v }

func share(owner domain.OwnerID, value float64) domain.OwnershipInput {
	return domain.OwnershipInput{OwnerID: owner, Role: domain.LinkOwner, SharePercent: percent(value)}
}

func draftFarm(name string) domain.FarmDetails {
	return domain.FarmDetails{Name: name, ProductionType: domain.ProductionCoffee, City: "Guaxupé", State: "MG", Status: domain.FarmDraft}
}

func activeFarm(name string, owners ...domain.OwnershipInput) domain.FarmDetails {
	ring := domain.Ring{{-47, -21}, {-47, -20.99}, {-46.99, -20.99}, {-46.99, -21}, {-47, -21}}
	perimeter := domain.Perimeter{{ring}}
	details := draftFarm(name)
	details.Status, details.Perimeter, details.Owners = domain.FarmActive, &perimeter, owners
	return details
}

// operatorOnly is operator A limited to farm-a1.
func (r *registryEnv) operatorOnly(t *testing.T, farms ...domain.FarmID) domain.Principal {
	t.Helper()
	scope, err := domain.NewSelectedFarms(farms)
	assertNoError(t, err)
	actor := r.asOperatorA()
	actor.FarmScope = scope
	return actor
}

func (r *registryEnv) assertRegistryAudit(t *testing.T, want ...string) {
	t.Helper()
	if got := r.reg.AuditActions(); !slices.Equal(got, want) {
		t.Fatalf("audit = %v, want %v", got, want)
	}
}

func farmNames(page usecase.FarmPage) []string {
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		names = append(names, item.Farm.Name)
	}
	return names
}

func ownerNames(page usecase.OwnerPage) []string {
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		names = append(names, item.Owner.Name)
	}
	return names
}

// ---- farms ---------------------------------------------------------------------------

func TestListFarms(t *testing.T) {
	r := newRegistryEnv(t)
	tests := []struct {
		name  string
		actor domain.Principal
		in    usecase.ListFarmsInput
		want  []string
	}{
		{"admin sees only its company", r.asAdminA(), usecase.ListFarmsInput{}, []string{"Boa Vista", "Santa Rita"}},
		{"other company", r.asAdminB(), usecase.ListFarmsInput{}, []string{"Boa Esperança"}},
		{"super admin sees its own company only", r.asSuper(), usecase.ListFarmsInput{}, []string{}},
		{"operator with selected scope", r.operatorOnly(t, "farm-a1"), usecase.ListFarmsInput{}, []string{"Santa Rita"}},
		{"search", r.asAdminA(), usecase.ListFarmsInput{Search: "vista"}, []string{"Boa Vista"}},
		{"pending is never stored", r.asAdminA(), usecase.ListFarmsInput{Status: domain.FarmPending}, []string{}},
		{"page size", r.asAdminA(), usecase.ListFarmsInput{PageSize: 1, Page: 2}, []string{"Santa Rita"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Actor = tt.actor
			page, err := usecase.NewListFarms(r.reg, r.settings).Execute(ctx(), tt.in)
			assertNoError(t, err)
			if got := farmNames(page); !slices.Equal(got, tt.want) {
				t.Fatalf("farms = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListFarmsRejectsUnknownFilters(t *testing.T) {
	r := newRegistryEnv(t)
	_, err := usecase.NewListFarms(r.reg, r.settings).Execute(ctx(), usecase.ListFarmsInput{
		Actor: r.asAdminA(), Status: "archived", ProductionType: "forestry",
	})
	assertValidationOn(t, err, "status")
	assertValidationOn(t, err, "production_type")
}

func TestGetFarm(t *testing.T) {
	r := newRegistryEnv(t)
	get := usecase.NewGetFarm(r.reg)

	listing, err := get.Execute(ctx(), usecase.GetFarmInput{Actor: r.asAdminA(), FarmID: "farm-a1"})
	assertNoError(t, err)
	if listing.OwnerNames["own-a1"] != "Helena Prado" || len(listing.Farm.Owners) != 1 {
		t.Fatalf("unexpected listing: %+v", listing)
	}

	_, err = get.Execute(ctx(), usecase.GetFarmInput{Actor: r.asAdminB(), FarmID: "farm-a1"})
	assertIs(t, err, usecase.ErrNotFound)
	_, err = get.Execute(ctx(), usecase.GetFarmInput{Actor: r.operatorOnly(t, "farm-a1"), FarmID: "farm-a2"})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestCreateFarm(t *testing.T) {
	r := newRegistryEnv(t)
	create := usecase.NewCreateFarm(r.farmDeps())

	listing, err := create.Execute(ctx(), usecase.CreateFarmInput{
		Actor: r.asAdminA(), Details: activeFarm("Recanto", share("own-a1", 60), share("own-a2", 40)),
	})
	assertNoError(t, err)
	if listing.Farm.CompanyID != r.companyA.ID || listing.Farm.Status != domain.FarmActive || len(listing.Farm.Owners) != 2 {
		t.Fatalf("unexpected farm: %+v", listing.Farm)
	}
	if listing.OwnerNames["own-a2"] != "Marcos Prado" {
		t.Fatalf("owner names = %v", listing.OwnerNames)
	}
	r.assertRegistryAudit(t, domain.AuditFarmCreated)
}

func TestCreateFarmRefusals(t *testing.T) {
	tests := []struct {
		name    string
		actor   func(*registryEnv) domain.Principal
		details domain.FarmDetails
		wantErr error
		field   string
	}{
		{"operator cannot write", (*registryEnv).asOperatorA, draftFarm("X"), usecase.ErrForbidden, ""},
		{"unknown owner", (*registryEnv).asAdminA, activeFarm("X", share("own-a1", 50), share("ghost", 50)), nil, "owners[1].owner_id"},
		{"owner of another company", (*registryEnv).asAdminA, activeFarm("X", share("own-b1", 100)), nil, "owners[0].owner_id"},
		{"active below 100%", (*registryEnv).asAdminA, activeFarm("X", share("own-a1", 50)), nil, "owners"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistryEnv(t)
			before := len(r.reg.Farms)
			_, err := usecase.NewCreateFarm(r.farmDeps()).Execute(ctx(), usecase.CreateFarmInput{Actor: tt.actor(r), Details: tt.details})
			if tt.wantErr != nil {
				assertIs(t, err, tt.wantErr)
			} else {
				assertValidationOn(t, err, tt.field)
			}
			if len(r.reg.Farms) != before || len(r.reg.Audit) != 0 {
				t.Fatal("a refused creation must store nothing")
			}
		})
	}
}

func TestUpdateFarm(t *testing.T) {
	r := newRegistryEnv(t)
	update := usecase.NewUpdateFarm(r.farmDeps())
	r.clock.T = r.clock.T.Add(time.Hour)

	listing, err := update.Execute(ctx(), usecase.UpdateFarmInput{
		Actor: r.asAdminA(), FarmID: "farm-a1", Details: activeFarm("Santa Rita II", share("own-a1", 100)),
	})
	assertNoError(t, err)
	if listing.Farm.Name != "Santa Rita II" || listing.Farm.Status != domain.FarmActive || listing.Farm.Version != 2 {
		t.Fatalf("unexpected farm: %+v", listing.Farm)
	}
	if !listing.Farm.UpdatedAt.Equal(r.clock.T) {
		t.Fatalf("updated_at = %v, want %v", listing.Farm.UpdatedAt, r.clock.T)
	}
	r.assertRegistryAudit(t, domain.AuditFarmUpdated)

	_, err = update.Execute(ctx(), usecase.UpdateFarmInput{Actor: r.asAdminB(), FarmID: "farm-a1", Details: draftFarm("X")})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestUpdateFarmRollsBackOnFailure(t *testing.T) {
	r := newRegistryEnv(t)
	r.reg.Fail("Record", errBoom)
	_, err := usecase.NewUpdateFarm(r.farmDeps()).Execute(ctx(), usecase.UpdateFarmInput{
		Actor: r.asAdminA(), FarmID: "farm-a1", Details: draftFarm("Outro nome"),
	})
	assertIs(t, err, errBoom)
	if r.reg.Farms["farm-a1"].Name != "Santa Rita" {
		t.Fatal("failed transaction must roll back the farm")
	}
}

// ---- owners --------------------------------------------------------------------------

func individual(name, document string) domain.OwnerDetails {
	return domain.OwnerDetails{PersonType: domain.PersonIndividual, Name: name, Document: document}
}

func TestListOwners(t *testing.T) {
	r := newRegistryEnv(t)
	tests := []struct {
		name  string
		actor domain.Principal
		in    usecase.ListOwnersInput
		want  []string
	}{
		{"admin sees its company", r.asAdminA(), usecase.ListOwnersInput{}, []string{"Helena Prado", "Marcos Prado"}},
		{"other company", r.asAdminB(), usecase.ListOwnersInput{}, []string{"Bruna Lima"}},
		{"search by masked document", r.asAdminA(), usecase.ListOwnersInput{Search: "111.444"}, []string{"Marcos Prado"}},
		{"search by name", r.asAdminA(), usecase.ListOwnersInput{Search: "helena"}, []string{"Helena Prado"}},
		{"operator sees owners of its farms", r.operatorOnly(t, "farm-a1"), usecase.ListOwnersInput{}, []string{"Helena Prado"}},
		{"operator scope without owners", r.operatorOnly(t, "farm-a2"), usecase.ListOwnersInput{}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.Actor = tt.actor
			page, err := usecase.NewListOwners(r.reg, r.settings).Execute(ctx(), tt.in)
			assertNoError(t, err)
			if got := ownerNames(page); !slices.Equal(got, tt.want) {
				t.Fatalf("owners = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetOwnerVisibility(t *testing.T) {
	r := newRegistryEnv(t)
	get := usecase.NewGetOwner(r.reg)

	listing, err := get.Execute(ctx(), usecase.GetOwnerInput{Actor: r.asAdminA(), OwnerID: "own-a1"})
	assertNoError(t, err)
	if len(listing.Links) != 1 || listing.Links[0].FarmName != "Santa Rita" {
		t.Fatalf("links = %+v", listing.Links)
	}
	_, err = get.Execute(ctx(), usecase.GetOwnerInput{Actor: r.asAdminB(), OwnerID: "own-a1"})
	assertIs(t, err, usecase.ErrNotFound)
	_, err = get.Execute(ctx(), usecase.GetOwnerInput{Actor: r.operatorOnly(t, "farm-a1"), OwnerID: "own-a2"})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestCreateOwner(t *testing.T) {
	r := newRegistryEnv(t)
	create := usecase.NewCreateOwner(r.reg, r.reg, r.clock, r.ids)

	listing, err := create.Execute(ctx(), usecase.SaveOwnerInput{Actor: r.asAdminA(), Details: domain.OwnerDetails{
		PersonType: domain.PersonCompany, Name: "Agro Prado Ltda", Document: "11.222.333/0001-81",
	}})
	assertNoError(t, err)
	if listing.Owner.CompanyID != r.companyA.ID || listing.Owner.Document != "11222333000181" || listing.Links == nil {
		t.Fatalf("unexpected owner: %+v", listing)
	}
	r.assertRegistryAudit(t, domain.AuditOwnerCreated)
	if _, leaked := r.reg.Audit[0].Metadata["document"]; leaked {
		t.Fatal("audit metadata must not carry the document")
	}

	_, err = create.Execute(ctx(), usecase.SaveOwnerInput{Actor: r.asAdminA(), Details: individual("Outra", "529.982.247-25")})
	assertIs(t, err, usecase.ErrOwnerDocumentTaken)

	_, err = create.Execute(ctx(), usecase.SaveOwnerInput{Actor: r.asOperatorA(), Details: individual("Outra", "390.533.447-05")})
	assertIs(t, err, usecase.ErrForbidden)
}

func TestUpdateOwner(t *testing.T) {
	r := newRegistryEnv(t)
	update := usecase.NewUpdateOwner(r.reg, r.reg, r.clock)

	details := individual("Helena P. Prado", "52998224725")
	details.Phone, details.WindowAlertsEnabled = "35998124470", true
	listing, err := update.Execute(ctx(), usecase.SaveOwnerInput{Actor: r.asAdminA(), OwnerID: "own-a1", Details: details})
	assertNoError(t, err)
	if listing.Owner.Name != "Helena P. Prado" || listing.Owner.Version != 2 || len(listing.Links) != 1 {
		t.Fatalf("unexpected owner: %+v", listing)
	}

	_, err = update.Execute(ctx(), usecase.SaveOwnerInput{Actor: r.asAdminA(), OwnerID: "own-a1", Details: individual("Helena", "11144477735")})
	assertValidationOn(t, err, "document")
	_, err = update.Execute(ctx(), usecase.SaveOwnerInput{Actor: r.asAdminB(), OwnerID: "own-a1", Details: details})
	assertIs(t, err, usecase.ErrNotFound)
}

func TestReplaceOwnerLinks(t *testing.T) {
	tests := []struct {
		name    string
		owner   domain.OwnerID
		links   []domain.OwnershipInput
		field   string
		wantErr error
	}{
		{"fills the farm to exactly 100%", "own-a2", []domain.OwnershipInput{{FarmID: "farm-a1", Role: domain.LinkOwner, SharePercent: percent(40)}}, "", nil},
		{"one hundredth above 100%", "own-a2", []domain.OwnershipInput{{FarmID: "farm-a1", Role: domain.LinkOwner, SharePercent: percent(40.01)}}, "links[0].share_percent", nil},
		{"own previous share is replaced, not added", "own-a1", []domain.OwnershipInput{{FarmID: "farm-a1", Role: domain.LinkOwner, SharePercent: percent(100)}}, "", nil},
		{"tenant needs no share", "own-a2", []domain.OwnershipInput{{FarmID: "farm-a1", Role: domain.LinkTenant}}, "", nil},
		{"farm of another company", "own-a2", []domain.OwnershipInput{{FarmID: "farm-b1", Role: domain.LinkTenant}}, "links[0].farm_id", nil},
		{"repeated farm", "own-a2", []domain.OwnershipInput{{FarmID: "farm-a2", Role: domain.LinkTenant}, {FarmID: "farm-a2", Role: domain.LinkTenant}}, "links[1].farm_id", nil},
		{"empty list removes every link", "own-a1", nil, "", nil},
		{"owner of another company", "own-b1", nil, "", usecase.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistryEnv(t)
			links, err := usecase.NewReplaceOwnerLinks(r.reg, r.reg, r.clock).Execute(ctx(), usecase.ReplaceOwnerLinksInput{
				Actor: r.asAdminA(), OwnerID: tt.owner, Links: tt.links,
			})
			switch {
			case tt.wantErr != nil:
				assertIs(t, err, tt.wantErr)
			case tt.field != "":
				assertValidationOn(t, err, tt.field)
				if len(r.reg.Audit) != 0 {
					t.Fatal("refused links must not be audited")
				}
			default:
				assertNoError(t, err)
				if len(links) != len(tt.links) {
					t.Fatalf("links = %+v, want %d", links, len(tt.links))
				}
				r.assertRegistryAudit(t, domain.AuditOwnerLinksReplaced)
			}
		})
	}
}

// ---- perimeter -----------------------------------------------------------------------

func TestImportPerimeter(t *testing.T) {
	r := newRegistryEnv(t)
	ring := domain.Ring{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}}
	tools := &fakes.PerimeterTools{
		Parsed: usecase.ParsedPerimeter{Format: usecase.PerimeterKML, Perimeter: domain.Perimeter{{ring}}, InnerPolygons: 2},
		Measure: usecase.PerimeterMeasure{
			Valid: true, AreaHa: 12.5, Centroid: domain.GeoPoint{Lat: 0.5, Lng: 0.5}, InteriorPoint: domain.GeoPoint{Lat: 0.4, Lng: 0.6},
		},
	}
	registry := &fakes.MunicipalityRegistry{Located: &domain.Municipality{Code: "3128709", Name: "Guaxupé", State: "MG"}}
	importer := usecase.NewImportPerimeter(tools, tools, registry)

	result, err := importer.Execute(ctx(), usecase.ImportPerimeterInput{Actor: r.asAdminA()})
	assertNoError(t, err)
	if result.AreaHa != 12.5 || result.InnerPolygons != 2 || result.Format != usecase.PerimeterKML {
		t.Fatalf("unexpected import: %+v", result)
	}
	if result.Municipality == nil || result.Municipality.Name != "Guaxupé" || len(registry.Points) != 1 || registry.Points[0].Lat != 0.4 {
		t.Fatalf("municipality = %+v, located points %v", result.Municipality, registry.Points)
	}
	registry.Located = nil
	result, err = importer.Execute(ctx(), usecase.ImportPerimeterInput{Actor: r.asAdminA()})
	assertNoError(t, err)
	if result.Municipality != nil {
		t.Fatalf("unknown municipality must be nil: %+v", result.Municipality)
	}

	tools.Measure.Valid = false
	_, err = importer.Execute(ctx(), usecase.ImportPerimeterInput{Actor: r.asAdminA()})
	assertValidationOn(t, err, "file")

	tools.Err = usecase.ErrUnsupportedPerimeterFormat
	_, err = importer.Execute(ctx(), usecase.ImportPerimeterInput{Actor: r.asAdminA()})
	assertIs(t, err, usecase.ErrUnsupportedPerimeterFormat)

	_, err = importer.Execute(ctx(), usecase.ImportPerimeterInput{Actor: r.asOperatorA()})
	assertIs(t, err, usecase.ErrForbidden)
}

// ---- municipalities ------------------------------------------------------------------

func boundary(code, name, state string) usecase.MunicipalityBoundary {
	return usecase.MunicipalityBoundary{
		Municipality: domain.Municipality{Code: code, Name: name, State: state},
		Boundary:     `{"type":"Polygon","coordinates":[[[0,0],[0,1],[1,1],[0,0]]]}`,
	}
}

func TestLoadMunicipalities(t *testing.T) {
	good := []usecase.MunicipalityBoundary{boundary("3128709", "Guaxupé", "MG"), boundary("3550308", "São Paulo", "SP")}
	tests := []struct {
		name    string
		source  []usecase.MunicipalityBoundary
		fetch   error
		minimum int
		wantErr bool
		wantIs  error
	}{
		{"stores a complete registry", good, nil, 2, false, nil},
		{"partial answer", good[:1], nil, 2, true, usecase.ErrIncompleteRegistry},
		{"source down", nil, errBoom, 1, true, errBoom},
		{"repeated code", []usecase.MunicipalityBoundary{good[0], good[0]}, nil, 1, true, nil},
		{"bad code", []usecase.MunicipalityBoundary{boundary("31", "X", "MG")}, nil, 1, true, nil},
		{"bad state", []usecase.MunicipalityBoundary{boundary("3128709", "X", "Minas")}, nil, 1, true, nil},
		{"no boundary", []usecase.MunicipalityBoundary{{Municipality: good[0].Municipality}}, nil, 1, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := &fakes.MunicipalityRegistry{Source: tt.source, FetchErr: tt.fetch}
			count, err := usecase.NewLoadMunicipalities(registry, registry, tt.minimum).Execute(ctx())
			if !tt.wantErr {
				assertNoError(t, err)
				if count != len(tt.source) || len(registry.Stored) != len(tt.source) {
					t.Fatalf("stored %d (count %d), want %d", len(registry.Stored), count, len(tt.source))
				}
				return
			}
			if err == nil {
				t.Fatal("want error")
			}
			if tt.wantIs != nil {
				assertIs(t, err, tt.wantIs)
			}
			if registry.Stored != nil {
				t.Fatal("a refused registry must store nothing")
			}
		})
	}
}
