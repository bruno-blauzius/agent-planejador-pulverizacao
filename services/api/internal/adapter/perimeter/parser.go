// Package perimeter reads farm perimeters from uploaded files (KML, KMZ,
// GeoJSON and zipped Shapefile, see shapefile.go) with the standard library only.
//
// Rules of the extraction: a file with a single geometry gives the whole
// geometry (a Polygon or a MultiPolygon of separate parts). A file with several
// polygons is read as a perimeter with fields drawn inside: the largest polygon
// is the perimeter and the others are counted as possible fields.
package perimeter

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	fileField = "file"
	// wgs84 is the datum of KML (by specification) and of GeoJSON without "crs" (RFC 7946).
	wgs84 = "WGS 84"
	// maxKMLBytes bounds the KML inflated from a KMZ, against zip bombs.
	maxKMLBytes = 64 << 20
)

// Parser implements usecase.PerimeterParser.
type Parser struct{}

// NewParser builds the parser.
func NewParser() Parser { return Parser{} }

var _ usecase.PerimeterParser = Parser{}

// ParsePerimeter reads the file according to its extension.
func (Parser) ParsePerimeter(file usecase.PerimeterFile) (usecase.ParsedPerimeter, error) {
	switch strings.ToLower(path.Ext(file.Name)) {
	case ".geojson", ".json":
		return fromPolygons(usecase.PerimeterGeoJSON, file.Content, parseGeoJSON)
	case ".kml":
		return fromPolygons(usecase.PerimeterKML, file.Content, parseKML)
	case ".kmz":
		kml, err := kmlFromKMZ(file.Content)
		if err != nil {
			return usecase.ParsedPerimeter{}, err
		}
		return fromPolygons(usecase.PerimeterKMZ, kml, parseKML)
	case ".zip":
		shp, datum, err := shapefileFromZip(file.Content)
		if err != nil {
			return usecase.ParsedPerimeter{}, err
		}
		parsed, err := fromPolygons(usecase.PerimeterShapefile, shp, parseShapefile)
		parsed.Datum = datum
		return parsed, err
	case ".shp":
		return usecase.ParsedPerimeter{}, domain.Invalid(fileField, "envie o Shapefile compactado em .zip, com os arquivos .shp e .prj")
	default:
		return usecase.ParsedPerimeter{}, fmt.Errorf("%w: %q", usecase.ErrUnsupportedPerimeterFormat, path.Ext(file.Name))
	}
}

// extraction is what a format reader finds: the geometries, each a list of polygons.
type extraction [][]domain.Polygon

func fromPolygons(format usecase.PerimeterFormat, content []byte, read func([]byte) (extraction, error)) (usecase.ParsedPerimeter, error) {
	geometries, err := read(content)
	if err != nil {
		return usecase.ParsedPerimeter{}, err
	}
	polygons, inner := choosePerimeter(geometries)
	if len(polygons) == 0 {
		return usecase.ParsedPerimeter{}, domain.Invalid(fileField, "nenhum polígono encontrado no arquivo")
	}
	perimeter, err := domain.NewPerimeter(fileField, polygons)
	if err != nil {
		return usecase.ParsedPerimeter{}, err
	}
	return usecase.ParsedPerimeter{Format: format, Perimeter: perimeter, Datum: wgs84, InnerPolygons: inner}, nil
}

// choosePerimeter applies the extraction rules of the package comment.
func choosePerimeter(geometries extraction) (polygons []domain.Polygon, inner int) {
	if len(geometries) == 1 {
		return geometries[0], 0
	}
	var all []domain.Polygon
	for _, geometry := range geometries {
		all = append(all, geometry...)
	}
	if len(all) == 0 {
		return nil, 0
	}
	largest := 0
	for i := range all {
		if planarArea(all[i]) > planarArea(all[largest]) {
			largest = i
		}
	}
	return []domain.Polygon{all[largest]}, len(all) - 1
}

// planarArea is the shoelace area of the outer ring in square degrees: good
// enough to compare polygons of the same file, never to report an area.
func planarArea(polygon domain.Polygon) float64 {
	if len(polygon) == 0 {
		return 0
	}
	ring := polygon[0]
	sum := 0.0
	for i := 0; i+1 < len(ring); i++ {
		sum += ring[i][0]*ring[i+1][1] - ring[i+1][0]*ring[i][1]
	}
	return math.Abs(sum) / 2
}

// closeRing appends the first position when the file left the ring open.
func closeRing(ring domain.Ring) domain.Ring {
	if len(ring) > 0 && ring[0] != ring[len(ring)-1] {
		ring = append(ring, ring[0])
	}
	return ring
}

// ---- GeoJSON ---------------------------------------------------------------------

type geoJSONObject struct {
	Type        string            `json:"type"`
	Coordinates json.RawMessage   `json:"coordinates"`
	Geometry    *geoJSONObject    `json:"geometry"`
	Geometries  []geoJSONObject   `json:"geometries"`
	Features    []geoJSONObject   `json:"features"`
	CRS         *geoJSONReference `json:"crs"`
}

type geoJSONReference struct {
	Properties struct {
		Name string `json:"name"`
	} `json:"properties"`
}

