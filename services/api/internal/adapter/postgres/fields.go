package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Crops and fields (DER: crops, fields). Geometry travels as GeoJSON; the
// area of a field is a generated column.

// ---- crops ---------------------------------------------------------------------------

// ListCrops implements usecase.CropReader.
func (r *Repository) ListCrops(ctx context.Context) ([]domain.Crop, error) {
	rows, err := r.q.ListActiveCrops(ctx)
	if err != nil {
		return nil, fmt.Errorf("list crops: %w", err)
	}
	crops := make([]domain.Crop, 0, len(rows))
	for _, row := range rows {
		crops = append(crops, domain.Crop{Code: row.Code, Name: row.Name})
	}
	return crops, nil
}

// FindCrop implements usecase.CropReader.
func (r *Repository) FindCrop(ctx context.Context, code string) (domain.Crop, error) {
	row, err := r.q.GetActiveCrop(ctx, code)
	if err != nil {
		return domain.Crop{}, translateRead("find crop", err)
	}
	return domain.Crop{Code: row.Code, Name: row.Name}, nil
}

// ---- fields --------------------------------------------------------------------------

// fieldColumns is the shape shared by the field reads (sqlc generates one row type per query).
type fieldColumns struct {
	ID, CompanyID, FarmID, CropCode, CropName, Name string
	Block, Variety, Season                          pgtype.Text
	PlantingDate                                    pgtype.Date
	RowSpacingM, PlantSpacingM                      pgtype.Numeric
	Polygon                                         string
	AreaHa                                          float64
	CreatedAt, UpdatedAt                            time.Time
	Version                                         int32
}

func fieldListingOf(c fieldColumns) (usecase.FieldListing, error) {
	polygon, err := decodePolygon(c.Polygon)
	if err != nil {
		return usecase.FieldListing{}, err
	}
	rowSpacing, err := numericPtr(c.RowSpacingM)
	if err != nil {
		return usecase.FieldListing{}, err
	}
	plantSpacing, err := numericPtr(c.PlantSpacingM)
	if err != nil {
		return usecase.FieldListing{}, err
	}
	field := domain.Field{
		ID: domain.FieldID(c.ID), CompanyID: domain.CompanyID(c.CompanyID), FarmID: domain.FarmID(c.FarmID),
		Name: c.Name, Block: textPtr(c.Block), Crop: domain.Crop{Code: c.CropCode, Name: c.CropName},
		Variety: textPtr(c.Variety), Season: textPtr(c.Season),
		RowSpacingM: rowSpacing, PlantSpacingM: plantSpacing, Polygon: polygon,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: int(c.Version),
	}
	if c.PlantingDate.Valid {
		date := c.PlantingDate.Time
		field.PlantingDate = &date
	}
	return usecase.FieldListing{Field: field, AreaHa: c.AreaHa}, nil
}

func numericPtr(value pgtype.Numeric) (*float64, error) {
	if !value.Valid {
		return nil, nil
	}
	number, err := value.Float64Value()
	if err != nil {
		return nil, fmt.Errorf("decode numeric: %w", err)
	}
	return &number.Float64, nil
}

func float8Of(value *float64) pgtype.Float8 {
	if value == nil {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: *value, Valid: true}
}

func dateOf(value *time.Time) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *value, Valid: true}
}

// ListFields implements usecase.FieldReader.
func (r *Repository) ListFields(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) ([]usecase.FieldListing, error) {
	listings := []usecase.FieldListing{}
	if !validUUID(string(companyID)) || !validUUID(string(farmID)) {
		return listings, nil
	}
	rows, err := r.q.ListFields(ctx, sqlcgen.ListFieldsParams{CompanyID: string(companyID), FarmID: string(farmID)})
	if err != nil {
		return nil, fmt.Errorf("list fields: %w", err)
	}
	for _, row := range rows {
		listing, err := fieldListingOf(fieldColumns(row))
		if err != nil {
			return nil, err
		}
		listings = append(listings, listing)
	}
	return listings, nil
}

