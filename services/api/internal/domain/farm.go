package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// FarmStatus is the registration status of a farm. FarmPending is derived
// (an owner with a pending document) and never stored.
type FarmStatus string

// Farm statuses.
const (
	FarmDraft   FarmStatus = "draft"
	FarmActive  FarmStatus = "active"
	FarmPending FarmStatus = "pending"
)

// Storable reports whether s can be sent by a client and stored.
func (s FarmStatus) Storable() bool { return s == FarmDraft || s == FarmActive }

// Valid reports whether s is a known status (filters accept pending too).
func (s FarmStatus) Valid() bool { return s.Storable() || s == FarmPending }

// ProductionType is the main activity of a farm.
type ProductionType string

// Production types.
const (
	ProductionCoffee    ProductionType = "coffee"
	ProductionGrains    ProductionType = "grains"
	ProductionLivestock ProductionType = "livestock"
	ProductionMixed     ProductionType = "mixed"
)

// Valid reports whether p is a known production type.
func (p ProductionType) Valid() bool {
	switch p {
	case ProductionCoffee, ProductionGrains, ProductionLivestock, ProductionMixed:
		return true
	}
	return false
}

const maxShortTextLength = 100

// GeoPoint is a WGS84 coordinate.
type GeoPoint struct {
	Lat float64
	Lng float64
}

// NewGeoPoint validates the coordinate ranges.
func NewGeoPoint(field string, lat, lng float64) (GeoPoint, error) {
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return GeoPoint{}, Invalid(field, "coordenada fora do intervalo")
	}
	return GeoPoint{Lat: lat, Lng: lng}, nil
}

// Position is a [lng, lat] pair, the GeoJSON order.
type Position [2]float64

// Ring is a closed line of positions (first equals last).
type Ring []Position

// Polygon is an outer ring followed by optional holes.
type Polygon []Ring

// Perimeter is the boundary of a farm: one or more polygons in WGS84 (a
// MultiPolygon). Topological validity (self-intersections) is checked by the
// database, which owns the geometry engine.
type Perimeter []Polygon

// NewPerimeter checks the structure: at least one polygon, rings closed with
// four or more positions and coordinates inside the WGS84 ranges.
func NewPerimeter(field string, polygons []Polygon) (Perimeter, error) {
	if len(polygons) == 0 {
		return nil, Invalid(field, "o perímetro precisa de ao menos um polígono")
	}
	for _, polygon := range polygons {
		if len(polygon) == 0 {
			return nil, Invalid(field, "polígono sem anel externo")
		}
		for _, ring := range polygon {
			if err := checkRing(field, ring); err != nil {
				return nil, err
			}
		}
	}
	return Perimeter(polygons), nil
}

func checkRing(field string, ring Ring) error {
	if len(ring) < 4 {
		return Invalid(field, "cada anel precisa de ao menos 4 posições")
	}
	if ring[0] != ring[len(ring)-1] {
		return Invalid(field, "o anel do polígono precisa ser fechado")
	}
	for _, position := range ring {
		if position[0] < -180 || position[0] > 180 || position[1] < -90 || position[1] > 90 {
			return Invalid(field, "coordenada fora do intervalo")
		}
	}
	return nil
}

// FarmDetails are the editable fields of a farm, as received from the client.
// Blank optional text means "not informed".
type FarmDetails struct {
	Name           string
	Code           string
	Cooperative    string
	ProductionType ProductionType
	City           string
	State          string
	Status         FarmStatus
	PostalCode     string
	CARNumber      string
	ForecastPoint  *GeoPoint
	Perimeter      *Perimeter
	Owners         []OwnershipInput
}

// Farm is a property of a company, with its location and owners. A draft may
// miss location and owners; an active farm has a perimeter and owners holding
// exactly 100%.
type Farm struct {
	ID             FarmID
	CompanyID      CompanyID
	Name           string
	Code           *string
	Cooperative    *string
	ProductionType ProductionType
	City           string
	State          string
	Status         FarmStatus
	PostalCode     *string
	CARNumber      *string
	ForecastPoint  *GeoPoint
	Perimeter      *Perimeter
	Owners         []Ownership
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
}

// NewFarm validates the details and returns a new farm of the company.
func NewFarm(id FarmID, companyID CompanyID, in FarmDetails, now time.Time) (Farm, error) {
	farm := Farm{ID: id, CompanyID: companyID, CreatedAt: now, UpdatedAt: now}
	if err := farm.apply(in); err != nil {
		return Farm{}, err
	}
	return farm, nil
}

