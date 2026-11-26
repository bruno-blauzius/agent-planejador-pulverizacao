package perimeter_test

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/perimeter"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	prjWGS84   = `GEOGCS["GCS_WGS_1984",DATUM["D_WGS_1984",SPHEROID["WGS_1984",6378137.0,298.257223563]],PRIMEM["Greenwich",0.0],UNIT["Degree",0.0174532925199433]]`
	prjSIRGAS  = `GEOGCS["GCS_SIRGAS_2000",DATUM["D_SIRGAS_2000",SPHEROID["GRS_1980",6378137.0,298.257222101]],PRIMEM["Greenwich",0.0],UNIT["Degree",0.0174532925199433]]`
	prjUTM     = `PROJCS["SIRGAS_2000_UTM_Zone_23S",GEOGCS["GCS_SIRGAS_2000",DATUM["D_SIRGAS_2000",SPHEROID["GRS_1980",6378137.0,298.257222101]]],PROJECTION["Transverse_Mercator"]]`
	prjSAD69   = `GEOGCS["GCS_South_American_1969",DATUM["D_South_American_1969",SPHEROID["GRS_1967_Truncated",6378160.0,298.25]]]`
	pointShape = 1
)

// Clockwise rings are outer boundaries in a shapefile; counterclockwise ones are holes.
var (
	farmRing  = domain.Ring{{-47, -21}, {-47, -20.99}, {-46.99, -20.99}, {-46.99, -21}, {-47, -21}}
	holeRing  = domain.Ring{{-46.998, -20.998}, {-46.996, -20.998}, {-46.996, -20.996}, {-46.998, -20.996}, {-46.998, -20.998}}
	fieldRing = domain.Ring{{-46.999, -20.999}, {-46.999, -20.998}, {-46.998, -20.998}, {-46.998, -20.999}, {-46.999, -20.999}}
	otherPart = domain.Ring{{-46.9, -21}, {-46.9, -20.99}, {-46.89, -20.99}, {-46.89, -21}, {-46.9, -21}}
)

// shp encodes records of the given shape type; each record is a list of rings (parts).
func shp(t *testing.T, shapeType uint32, records ...[]domain.Ring) []byte {
	t.Helper()
	var body bytes.Buffer
	write := func(w *bytes.Buffer, order binary.ByteOrder, v any) {
		if err := binary.Write(w, order, v); err != nil {
			t.Fatal(err)
		}
	}
	for n, rings := range records {
		var content bytes.Buffer
		le := func(v any) { write(&content, binary.LittleEndian, v) }
		le(shapeType)
		le([4]float64{-47, -21, -46.89, -20.99})
		points := 0
		for _, ring := range rings {
			points += len(ring)
		}
		le(int32(len(rings)))
		le(int32(points))
		start := 0
		for _, ring := range rings {
			le(int32(start))
			start += len(ring)
		}
		for _, ring := range rings {
			for _, p := range ring {
				le([2]float64{p[0], p[1]})
			}
		}
		if shapeType == 15 { // PolygonZ: Z range, Z values, M range, M values
			le([2]float64{0, 0})
			le(make([]float64, points))
			le([2]float64{0, 0})
			le(make([]float64, points))
		}
		write(&body, binary.BigEndian, int32(n+1))
		write(&body, binary.BigEndian, int32(content.Len()/2))
		body.Write(content.Bytes())
	}
	header := make([]byte, 100)
	binary.BigEndian.PutUint32(header[0:], 9994)
	binary.BigEndian.PutUint32(header[24:], uint32((100+body.Len())/2))
	binary.LittleEndian.PutUint32(header[28:], 1000)
	binary.LittleEndian.PutUint32(header[32:], shapeType)
	return append(header, body.Bytes()...)
}

func reversed(ring domain.Ring) domain.Ring {
	out := make(domain.Ring, len(ring))
	for i := range ring {
		out[len(ring)-1-i] = ring[i]
	}
	return out
}

