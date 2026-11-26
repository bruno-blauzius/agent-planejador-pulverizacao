package perimeter

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"path"
	"strings"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Shapefile (ESRI, 1998) read with the standard library (ADR 0018). The .zip
// must carry one .shp and its .prj; the attributes (.dbf) are not needed.
// Only polygon shapes are read (Polygon, PolygonZ and PolygonM, whose Z and M
// values are ignored). Coordinates must be geographic in WGS 84 or SIRGAS 2000
// (the datum of the CAR; both coincide at the scale of a farm). Projected
// systems such as UTM would need a reprojection library, so they are refused
// with a message that tells the user how to export the file.

const (
	shpFileCode    = 9994
	shpVersion     = 1000
	shpHeaderBytes = 100
	// maxShapefileBytes bounds the .shp inflated from the .zip, against zip bombs.
	maxShapefileBytes = 64 << 20
	// maxPrjBytes bounds the .prj (a single WKT line in practice).
	maxPrjBytes = 64 << 10
	// maxShapeRecords bounds the records read from one file.
	maxShapeRecords = 10_000
)

// Shape types (ESRI Shapefile Technical Description, table 1).
const (
	shapeNull     = 0
	shapePolygon  = 5
	shapePolygonZ = 15
	shapePolygonM = 25
)

var (
	errShapefileBroken = domain.Invalid(fileField, "o Shapefile está corrompido ou incompleto")
	errNotPolygons     = domain.Invalid(fileField, "o Shapefile não é de polígonos (use um arquivo de áreas, não de pontos ou linhas)")
)

// shapefileFromZip returns the .shp and .prj of the archive and the datum the
// .prj declares.
func shapefileFromZip(content []byte) (shp []byte, datum string, err error) {
	archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, "", domain.Invalid(fileField, "o arquivo não é um .zip válido")
	}
	var shpEntry *zip.File
	prjEntries := map[string]*zip.File{}
	for _, entry := range archive.File {
		if strings.HasPrefix(path.Base(entry.Name), "._") { // macOS metadata
			continue
		}
		base := strings.TrimSuffix(entry.Name, path.Ext(entry.Name))
		switch strings.ToLower(path.Ext(entry.Name)) {
		case ".shp":
			if shpEntry != nil {
				return nil, "", domain.Invalid(fileField, "envie um único Shapefile por .zip")
			}
			shpEntry = entry
		case ".prj":
			prjEntries[strings.ToLower(base)] = entry
		}
	}
	if shpEntry == nil {
		return nil, "", domain.Invalid(fileField, "o .zip não contém um arquivo .shp")
	}
	prjEntry, ok := prjEntries[strings.ToLower(strings.TrimSuffix(shpEntry.Name, path.Ext(shpEntry.Name)))]
	if !ok {
		return nil, "", domain.Invalid(fileField, "o .zip precisa do arquivo .prj (sistema de coordenadas) junto do .shp")
	}
	prj, err := readZipEntry(prjEntry, maxPrjBytes)
	if err != nil {
		return nil, "", err
	}
	datum, err = geographicDatum(string(prj))
	if err != nil {
		return nil, "", err
	}
	shp, err = readZipEntry(shpEntry, maxShapefileBytes)
	if err != nil {
		return nil, "", err
	}
	return shp, datum, nil
}

func readZipEntry(entry *zip.File, limit int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, domain.Invalid(fileField, "arquivo ilegível dentro do .zip: %s", path.Base(entry.Name))
	}
	content, readErr := io.ReadAll(io.LimitReader(reader, limit+1))
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return nil, domain.Invalid(fileField, "arquivo ilegível dentro do .zip: %s", path.Base(entry.Name))
	}
	if int64(len(content)) > limit {
		return nil, domain.Invalid(fileField, "arquivo grande demais dentro do .zip: %s", path.Base(entry.Name))
	}
	return content, nil
}

// geographicDatum reads the WKT of the .prj: only geographic WGS 84 or SIRGAS
// 2000 are accepted.
func geographicDatum(wkt string) (string, error) {
	normalized := strings.ToUpper(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(wkt))
	switch {
	case strings.HasPrefix(normalized, "PROJCS") || strings.HasPrefix(normalized, "PROJCRS"):
		return "", domain.Invalid(fileField,
			"o Shapefile está em coordenadas projetadas (ex.: UTM); exporte em coordenadas geográficas WGS 84 ou SIRGAS 2000")
	case !strings.HasPrefix(normalized, "GEOGCS") && !strings.HasPrefix(normalized, "GEOGCRS"):
		return "", domain.Invalid(fileField, "o .prj do Shapefile não descreve um sistema de coordenadas reconhecido")
	case strings.Contains(normalized, "SIRGAS2000") || strings.Contains(normalized, "SIRGAS"):
		return "SIRGAS 2000", nil
	case strings.Contains(normalized, "WGS1984") || strings.Contains(normalized, "WGS84"):
		return wgs84, nil
	default:
		return "", domain.Invalid(fileField,
			"o Shapefile usa um datum não suportado (ex.: SAD69, Córrego Alegre); exporte em WGS 84 ou SIRGAS 2000")
	}
}

