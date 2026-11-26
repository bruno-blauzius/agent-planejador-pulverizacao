package domain_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// fieldsOf returns the field paths of a validation error (nil when err is nil).
func fieldsOf(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("want *ValidationError, got %v", err)
	}
	fields := make([]string, 0, len(verr.Fields))
	for _, f := range verr.Fields {
		fields = append(fields, f.Field)
	}
	return fields
}

func ptr[T any](v T) *T { return &v }

func TestParseTaxID(t *testing.T) {
	tests := []struct {
		name       string
		personType domain.PersonType
		value      string
		want       domain.TaxID
		wantErr    bool
	}{
		{"valid CPF with mask", domain.PersonIndividual, "529.982.247-25", "52998224725", false},
		{"valid CPF digits", domain.PersonIndividual, "11144477735", "11144477735", false},
		{"CPF wrong check digit", domain.PersonIndividual, "529.982.247-24", "", true},
		{"CPF repeated digits", domain.PersonIndividual, "111.111.111-11", "", true},
		{"CPF given to company", domain.PersonCompany, "52998224725", "", true},
		{"valid CNPJ with mask", domain.PersonCompany, "11.222.333/0001-81", "11222333000181", false},
		{"CNPJ wrong check digit", domain.PersonCompany, "11.222.333/0001-80", "", true},
		{"CNPJ given to individual", domain.PersonIndividual, "11222333000181", "", true},
		{"empty", domain.PersonIndividual, "", "", true},
		{"unknown person type", domain.PersonType("alien"), "52998224725", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseTaxID(tt.personType, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSharePercent(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		want    domain.SharePercent
		wantErr bool
	}{
		{"exactly 100", 100, 10000, false},
		{"two decimals", 33.33, 3333, false},
		{"smallest share", 0.01, 1, false},
		{"just above 100", 100.01, 0, true},
		{"zero", 0, 0, true},
		{"negative", -5, 0, true},
		{"three decimals", 33.333, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParseSharePercent("share_percent", tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNewOwnership(t *testing.T) {
	tests := []struct {
		name       string
		in         domain.OwnershipInput
		wantFields []string
	}{
		{"owner with share", domain.OwnershipInput{OwnerID: "o1", Role: domain.LinkOwner, SharePercent: ptr(50.0)}, nil},
		{"owner without share", domain.OwnershipInput{OwnerID: "o1", Role: domain.LinkOwner}, []string{"links[0].share_percent"}},
		{"tenant without share", domain.OwnershipInput{OwnerID: "o1", Role: domain.LinkTenant}, nil},
		{"tenant with share", domain.OwnershipInput{OwnerID: "o1", Role: domain.LinkTenant, SharePercent: ptr(10.0)}, []string{"links[0].share_percent"}},
		{"unknown role", domain.OwnershipInput{OwnerID: "o1", Role: "boss"}, []string{"links[0].role"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewOwnership("links[0]", tt.in)
			if got := fieldsOf(t, err); !slices.Equal(got, tt.wantFields) {
				t.Fatalf("fields = %v, want %v", got, tt.wantFields)
			}
		})
	}
}

func TestFitsInFarm(t *testing.T) {
	sixty, thirty := domain.SharePercent(6000), domain.SharePercent(3000)
	others := []domain.Ownership{
		{OwnerID: "a", Role: domain.LinkOwner, Share: &sixty},
		{OwnerID: "b", Role: domain.LinkOwner, Share: &thirty},
		{OwnerID: "c", Role: domain.LinkTenant},
	}
	if !domain.FitsInFarm(others, 1000) {
		t.Fatal("90% + 10% must fit")
	}
	if domain.FitsInFarm(others, 1001) {
		t.Fatal("90% + 10.01% must not fit")
	}
}

func square() *domain.Perimeter {
	ring := domain.Ring{{-47, -21}, {-47, -20.99}, {-46.99, -20.99}, {-46.99, -21}, {-47, -21}}
	perimeter, err := domain.NewPerimeter("location.perimeter", []domain.Polygon{{ring}})
	if err != nil {
		panic(err)
	}
	return &perimeter
}

func TestNewPerimeter(t *testing.T) {
	tests := []struct {
		name    string
		rings   []domain.Ring
		wantErr bool
	}{
		{"closed square", []domain.Ring{{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0}}}, false},
		{"open ring", []domain.Ring{{{0, 0}, {0, 1}, {1, 1}, {1, 0}, {0, 0.5}}}, true},
		{"too few positions", []domain.Ring{{{0, 0}, {0, 1}, {0, 0}}}, true},
		{"latitude out of range", []domain.Ring{{{0, 0}, {0, 91}, {1, 1}, {0, 0}}}, true},
		{"no rings", []domain.Ring{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewPerimeter("location.perimeter", []domain.Polygon{tt.rings})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
	if _, err := domain.NewPerimeter("location.perimeter", nil); err == nil {
		t.Fatal("no polygons must fail")
	}
}

func farmDetails(status domain.FarmStatus, owners ...domain.OwnershipInput) domain.FarmDetails {
	return domain.FarmDetails{
		Name: " Fazenda Boa Vista ", ProductionType: domain.ProductionCoffee, City: "Guaxupé", State: "mg",
		Status: status, Perimeter: square(), Owners: owners,
	}
}

func share(owner domain.OwnerID, percent float64) domain.OwnershipInput {
	return domain.OwnershipInput{OwnerID: owner, Role: domain.LinkOwner, SharePercent: ptr(percent)}
}

func TestNewFarm(t *testing.T) {
	noPerimeter := farmDetails(domain.FarmActive, share("a", 100))
	noPerimeter.Perimeter = nil
	draftNoPerimeter := farmDetails(domain.FarmDraft)
	draftNoPerimeter.Perimeter = nil
	badPostal := farmDetails(domain.FarmDraft)
	badPostal.PostalCode = "123"
	pending := farmDetails(domain.FarmPending)
	missing := domain.FarmDetails{Status: domain.FarmDraft}

	tests := []struct {
		name       string
		in         domain.FarmDetails
		wantFields []string
	}{
		{"active with 100%", farmDetails(domain.FarmActive, share("a", 60), share("b", 40)), nil},
		{"active with 99.99%", farmDetails(domain.FarmActive, share("a", 60), share("b", 39.99)), []string{"owners"}},
		{"active with 100.01%", farmDetails(domain.FarmActive, share("a", 60), share("b", 40.01)), []string{"owners"}},
		{"active with tenant only", farmDetails(domain.FarmActive, domain.OwnershipInput{OwnerID: "a", Role: domain.LinkTenant}), []string{"owners"}},
		{"active without perimeter", noPerimeter, []string{"location.perimeter"}},
		{"draft below 100%", farmDetails(domain.FarmDraft, share("a", 30)), nil},
		{"draft above 100%", farmDetails(domain.FarmDraft, share("a", 70), share("b", 31)), []string{"owners"}},
		{"draft without perimeter or owners", draftNoPerimeter, nil},
		{"repeated owner", farmDetails(domain.FarmDraft, share("a", 10), share("a", 20)), []string{"owners[1].owner_id"}},
		{"owner item error keeps index", farmDetails(domain.FarmDraft, share("a", 10), domain.OwnershipInput{OwnerID: "b", Role: domain.LinkOwner}), []string{"owners[1].share_percent"}},
		{"bad postal code", badPostal, []string{"location.postal_code"}},
		{"pending is never stored", pending, []string{"status"}},
		{"missing required fields", missing, []string{"name", "production_type", "city", "state"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			farm, err := domain.NewFarm("f1", "c1", tt.in, now)
			if got := fieldsOf(t, err); !slices.Equal(got, tt.wantFields) {
				t.Fatalf("fields = %v, want %v", got, tt.wantFields)
			}
			if err != nil {
				return
			}
			if farm.Name != "Fazenda Boa Vista" || farm.State != "MG" {
				t.Fatalf("not normalized: %q %q", farm.Name, farm.State)
			}
			for _, link := range farm.Owners {
				if link.FarmID != "f1" {
					t.Fatalf("link farm = %q, want f1", link.FarmID)
				}
			}
		})
	}
}

func TestNewFarmReportsEveryRequiredField(t *testing.T) {
	_, err := domain.NewFarm("f1", "c1", domain.FarmDetails{Name: "F", Status: domain.FarmDraft}, now)
	want := []string{"production_type", "city", "state"}
	if got := fieldsOf(t, err); !slices.Equal(got, want) {
		t.Fatalf("fields = %v, want %v", got, want)
	}
}

func TestFarmRevise(t *testing.T) {
	farm, err := domain.NewFarm("f1", "c1", farmDetails(domain.FarmDraft), now)
	if err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Hour)
	if err := farm.Revise(farmDetails(domain.FarmActive, share("a", 50)), later); err == nil {
		t.Fatal("activating with 50% must fail")
	}
	if farm.Status != domain.FarmDraft || !farm.UpdatedAt.Equal(now) {
		t.Fatal("a failed revision must leave the farm unchanged")
	}
	if err := farm.Revise(farmDetails(domain.FarmActive, share("a", 100)), later); err != nil {
		t.Fatal(err)
	}
	if farm.Status != domain.FarmActive || !farm.UpdatedAt.Equal(later) || !farm.CreatedAt.Equal(now) {
		t.Fatalf("unexpected farm after revision: %+v", farm)
	}
}

func ownerDetails() domain.OwnerDetails {
	return domain.OwnerDetails{
		PersonType: domain.PersonIndividual, Name: "Helena Prado", Document: "529.982.247-25",
		BirthDate: ptr(time.Date(1979, 3, 12, 0, 0, 0, 0, time.UTC)), Phone: "(35) 99812-4470",
		Email: "Helena@Example.com", WindowAlertsEnabled: true,
	}
}

func TestNewOwner(t *testing.T) {
	tomorrow := now.AddDate(0, 0, 1)
	tests := []struct {
		name       string
		edit       func(*domain.OwnerDetails)
		wantFields []string
	}{
		{"valid individual", func(*domain.OwnerDetails) {}, nil},
		{"valid company without birth date", func(d *domain.OwnerDetails) {
			d.PersonType, d.Document, d.BirthDate = domain.PersonCompany, "11.222.333/0001-81", nil
		}, nil},
		{"company with birth date", func(d *domain.OwnerDetails) {
			d.PersonType, d.Document = domain.PersonCompany, "11.222.333/0001-81"
		}, []string{"birth_date"}},
		{"birth date in the future", func(d *domain.OwnerDetails) { d.BirthDate = &tomorrow }, []string{"birth_date"}},
		{"birth date today", func(d *domain.OwnerDetails) { d.BirthDate = ptr(now) }, nil},
		{"short phone", func(d *domain.OwnerDetails) { d.Phone = "3599" }, []string{"phone"}},
		{"alerts without phone", func(d *domain.OwnerDetails) { d.Phone = "" }, []string{"phone"}},
		{"no alerts, no phone", func(d *domain.OwnerDetails) { d.Phone, d.WindowAlertsEnabled = "", false }, nil},
		{"bad e-mail", func(d *domain.OwnerDetails) { d.Email = "helena@" }, []string{"email"}},
		{"bad document and name", func(d *domain.OwnerDetails) { d.Name, d.Document = " ", "123" }, []string{"name", "document"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ownerDetails()
			tt.edit(&in)
			owner, err := domain.NewOwner("o1", "c1", in, now)
			if got := fieldsOf(t, err); !slices.Equal(got, tt.wantFields) {
				t.Fatalf("fields = %v, want %v", got, tt.wantFields)
			}
			if err == nil && owner.Email != nil && *owner.Email != "helena@example.com" {
				t.Fatalf("email not normalized: %q", *owner.Email)
			}
		})
	}
}

func TestOwnerReviseKeepsDocument(t *testing.T) {
	owner, err := domain.NewOwner("o1", "c1", ownerDetails(), now)
	if err != nil {
		t.Fatal(err)
	}
	changed := ownerDetails()
	changed.Document = "111.444.777-35"
	if got := fieldsOf(t, owner.Revise(changed, now)); !slices.Equal(got, []string{"document"}) {
		t.Fatalf("fields = %v, want [document]", got)
	}
	renamed := ownerDetails()
	renamed.Name = "Helena P. Prado"
	if err := owner.Revise(renamed, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if owner.Name != "Helena P. Prado" || owner.Document != "52998224725" {
		t.Fatalf("unexpected owner: %+v", owner)
	}
}