func zipOf(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	for name, content := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func shapefileZip(t *testing.T, prj string, shpContent []byte) usecase.PerimeterFile {
	t.Helper()
	files := map[string][]byte{"CAR/area_imovel.shp": shpContent, "CAR/area_imovel.dbf": {0x03}, "CAR/area_imovel.shx": {}}
	if prj != "" {
		files["CAR/area_imovel.prj"] = []byte(prj)
	}
	return usecase.PerimeterFile{Name: "car.ZIP", Content: zipOf(t, files)}
}

func TestParseShapefile(t *testing.T) {
	tests := []struct {
		name         string
		file         usecase.PerimeterFile
		wantDatum    string
		wantPolygons int
		wantRings    int
		wantInner    int
	}{
		{"one polygon in WGS 84", shapefileZip(t, prjWGS84, shp(t, 5, []domain.Ring{farmRing})), "WGS 84", 1, 1, 0},
		{"SIRGAS 2000 (CAR)", shapefileZip(t, prjSIRGAS, shp(t, 5, []domain.Ring{farmRing})), "SIRGAS 2000", 1, 1, 0},
		{"hole goes to its outer ring", shapefileZip(t, prjWGS84, shp(t, 5, []domain.Ring{farmRing, holeRing})), "WGS 84", 1, 2, 0},
		{"multipart record keeps every part", shapefileZip(t, prjWGS84, shp(t, 5, []domain.Ring{farmRing, otherPart})), "WGS 84", 2, 1, 0},
		{"perimeter and fields as separate records", shapefileZip(t, prjWGS84, shp(t, 5, []domain.Ring{farmRing}, []domain.Ring{fieldRing})), "WGS 84", 1, 1, 1},
		{"PolygonZ ignores the heights", shapefileZip(t, prjWGS84, shp(t, 15, []domain.Ring{farmRing})), "WGS 84", 1, 1, 0},
		{"writer that ignores orientation", shapefileZip(t, prjWGS84, shp(t, 5, []domain.Ring{reversed(farmRing)})), "WGS 84", 1, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := perimeter.NewParser().ParsePerimeter(tt.file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.Format != usecase.PerimeterShapefile || parsed.Datum != tt.wantDatum ||
				len(parsed.Perimeter) != tt.wantPolygons || len(parsed.Perimeter[0]) != tt.wantRings || parsed.InnerPolygons != tt.wantInner {
				t.Fatalf("parsed = %+v", parsed)
			}
			if got := parsed.Perimeter[0][0][0]; math.Abs(got[0]) > 180 || math.Abs(got[1]) > 90 {
				t.Fatalf("coordinates out of range: %v", got)
			}
		})
	}
}

func TestParseShapefileRefusals(t *testing.T) {
	polygon := shp(t, 5, []domain.Ring{farmRing})
	truncated := polygon[:len(polygon)-20]
	tests := []struct {
		name string
		file usecase.PerimeterFile
	}{
		{"not a zip", usecase.PerimeterFile{Name: "f.zip", Content: []byte("PK")}},
		{"bare .shp", usecase.PerimeterFile{Name: "f.shp", Content: polygon}},
		{"zip without .shp", usecase.PerimeterFile{Name: "f.zip", Content: zipOf(t, map[string][]byte{"a.txt": []byte("x")})}},
		{"two shapefiles", usecase.PerimeterFile{Name: "f.zip", Content: zipOf(t, map[string][]byte{
			"a.shp": polygon, "a.prj": []byte(prjWGS84), "b.shp": polygon, "b.prj": []byte(prjWGS84),
		})}},
		{"without .prj", shapefileZip(t, "", polygon)},
		{"projected (UTM)", shapefileZip(t, prjUTM, polygon)},
		{"unsupported datum", shapefileZip(t, prjSAD69, polygon)},
		{"unreadable .prj", shapefileZip(t, "hello", polygon)},
		{"points, not polygons", shapefileZip(t, prjWGS84, shp(t, pointShape))},
		{"truncated record", shapefileZip(t, prjWGS84, truncated)},
		{"not a .shp", shapefileZip(t, prjWGS84, []byte("garbage"))},
		{"only null shapes", shapefileZip(t, prjWGS84, shp(t, 0))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := perimeter.NewParser().ParsePerimeter(tt.file)
			var verr *domain.ValidationError
			if !errors.As(err, &verr) || verr.Fields[0].Field != "file" {
				t.Fatalf("error = %v, want validation on file", err)
			}
		})
	}
}