// parseShapefile reads every polygon record of a .shp; each record is one
// geometry (its outer rings, with the holes that fall inside them).
func parseShapefile(content []byte) (extraction, error) {
	if len(content) < shpHeaderBytes ||
		binary.BigEndian.Uint32(content[0:4]) != shpFileCode ||
		binary.LittleEndian.Uint32(content[28:32]) != shpVersion {
		return nil, domain.Invalid(fileField, "o .shp não é um Shapefile válido")
	}
	switch binary.LittleEndian.Uint32(content[32:36]) {
	case shapeNull, shapePolygon, shapePolygonZ, shapePolygonM:
	default:
		return nil, errNotPolygons
	}
	var geometries extraction
	for offset := shpHeaderBytes; offset < len(content); {
		if len(geometries) >= maxShapeRecords {
			return nil, domain.Invalid(fileField, "o Shapefile tem polígonos demais (máximo de %d)", maxShapeRecords)
		}
		if offset+8 > len(content) {
			return nil, errShapefileBroken
		}
		length := int(binary.BigEndian.Uint32(content[offset+4:offset+8])) * 2 // 16-bit words
		start, end := offset+8, offset+8+length
		if length < 4 || end > len(content) || end < start {
			return nil, errShapefileBroken
		}
		polygons, err := shapeRecord(content[start:end])
		if err != nil {
			return nil, err
		}
		if len(polygons) > 0 {
			geometries = append(geometries, polygons)
		}
		offset = end
	}
	return geometries, nil
}

// shapeRecord decodes one record: shape type, bounding box, parts and points.
func shapeRecord(record []byte) ([]domain.Polygon, error) {
	switch binary.LittleEndian.Uint32(record[0:4]) {
	case shapeNull:
		return nil, nil
	case shapePolygon, shapePolygonZ, shapePolygonM:
	default:
		return nil, errNotPolygons
	}
	const fixed = 4 + 32 + 4 + 4 // type, box, number of parts, number of points
	if len(record) < fixed {
		return nil, errShapefileBroken
	}
	numParts := int(binary.LittleEndian.Uint32(record[36:40]))
	numPoints := int(binary.LittleEndian.Uint32(record[40:44]))
	pointsAt := fixed + 4*numParts
	if numParts < 1 || numPoints < numParts || numParts > len(record) || numPoints > len(record) ||
		pointsAt+16*numPoints > len(record) {
		return nil, errShapefileBroken
	}
	rings := make([]domain.Ring, 0, numParts)
	for part := range numParts {
		first := int(binary.LittleEndian.Uint32(record[fixed+4*part:]))
		last := numPoints
		if part+1 < numParts {
			last = int(binary.LittleEndian.Uint32(record[fixed+4*(part+1):]))
		}
		if first < 0 || last > numPoints || first >= last {
			return nil, errShapefileBroken
		}
		ring := make(domain.Ring, 0, last-first)
		for i := first; i < last; i++ {
			at := pointsAt + 16*i
			x := math.Float64frombits(binary.LittleEndian.Uint64(record[at:]))
			y := math.Float64frombits(binary.LittleEndian.Uint64(record[at+8:]))
			ring = append(ring, domain.Position{x, y})
		}
		rings = append(rings, closeRing(ring))
	}
	return assembleRings(rings), nil
}

// assembleRings groups the rings of a record: in a shapefile outer rings are
// clockwise and holes counterclockwise. Each hole goes to the outer ring whose
// bounding box holds its first vertex (the last outer ring otherwise).
func assembleRings(rings []domain.Ring) []domain.Polygon {
	var polygons []domain.Polygon
	var holes []domain.Ring
	for _, ring := range rings {
		if signedArea(ring) <= 0 {
			polygons = append(polygons, domain.Polygon{ring})
		} else {
			holes = append(holes, ring)
		}
	}
	if len(polygons) == 0 {
		// Some writers ignore the orientation rule: take every ring as an outer ring.
		for _, ring := range holes {
			polygons = append(polygons, domain.Polygon{ring})
		}
		return polygons
	}
	for _, hole := range holes {
		owner := len(polygons) - 1
		for i, polygon := range polygons {
			if len(hole) > 0 && boxHolds(polygon[0], hole[0]) {
				owner = i
				break
			}
		}
		polygons[owner] = append(polygons[owner], hole)
	}
	return polygons
}

// signedArea is the shoelace sum: positive for counterclockwise rings.
func signedArea(ring domain.Ring) float64 {
	sum := 0.0
	for i := 0; i+1 < len(ring); i++ {
		sum += ring[i][0]*ring[i+1][1] - ring[i+1][0]*ring[i][1]
	}
	return sum / 2
}

func boxHolds(ring domain.Ring, position domain.Position) bool {
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range ring {
		minX, maxX = min(minX, p[0]), max(maxX, p[0])
		minY, maxY = min(minY, p[1]), max(maxY, p[1])
	}
	return position[0] >= minX && position[0] <= maxX && position[1] >= minY && position[1] <= maxY
}
