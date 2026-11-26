package httpadapter

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Wire shapes of the registry (Farms and Owners tags of the contract).

// ---- geometry ------------------------------------------------------------------------

type geoPointBody struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// geometryBody is a GeoJSON Polygon or MultiPolygon.
type geometryBody struct {
	Type        string `json:"type"`
	Coordinates any    `json:"coordinates"`
}

func pointOf(point *domain.GeoPoint) *geoPointBody {
	if point == nil {
		return nil
	}
	return &geoPointBody{Lat: point.Lat, Lng: point.Lng}
}

// perimeterOf answers a single polygon as Polygon and anything else as MultiPolygon.
func perimeterOf(perimeter *domain.Perimeter) *geometryBody {
	if perimeter == nil || len(*perimeter) == 0 {
		return nil
	}
	if len(*perimeter) == 1 {
		return &geometryBody{Type: "Polygon", Coordinates: (*perimeter)[0]}
	}
	return &geometryBody{Type: "MultiPolygon", Coordinates: *perimeter}
}

// parsePerimeterBody reads a GeoJSON Polygon or MultiPolygon. Members other
// than type and coordinates (bbox, crs) are ignored; null means no perimeter.
func parsePerimeterBody(field string, raw json.RawMessage) (*domain.Perimeter, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var geometry struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &geometry); err != nil {
		return nil, domain.Invalid(field, "o perímetro deve ser um GeoJSON")
	}
	var polygons []domain.Polygon
	switch geometry.Type {
	case "Polygon":
		var polygon domain.Polygon
		if err := json.Unmarshal(geometry.Coordinates, &polygon); err != nil {
			return nil, domain.Invalid(field, "coordenadas de polígono inválidas")
		}
		polygons = []domain.Polygon{polygon}
	case "MultiPolygon":
		if err := json.Unmarshal(geometry.Coordinates, &polygons); err != nil {
			return nil, domain.Invalid(field, "coordenadas de multipolígono inválidas")
		}
	default:
		return nil, domain.Invalid(field, "o perímetro deve ser Polygon ou MultiPolygon")
	}
	perimeter, err := domain.NewPerimeter(field, polygons)
	if err != nil {
		return nil, err
	}
	return &perimeter, nil
}

// ---- farms ---------------------------------------------------------------------------

type farmOwnerShareBody struct {
	OwnerID      string   `json:"owner_id"`
	Name         string   `json:"name"`
	Role         string   `json:"role"`
	SharePercent *float64 `json:"share_percent"`
}

type cropBody struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// farmSummaryBody is FarmSummary. The spray outlook comes from the planner,
// not integrated yet: always null.
type farmSummaryBody struct {
	CompanyID        string               `json:"company_id"`
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Code             *string              `json:"code"`
	City             string               `json:"city"`
	State            string               `json:"state"`
	Status           string               `json:"status"`
	ProductionType   string               `json:"production_type"`
	TotalAreaHa      float64              `json:"total_area_ha"`
	ProductiveAreaHa float64              `json:"productive_area_ha"`
	FieldCount       int                  `json:"field_count"`
	Crops            []cropBody           `json:"crops"`
	Owners           []farmOwnerShareBody `json:"owners"`
	SprayOutlook     *struct{}            `json:"spray_outlook"`
	ForecastPoint    *geoPointBody        `json:"forecast_point"`
	Perimeter        *geometryBody        `json:"perimeter"`
	CreatedAt        time.Time            `json:"created_at"`
}

type farmLocationBody struct {
	PostalCode    *string       `json:"postal_code"`
	CarNumber     *string       `json:"car_number"`
	ForecastPoint *geoPointBody `json:"forecast_point"`
	Perimeter     *geometryBody `json:"perimeter"`
}