// Revise replaces every editable field, with the same rules as the registration.
func (f *Farm) Revise(in FarmDetails, now time.Time) error {
	updated := *f
	if err := updated.apply(in); err != nil {
		return err
	}
	updated.UpdatedAt = now
	*f = updated
	return nil
}

// OwnerIDs lists the owners linked to the farm, in link order.
func (f Farm) OwnerIDs() []OwnerID {
	ids := make([]OwnerID, 0, len(f.Owners))
	for _, link := range f.Owners {
		ids = append(ids, link.OwnerID)
	}
	return ids
}

func (f *Farm) apply(in FarmDetails) error {
	var problems Violations
	name, err := ParseName("name", in.Name)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return mergeErr
	}
	if !in.ProductionType.Valid() {
		problems.Add("production_type", "tipo de produção desconhecido")
	}
	city, state := strings.TrimSpace(in.City), normalizeState(in.State)
	if city == "" {
		problems.Add("city", "informe o município")
	}
	if !validState(state) {
		problems.Add("state", "UF deve ter 2 letras")
	}
	if !in.Status.Storable() {
		problems.Add("status", "status deve ser draft ou active")
	}
	code := optionalShortText(&problems, "code", in.Code)
	cooperative := optionalShortText(&problems, "cooperative", in.Cooperative)
	carNumber := optionalShortText(&problems, "location.car_number", in.CARNumber)
	postalCode := optionalPostalCode(&problems, in.PostalCode)
	owners := farmOwnerships(&problems, f.ID, in.Owners)
	if in.Status == FarmActive {
		requireCompleteRegistration(&problems, in.Perimeter, owners)
	}
	if err := problems.Err(); err != nil {
		return err
	}

	f.Name, f.Code, f.Cooperative = name, code, cooperative
	f.ProductionType, f.City, f.State, f.Status = in.ProductionType, city, state, in.Status
	f.PostalCode, f.CARNumber = postalCode, carNumber
	f.ForecastPoint, f.Perimeter = in.ForecastPoint, in.Perimeter
	f.Owners = owners
	return nil
}

// farmOwnerships validates the owners of the farm: no repeated owner and never
// more than 100% in total.
func farmOwnerships(problems *Violations, farmID FarmID, inputs []OwnershipInput) []Ownership {
	owners := make([]Ownership, 0, len(inputs))
	seen := make(map[OwnerID]struct{}, len(inputs))
	for i, input := range inputs {
		field := fmt.Sprintf("owners[%d]", i)
		if input.OwnerID == "" {
			problems.Add(field+".owner_id", "informe o proprietário")
			continue
		}
		if _, dup := seen[input.OwnerID]; dup {
			problems.Add(field+".owner_id", "proprietário repetido")
			continue
		}
		seen[input.OwnerID] = struct{}{}
		input.FarmID = farmID
		link, err := NewOwnership(field, input)
		if err != nil {
			// NewOwnership only fails with field errors; anything else is reported on the item.
			if other := problems.Merge(err); other != nil {
				problems.Add(field, other.Error())
			}
			continue
		}
		owners = append(owners, link)
	}
	if TotalShare(owners) > FullOwnership {
		problems.Add("owners", "a soma das participações passa de 100%")
	}
	return owners
}

// requireCompleteRegistration enforces what an active farm must have.
func requireCompleteRegistration(problems *Violations, perimeter *Perimeter, owners []Ownership) {
	if perimeter == nil {
		problems.Add("location.perimeter", "informe o perímetro para ativar a fazenda")
	}
	total := TotalShare(owners)
	switch {
	case total == 0:
		problems.Add("owners", "vincule ao menos um proprietário com participação")
	case total < FullOwnership:
		problems.Add("owners", fmt.Sprintf("as participações somam %s; precisam somar 100%%", total))
	}
}

func optionalShortText(problems *Violations, field, value string) *string {
	text := strings.TrimSpace(value)
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) > maxShortTextLength {
		problems.Add(field, "texto muito longo")
		return nil
	}
	return &text
}

func optionalPostalCode(problems *Violations, value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	digits := digitsOnly(value)
	if len(digits) != 8 {
		problems.Add("location.postal_code", "CEP deve ter 8 dígitos")
		return nil
	}
	return &digits
}