// FindField implements usecase.FieldReader.
func (r *Repository) FindField(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.FieldID) (usecase.FieldListing, error) {
	row, err := r.q.GetField(ctx, sqlcgen.GetFieldParams{CompanyID: string(companyID), FarmID: string(farmID), ID: string(id)})
	if err != nil {
		return usecase.FieldListing{}, translateRead("find field", err)
	}
	return fieldListingOf(fieldColumns(row))
}

// LockField implements usecase.FieldStore.
func (r *Repository) LockField(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.FieldID) (domain.Field, error) {
	row, err := r.q.LockField(ctx, sqlcgen.LockFieldParams{CompanyID: string(companyID), FarmID: string(farmID), ID: string(id)})
	if err != nil {
		return domain.Field{}, translateRead("lock field", err)
	}
	listing, err := fieldListingOf(fieldColumns(row))
	if err != nil {
		return domain.Field{}, err
	}
	return listing.Field, nil
}

// FieldFootprint implements usecase.FieldStore with PostGIS.
func (r *Repository) FieldFootprint(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, exclude domain.FieldID, polygon domain.Polygon) (domain.FieldFootprint, error) {
	text, err := encodePolygon(polygon)
	if err != nil {
		return domain.FieldFootprint{}, err
	}
	measure, err := r.q.MeasureField(ctx, sqlcgen.MeasureFieldParams{Polygon: text, CompanyID: string(companyID), FarmID: string(farmID)})
	if err != nil {
		return domain.FieldFootprint{}, translateRead("measure field", err)
	}
	footprint := domain.FieldFootprint{Valid: measure.Valid, AreaHa: measure.AreaHa, OutsidePerimeterM2: measure.OutsideM2}
	if !measure.Valid {
		return footprint, nil
	}
	overlaps, err := r.q.ListFieldOverlaps(ctx, sqlcgen.ListFieldOverlapsParams{
		Polygon: text, CompanyID: string(companyID), FarmID: string(farmID), ExcludeID: string(exclude),
	})
	if err != nil {
		return domain.FieldFootprint{}, fmt.Errorf("list field overlaps: %w", err)
	}
	for _, overlap := range overlaps {
		footprint.Overlaps = append(footprint.Overlaps, domain.FieldOverlap{
			FieldID: domain.FieldID(overlap.ID), Name: overlap.Name, AreaM2: overlap.OverlapM2,
		})
	}
	return footprint, nil
}

// FarmCoverage implements usecase.FarmStore with PostGIS.
func (r *Repository) FarmCoverage(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, perimeter *domain.Perimeter) (domain.FarmCoverage, error) {
	text, err := perimeterGeoJSON(perimeter)
	if err != nil {
		return domain.FarmCoverage{}, err
	}
	row, err := r.q.MeasureFarmCoverage(ctx, sqlcgen.MeasureFarmCoverageParams{
		Perimeter: text, CompanyID: string(companyID), FarmID: string(farmID),
	})
	if err != nil {
		return domain.FarmCoverage{}, translateRead("measure farm coverage", err)
	}
	return domain.FarmCoverage{FieldCount: int(row.FieldCount), FieldsAreaHa: row.FieldsAreaHa, OutsidePerimeterM2: row.OutsideM2}, nil
}

// InsertField implements usecase.FieldStore.
func (r *Repository) InsertField(ctx context.Context, field domain.Field) error {
	text, err := encodePolygon(field.Polygon)
	if err != nil {
		return err
	}
	err = r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return translateWrite("insert field", q.InsertField(ctx, sqlcgen.InsertFieldParams{
			ID: string(field.ID), CompanyID: string(field.CompanyID), FarmID: string(field.FarmID),
			CropCode: field.Crop.Code, Name: field.Name, Block: ptrText(field.Block), Variety: ptrText(field.Variety),
			Season: ptrText(field.Season), PlantingDate: dateOf(field.PlantingDate),
			RowSpacingM: float8Of(field.RowSpacingM), PlantSpacingM: float8Of(field.PlantSpacingM),
			Polygon: text, CreatedAt: field.CreatedAt, UpdatedAt: field.UpdatedAt,
		}))
	})
	if err != nil {
		return fmt.Errorf("insert field: %w", err)
	}
	return nil
}