type farmBody struct {
	farmSummaryBody
	Cooperative *string          `json:"cooperative"`
	Location    farmLocationBody `json:"location"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

func sharePercentOf(share *domain.SharePercent) *float64 {
	if share == nil {
		return nil
	}
	value := share.Percent()
	return &value
}

func farmSummaryOf(listing usecase.FarmListing) farmSummaryBody {
	farm := listing.Farm
	owners := make([]farmOwnerShareBody, 0, len(farm.Owners))
	for _, link := range farm.Owners {
		owners = append(owners, farmOwnerShareBody{
			OwnerID: string(link.OwnerID), Name: listing.OwnerNames[link.OwnerID],
			Role: string(link.Role), SharePercent: sharePercentOf(link.Share),
		})
	}
	return farmSummaryBody{
		CompanyID: string(farm.CompanyID), ID: string(farm.ID), Name: farm.Name, Code: farm.Code,
		City: farm.City, State: farm.State, Status: string(domain.DisplayStatus(farm.Status, listing.OwnerDocumentsPending)),
		ProductionType: string(farm.ProductionType),
		TotalAreaHa:    listing.AreaHa, ProductiveAreaHa: listing.ProductiveAreaHa, FieldCount: listing.FieldCount,
		Crops: cropsOf(listing.Crops), Owners: owners,
		ForecastPoint: pointOf(farm.ForecastPoint), Perimeter: perimeterOf(listing.Outline),
		CreatedAt: farm.CreatedAt.UTC(),
	}
}

func farmOf(listing usecase.FarmListing) farmBody {
	farm := listing.Farm
	return farmBody{
		farmSummaryBody: farmSummaryOf(listing),
		Cooperative:     farm.Cooperative,
		Location: farmLocationBody{
			PostalCode: farm.PostalCode, CarNumber: farm.CARNumber,
			ForecastPoint: pointOf(farm.ForecastPoint), Perimeter: perimeterOf(farm.Perimeter),
		},
		UpdatedAt: farm.UpdatedAt.UTC(),
	}
}

func farmPageOf(page usecase.FarmPage) pageBody[farmSummaryBody] {
	items := make([]farmSummaryBody, 0, len(page.Items))
	for _, listing := range page.Items {
		items = append(items, farmSummaryOf(listing))
	}
	return pageBody[farmSummaryBody]{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

// farmInputBody is FarmInput. The perimeter stays raw so GeoJSON members the
// contract does not list (bbox) do not trip the strict decoder.
type farmInputBody struct {
	Name           string  `json:"name"`
	Code           *string `json:"code"`
	Cooperative    *string `json:"cooperative"`
	ProductionType string  `json:"production_type"`
	City           string  `json:"city"`
	State          string  `json:"state"`
	Status         string  `json:"status"`
	Location       struct {
		PostalCode    *string         `json:"postal_code"`
		CarNumber     *string         `json:"car_number"`
		ForecastPoint *geoPointBody   `json:"forecast_point"`
		Perimeter     json.RawMessage `json:"perimeter"`
	} `json:"location"`
	Owners []struct {
		OwnerID      string   `json:"owner_id"`
		Role         string   `json:"role"`
		SharePercent *float64 `json:"share_percent"`
	} `json:"owners"`
}

func (b farmInputBody) details() (domain.FarmDetails, error) {
	var problems domain.Violations
	details := domain.FarmDetails{
		Name: b.Name, Code: textOrEmpty(b.Code), Cooperative: textOrEmpty(b.Cooperative),
		ProductionType: domain.ProductionType(b.ProductionType), City: b.City, State: b.State,
		Status: domain.FarmStatus(b.Status), PostalCode: textOrEmpty(b.Location.PostalCode),
		CARNumber: textOrEmpty(b.Location.CarNumber),
	}
	if point := b.Location.ForecastPoint; point != nil {
		parsed, err := domain.NewGeoPoint("location.forecast_point", point.Lat, point.Lng)
		if mergeErr := problems.Merge(err); mergeErr != nil {
			return domain.FarmDetails{}, mergeErr
		}
		if err == nil {
			details.ForecastPoint = &parsed
		}
	}
	perimeter, err := parsePerimeterBody("location.perimeter", b.Location.Perimeter)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return domain.FarmDetails{}, mergeErr
	}
	details.Perimeter = perimeter
	for _, owner := range b.Owners {
		details.Owners = append(details.Owners, domain.OwnershipInput{
			OwnerID: domain.OwnerID(strings.ToLower(owner.OwnerID)), Role: domain.LinkRole(owner.Role), SharePercent: owner.SharePercent,
		})
	}
	return details, problems.Err()
}

func textOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type perimeterImportBody struct {
	SourceFormat string        `json:"source_format"`
	Perimeter    *geometryBody `json:"perimeter"`
	AreaHa       float64       `json:"area_ha"`
	Centroid     geoPointBody  `json:"centroid"`
	Municipality *string       `json:"municipality"`
	State        *string       `json:"state"`
	Datum        *string       `json:"datum"`
	FieldCount   int           `json:"field_count"`
}

// perimeterImportOf answers municipality and state as null when the IBGE
// registry does not know the place (or was never loaded).
func perimeterImportOf(result usecase.PerimeterImport) perimeterImportBody {
	var datum *string
	if result.Datum != "" {
		datum = &result.Datum
	}
	body := perimeterImportBody{
		SourceFormat: string(result.Format), Perimeter: perimeterOf(&result.Perimeter), AreaHa: result.AreaHa,
		Centroid: geoPointBody{Lat: result.Centroid.Lat, Lng: result.Centroid.Lng},
		Datum:    datum, FieldCount: result.InnerPolygons,
	}
	if place := result.Municipality; place != nil {
		body.Municipality, body.State = &place.Name, &place.State
	}
	return body
}

// ---- crops and fields -----------------------------------------------------------------

type cropListBody struct {
	Items []cropBody `json:"items"`
}

func cropsOf(crops []domain.Crop) []cropBody {
	out := make([]cropBody, 0, len(crops))
	for _, crop := range crops {
		out = append(out, cropBody{Code: crop.Code, Name: crop.Name})
	}
	return out
}

// fieldBody is Field. Spray condition and last application come from the
// planner and the applications, not integrated yet: always null.
type fieldBody struct {
	ID                string       `json:"id"`
	FarmID            string       `json:"farm_id"`
	Name              string       `json:"name"`
	Block             *string      `json:"block"`
	AreaHa            float64      `json:"area_ha"`
	Crop              cropBody     `json:"crop"`
	Variety           *string      `json:"variety"`
	Season            *string      `json:"season"`
	PlantingDate      *string      `json:"planting_date"`
	RowSpacingM       *float64     `json:"row_spacing_m"`
	PlantSpacingM     *float64     `json:"plant_spacing_m"`
	PlantsPerHa       *int         `json:"plants_per_ha"`
	Polygon           geometryBody `json:"polygon"`
	SprayCondition    *struct{}    `json:"spray_condition"`
	LastApplicationAt *time.Time   `json:"last_application_at"`
}

type fieldListBody struct {
	Items []fieldBody `json:"items"`
}

func fieldOf(listing usecase.FieldListing) fieldBody {
	field := listing.Field
	body := fieldBody{
		ID: string(field.ID), FarmID: string(field.FarmID), Name: field.Name, Block: field.Block,
		AreaHa: listing.AreaHa, Crop: cropBody{Code: field.Crop.Code, Name: field.Crop.Name},
		Variety: field.Variety, Season: field.Season, RowSpacingM: field.RowSpacingM, PlantSpacingM: field.PlantSpacingM,
		PlantsPerHa: field.PlantsPerHa(), Polygon: geometryBody{Type: "Polygon", Coordinates: field.Polygon},
	}
	if field.PlantingDate != nil {
		date := field.PlantingDate.Format(dateLayout)
		body.PlantingDate = &date
	}
	return body
}

func fieldsOf(listings []usecase.FieldListing) []fieldBody {
	out := make([]fieldBody, 0, len(listings))
	for _, listing := range listings {
		out = append(out, fieldOf(listing))
	}
	return out
}

// fieldInputBody is FieldInput. The polygon stays raw, like the farm perimeter.
type fieldInputBody struct {
	Name          string          `json:"name"`
	Block         *string         `json:"block"`
	CropCode      string          `json:"crop_code"`
	Variety       *string         `json:"variety"`
	Season        *string         `json:"season"`
	PlantingDate  *string         `json:"planting_date"`
	RowSpacingM   *float64        `json:"row_spacing_m"`
	PlantSpacingM *float64        `json:"plant_spacing_m"`
	Polygon       json.RawMessage `json:"polygon"`
}

func (b fieldInputBody) details() (domain.FieldDetails, error) {
	var problems domain.Violations
	details := domain.FieldDetails{
		Name: b.Name, Block: textOrEmpty(b.Block), CropCode: b.CropCode, Variety: textOrEmpty(b.Variety),
		Season: textOrEmpty(b.Season), RowSpacingM: b.RowSpacingM, PlantSpacingM: b.PlantSpacingM,
	}
	if b.PlantingDate != nil && strings.TrimSpace(*b.PlantingDate) != "" {
		date, err := time.Parse(dateLayout, strings.TrimSpace(*b.PlantingDate))
		if err != nil {
			problems.Add("planting_date", "data inválida (use AAAA-MM-DD)")
		} else {
			details.PlantingDate = &date
		}
	}
	polygon, err := parsePolygonBody("polygon", b.Polygon)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return domain.FieldDetails{}, mergeErr
	}
	details.Polygon = polygon
	return details, problems.Err()
}

// parsePolygonBody reads a GeoJSON Polygon (members other than type and
// coordinates are ignored). The domain checks the rings.
func parsePolygonBody(field string, raw json.RawMessage) (domain.Polygon, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, domain.Invalid(field, "desenhe o polígono do talhão")
	}
	var geometry struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &geometry); err != nil || geometry.Type != "Polygon" {
		return nil, domain.Invalid(field, "o polígono deve ser um GeoJSON Polygon")
	}
	var polygon domain.Polygon
	if err := json.Unmarshal(geometry.Coordinates, &polygon); err != nil {
		return nil, domain.Invalid(field, "coordenadas de polígono inválidas")
	}
	return polygon, nil
}

// ---- owners --------------------------------------------------------------------------

type ownerFarmLinkBody struct {
	FarmID       string   `json:"farm_id"`
	FarmName     string   `json:"farm_name"`
	Role         string   `json:"role"`
	SharePercent *float64 `json:"share_percent"`
}

type ownerSummaryBody struct {
	ID                  string              `json:"id"`
	PersonType          string              `json:"person_type"`
	Name                string              `json:"name"`
	Document            string              `json:"document"`
	HasPendingDocuments bool                `json:"has_pending_documents"`
	Links               []ownerFarmLinkBody `json:"links"`
}

type ownerDocumentBody struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	FileName   string    `json:"file_name"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// ownerBody is Owner. Documents are listed on the detail only.
type ownerBody struct {
	ownerSummaryBody
	BirthDate           *string             `json:"birth_date"`
	Phone               *string             `json:"phone"`
	Email               *string             `json:"email"`
	WindowAlertsEnabled bool                `json:"window_alerts_enabled"`
	Documents           []ownerDocumentBody `json:"documents"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
}

type ownerLinkListBody struct {
	Items []ownerFarmLinkBody `json:"items"`
}

const dateLayout = "2006-01-02"

func ownerLinksOf(links []usecase.OwnerLink) []ownerFarmLinkBody {
	out := make([]ownerFarmLinkBody, 0, len(links))
	for _, link := range links {
		out = append(out, ownerFarmLinkBody{
			FarmID: string(link.FarmID), FarmName: link.FarmName, Role: string(link.Role), SharePercent: sharePercentOf(link.Share),
		})
	}
	return out
}

func ownerSummaryOf(listing usecase.OwnerListing) ownerSummaryBody {
	owner := listing.Owner
	return ownerSummaryBody{
		ID: string(owner.ID), PersonType: string(owner.PersonType), Name: owner.Name,
		Document: string(owner.Document), HasPendingDocuments: owner.DocumentsPending, Links: ownerLinksOf(listing.Links),
	}
}

func ownerOf(listing usecase.OwnerListing) ownerBody {
	owner := listing.Owner
	body := ownerBody{
		ownerSummaryBody:    ownerSummaryOf(listing),
		WindowAlertsEnabled: owner.WindowAlertsEnabled,
		Documents:           documentsOf(listing.Documents),
		CreatedAt:           owner.CreatedAt.UTC(),
		UpdatedAt:           owner.UpdatedAt.UTC(),
	}
	if owner.BirthDate != nil {
		date := owner.BirthDate.Format(dateLayout)
		body.BirthDate = &date
	}
	if owner.Phone != nil {
		phone := string(*owner.Phone)
		body.Phone = &phone
	}
	if owner.Email != nil {
		email := owner.Email.String()
		body.Email = &email
	}
	return body
}

func documentsOf(documents []domain.OwnerDocument) []ownerDocumentBody {
	out := make([]ownerDocumentBody, 0, len(documents))
	for _, document := range documents {
		out = append(out, ownerDocumentOf(document))
	}
	return out
}

func ownerPageOf(page usecase.OwnerPage) pageBody[ownerSummaryBody] {
	items := make([]ownerSummaryBody, 0, len(page.Items))
	for _, listing := range page.Items {
		items = append(items, ownerSummaryOf(listing))
	}
	return pageBody[ownerSummaryBody]{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize}
}

// ownerInputBody is OwnerInput.
type ownerInputBody struct {
	PersonType          string  `json:"person_type"`
	Name                string  `json:"name"`
	Document            string  `json:"document"`
	BirthDate           *string `json:"birth_date"`
	Phone               *string `json:"phone"`
	Email               *string `json:"email"`
	WindowAlertsEnabled bool    `json:"window_alerts_enabled"`
}

func (b ownerInputBody) details() (domain.OwnerDetails, error) {
	details := domain.OwnerDetails{
		PersonType: domain.PersonType(b.PersonType), Name: b.Name, Document: b.Document,
		Phone: textOrEmpty(b.Phone), Email: textOrEmpty(b.Email), WindowAlertsEnabled: b.WindowAlertsEnabled,
	}
	if b.BirthDate != nil && strings.TrimSpace(*b.BirthDate) != "" {
		date, err := time.Parse(dateLayout, strings.TrimSpace(*b.BirthDate))
		if err != nil {
			return domain.OwnerDetails{}, domain.Invalid("birth_date", "data inválida (use AAAA-MM-DD)")
		}
		details.BirthDate = &date
	}
	return details, nil
}

// ownerLinksBody is the body of PUT /owners/{ownerId}/links.
type ownerLinksBody struct {
	Links []struct {
		FarmID       string   `json:"farm_id"`
		Role         string   `json:"role"`
		SharePercent *float64 `json:"share_percent"`
	} `json:"links"`
}

func (b ownerLinksBody) inputs() []domain.OwnershipInput {
	inputs := make([]domain.OwnershipInput, 0, len(b.Links))
	for _, link := range b.Links {
		inputs = append(inputs, domain.OwnershipInput{
			FarmID: domain.FarmID(strings.ToLower(link.FarmID)), Role: domain.LinkRole(link.Role), SharePercent: link.SharePercent,
		})
	}
	return inputs
}