func parseGeoJSON(content []byte) (extraction, error) {
	var root geoJSONObject
	if err := json.Unmarshal(content, &root); err != nil {
		return nil, domain.Invalid(fileField, "o arquivo não é um GeoJSON válido")
	}
	if root.CRS != nil && !isWGS84(root.CRS.Properties.Name) {
		return nil, domain.Invalid(fileField, "o GeoJSON precisa estar em WGS84 (EPSG:4326)")
	}
	var geometries extraction
	if err := collectGeoJSON(root, &geometries); err != nil {
		return nil, err
	}
	return geometries, nil
}

func isWGS84(name string) bool {
	upper := strings.ToUpper(name)
	return strings.Contains(upper, "CRS84") || strings.HasSuffix(upper, "4326")
}

func collectGeoJSON(object geoJSONObject, out *extraction) error {
	switch object.Type {
	case "FeatureCollection":
		for _, feature := range object.Features {
			if err := collectGeoJSON(feature, out); err != nil {
				return err
			}
		}
	case "Feature":
		if object.Geometry != nil {
			return collectGeoJSON(*object.Geometry, out)
		}
	case "GeometryCollection":
		for _, geometry := range object.Geometries {
			if err := collectGeoJSON(geometry, out); err != nil {
				return err
			}
		}
	case "Polygon":
		var polygon domain.Polygon
		if err := json.Unmarshal(object.Coordinates, &polygon); err != nil {
			return domain.Invalid(fileField, "coordenadas de polígono inválidas")
		}
		*out = append(*out, []domain.Polygon{closePolygon(polygon)})
	case "MultiPolygon":
		var polygons []domain.Polygon
		if err := json.Unmarshal(object.Coordinates, &polygons); err != nil {
			return domain.Invalid(fileField, "coordenadas de multipolígono inválidas")
		}
		for i := range polygons {
			polygons[i] = closePolygon(polygons[i])
		}
		*out = append(*out, polygons)
	}
	// Points and lines (headquarters, roads) are ignored: only areas matter here.
	return nil
}

func closePolygon(polygon domain.Polygon) domain.Polygon {
	for i := range polygon {
		polygon[i] = closeRing(polygon[i])
	}
	return polygon
}

// ---- KML / KMZ -------------------------------------------------------------------

type kmlPolygon struct {
	Outer string   `xml:"outerBoundaryIs>LinearRing>coordinates"`
	Inner []string `xml:"innerBoundaryIs>LinearRing>coordinates"`
}

// parseKML streams the document and decodes every <Polygon>, wherever it is
// (Placemark, MultiGeometry, Folder). Each polygon is a geometry of its own.
func parseKML(content []byte) (extraction, error) {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var geometries extraction
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return geometries, nil
		}
		if err != nil {
			return nil, domain.Invalid(fileField, "o arquivo não é um KML válido")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Polygon" {
			continue
		}
		var raw kmlPolygon
		if err := decoder.DecodeElement(&raw, &start); err != nil {
			return nil, domain.Invalid(fileField, "polígono do KML ilegível")
		}
		polygon, err := kmlRings(raw)
		if err != nil {
			return nil, err
		}
		geometries = append(geometries, []domain.Polygon{polygon})
	}
}

func kmlRings(raw kmlPolygon) (domain.Polygon, error) {
	outer, err := kmlCoordinates(raw.Outer)
	if err != nil {
		return nil, err
	}
	polygon := domain.Polygon{outer}
	for _, text := range raw.Inner {
		hole, err := kmlCoordinates(text)
		if err != nil {
			return nil, err
		}
		polygon = append(polygon, hole)
	}
	return polygon, nil
}

// kmlCoordinates reads "lng,lat[,alt] lng,lat[,alt] ..." (altitude ignored).
func kmlCoordinates(text string) (domain.Ring, error) {
	var ring domain.Ring
	for _, tuple := range strings.Fields(text) {
		parts := strings.Split(tuple, ",")
		if len(parts) < 2 {
			return nil, domain.Invalid(fileField, "coordenada do KML inválida")
		}
		lng, errLng := strconv.ParseFloat(parts[0], 64)
		lat, errLat := strconv.ParseFloat(parts[1], 64)
		if errLng != nil || errLat != nil {
			return nil, domain.Invalid(fileField, "coordenada do KML inválida")
		}
		ring = append(ring, domain.Position{lng, lat})
	}
	return closeRing(ring), nil
}

// kmlFromKMZ returns the main KML of a KMZ: doc.kml, or the first .kml found.
func kmlFromKMZ(content []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, domain.Invalid(fileField, "o arquivo não é um KMZ válido")
	}
	var chosen *zip.File
	for _, entry := range archive.File {
		if !strings.EqualFold(path.Ext(entry.Name), ".kml") {
			continue
		}
		if chosen == nil || strings.EqualFold(path.Base(entry.Name), "doc.kml") {
			chosen = entry
		}
	}
	if chosen == nil {
		return nil, domain.Invalid(fileField, "o KMZ não contém um arquivo .kml")
	}
	reader, err := chosen.Open()
	if err != nil {
		return nil, domain.Invalid(fileField, "o KML dentro do KMZ está ilegível")
	}
	kml, readErr := io.ReadAll(io.LimitReader(reader, maxKMLBytes+1))
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return nil, domain.Invalid(fileField, "o KML dentro do KMZ está ilegível")
	}
	if len(kml) > maxKMLBytes {
		return nil, domain.Invalid(fileField, "o KML dentro do KMZ é grande demais")
	}
	return kml, nil
}