// UpdateField implements usecase.FieldStore with optimistic concurrency.
func (r *Repository) UpdateField(ctx context.Context, field domain.Field) error {
	text, err := encodePolygon(field.Polygon)
	if err != nil {
		return err
	}
	return r.atomic(ctx, func(q *sqlcgen.Queries) error {
		updated, err := q.UpdateField(ctx, sqlcgen.UpdateFieldParams{
			CompanyID: string(field.CompanyID), FarmID: string(field.FarmID), ID: string(field.ID),
			Version: normalizeVersion(field.Version), CropCode: field.Crop.Code, Name: field.Name,
			Block: ptrText(field.Block), Variety: ptrText(field.Variety), Season: ptrText(field.Season),
			PlantingDate: dateOf(field.PlantingDate), RowSpacingM: float8Of(field.RowSpacingM),
			PlantSpacingM: float8Of(field.PlantSpacingM), Polygon: text, UpdatedAt: field.UpdatedAt,
		})
		if err != nil {
			return translateWrite("update field", err)
		}
		if updated > 0 {
			return nil
		}
		return missingOrStale(ctx, q.FieldExists, sqlcgen.FieldExistsParams{
			CompanyID: string(field.CompanyID), FarmID: string(field.FarmID), ID: string(field.ID),
		}, "update field")
	})
}

// attachFieldSummaries fills the field count, productive area and crops of
// every listed farm in two queries.
func (r *Repository) attachFieldSummaries(ctx context.Context, companyID domain.CompanyID, items []usecase.FarmListing) error {
	ids := make([]string, 0, len(items))
	byID := make(map[string]*usecase.FarmListing, len(items))
	for i := range items {
		items[i].Crops = []domain.Crop{}
		ids = append(ids, string(items[i].Farm.ID))
		byID[string(items[i].Farm.ID)] = &items[i]
	}
	summaries, err := r.q.SummarizeFieldsByFarms(ctx, sqlcgen.SummarizeFieldsByFarmsParams{CompanyID: string(companyID), FarmIds: ids})
	if err != nil {
		return fmt.Errorf("summarize fields: %w", err)
	}
	for _, row := range summaries {
		if item, ok := byID[row.FarmID]; ok {
			item.FieldCount, item.ProductiveAreaHa = int(row.FieldCount), row.AreaHa
		}
	}
	crops, err := r.q.ListCropsByFarms(ctx, sqlcgen.ListCropsByFarmsParams{CompanyID: string(companyID), FarmIds: ids})
	if err != nil {
		return fmt.Errorf("list crops of farms: %w", err)
	}
	for _, row := range crops {
		if item, ok := byID[row.FarmID]; ok {
			item.Crops = append(item.Crops, domain.Crop{Code: row.Code, Name: row.Name})
		}
	}
	return nil
}

// ---- geometry ------------------------------------------------------------------------

func encodePolygon(polygon domain.Polygon) (string, error) {
	coordinates, err := json.Marshal(polygon)
	if err != nil {
		return "", fmt.Errorf("encode polygon: %w", err)
	}
	body, err := json.Marshal(geoJSONGeometry{Type: "Polygon", Coordinates: coordinates})
	if err != nil {
		return "", fmt.Errorf("encode polygon: %w", err)
	}
	return string(body), nil
}

func decodePolygon(text string) (domain.Polygon, error) {
	var geometry geoJSONGeometry
	if err := json.Unmarshal([]byte(text), &geometry); err != nil {
		return nil, fmt.Errorf("decode polygon: %w", err)
	}
	var polygon domain.Polygon
	if err := json.Unmarshal(geometry.Coordinates, &polygon); err != nil {
		return nil, fmt.Errorf("decode polygon: %w", err)
	}
	return polygon, nil
}
