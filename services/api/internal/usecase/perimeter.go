package usecase

import (
	"context"
	"fmt"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// ImportPerimeterInput is the request of POST /perimeter-imports.
type ImportPerimeterInput struct {
	Actor domain.Principal
	File  PerimeterFile
}

// PerimeterImport is the normalized perimeter of a file, ready to be sent in
// POST /farms. Nothing is stored.
type PerimeterImport struct {
	Format        PerimeterFormat
	Perimeter     domain.Perimeter
	AreaHa        float64
	Centroid      domain.GeoPoint
	Datum         string
	InnerPolygons int
	// Municipality holds the perimeter (nil when unknown).
	Municipality *domain.Municipality
}

// ImportPerimeter reads a perimeter file, measures it and finds its municipality.
type ImportPerimeter struct {
	parser   PerimeterParser
	measurer PerimeterMeasurer
	locator  MunicipalityLocator
}

// NewImportPerimeter builds the use case.
func NewImportPerimeter(parser PerimeterParser, measurer PerimeterMeasurer, locator MunicipalityLocator) *ImportPerimeter {
	return &ImportPerimeter{parser: parser, measurer: measurer, locator: locator}
}

// Execute parses the file, refuses an invalid geometry and returns area and centroid.
func (uc *ImportPerimeter) Execute(ctx context.Context, in ImportPerimeterInput) (PerimeterImport, error) {
	if err := requireCan(in.Actor, domain.PermRegistryWrite); err != nil {
		return PerimeterImport{}, err
	}
	parsed, err := uc.parser.ParsePerimeter(in.File)
	if err != nil {
		return PerimeterImport{}, fmt.Errorf("parse perimeter: %w", err)
	}
	measure, err := uc.measurer.MeasurePerimeter(ctx, parsed.Perimeter)
	if err != nil {
		return PerimeterImport{}, fmt.Errorf("measure perimeter: %w", err)
	}
	if !measure.Valid {
		return PerimeterImport{}, domain.Invalid("file", "o perímetro do arquivo é inválido (polígono com autointerseção)")
	}
	municipality, err := uc.locator.LocateMunicipality(ctx, measure.InteriorPoint)
	if err != nil {
		return PerimeterImport{}, fmt.Errorf("locate municipality: %w", err)
	}
	return PerimeterImport{
		Municipality:  municipality,
		Format:        parsed.Format,
		Perimeter:     parsed.Perimeter,
		AreaHa:        measure.AreaHa,
		Centroid:      measure.Centroid,
		Datum:         parsed.Datum,
		InnerPolygons: parsed.InnerPolygons,
	}, nil
}
