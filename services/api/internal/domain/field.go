package domain

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Crop is an entry of the global crop catalog.
type Crop struct {
	Code string
	Name string
}

// maxSpacingM is the largest spacing the database stores (numeric(6,3)).
const maxSpacingM = 999.999

// seasonPattern is a crop season such as 2026/27.
var seasonPattern = regexp.MustCompile(`^(\d{4})/(\d{2})$`)

// FieldDetails are the editable data of a field, as received from the client.
// Blank optional text means "not informed".
type FieldDetails struct {
	Name          string
	Block         string
	CropCode      string
	Variety       string
	Season        string
	PlantingDate  *time.Time
	RowSpacingM   *float64
	PlantSpacingM *float64
	Polygon       Polygon
}

// Field is a management area of a farm, drawn as one polygon. Its area is
// computed by the geometry engine (the database) from the polygon.
type Field struct {
	ID            FieldID
	CompanyID     CompanyID
	FarmID        FarmID
	Name          string
	Block         *string
	Crop          Crop
	Variety       *string
	Season        *string
	PlantingDate  *time.Time
	RowSpacingM   *float64
	PlantSpacingM *float64
	Polygon       Polygon
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// NewField validates the details and returns a new field of the farm. crop is
// the catalog entry of details.CropCode, already looked up by the caller.
func NewField(id FieldID, farm Farm, in FieldDetails, crop Crop, now time.Time) (Field, error) {
	field := Field{ID: id, CompanyID: farm.CompanyID, FarmID: farm.ID, CreatedAt: now, UpdatedAt: now}
	if err := field.apply(in, crop); err != nil {
		return Field{}, err
	}
	return field, nil
}

// Revise replaces every editable field, with the same rules as the registration.
func (f *Field) Revise(in FieldDetails, crop Crop, now time.Time) error {
	updated := *f
	if err := updated.apply(in, crop); err != nil {
		return err
	}
	updated.UpdatedAt = now
	*f = updated
	return nil
}

// PlantsPerHa is the stand computed from the spacings (nil without both).
func (f Field) PlantsPerHa() *int {
	if f.RowSpacingM == nil || f.PlantSpacingM == nil {
		return nil
	}
	stand := int(math.Round(10000 / (*f.RowSpacingM * *f.PlantSpacingM)))
	return &stand
}

func (f *Field) apply(in FieldDetails, crop Crop) error {
	var problems Violations
	name, err := ParseName("name", in.Name)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return mergeErr
	}
	if strings.TrimSpace(in.CropCode) == "" {
		problems.Add("crop_code", "informe a cultura")
	} else if crop.Code != in.CropCode {
		problems.Add("crop_code", "cultura desconhecida")
	}
	block := optionalShortText(&problems, "block", in.Block)
	variety := optionalShortText(&problems, "variety", in.Variety)
	season := optionalSeason(&problems, in.Season)
	rowSpacing := optionalSpacing(&problems, "row_spacing_m", in.RowSpacingM)
	plantSpacing := optionalSpacing(&problems, "plant_spacing_m", in.PlantSpacingM)
	polygon, err := NewFieldPolygon("polygon", in.Polygon)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return mergeErr
	}
	if err := problems.Err(); err != nil {
		return err
	}

	f.Name, f.Block, f.Crop, f.Variety, f.Season = name, block, crop, variety, season
	f.PlantingDate, f.RowSpacingM, f.PlantSpacingM, f.Polygon = in.PlantingDate, rowSpacing, plantSpacing, polygon
	return nil
}

// NewFieldPolygon checks the structure of a field polygon: an outer ring and
// optional holes, closed, with WGS84 coordinates. Topological validity is the
// database's (see Perimeter).
func NewFieldPolygon(field string, polygon Polygon) (Polygon, error) {
	if len(polygon) == 0 {
		return nil, Invalid(field, "desenhe o polígono do talhão")
	}
	for _, ring := range polygon {
		if err := checkRing(field, ring); err != nil {
			return nil, err
		}
	}
	return polygon, nil
}

func optionalSeason(problems *Violations, value string) *string {
	season := strings.TrimSpace(value)
	if season == "" {
		return nil
	}
	match := seasonPattern.FindStringSubmatch(season)
	if match == nil {
		problems.Add("season", "safra no formato 2026/27")
		return nil
	}
	start, _ := strconv.Atoi(match[1]) // the pattern guarantees digits
	end, _ := strconv.Atoi(match[2])
	if (start+1)%100 != end {
		problems.Add("season", fmt.Sprintf("a safra %s termina em %02d", match[1], (start+1)%100))
		return nil
	}
	return &season
}

