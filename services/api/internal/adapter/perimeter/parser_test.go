package perimeter_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/perimeter"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	squareKML = `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2"><Document><Folder>
  <Placemark><name>Sede</name><Point><coordinates>-46.995,-20.995,0</coordinates></Point></Placemark>
  <Placemark><name>Perímetro</name><Polygon><outerBoundaryIs><LinearRing><coordinates>
    -47.0,-21.0,0 -47.0,-20.99,0 -46.99,-20.99,0 -46.99,-21.0,0 -47.0,-21.0,0
  </coordinates></LinearRing></outerBoundaryIs></Polygon></Placemark>
  <Placemark><name>Talhão 1</name><Polygon><outerBoundaryIs><LinearRing><coordinates>
    -46.999,-20.999 -46.999,-20.998 -46.998,-20.998 -46.998,-20.999
  </coordinates></LinearRing></outerBoundaryIs></Polygon></Placemark>
</Folder></Document></kml>`

	multiPolygonGeoJSON = `{"type":"Feature","properties":{},"geometry":{"type":"MultiPolygon","coordinates":[
  [[[-47,-21],[-47,-20.99],[-46.99,-20.99],[-46.99,-21],[-47,-21]]],
  [[[-46.9,-21],[-46.9,-20.99],[-46.89,-20.99],[-46.89,-21],[-46.9,-21]]]]}}`
)

func kmz(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	entry, err := archive.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParsePerimeter(t *testing.T) {
	tests := []struct {
		name         string
		file         usecase.PerimeterFile
		wantFormat   usecase.PerimeterFormat
		wantPolygons int
		wantInner    int
	}{
		{"KML: largest polygon is the perimeter, others are fields; points ignored",
			usecase.PerimeterFile{Name: "fazenda.KML", Content: []byte(squareKML)}, usecase.PerimeterKML, 1, 1},
		{"KMZ with doc.kml",
			usecase.PerimeterFile{Name: "fazenda.kmz", Content: kmz(t, "files/doc.kml", squareKML)}, usecase.PerimeterKMZ, 1, 1},
		{"GeoJSON MultiPolygon in one feature keeps every part",
			usecase.PerimeterFile{Name: "fazenda.geojson", Content: []byte(multiPolygonGeoJSON)}, usecase.PerimeterGeoJSON, 2, 0},
		{"GeoJSON bare polygon with an open ring",
			usecase.PerimeterFile{Name: "f.json", Content: []byte(`{"type":"Polygon","coordinates":[[[0,0],[0,1],[1,1],[1,0]]]}`)}, usecase.PerimeterGeoJSON, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := perimeter.NewParser().ParsePerimeter(tt.file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.Format != tt.wantFormat || len(parsed.Perimeter) != tt.wantPolygons || parsed.InnerPolygons != tt.wantInner || parsed.Datum != "WGS 84" {
				t.Fatalf("parsed = %+v", parsed)
			}
			outer := parsed.Perimeter[0][0]
			if outer[0] != outer[len(outer)-1] {
				t.Fatal("rings must come back closed")
			}
		})
	}
}

func TestParsePerimeterRefusals(t *testing.T) {
	tests := []struct {
		name        string
		file        usecase.PerimeterFile
		unsupported bool
	}{
		{"Esri file geodatabase", usecase.PerimeterFile{Name: "fazenda.gdb", Content: []byte("x")}, true},
		{"unknown extension", usecase.PerimeterFile{Name: "fazenda.pdf", Content: []byte("%PDF")}, true},
		{"broken GeoJSON", usecase.PerimeterFile{Name: "f.geojson", Content: []byte(`{"type":`)}, false},
		{"GeoJSON in another CRS", usecase.PerimeterFile{Name: "f.geojson", Content: []byte(
			`{"type":"Polygon","crs":{"type":"name","properties":{"name":"EPSG:31983"}},"coordinates":[[[0,0],[0,1],[1,1],[0,0]]]}`)}, false},
		{"no polygon at all", usecase.PerimeterFile{Name: "f.geojson", Content: []byte(`{"type":"Point","coordinates":[0,0]}`)}, false},
		{"broken KML", usecase.PerimeterFile{Name: "f.kml", Content: []byte(`<kml><Polygon>`)}, false},
		{"KML with bad coordinates", usecase.PerimeterFile{Name: "f.kml", Content: []byte(
			`<kml><Polygon><outerBoundaryIs><LinearRing><coordinates>a,b c,d</coordinates></LinearRing></outerBoundaryIs></Polygon></kml>`)}, false},
		{"KMZ without KML", usecase.PerimeterFile{Name: "f.kmz", Content: kmz(t, "readme.txt", "hi")}, false},
		{"KMZ that is not a zip", usecase.PerimeterFile{Name: "f.kmz", Content: []byte("nope")}, false},
		{"degenerate ring", usecase.PerimeterFile{Name: "f.json", Content: []byte(`{"type":"Polygon","coordinates":[[[0,0],[0,1]]]}`)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := perimeter.NewParser().ParsePerimeter(tt.file)
			if tt.unsupported {
				if !errors.Is(err, usecase.ErrUnsupportedPerimeterFormat) {
					t.Fatalf("error = %v, want unsupported format", err)
				}
				return
			}
			var verr *domain.ValidationError
			if !errors.As(err, &verr) || verr.Fields[0].Field != "file" {
				t.Fatalf("error = %v, want validation on file", err)
			}
		})
	}
}