func optionalSpacing(problems *Violations, field string, value *float64) *float64 {
	if value == nil {
		return nil
	}
	switch {
	case math.IsNaN(*value) || *value <= 0:
		problems.Add(field, "deve ser maior que zero")
		return nil
	case *value > maxSpacingM:
		problems.Add(field, "espaçamento muito grande")
		return nil
	}
	spacing := math.Round(*value*1000) / 1000
	return &spacing
}

// ---- areas ---------------------------------------------------------------------------

// AreaRules are the configured limits on the size of farms and fields. Drawn
// polygons never match exactly, so the geometric checks accept a tolerance.
type AreaRules struct {
	// MinFarmAreaHa is the smallest perimeter a farm may have.
	MinFarmAreaHa float64
	// MinFieldAreaHa is the smallest polygon a field may have.
	MinFieldAreaHa float64
	// ToleranceM2 is the overlap, or the area outside the perimeter, ignored as drawing noise.
	ToleranceM2 float64
}

// FieldOverlap is another field of the same farm that a polygon overlaps.
type FieldOverlap struct {
	FieldID FieldID
	Name    string
	AreaM2  float64
}

// FieldFootprint is what the geometry engine measures of a field polygon
// inside its farm.
type FieldFootprint struct {
	// Valid is false for a polygon with self-intersections; nothing else is measured then.
	Valid  bool
	AreaHa float64
	// OutsidePerimeterM2 is the part of the polygon outside the farm perimeter.
	OutsidePerimeterM2 float64
	Overlaps           []FieldOverlap
}

// CheckField enforces the area rules of a field: the farm has a measured
// perimeter, the field has its own area and fits in the farm without
// overlapping another field.
func (r AreaRules) CheckField(farm Farm, farmAreaHa float64, footprint FieldFootprint) error {
	if farm.Perimeter == nil || farmAreaHa <= 0 {
		return Invalid("polygon", "a fazenda ainda não tem perímetro com área em hectares; cadastre-o antes dos talhões")
	}
	if !footprint.Valid {
		return Invalid("polygon", "o polígono do talhão é inválido (autointerseção)")
	}
	var problems Violations
	if footprint.AreaHa < r.MinFieldAreaHa {
		problems.Add("polygon", fmt.Sprintf("o talhão precisa de ao menos %s ha", formatHectares(r.MinFieldAreaHa)))
	}
	if footprint.OutsidePerimeterM2 > r.ToleranceM2 {
		problems.Add("polygon", fmt.Sprintf("o talhão sai do perímetro da fazenda (%.0f m² fora)", footprint.OutsidePerimeterM2))
	}
	for _, overlap := range footprint.Overlaps {
		if overlap.AreaM2 > r.ToleranceM2 {
			problems.Add("polygon", fmt.Sprintf("o talhão sobrepõe %s (%.0f m²)", overlap.Name, overlap.AreaM2))
		}
	}
	return problems.Err()
}

// FarmCoverage is how the fields of a farm sit in a (new) perimeter.
type FarmCoverage struct {
	FieldCount   int
	FieldsAreaHa float64
	// OutsidePerimeterM2 is the area of the fields outside the perimeter.
	OutsidePerimeterM2 float64
}

// CheckFarm enforces the area rules of a farm perimeter: it has its own area
// and still holds every field already registered. perimeterAreaHa is 0 for a
// farm without perimeter.
func (r AreaRules) CheckFarm(hasPerimeter bool, perimeterAreaHa float64, coverage FarmCoverage) error {
	const field = "location.perimeter"
	if !hasPerimeter {
		if coverage.FieldCount > 0 {
			return Invalid(field, "a fazenda tem talhões; o perímetro não pode ser removido")
		}
		return nil
	}
	var problems Violations
	if perimeterAreaHa < r.MinFarmAreaHa {
		problems.Add(field, fmt.Sprintf("a fazenda precisa de ao menos %s ha", formatHectares(r.MinFarmAreaHa)))
	}
	if coverage.OutsidePerimeterM2 > r.ToleranceM2 {
		problems.Add(field, fmt.Sprintf("o perímetro deixa talhões de fora (%.0f m²)", coverage.OutsidePerimeterM2))
	}
	if coverage.FieldsAreaHa-perimeterAreaHa > r.ToleranceM2/10000 {
		problems.Add(field, fmt.Sprintf("os talhões somam %s ha, mais que a área da fazenda (%s ha)",
			formatHectares(coverage.FieldsAreaHa), formatHectares(perimeterAreaHa)))
	}
	return problems.Err()
}

func formatHectares(value float64) string {
	return strings.Replace(strconv.FormatFloat(value, 'f', -1, 64), ".", ",", 1)
}
